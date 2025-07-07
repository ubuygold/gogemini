package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/ubuygold/gogemini/internal/admin"
	"github.com/ubuygold/gogemini/internal/auth"
	"github.com/ubuygold/gogemini/internal/balancer"
	"github.com/ubuygold/gogemini/internal/config"
	"github.com/ubuygold/gogemini/internal/db"
	"github.com/ubuygold/gogemini/internal/keymanager"
	"github.com/ubuygold/gogemini/internal/logger"
	"github.com/ubuygold/gogemini/internal/proxy"
	"github.com/ubuygold/gogemini/internal/scheduler"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var webUI embed.FS
var indexHTML []byte

var newDBService = db.NewService

// customRecovery is a middleware that recovers from panics and handles http.ErrAbortHandler gracefully.
func customRecovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					log.Warn("Client connection aborted", "path", c.Request.URL.Path)
					c.Abort()
					return
				}

				log.Error("Panic recovered",
					"error", recovered,
					"path", c.Request.URL.Path,
					"stack", string(debug.Stack()),
				)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

func setupAndRunServer(cfg *config.Config, log *slog.Logger, dbService db.Service) error {
	var err error
	indexHTML, err = webUI.ReadFile("dist/index.html")
	if err != nil {
		log.Error("failed to read index.html from embedded fs", "error", err)
		return err
	}

	// Initialize the central KeyManager
	keyManager, err := keymanager.NewKeyManager(dbService, cfg, log)
	if err != nil {
		log.Error("Error creating KeyManager", "error", err)
		return err
	}

	// Start the scheduler
	s := scheduler.NewScheduler(dbService, cfg, keyManager)
	s.Start()
	log.Info("Scheduler started")

	// Create the new SDK-based handler for Gemini
	geminiHandler, err := balancer.NewBalancer(keyManager, log)
	if err != nil {
		log.Error("Error creating Gemini handler", "error", err)
		return err
	}

	// Create the new reverse proxy for OpenAI
	openaiProxy, err := proxy.NewOpenAIProxy(keyManager, cfg, log)
	if err != nil {
		log.Error("Error creating OpenAI proxy", "error", err)
		return err
	}

	// Create a Gin router
	router := gin.New()
	router.RedirectTrailingSlash = false
	// Use our custom recovery middleware instead of the default one.
	router.Use(customRecovery(log))

	// If debug mode is enabled, add the logger middleware
	if cfg.Debug {
		// This uses the default gin logger, which is fine for development.
		router.Use(gin.Logger())
	}

	// Setup admin routes
	admin.SetupRoutes(router, dbService, keyManager, cfg)

	// Create a group for Gemini routes
	geminiHandlerFunc := func(c *gin.Context) {
		http.StripPrefix("/gemini", geminiHandler).ServeHTTP(c.Writer, c.Request)
	}
	geminiGroup := router.Group("/gemini")
	geminiGroup.Use(auth.AuthMiddleware(dbService))
	geminiGroup.GET("/*path", geminiHandlerFunc)
	geminiGroup.POST("/*path", geminiHandlerFunc)

	// Create a group for OpenAI routes
	openaiHandlerFunc := func(c *gin.Context) {
		http.StripPrefix("/openai", openaiProxy).ServeHTTP(c.Writer, c.Request)
	}
	openaiGroup := router.Group("/openai")
	openaiGroup.Use(auth.AuthMiddleware(dbService))
	openaiGroup.GET("/*path", openaiHandlerFunc)
	openaiGroup.POST("/*path", openaiHandlerFunc)

	// Serve frontend
	distFS, err := fs.Sub(webUI, "dist")
	if err != nil {
		log.Error("failed to create sub file system for frontend", "error", err)
		return err
	}

	// Serve static files from the 'assets' directory
	assetsFS, err := fs.Sub(distFS, "assets")
	if err != nil {
		log.Error("failed to create sub file system for assets", "error", err)
		return err
	}
	router.StaticFS("/assets", http.FS(assetsFS))

	// Serve other static files from the root of dist
	router.StaticFileFS("/vite.svg", "vite.svg", http.FS(distFS))

	// Serve index.html for the root path and any other non-API routes
	handler := func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	}
	router.GET("/", handler)
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/api") &&
			!strings.HasPrefix(path, "/gemini") &&
			!strings.HasPrefix(path, "/openai") {
			handler(c)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"code": "PAGE_NOT_FOUND", "message": "Page not found"})
	})

	// Create and start the main server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		log.Info("Starting server", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Failed to start server", "error", err)
			// In a real app, you might want to signal the main goroutine to exit.
			// For this refactoring, we'll just log it. The original os.Exit(1) is now handled in main.
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("Shutting down server...")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Close the handlers to stop their background tasks
	keyManager.Close()
	openaiProxy.Close()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
		return err
	}

	log.Info("Server exiting")
	return nil
}

func main() {
	// Load configuration
	cfg, warning, err := config.LoadConfig("config.yaml")
	if err != nil {
		// Use a temporary logger for startup errors
		slog.Error("Error loading configuration", "error", err)
		os.Exit(1)
	}

	// Setup logger
	log := logger.New(cfg.Debug)
	log.Info("Logger initialized", "debug_mode", cfg.Debug)
	if warning != "" {
		log.Warn(warning)
	}

	// Initialize database service
	dbService, err := newDBService(cfg.Database)
	if err != nil {
		log.Error("Error initializing database service", "error", err)
		os.Exit(1)
	}
	log.Info("Database service initialized", "type", cfg.Database.Type)

	if err := setupAndRunServer(cfg, log, dbService); err != nil {
		os.Exit(1)
	}
}
