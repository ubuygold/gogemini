package auth

import (
	"gogemini/internal/model"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	err = db.AutoMigrate(&model.APIKey{})
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}
	return db
}

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)

	// Populate test data
	db.Create(&model.APIKey{Key: "valid-key", Status: "active"})
	db.Create(&model.APIKey{Key: "revoked-key", Status: "revoked"})
	db.Create(&model.APIKey{Key: "expired-key", Status: "active", ExpiresAt: time.Now().Add(-time.Hour)})

	router := gin.New()
	router.Use(AuthMiddleware(db))
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
