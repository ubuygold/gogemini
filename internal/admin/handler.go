package admin

import (
	"gogemini/internal/db"
	"net/http"

	"github.com/gin-gonic/gin"
)

type KeysRequest struct {
	Keys []string `json:"keys"`
}

type Handler struct {
	db db.Service
}

func NewHandler(dbService db.Service) *Handler {
	return &Handler{db: dbService}
}

func (h *Handler) AddKeysHandler(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not configured"})
		return
	}
	var req KeysRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if len(req.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Keys list cannot be empty"})
		return
	}

	if err := h.db.BatchAddGeminiKeys(req.Keys); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add keys"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Keys added successfully"})
}

func (h *Handler) DeleteKeysHandler(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not configured"})
		return
	}
	var req KeysRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if len(req.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Keys list cannot be empty"})
		return
	}

	if err := h.db.BatchDeleteGeminiKeys(req.Keys); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete keys"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Keys deleted successfully"})
}
