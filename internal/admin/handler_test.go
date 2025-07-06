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

	"errors"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// mockDBService is a mock implementation of the db.Service interface for testing.
type mockDBService struct {
	db.Service
	listGeminiKeysErr  error
	createGeminiKeyErr error
	getGeminiKeyErr    error
	updateGeminiKeyErr error
	deleteGeminiKeyErr error
	listClientKeysErr  error
	createClientKeyErr error
	getClientKeyErr    error
	updateClientKeyErr error
	deleteClientKeyErr error
}

func (m *mockDBService) ListGeminiKeys() ([]model.GeminiKey, error) {
	if m.listGeminiKeysErr != nil {
		return nil, m.listGeminiKeysErr
	}
	return []model.GeminiKey{}, nil
}

func (m *mockDBService) CreateGeminiKey(key *model.GeminiKey) error {
	return m.createGeminiKeyErr
}

func (m *mockDBService) GetGeminiKey(id uint) (*model.GeminiKey, error) {
	if m.getGeminiKeyErr != nil {
		return nil, m.getGeminiKeyErr
	}
	return &model.GeminiKey{Model: gorm.Model{ID: id}}, nil
}

func (m *mockDBService) UpdateGeminiKey(key *model.GeminiKey) error {
	return m.updateGeminiKeyErr
}

func (m *mockDBService) DeleteGeminiKey(id uint) error {
	return m.deleteGeminiKeyErr
}

func (m *mockDBService) ListAPIKeys() ([]model.APIKey, error) {
	if m.listClientKeysErr != nil {
		return nil, m.listClientKeysErr
	}
	return []model.APIKey{}, nil
}

func (m *mockDBService) CreateAPIKey(key *model.APIKey) error {
	return m.createClientKeyErr
}

func (m *mockDBService) GetAPIKey(id uint) (*model.APIKey, error) {
	if m.getClientKeyErr != nil {
		return nil, m.getClientKeyErr
	}
	return &model.APIKey{Model: gorm.Model{ID: id}}, nil
}

func (m *mockDBService) UpdateAPIKey(key *model.APIKey) error {
	return m.updateClientKeyErr
}

func (m *mockDBService) DeleteAPIKey(id uint) error {
	return m.deleteClientKeyErr
}

