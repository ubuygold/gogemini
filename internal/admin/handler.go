package admin

import (
	"errors"
	"gogemini/internal/db"
	"gogemini/internal/keymanager"
	"gogemini/internal/model"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	db         db.Service
	KeyManager keymanager.Manager
}

func NewHandler(dbService db.Service, km keymanager.Manager) *Handler {
	return &Handler{db: dbService, KeyManager: km}
}

// Gemini Key Handlers

type CreateGeminiKeyRequest struct {
	Key string `json:"key" binding:"required"`
}

type UpdateGeminiKeyRequest struct {
	Key    string `json:"key"`
	Status string `json:"status"`
}

func (h *Handler) ListGeminiKeysHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	statusFilter := c.DefaultQuery("status", "all")
	minFailureCount, _ := strconv.Atoi(c.DefaultQuery("minFailureCount", "0"))

	keys, total, err := h.db.ListGeminiKeys(page, limit, statusFilter, minFailureCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list gemini keys"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"keys":  keys,
		"total": total,
	})
}

func (h *Handler) CreateGeminiKeyHandler(c *gin.Context) {
	var req CreateGeminiKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newKey := &model.GeminiKey{
		Key:    req.Key,
		Status: "active",
	}

	if err := h.db.CreateGeminiKey(newKey); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create gemini key"})
		return
	}
	c.JSON(http.StatusCreated, newKey)
}

func (h *Handler) GetGeminiKeyHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key ID"})
		return
	}
	key, err := h.db.GetGeminiKey(uint(id))
	if err != nil {
		if errors.Is(err, db.ErrGeminiKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Gemini key not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve gemini key"})
		}
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
	var req UpdateGeminiKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Fetch the existing key first
	key, err := h.db.GetGeminiKey(uint(id))
	if err != nil {
		if errors.Is(err, db.ErrGeminiKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Gemini key not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve gemini key"})
		}
		return
	}

	// Apply changes from the request
	if req.Key != "" {
		key.Key = req.Key
	}
	if req.Status != "" {
		key.Status = req.Status
	}

	if err := h.db.UpdateGeminiKey(key); err != nil {
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
func (h *Handler) BatchCreateGeminiKeysHandler(c *gin.Context) {
	var req struct {
		Keys []string `json:"keys"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if err := h.db.BatchAddGeminiKeys(req.Keys); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to batch create gemini keys"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Keys created successfully"})
}

func (h *Handler) BatchDeleteGeminiKeysHandler(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if err := h.db.BatchDeleteGeminiKeys(req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to batch delete gemini keys"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Keys deleted successfully"})
}

func (h *Handler) TestGeminiKeyHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key ID"})
		return
	}

	err = h.KeyManager.TestKeyByID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "failed",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) TestAllGeminiKeysHandler(c *gin.Context) {
	h.KeyManager.TestAllKeysAsync()
	c.JSON(http.StatusAccepted, gin.H{"message": "Batch key test initiated in the background."})
}

// Client Key Handlers

type UpdateClientKeyRequest struct {
	Key         string `json:"key"`
	Status      string `json:"status"`
	Permissions string `json:"permissions"`
}

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
		if errors.Is(err, db.ErrAPIKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Client key not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve client key"})
		}
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
	var req UpdateClientKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	key, err := h.db.GetAPIKey(uint(id))
	if err != nil {
		if errors.Is(err, db.ErrAPIKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Client key not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve client key"})
		}
		return
	}

	if req.Key != "" {
		key.Key = req.Key
	}
	if req.Status != "" {
		key.Status = req.Status
	}
	if req.Permissions != "" {
		key.Permissions = req.Permissions
	}

	if err := h.db.UpdateAPIKey(key); err != nil {
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
