package admin

import (
	"github.com/ubuygold/gogemini/internal/auth"
	"github.com/ubuygold/gogemini/internal/config"
	"github.com/ubuygold/gogemini/internal/db"
	"github.com/ubuygold/gogemini/internal/keymanager"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(router *gin.Engine, dbService db.Service, km keymanager.Manager, cfg *config.Config) {
	handler := NewHandler(dbService, km)

	adminGroup := router.Group("/admin")
	adminGroup.Use(auth.AdminAuthMiddleware(cfg.Admin.Password))
	{
		geminiKeysGroup := adminGroup.Group("/gemini-keys")
		{
			geminiKeysGroup.GET("", handler.ListGeminiKeysHandler)
			geminiKeysGroup.POST("", handler.CreateGeminiKeyHandler)
			geminiKeysGroup.POST("/batch", handler.BatchCreateGeminiKeysHandler)
			geminiKeysGroup.DELETE("/batch", handler.BatchDeleteGeminiKeysHandler)
			geminiKeysGroup.POST("/test", handler.TestAllGeminiKeysHandler) // Bulk test
			geminiKeysGroup.GET("/:id", handler.GetGeminiKeyHandler)
			geminiKeysGroup.PUT("/:id", handler.UpdateGeminiKeyHandler)
			geminiKeysGroup.DELETE("/:id", handler.DeleteGeminiKeyHandler)
			geminiKeysGroup.POST("/:id/test", handler.TestGeminiKeyHandler) // Single test
		}

		clientKeysGroup := adminGroup.Group("/client-keys")
		{
			clientKeysGroup.GET("", handler.ListClientKeysHandler)
			clientKeysGroup.POST("", handler.CreateClientKeyHandler)
			clientKeysGroup.GET("/:id", handler.GetClientKeyHandler)
			clientKeysGroup.PUT("/:id", handler.UpdateClientKeyHandler)
			clientKeysGroup.DELETE("/:id", handler.DeleteClientKeyHandler)
			clientKeysGroup.POST("/:id/reset", handler.ResetClientKeyHandler)
		}
	}
}
