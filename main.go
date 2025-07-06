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

	"github.com/gin-gonic/gin"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	ctx := context.Background()

	// Load configuration
	config, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Create the authorized keys map for the middleware
	authorizedKeys := make(map[string]bool)
	for _, key := range config.ClientKeys {
		authorizedKeys[key] = true
	}

	// Create the new SDK-based handler for Gemini
	geminiHandler, err := NewBalancer(config)
	if err != nil {
		log.Fatalf("Error creating Gemini handler: %v", err)
	}

	// Create the new reverse proxy for OpenAI
	openaiProxy, err := NewOpenAIProxy(config.GeminiKeys)
	if err != nil {
		log.Fatalf("Error creating OpenAI proxy: %v", err)
	}

	// Create a Gin router
	router := gin.Default()

	// Create a group for Gemini routes
	geminiGroup := router.Group("/gemini")
	geminiGroup.Use(AuthMiddleware(authorizedKeys))
	geminiGroup.Any("/*path", func(c *gin.Context) {
		// By creating a custom handler, we can ensure that the underlying
		// http.ResponseWriter is not buffered by Gin, which is crucial for streaming.
		http.StripPrefix("/gemini", geminiHandler).ServeHTTP(c.Writer, c.Request)
	})

	// Create a group for OpenAI routes
	openaiGroup := router.Group("/openai")
	openaiGroup.Use(AuthMiddleware(authorizedKeys))
	openaiGroup.Any("/*path", func(c *gin.Context) {
		http.StripPrefix("/openai", openaiProxy).ServeHTTP(c.Writer, c.Request)
	})

	// Create and start the main server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Starting server on port %d. Endpoints: /gemini/ and /openai/", config.Port)
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
