package admin

import (
	"gogemini/internal/db"
	"gogemini/internal/model"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	db db.Service
}

func NewHandler(dbService db.Service) *Handler {
	return &Handler{db: dbService}
}

// Gemini Key Handlers

func (h *Handler) ListGeminiKeysHandler(c *gin.Context) {
	keys, err := h.db.ListGeminiKeys()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list gemini keys"})
		return
	}
	c.JSON(http.StatusOK, keys)
}

func (h *Handler) CreateGeminiKeyHandler(c *gin.Context) {
	var key model.GeminiKey
	if err := c.ShouldBindJSON(&key); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if err := h.db.CreateGeminiKey(&key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create gemini key"})
		return
	}
	c.JSON(http.StatusCreated, key)
}

func (h *Handler) GetGeminiKeyHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key ID"})
		return
	}
	key, err := h.db.GetGeminiKey(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gemini key not found"})
		return
	}
	c.JSON(http.StatusOK, key)
}

func (h *Handler) UpdateGeminiKeyHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key ID"})
		return
	}
	var key model.GeminiKey
	if err := c.ShouldBindJSON(&key); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	key.ID = uint(id)
	if err := h.db.UpdateGeminiKey(&key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update gemini key"})
		return
	}
	c.JSON(http.StatusOK, key)
}

func (h *Handler) DeleteGeminiKeyHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key ID"})
		return
	}
	if err := h.db.DeleteGeminiKey(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete gemini key"})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

// Client Key Handlers

func (h *Handler) ListClientKeysHandler(c *gin.Context) {
	keys, err := h.db.ListAPIKeys()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list client keys"})
		return
	}
	c.JSON(http.StatusOK, keys)
}

func (h *Handler) CreateClientKeyHandler(c *gin.Context) {
	var key model.APIKey
	if err := c.ShouldBindJSON(&key); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if err := h.db.CreateAPIKey(&key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create client key"})
		return
	}
	c.JSON(http.StatusCreated, key)
}

func (h *Handler) GetClientKeyHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key ID"})
		return
	}
	key, err := h.db.GetAPIKey(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Client key not found"})
		return
	}
	c.JSON(http.StatusOK, key)
}

func (h *Handler) UpdateClientKeyHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key ID"})
		return
	}
	var key model.APIKey
	if err := c.ShouldBindJSON(&key); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	key.ID = uint(id)
	if err := h.db.UpdateAPIKey(&key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update client key"})
		return
	}
	c.JSON(http.StatusOK, key)
}

func (h *Handler) DeleteClientKeyHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key ID"})
		return
	}
	if err := h.db.DeleteAPIKey(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete client key"})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}
