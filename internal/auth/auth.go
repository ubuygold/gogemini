package auth

import (
	"gogemini/internal/db"
	"gogemini/internal/model"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func AuthMiddleware(dbService db.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var token string
		// Check for OpenAI-style Bearer token
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}

		// If no Bearer token, check for Gemini-style API key
		if token == "" {
			token = c.GetHeader("x-goog-api-key")
		}

		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API key is required"})
			return
		}

		var apiKey model.APIKey
		// Use the raw GORM DB instance for the lookup
		result := dbService.GetDB().Where("key = ?", token).First(&apiKey)
		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		if apiKey.Status != "active" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "API key is not active"})
			return
		}

		if !apiKey.ExpiresAt.IsZero() && apiKey.ExpiresAt.Before(time.Now()) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "API key has expired"})
			return
		}

		// Increment usage count in a goroutine to not slow down the request
		go func() {
			_ = dbService.IncrementAPIKeyUsageCount(token)
		}()

		c.Next()
	}
}

func AdminAuthMiddleware(adminPassword string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, password, hasAuth := c.Request.BasicAuth()
		if !hasAuth || user != "admin" || password != adminPassword {
			c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		c.Next()
	}
}
