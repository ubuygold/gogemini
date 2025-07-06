package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gogemini/internal/auth"
	"gogemini/internal/balancer"
	"gogemini/internal/config"
	"gogemini/internal/proxy"

	"github.com/gin-gonic/gin"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	ctx := context.Background()

	// Load configuration
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Create the authorized keys map for the middleware
	authorizedKeys := make(map[string]bool)
	for _, key := range cfg.ClientKeys {
		authorizedKeys[key] = true
	}

	// Create the new SDK-based handler for Gemini
	geminiHandler, err := balancer.NewBalancer(cfg)
	if err != nil {
		log.Fatalf("Error creating Gemini handler: %v", err)
	}

	// Create the new reverse proxy for OpenAI
	openaiProxy, err := proxy.NewOpenAIProxy(cfg.GeminiKeys)
	if err != nil {
		log.Fatalf("Error creating OpenAI proxy: %v", err)
	}

	// Create a Gin router without the default logger
	router := gin.New()
	router.Use(gin.Recovery()) // Add the recovery middleware

	// Create a group for Gemini routes
	geminiHandlerFunc := func(c *gin.Context) {
		http.StripPrefix("/gemini", geminiHandler).ServeHTTP(c.Writer, c.Request)
	}
	geminiGroup := router.Group("/gemini")
	geminiGroup.Use(auth.AuthMiddleware(authorizedKeys))
	geminiGroup.GET("/*path", geminiHandlerFunc)
	geminiGroup.POST("/*path", geminiHandlerFunc)

	// Create a group for OpenAI routes
	openaiHandlerFunc := func(c *gin.Context) {
		http.StripPrefix("/openai", openaiProxy).ServeHTTP(c.Writer, c.Request)
	}
	openaiGroup := router.Group("/openai")
	openaiGroup.Use(auth.AuthMiddleware(authorizedKeys))
	openaiGroup.GET("/*path", openaiHandlerFunc)
	openaiGroup.POST("/*path", openaiHandlerFunc)

	// Create and start the main server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Starting server on port %d. Endpoints: /gemini/ and /openai/", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}
