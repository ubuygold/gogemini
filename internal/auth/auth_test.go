package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	authorizedKeys := map[string]bool{
		"valid-client-key": true,
	}

	router := gin.New()
	router.Use(AuthMiddleware(authorizedKeys))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Test with no key
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	// Test with invalid Bearer token
	req.Header.Set("Authorization", "Bearer invalid-key")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	// Test with valid Bearer token
	req.Header.Set("Authorization", "Bearer valid-client-key")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Test with invalid Gemini key
	req.Header.Del("Authorization")
	req.Header.Set("x-goog-api-key", "invalid-key")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	// Test with valid Gemini key
	req.Header.Set("x-goog-api-key", "valid-client-key")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
}
