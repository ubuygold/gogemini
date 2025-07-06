package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"gogemini/internal/auth"
	"gogemini/internal/balancer"
	"gogemini/internal/config"
	"gogemini/internal/db"
	"gogemini/internal/logger"
	"gogemini/internal/proxy"
	"gogemini/internal/scheduler"

	"github.com/gin-gonic/gin"
)

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

	// Initialize database
	database, err := db.Init(cfg.Database)
	if err != nil {
		log.Error("Error initializing database", "error", err)
		os.Exit(1)
	}
	log.Info("Database initialized", "type", cfg.Database.Type)

	// Start the scheduler
	scheduler.StartScheduler(database)
	log.Info("Scheduler started")

	// Create the new SDK-based handler for Gemini
	geminiHandler, err := balancer.NewBalancer(database, log)
	if err != nil {
		log.Error("Error creating Gemini handler", "error", err)
		os.Exit(1)
	}

	// Create the new reverse proxy for OpenAI
	openaiProxy, err := proxy.NewOpenAIProxy(database, cfg.Debug, log)
	if err != nil {
		log.Error("Error creating OpenAI proxy", "error", err)
		os.Exit(1)
	}

	// Create a Gin router
	router := gin.New()
	// Use our custom recovery middleware instead of the default one.
	router.Use(customRecovery(log))

	// If debug mode is enabled, add the logger middleware
	if cfg.Debug {
		// This uses the default gin logger, which is fine for development.
		router.Use(gin.Logger())
	}

	// Create a group for Gemini routes
	geminiHandlerFunc := func(c *gin.Context) {
		http.StripPrefix("/gemini", geminiHandler).ServeHTTP(c.Writer, c.Request)
	}
	geminiGroup := router.Group("/gemini")
	geminiGroup.Use(auth.AuthMiddleware(database))
	geminiGroup.GET("/*path", geminiHandlerFunc)
	geminiGroup.POST("/*path", geminiHandlerFunc)

	// Create a group for OpenAI routes
	openaiHandlerFunc := func(c *gin.Context) {
		http.StripPrefix("/openai", openaiProxy).ServeHTTP(c.Writer, c.Request)
	}
	openaiGroup := router.Group("/openai")
	openaiGroup.Use(auth.AuthMiddleware(database))
	openaiGroup.GET("/*path", openaiHandlerFunc)
	openaiGroup.POST("/*path", openaiHandlerFunc)

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
			os.Exit(1)
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
	geminiHandler.Close()
	openaiProxy.Close()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	log.Info("Server exiting")
}
