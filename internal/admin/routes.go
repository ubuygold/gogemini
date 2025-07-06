package admin

import (
	"gogemini/internal/auth"
	"gogemini/internal/config"
	"gogemini/internal/db"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(router *gin.Engine, dbService db.Service, cfg *config.Config) {
	handler := NewHandler(dbService)

	adminGroup := router.Group("/admin")
	adminGroup.Use(auth.AdminAuthMiddleware(cfg.Admin.Password))
	{
		geminiKeysGroup := adminGroup.Group("/gemini-keys")
		{
			geminiKeysGroup.GET("", handler.ListGeminiKeysHandler)
			geminiKeysGroup.POST("", handler.CreateGeminiKeyHandler)
			geminiKeysGroup.GET("/:id", handler.GetGeminiKeyHandler)
			geminiKeysGroup.PUT("/:id", handler.UpdateGeminiKeyHandler)
			geminiKeysGroup.DELETE("/:id", handler.DeleteGeminiKeyHandler)
		}

		clientKeysGroup := adminGroup.Group("/client-keys")
		{
			clientKeysGroup.GET("", handler.ListClientKeysHandler)
			clientKeysGroup.POST("", handler.CreateClientKeyHandler)
			clientKeysGroup.GET("/:id", handler.GetClientKeyHandler)
			clientKeysGroup.PUT("/:id", handler.UpdateClientKeyHandler)
			clientKeysGroup.DELETE("/:id", handler.DeleteClientKeyHandler)
		}
	}
}
