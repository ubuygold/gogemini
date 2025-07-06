package admin

import (
	"gogemini/internal/db"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(router *gin.Engine, dbService db.Service) {
	handler := NewHandler(dbService)

	adminGroup := router.Group("/admin")
	adminGroup.POST("/keys/add", handler.AddKeysHandler)
	adminGroup.POST("/keys/delete", handler.DeleteKeysHandler)
}
