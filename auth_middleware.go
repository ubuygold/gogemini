package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware(authorizedKeys map[string]bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check for OpenAI-style Bearer token
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				if authorizedKeys[parts[1]] {
					c.Next()
					return
				}
			}
		}

		// Check for Gemini-style API key
		geminiKey := c.GetHeader("x-goog-api-key")
		if geminiKey != "" {
			if authorizedKeys[geminiKey] {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
	}
}
