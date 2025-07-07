package auth

import (
	"errors"
	"gogemini/internal/db"
	"gogemini/internal/model"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// mockAuthDBService is a mock implementation of the db.Service for auth tests.
// It embeds a real GORM DB instance for in-memory testing.
type mockAuthDBService struct {
	db *gorm.DB
}

func (m *mockAuthDBService) FindAPIKeyByKey(key string) (*model.APIKey, error) {
	var apiKey model.APIKey
	if err := m.db.Where("key = ?", key).First(&apiKey).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrAPIKeyNotFound
		}
		return nil, err
	}
	return &apiKey, nil
}

// --- Dummy implementations for the rest of the db.Service interface ---
func (m *mockAuthDBService) CreateGeminiKey(key *model.GeminiKey) error { return nil }
func (m *mockAuthDBService) BatchAddGeminiKeys(keys []string) error     { return nil }
func (m *mockAuthDBService) BatchDeleteGeminiKeys(ids []uint) error     { return nil }
func (m *mockAuthDBService) ListGeminiKeys(page, limit int, statusFilter string, minFailureCount int) ([]model.GeminiKey, int64, error) {
	return nil, 0, nil
}
func (m *mockAuthDBService) GetGeminiKey(id uint) (*model.GeminiKey, error)   { return nil, nil }
func (m *mockAuthDBService) UpdateGeminiKey(key *model.GeminiKey) error       { return nil }
func (m *mockAuthDBService) DeleteGeminiKey(id uint) error                    { return nil }
func (m *mockAuthDBService) LoadActiveGeminiKeys() ([]model.GeminiKey, error) { return nil, nil }
func (m *mockAuthDBService) HandleGeminiKeyFailure(key string, disableThreshold int) (bool, error) {
	return false, nil
}
func (m *mockAuthDBService) ResetGeminiKeyFailureCount(key string) error    { return nil }
func (m *mockAuthDBService) IncrementGeminiKeyUsageCount(key string) error  { return nil }
func (m *mockAuthDBService) UpdateGeminiKeyStatus(key, status string) error { return nil }
func (m *mockAuthDBService) CreateAPIKey(key *model.APIKey) error           { return nil }
func (m *mockAuthDBService) ListAPIKeys() ([]model.APIKey, error)           { return nil, nil }
func (m_ *mockAuthDBService) GetAPIKey(id uint) (*model.APIKey, error)      { return nil, nil }
func (m *mockAuthDBService) UpdateAPIKey(key *model.APIKey) error           { return nil }
func (m *mockAuthDBService) DeleteAPIKey(id uint) error                     { return nil }
func (m *mockAuthDBService) IncrementAPIKeyUsageCount(key string) error     { return nil }
func (m *mockAuthDBService) ResetAllAPIKeyUsage() error                     { return nil }

// Ensure mockAuthDBService implements the interface
var _ db.Service = (*mockAuthDBService)(nil)

func setupTestAuthDB(t *testing.T) (db.Service, *gorm.DB) {
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	err = gormDB.AutoMigrate(&model.APIKey{})
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}
	mockService := &mockAuthDBService{db: gormDB}
	return mockService, gormDB
}

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockService, db := setupTestAuthDB(t)

	// Populate test data
	db.Create(&model.APIKey{Key: "valid-key", Status: "active"})
	db.Create(&model.APIKey{Key: "revoked-key", Status: "revoked"})
	db.Create(&model.APIKey{Key: "expired-key", Status: "active", ExpiresAt: time.Now().Add(-time.Hour)})

	router := gin.New()
	router.Use(AuthMiddleware(mockService))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	testCases := []struct {
		name           string
		key            string
		header         string
		expectedStatus int
	}{
		{"no key", "", "", http.StatusUnauthorized},
		{"invalid bearer key", "Bearer invalid-key", "Authorization", http.StatusUnauthorized},
		{"valid bearer key", "Bearer valid-key", "Authorization", http.StatusOK},
		{"revoked bearer key", "Bearer revoked-key", "Authorization", http.StatusForbidden},
		{"expired bearer key", "Bearer expired-key", "Authorization", http.StatusForbidden},
		{"invalid gemini key", "invalid-key", "x-goog-api-key", http.StatusUnauthorized},
		{"valid gemini key", "valid-key", "x-goog-api-key", http.StatusOK},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/", nil)
			if tc.key != "" {
				req.Header.Set(tc.header, tc.key)
			}
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != tc.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tc.expectedStatus, rr.Code)
			}
		})
	}
}

func TestAdminAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const adminPassword = "test-password"

	router := gin.New()
	router.Use(AdminAuthMiddleware(adminPassword))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	testCases := []struct {
		name           string
		username       string
		password       string
		expectedStatus int
	}{
		{"no auth", "", "", http.StatusUnauthorized},
		{"wrong username", "user", adminPassword, http.StatusUnauthorized},
		{"wrong password", "admin", "wrong-password", http.StatusUnauthorized},
		{"correct auth", "admin", adminPassword, http.StatusOK},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/", nil)
			if tc.username != "" || tc.password != "" {
				req.SetBasicAuth(tc.username, tc.password)
			}
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != tc.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tc.expectedStatus, rr.Code)
			}
		})
	}
}
