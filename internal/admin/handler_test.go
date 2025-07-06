package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gogemini/internal/config"
	"gogemini/internal/db"
	"gogemini/internal/model"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// mockDBService is a mock implementation of the db.Service interface for testing.
type mockDBService struct {
	batchAddError    error
	batchDeleteError error
}

func (m *mockDBService) BatchAddGeminiKeys(keys []string) error           { return m.batchAddError }
func (m *mockDBService) BatchDeleteGeminiKeys(keys []string) error        { return m.batchDeleteError }
func (m *mockDBService) LoadActiveGeminiKeys() ([]model.GeminiKey, error) { return nil, nil }
func (m *mockDBService) HandleGeminiKeyFailure(key string, disableThreshold int) (bool, error) {
	return false, nil
}
func (m *mockDBService) ResetGeminiKeyFailureCount(key string) error   { return nil }
func (m *mockDBService) IncrementGeminiKeyUsageCount(key string) error { return nil }
func (m *mockDBService) IncrementAPIKeyUsageCount(key string) error    { return nil }
func (m *mockDBService) ResetAllAPIKeyUsage() error                    { return nil }
func (m *mockDBService) GetDB() *gorm.DB                               { return nil }

func setupTestRouter(dbService db.Service) *gin.Engine {
	handler := NewHandler(dbService)
	router := gin.New()
	adminGroup := router.Group("/admin")
	adminGroup.POST("/keys/add", handler.AddKeysHandler)
	adminGroup.POST("/keys/delete", handler.DeleteKeysHandler)
	return router
}

func setupRealDB(t *testing.T) db.Service {
	service, err := db.NewService(config.DatabaseConfig{
		Type: "sqlite",
		DSN:  "file::memory:",
	})
	if err != nil {
		t.Fatalf("Failed to create real db service: %v", err)
	}
	return service
}

func TestAddKeysHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success", func(t *testing.T) {
		dbService := setupRealDB(t)
		router := setupTestRouter(dbService)

		keys := []string{"key1", "key2"}
		body, _ := json.Marshal(map[string][]string{"keys": keys})
		req, _ := http.NewRequest(http.MethodPost, "/admin/keys/add", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), "Keys added successfully")
	})

	t.Run("bad request - empty keys", func(t *testing.T) {
		dbService := setupRealDB(t)
		router := setupTestRouter(dbService)

		req, _ := http.NewRequest(http.MethodPost, "/admin/keys/add", bytes.NewBufferString(`{"keys":[]}`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "Keys list cannot be empty")
	})

	t.Run("db error", func(t *testing.T) {
		mockDB := &mockDBService{batchAddError: fmt.Errorf("db error")}
		router := setupTestRouter(mockDB)

		keys := []string{"key1", "key2"}
		body, _ := json.Marshal(map[string][]string{"keys": keys})
		req, _ := http.NewRequest(http.MethodPost, "/admin/keys/add", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		assert.Contains(t, resp.Body.String(), "Failed to add keys")
	})

	t.Run("nil db", func(t *testing.T) {
		router := setupTestRouter(nil) // Pass nil for the db service

		keys := []string{"key1", "key2"}
		body, _ := json.Marshal(map[string][]string{"keys": keys})
		req, _ := http.NewRequest(http.MethodPost, "/admin/keys/add", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		assert.Contains(t, resp.Body.String(), "Database not configured")
	})
}

func TestDeleteKeysHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success", func(t *testing.T) {
		dbService := setupRealDB(t)
		router := setupTestRouter(dbService)

		keys := []string{"key1", "key2"}
		body, _ := json.Marshal(map[string][]string{"keys": keys})
		req, _ := http.NewRequest(http.MethodPost, "/admin/keys/delete", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), "Keys deleted successfully")
	})

	t.Run("invalid json", func(t *testing.T) {
		dbService := setupRealDB(t)
		router := setupTestRouter(dbService)

		req, _ := http.NewRequest(http.MethodPost, "/admin/keys/delete", bytes.NewBufferString(`{`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "Invalid request body")
	})

	t.Run("bad request - empty keys", func(t *testing.T) {
		dbService := setupRealDB(t)
		router := setupTestRouter(dbService)

		req, _ := http.NewRequest(http.MethodPost, "/admin/keys/delete", bytes.NewBufferString(`{"keys":[]}`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "Keys list cannot be empty")
	})

	t.Run("db error", func(t *testing.T) {
		mockDB := &mockDBService{batchDeleteError: fmt.Errorf("db error")}
		router := setupTestRouter(mockDB)

		keys := []string{"key1", "key2"}
		body, _ := json.Marshal(map[string][]string{"keys": keys})
		req, _ := http.NewRequest(http.MethodPost, "/admin/keys/delete", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		assert.Contains(t, resp.Body.String(), "Failed to delete keys")
	})

	t.Run("nil db", func(t *testing.T) {
		router := setupTestRouter(nil) // Pass nil for the db service

		keys := []string{"key1", "key2"}
		body, _ := json.Marshal(map[string][]string{"keys": keys})
		req, _ := http.NewRequest(http.MethodPost, "/admin/keys/delete", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		assert.Contains(t, resp.Body.String(), "Database not configured")
	})
}