func setupTestRouter(dbService db.Service, cfg *config.Config) *gin.Engine {
	router := gin.New()
	SetupRoutes(router, dbService, cfg)
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

func TestGeminiKeyHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbService := setupRealDB(t)
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	router := setupTestRouter(dbService, cfg)

	// Test without auth
	req, _ := http.NewRequest(http.MethodGet, "/admin/gemini-keys", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	// 1. Create a key
	createBody := `{"key": "test-gemini-key"}`
	req, _ = http.NewRequest(http.MethodPost, "/admin/gemini-keys", bytes.NewBufferString(createBody))
	req.SetBasicAuth("admin", "test-password")
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusCreated, resp.Code)
	var createdKey model.GeminiKey
	err := json.Unmarshal(resp.Body.Bytes(), &createdKey)
	assert.NoError(t, err)
	assert.Equal(t, "test-gemini-key", createdKey.Key)

	// 2. Get the key
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/admin/gemini-keys/%d", createdKey.ID), nil)
	req.SetBasicAuth("admin", "test-password")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var fetchedKey model.GeminiKey
	err = json.Unmarshal(resp.Body.Bytes(), &fetchedKey)
	assert.NoError(t, err)
	assert.Equal(t, createdKey.ID, fetchedKey.ID)

	// 3. List keys
	req, _ = http.NewRequest(http.MethodGet, "/admin/gemini-keys", nil)
	req.SetBasicAuth("admin", "test-password")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var keys []model.GeminiKey
	err = json.Unmarshal(resp.Body.Bytes(), &keys)
	assert.NoError(t, err)
	assert.NotEmpty(t, keys)

	// 4. Update the key
	updateBody := `{"key": "updated-gemini-key", "status": "disabled"}`
	req, _ = http.NewRequest(http.MethodPut, fmt.Sprintf("/admin/gemini-keys/%d", createdKey.ID), bytes.NewBufferString(updateBody))
	req.SetBasicAuth("admin", "test-password")
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var updatedKey model.GeminiKey
	err = json.Unmarshal(resp.Body.Bytes(), &updatedKey)
	assert.NoError(t, err)
	assert.Equal(t, "updated-gemini-key", updatedKey.Key)
	assert.Equal(t, "disabled", updatedKey.Status)

	// 5. Delete the key
	req, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/gemini-keys/%d", createdKey.ID), nil)
	req.SetBasicAuth("admin", "test-password")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestGeminiKeyHandlers_ErrorCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}

	t.Run("ListGeminiKeysHandler returns error", func(t *testing.T) {
		mockDB := &mockDBService{listGeminiKeysErr: errors.New("db error")}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodGet, "/admin/gemini-keys", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("CreateGeminiKeyHandler returns error on binding", func(t *testing.T) {
		mockDB := &mockDBService{}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys", bytes.NewBufferString(`{"key":`))
		req.SetBasicAuth("admin", "test-password")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("CreateGeminiKeyHandler returns error on db", func(t *testing.T) {
		mockDB := &mockDBService{createGeminiKeyErr: errors.New("db error")}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys", bytes.NewBufferString(`{"key": "test-key"}`))
		req.SetBasicAuth("admin", "test-password")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("GetGeminiKeyHandler returns error on invalid id", func(t *testing.T) {
		mockDB := &mockDBService{}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodGet, "/admin/gemini-keys/abc", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("GetGeminiKeyHandler returns error on db", func(t *testing.T) {
		mockDB := &mockDBService{getGeminiKeyErr: errors.New("db error")}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodGet, "/admin/gemini-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("UpdateGeminiKeyHandler returns error on invalid id", func(t *testing.T) {
		mockDB := &mockDBService{}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodPut, "/admin/gemini-keys/abc", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("UpdateGeminiKeyHandler returns error on binding", func(t *testing.T) {
		mockDB := &mockDBService{}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodPut, "/admin/gemini-keys/1", bytes.NewBufferString(`{"key":`))
		req.SetBasicAuth("admin", "test-password")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("UpdateGeminiKeyHandler returns error on db", func(t *testing.T) {
		mockDB := &mockDBService{updateGeminiKeyErr: errors.New("db error")}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodPut, "/admin/gemini-keys/1", bytes.NewBufferString(`{"key": "test-key"}`))
		req.SetBasicAuth("admin", "test-password")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("DeleteGeminiKeyHandler returns error on invalid id", func(t *testing.T) {
		mockDB := &mockDBService{}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodDelete, "/admin/gemini-keys/abc", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("DeleteGeminiKeyHandler returns error on db", func(t *testing.T) {
		mockDB := &mockDBService{deleteGeminiKeyErr: errors.New("db error")}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodDelete, "/admin/gemini-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})
}

func TestClientKeyHandlers_ErrorCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}

	t.Run("ListClientKeysHandler returns error", func(t *testing.T) {
		mockDB := &mockDBService{listClientKeysErr: errors.New("db error")}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodGet, "/admin/client-keys", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("CreateClientKeyHandler returns error on binding", func(t *testing.T) {
		mockDB := &mockDBService{}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodPost, "/admin/client-keys", bytes.NewBufferString(`{"key":`))
		req.SetBasicAuth("admin", "test-password")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("CreateClientKeyHandler returns error on db", func(t *testing.T) {
		mockDB := &mockDBService{createClientKeyErr: errors.New("db error")}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodPost, "/admin/client-keys", bytes.NewBufferString(`{"key": "test-key"}`))
		req.SetBasicAuth("admin", "test-password")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("GetClientKeyHandler returns error on invalid id", func(t *testing.T) {
		mockDB := &mockDBService{}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodGet, "/admin/client-keys/abc", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("GetClientKeyHandler returns error on db", func(t *testing.T) {
		mockDB := &mockDBService{getClientKeyErr: errors.New("db error")}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodGet, "/admin/client-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("UpdateClientKeyHandler returns error on invalid id", func(t *testing.T) {
		mockDB := &mockDBService{}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodPut, "/admin/client-keys/abc", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("UpdateClientKeyHandler returns error on binding", func(t *testing.T) {
		mockDB := &mockDBService{}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodPut, "/admin/client-keys/1", bytes.NewBufferString(`{"key":`))
		req.SetBasicAuth("admin", "test-password")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("UpdateClientKeyHandler returns error on db", func(t *testing.T) {
		mockDB := &mockDBService{updateClientKeyErr: errors.New("db error")}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodPut, "/admin/client-keys/1", bytes.NewBufferString(`{"key": "test-key"}`))
		req.SetBasicAuth("admin", "test-password")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("DeleteClientKeyHandler returns error on invalid id", func(t *testing.T) {
		mockDB := &mockDBService{}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodDelete, "/admin/client-keys/abc", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("DeleteClientKeyHandler returns error on db", func(t *testing.T) {
		mockDB := &mockDBService{deleteClientKeyErr: errors.New("db error")}
		router := setupTestRouter(mockDB, cfg)
		req, _ := http.NewRequest(http.MethodDelete, "/admin/client-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})
}

func TestClientKeyHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbService := setupRealDB(t)
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	router := setupTestRouter(dbService, cfg)

	// 1. Create a key
	createBody := `{"key": "test-client-key", "permissions": "read,write"}`
	req, _ := http.NewRequest(http.MethodPost, "/admin/client-keys", bytes.NewBufferString(createBody))
	req.SetBasicAuth("admin", "test-password")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusCreated, resp.Code)
	var createdKey model.APIKey
	err := json.Unmarshal(resp.Body.Bytes(), &createdKey)
	assert.NoError(t, err)
	assert.Equal(t, "test-client-key", createdKey.Key)

	// 2. Get the key
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/admin/client-keys/%d", createdKey.ID), nil)
	req.SetBasicAuth("admin", "test-password")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var fetchedKey model.APIKey
	err = json.Unmarshal(resp.Body.Bytes(), &fetchedKey)
	assert.NoError(t, err)
	assert.Equal(t, createdKey.ID, fetchedKey.ID)

	// 3. List keys
	req, _ = http.NewRequest(http.MethodGet, "/admin/client-keys", nil)
	req.SetBasicAuth("admin", "test-password")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var keys []model.APIKey
	err = json.Unmarshal(resp.Body.Bytes(), &keys)
	assert.NoError(t, err)
	assert.NotEmpty(t, keys)

	// 4. Update the key
	updateBody := `{"key": "updated-client-key", "status": "inactive"}`
	req, _ = http.NewRequest(http.MethodPut, fmt.Sprintf("/admin/client-keys/%d", createdKey.ID), bytes.NewBufferString(updateBody))
	req.SetBasicAuth("admin", "test-password")
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var updatedKey model.APIKey
	err = json.Unmarshal(resp.Body.Bytes(), &updatedKey)
	assert.NoError(t, err)
	assert.Equal(t, "updated-client-key", updatedKey.Key)
	assert.Equal(t, "inactive", updatedKey.Status)

	// 5. Delete the key
	req, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/client-keys/%d", createdKey.ID), nil)
	req.SetBasicAuth("admin", "test-password")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNoContent, resp.Code)
}
