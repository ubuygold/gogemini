package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration
	config, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Parse target URL
	targetURL, err := url.Parse("https://generativelanguage.googleapis.com")
	if err != nil {
		log.Fatalf("Error parsing target URL: %v", err)
	}

	// Create a new balancer
	balancer := NewBalancer(config, targetURL)

	// Create a new server and register the handler
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: balancer,
	}

	// Start the server
	log.Printf("Server starting on port %s, balancing %d keys. Debug mode: %v", server.Addr, len(config.GeminiKeys), config.Debug)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
	}
}
