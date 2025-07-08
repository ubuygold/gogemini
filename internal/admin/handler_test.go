package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ubuygold/gogemini/internal/config"
	"github.com/ubuygold/gogemini/internal/db"
	"github.com/ubuygold/gogemini/internal/keymanager"
	"github.com/ubuygold/gogemini/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// mockDBService is a mock implementation of the db.Service interface for testing.
type mockDBService struct {
	db.Service // Embed interface to avoid implementing all methods
	mock.Mock
}

func (m *mockDBService) GetGeminiKey(id uint) (*model.GeminiKey, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.GeminiKey), args.Error(1)
}

func (m *mockDBService) CreateGeminiKey(key *model.GeminiKey) error {
	args := m.Called(key)
	return args.Error(0)
}

func (m *mockDBService) ListGeminiKeys(page, limit int, statusFilter string, minFailureCount int) ([]model.GeminiKey, int64, error) {
	args := m.Called(page, limit, statusFilter, minFailureCount)
	return args.Get(0).([]model.GeminiKey), int64(args.Int(1)), args.Error(2)
}

func (m *mockDBService) UpdateGeminiKey(key *model.GeminiKey) error {
	args := m.Called(key)
	return args.Error(0)
}

func (m *mockDBService) DeleteGeminiKey(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *mockDBService) BatchAddGeminiKeys(keys []string) error {
	args := m.Called(keys)
	return args.Error(0)
}

func (m *mockDBService) BatchDeleteGeminiKeys(ids []uint) error {
	args := m.Called(ids)
	return args.Error(0)
}

func (m *mockDBService) ListAPIKeys() ([]model.APIKey, error) {
	args := m.Called()
	return args.Get(0).([]model.APIKey), args.Error(1)
}

func (m *mockDBService) CreateAPIKey(key *model.APIKey) error {
	args := m.Called(key)
	return args.Error(0)
}

func (m *mockDBService) GetAPIKey(id uint) (*model.APIKey, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.APIKey), args.Error(1)
}

func (m *mockDBService) UpdateAPIKey(key *model.APIKey) error {
	args := m.Called(key)
	return args.Error(0)
}

func (m *mockDBService) DeleteAPIKey(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

// MockKeyManager is a mock for the KeyManager.
type MockKeyManager struct {
	mock.Mock
}

func (m *MockKeyManager) GetNextKey() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}
func (m *MockKeyManager) HandleKeyFailure(key string) { m.Called(key) }
func (m *MockKeyManager) HandleKeySuccess(key string) { m.Called(key) }
func (m *MockKeyManager) ReviveDisabledKeys()         { m.Called() }
func (m *MockKeyManager) CheckAllKeysHealth()         { m.Called() }
func (m *MockKeyManager) GetAvailableKeyCount() int   { args := m.Called(); return args.Int(0) }
func (m *MockKeyManager) TestKeyByID(id uint) error   { args := m.Called(id); return args.Error(0) }
func (m *MockKeyManager) TestAllKeysAsync()           { m.Called() }
func (m *MockKeyManager) Close()                      { m.Called() }

func setupTestRouter(dbService db.Service, km keymanager.Manager, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	SetupRoutes(router, dbService, km, cfg)
	return router
}

func TestKeyTestHandlers(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	mockKM := &MockKeyManager{}

	router := setupTestRouter(mockDB, mockKM, cfg)

	t.Run("TestGeminiKeyHandler success", func(t *testing.T) {
		mockKM.On("TestKeyByID", uint(1)).Return(nil).Once()

		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys/1/test", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var jsonResp map[string]string
		err := json.Unmarshal(resp.Body.Bytes(), &jsonResp)
		assert.NoError(t, err)
		assert.Equal(t, "ok", jsonResp["status"])

		mockKM.AssertExpectations(t)
	})

	t.Run("TestGeminiKeyHandler failure", func(t *testing.T) {
		mockKM.On("TestKeyByID", uint(2)).Return(errors.New("key is invalid")).Once()

		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys/2/test", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		var jsonResp map[string]string
		err := json.Unmarshal(resp.Body.Bytes(), &jsonResp)
		assert.NoError(t, err)
		assert.Equal(t, "failed", jsonResp["status"])
		assert.Contains(t, jsonResp["error"], "key is invalid")

		mockKM.AssertExpectations(t)
	})

	t.Run("TestAllGeminiKeysHandler", func(t *testing.T) {
		mockKM.On("TestAllKeysAsync").Return().Once()

		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys/test", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusAccepted, resp.Code)
		var jsonResp map[string]string
		err := json.Unmarshal(resp.Body.Bytes(), &jsonResp)
		assert.NoError(t, err)
		assert.Equal(t, "Batch key test initiated in the background.", jsonResp["message"])

		mockKM.AssertExpectations(t)
	})

	t.Run("TestGeminiKeyHandler invalid id", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys/abc/test", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

// Note: The original extensive tests are simplified here to focus on the new functionality.
// A full test suite would have more comprehensive checks for all handlers.
func TestGetGeminiKeyHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	// For this test, the key manager is not used, so we can pass a nil mock.
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("GetGeminiKeyHandler success", func(t *testing.T) {
		expectedKey := &model.GeminiKey{Model: gorm.Model{ID: 1}, Key: "test-key"}
		mockDB.On("GetGeminiKey", uint(1)).Return(expectedKey, nil).Once()

		req, _ := http.NewRequest(http.MethodGet, "/admin/gemini-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var fetchedKey model.GeminiKey
		err := json.Unmarshal(resp.Body.Bytes(), &fetchedKey)
		assert.NoError(t, err)
		assert.Equal(t, *expectedKey, fetchedKey)
		mockDB.AssertExpectations(t)
	})

	t.Run("GetGeminiKeyHandler not found", func(t *testing.T) {
		mockDB.On("GetGeminiKey", uint(2)).Return(nil, db.ErrGeminiKeyNotFound).Once()

		req, _ := http.NewRequest(http.MethodGet, "/admin/gemini-keys/2", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNotFound, resp.Code)
		mockDB.AssertExpectations(t)
	})
}

func TestCreateGeminiKeyHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("CreateGeminiKeyHandler success", func(t *testing.T) {
		mockDB.On("CreateGeminiKey", mock.AnythingOfType("*model.GeminiKey")).Return(nil).Once()

		body := `{"key": "new-key"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("CreateGeminiKeyHandler invalid body", func(t *testing.T) {
		body := `{"key":}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("CreateGeminiKeyHandler db error", func(t *testing.T) {
		mockDB.On("CreateGeminiKey", mock.AnythingOfType("*model.GeminiKey")).Return(errors.New("db error")).Once()

		body := `{"key": "new-key"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})
}

func TestUpdateGeminiKeyHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("UpdateGeminiKeyHandler success", func(t *testing.T) {
		existingKey := &model.GeminiKey{Model: gorm.Model{ID: 1}, Key: "old-key", Status: "active"}
		mockDB.On("GetGeminiKey", uint(1)).Return(existingKey, nil).Once()
		mockDB.On("UpdateGeminiKey", mock.AnythingOfType("*model.GeminiKey")).Return(nil).Once()

		body := `{"key": "new-key", "status": "disabled"}`
		req, _ := http.NewRequest(http.MethodPut, "/admin/gemini-keys/1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var updatedKey model.GeminiKey
		json.Unmarshal(resp.Body.Bytes(), &updatedKey)
		assert.Equal(t, "new-key", updatedKey.Key)
		assert.Equal(t, "disabled", updatedKey.Status)
		mockDB.AssertExpectations(t)
	})

	t.Run("UpdateGeminiKeyHandler not found", func(t *testing.T) {
		mockDB.On("GetGeminiKey", uint(1)).Return(nil, db.ErrGeminiKeyNotFound).Once()

		body := `{"key": "new-key"}`
		req, _ := http.NewRequest(http.MethodPut, "/admin/gemini-keys/1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNotFound, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("UpdateGeminiKeyHandler invalid id", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPut, "/admin/gemini-keys/abc", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestDeleteGeminiKeyHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("DeleteGeminiKeyHandler success", func(t *testing.T) {
		mockDB.On("DeleteGeminiKey", uint(1)).Return(nil).Once()

		req, _ := http.NewRequest(http.MethodDelete, "/admin/gemini-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNoContent, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("DeleteGeminiKeyHandler db error", func(t *testing.T) {
		mockDB.On("DeleteGeminiKey", uint(1)).Return(errors.New("db error")).Once()

		req, _ := http.NewRequest(http.MethodDelete, "/admin/gemini-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("DeleteGeminiKeyHandler invalid id", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, "/admin/gemini-keys/abc", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestBatchCreateGeminiKeysHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("BatchCreateGeminiKeysHandler success", func(t *testing.T) {
		keys := []string{"key1", "key2"}
		mockDB.On("BatchAddGeminiKeys", keys).Return(nil).Once()

		body := `{"keys": ["key1", "key2"]}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys/batch", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("BatchCreateGeminiKeysHandler invalid body", func(t *testing.T) {
		body := `{"keys": "not-an-array"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys/batch", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestBatchDeleteGeminiKeysHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("BatchDeleteGeminiKeysHandler success", func(t *testing.T) {
		ids := []uint{1, 2}
		mockDB.On("BatchDeleteGeminiKeys", ids).Return(nil).Once()

		body := `{"ids": [1, 2]}`
		req, _ := http.NewRequest(http.MethodDelete, "/admin/gemini-keys/batch", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("BatchDeleteGeminiKeysHandler invalid body", func(t *testing.T) {
		body := `{"ids": "not-an-array"}`
		req, _ := http.NewRequest(http.MethodDelete, "/admin/gemini-keys/batch", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestListClientKeysHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("ListClientKeysHandler success", func(t *testing.T) {
		expectedKeys := []model.APIKey{{Model: gorm.Model{ID: 1}, Key: "client-key-1"}}
		mockDB.On("ListAPIKeys").Return(expectedKeys, nil).Once()

		req, _ := http.NewRequest(http.MethodGet, "/admin/client-keys", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var keys []model.APIKey
		json.Unmarshal(resp.Body.Bytes(), &keys)
		assert.Len(t, keys, 1)
		assert.Equal(t, "client-key-1", keys[0].Key)
		mockDB.AssertExpectations(t)
	})
}

func TestCreateClientKeyHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("CreateClientKeyHandler success", func(t *testing.T) {
		mockDB.On("CreateAPIKey", mock.AnythingOfType("*model.APIKey")).Return(nil).Once()

		body := `{"key": "new-client-key", "permissions": "read"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/client-keys", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)
		mockDB.AssertExpectations(t)
	})
}

func TestGetClientKeyHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("GetClientKeyHandler success", func(t *testing.T) {
		expectedKey := &model.APIKey{Model: gorm.Model{ID: 1}, Key: "client-key-1"}
		mockDB.On("GetAPIKey", uint(1)).Return(expectedKey, nil).Once()

		req, _ := http.NewRequest(http.MethodGet, "/admin/client-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var key model.APIKey
		json.Unmarshal(resp.Body.Bytes(), &key)
		assert.Equal(t, "client-key-1", key.Key)
		mockDB.AssertExpectations(t)
	})

	t.Run("GetClientKeyHandler not found", func(t *testing.T) {
		mockDB.On("GetAPIKey", uint(1)).Return(nil, db.ErrAPIKeyNotFound).Once()

		req, _ := http.NewRequest(http.MethodGet, "/admin/client-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNotFound, resp.Code)
		mockDB.AssertExpectations(t)
	})
}

func TestUpdateClientKeyHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("UpdateClientKeyHandler success", func(t *testing.T) {
		existingKey := &model.APIKey{Model: gorm.Model{ID: 1}, Key: "old-key"}
		mockDB.On("GetAPIKey", uint(1)).Return(existingKey, nil).Once()
		mockDB.On("UpdateAPIKey", mock.AnythingOfType("*model.APIKey")).Return(nil).Once()

		body := `{"key": "new-key"}`
		req, _ := http.NewRequest(http.MethodPut, "/admin/client-keys/1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		mockDB.AssertExpectations(t)
	})
}

func TestDeleteClientKeyHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("DeleteClientKeyHandler success", func(t *testing.T) {
		mockDB.On("DeleteAPIKey", uint(1)).Return(nil).Once()

		req, _ := http.NewRequest(http.MethodDelete, "/admin/client-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNoContent, resp.Code)
		mockDB.AssertExpectations(t)
	})
}

func TestListGeminiKeysHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("ListGeminiKeysHandler success", func(t *testing.T) {
		expectedKeys := []model.GeminiKey{{Model: gorm.Model{ID: 1}, Key: "key1"}, {Model: gorm.Model{ID: 2}, Key: "key2"}}
		mockDB.On("ListGeminiKeys", 1, 10, "all", 0).Return(expectedKeys, 2, nil).Once()

		req, _ := http.NewRequest(http.MethodGet, "/admin/gemini-keys", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var result map[string]interface{}
		json.Unmarshal(resp.Body.Bytes(), &result)
		assert.Equal(t, float64(2), result["total"])
		keys, ok := result["keys"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, keys, 2)
		mockDB.AssertExpectations(t)
	})

	t.Run("ListGeminiKeysHandler db error", func(t *testing.T) {
		mockDB.On("ListGeminiKeys", 1, 10, "all", 0).Return([]model.GeminiKey{}, 0, errors.New("db error")).Once()

		req, _ := http.NewRequest(http.MethodGet, "/admin/gemini-keys", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})
}

func TestHandlerDBErrors(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	// Key manager is not used in these specific error paths
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("ListGeminiKeysHandler db error", func(t *testing.T) {
		mockDB.On("ListGeminiKeys", 1, 10, "all", 0).Return([]model.GeminiKey{}, 0, errors.New("db error")).Once()

		req, _ := http.NewRequest(http.MethodGet, "/admin/gemini-keys", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("GetGeminiKeyHandler db error", func(t *testing.T) {
		mockDB.On("GetGeminiKey", uint(1)).Return(nil, errors.New("db error")).Once()

		req, _ := http.NewRequest(http.MethodGet, "/admin/gemini-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("UpdateGeminiKeyHandler get error", func(t *testing.T) {
		mockDB.On("GetGeminiKey", uint(1)).Return(nil, errors.New("db error")).Once()

		body := `{"key": "new-key"}`
		req, _ := http.NewRequest(http.MethodPut, "/admin/gemini-keys/1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("UpdateGeminiKeyHandler update error", func(t *testing.T) {
		existingKey := &model.GeminiKey{Model: gorm.Model{ID: 1}}
		mockDB.On("GetGeminiKey", uint(1)).Return(existingKey, nil).Once()
		mockDB.On("UpdateGeminiKey", existingKey).Return(errors.New("db error")).Once()

		body := `{"key": "new-key"}`
		req, _ := http.NewRequest(http.MethodPut, "/admin/gemini-keys/1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("BatchCreateGeminiKeysHandler db error", func(t *testing.T) {
		keys := []string{"key1"}
		mockDB.On("BatchAddGeminiKeys", keys).Return(errors.New("db error")).Once()

		body := `{"keys": ["key1"]}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/gemini-keys/batch", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("BatchDeleteGeminiKeysHandler db error", func(t *testing.T) {
		ids := []uint{1}
		mockDB.On("BatchDeleteGeminiKeys", ids).Return(errors.New("db error")).Once()

		body := `{"ids": [1]}`
		req, _ := http.NewRequest(http.MethodDelete, "/admin/gemini-keys/batch", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("ListClientKeysHandler db error", func(t *testing.T) {
		mockDB.On("ListAPIKeys").Return([]model.APIKey{}, errors.New("db error")).Once()

		req, _ := http.NewRequest(http.MethodGet, "/admin/client-keys", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("CreateClientKeyHandler db error", func(t *testing.T) {
		mockDB.On("CreateAPIKey", mock.AnythingOfType("*model.APIKey")).Return(errors.New("db error")).Once()

		body := `{"key": "new-key"}`
		req, _ := http.NewRequest(http.MethodPost, "/admin/client-keys", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("GetClientKeyHandler db error", func(t *testing.T) {
		mockDB.On("GetAPIKey", uint(1)).Return(nil, errors.New("db error")).Once()

		req, _ := http.NewRequest(http.MethodGet, "/admin/client-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("UpdateClientKeyHandler get error", func(t *testing.T) {
		mockDB.On("GetAPIKey", uint(1)).Return(nil, errors.New("db error")).Once()

		body := `{"key": "new-key"}`
		req, _ := http.NewRequest(http.MethodPut, "/admin/client-keys/1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("UpdateClientKeyHandler update error", func(t *testing.T) {
		existingKey := &model.APIKey{Model: gorm.Model{ID: 1}}
		mockDB.On("GetAPIKey", uint(1)).Return(existingKey, nil).Once()
		mockDB.On("UpdateAPIKey", existingKey).Return(errors.New("db error")).Once()

		body := `{"key": "new-key"}`
		req, _ := http.NewRequest(http.MethodPut, "/admin/client-keys/1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})

	t.Run("DeleteClientKeyHandler db error", func(t *testing.T) {
		mockDB.On("DeleteAPIKey", uint(1)).Return(errors.New("db error")).Once()

		req, _ := http.NewRequest(http.MethodDelete, "/admin/client-keys/1", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockDB.AssertExpectations(t)
	})
}

func TestResetClientKeyHandler(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Password: "test-password"}}
	mockDB := &mockDBService{}
	router := setupTestRouter(mockDB, &MockKeyManager{}, cfg)

	t.Run("ResetClientKeyHandler success", func(t *testing.T) {
		originalKey := &model.APIKey{
			Model:      gorm.Model{ID: 1},
			Key:        "original-secret-key",
			UsageCount: 100,
			Status:     "active",
		}

		// When GetAPIKey is called with ID 1, return the original key
		mockDB.On("GetAPIKey", uint(1)).Return(originalKey, nil).Once()

		// We expect UpdateAPIKey to be called. We capture the argument to check it.
		var updatedKey model.APIKey
		mockDB.On("UpdateAPIKey", mock.AnythingOfType("*model.APIKey")).Run(func(args mock.Arguments) {
			keyArg := args.Get(0).(*model.APIKey)
			updatedKey = *keyArg // Dereference and copy
		}).Return(nil).Once()

		req, _ := http.NewRequest(http.MethodPost, "/admin/client-keys/1/reset", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		// Assertions
		assert.Equal(t, http.StatusOK, resp.Code)
		mockDB.AssertExpectations(t)

		// Verify the captured key
		assert.Equal(t, uint(1), updatedKey.ID)
		assert.Equal(t, 0, updatedKey.UsageCount, "UsageCount should be reset to 0")
		assert.Equal(t, "original-secret-key", updatedKey.Key, "Key string should not change")
	})

	t.Run("ResetClientKeyHandler not found", func(t *testing.T) {
		mockDB.On("GetAPIKey", uint(2)).Return(nil, db.ErrAPIKeyNotFound).Once()

		req, _ := http.NewRequest(http.MethodPost, "/admin/client-keys/2/reset", nil)
		req.SetBasicAuth("admin", "test-password")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNotFound, resp.Code)
		mockDB.AssertExpectations(t)
	})
}
