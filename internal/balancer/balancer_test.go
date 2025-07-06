package balancer

import (
	"fmt"
	"gogemini/internal/config"
	"gogemini/internal/db"
	"gogemini/internal/model"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// setupTestDB creates a new in-memory SQLite database for testing.
// It returns the db.Service and the raw *gorm.DB for assertions.
func setupTestDB(t *testing.T) (db.Service, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	service, err := db.NewService(config.DatabaseConfig{
		Type: "sqlite",
		DSN:  dsn,
	})
	if err != nil {
		t.Fatalf("Failed to create test db service: %v", err)
	}
	err = service.GetDB().AutoMigrate(&model.GeminiKey{})
	if err != nil {
		t.Fatalf("Failed to auto-migrate schema: %v", err)
	}
	return service, service.GetDB()
}

func TestNewBalancer_NoKeysInDB(t *testing.T) {
	dbService, _ := setupTestDB(t)

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	// We expect the balancer to be created successfully even with no keys.
	balancer, err := NewBalancer(dbService, testLogger)
	assert.NoError(t, err, "NewBalancer should not return an error when no keys are in the database")
	assert.NotNil(t, balancer, "NewBalancer should return a non-nil balancer instance")
	assert.Empty(t, balancer.keys, "Balancer should have no keys loaded initially")
}

func TestBalancer_ServeHTTP_NoKeys(t *testing.T) {
	dbService, _ := setupTestDB(t)

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	balancer, err := NewBalancer(dbService, testLogger)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/gemini/v1/models/gemini-pro:generateContent", nil)
	rr := httptest.NewRecorder()

	balancer.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code, "Expected status code 503 Service Unavailable")
	assert.Contains(t, rr.Body.String(), "Service Unavailable: No active API keys", "Expected error message in response body")
}

func TestBalancer_ServeHTTP_KeysRemoved(t *testing.T) {
	dbService, gormDB := setupTestDB(t)

	// 1. Start with a key
	key := model.GeminiKey{Key: "key1", Status: "active"}
	gormDB.Create(&key)

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	balancer, err := NewBalancer(dbService, testLogger)
	assert.NoError(t, err)
	assert.Len(t, balancer.keys, 1)

	// 2. Remove the key from the database
	gormDB.Delete(&key)

	// 3. Manually trigger the key updater
	balancer.updateKeys()

	// 4. Assert that the balancer has no more keys
	balancer.mutex.Lock()
	assert.Empty(t, balancer.keys, "Keys should be empty after update")
	balancer.mutex.Unlock()

	// 5. Make a request and expect a 503 error
	req := httptest.NewRequest(http.MethodPost, "/gemini/v1/models/gemini-pro:generateContent", nil)
	rr := httptest.NewRecorder()
	balancer.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code, "Expected status code 503 after keys are removed")
}

func TestBalancer_Proxy_LeastUsage(t *testing.T) {
	dbService, gormDB := setupTestDB(t)

	// 1. Add keys to the test database
	keys := []model.GeminiKey{
		{Key: "key1", Status: "active", UsageCount: 100},
		{Key: "key2", Status: "active", UsageCount: 5}, // key2 is the least used
		{Key: "key3", Status: "inactive", UsageCount: 0},
	}
	gormDB.Create(&keys)

	// 2. Create a mock upstream server
	var receivedKey string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("x-goog-api-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	// 3. Create the balancer
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	balancer, err := NewBalancer(dbService, testLogger)
	if err != nil {
		t.Fatalf("Failed to create balancer: %v", err)
	}
	// We will call Close manually at the end to ensure all updates are flushed.
	// defer balancer.Close()

	// 4. Point the balancer to the mock upstream server
	targetURL, err := url.Parse(upstreamServer.URL)
	if err != nil {
		t.Fatalf("Failed to parse upstream server URL: %v", err)
	}
	originalDirector := balancer.proxy.Director
	balancer.proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host
	}

	req := httptest.NewRequest(http.MethodPost, "/gemini-pro:generateContent", nil)

	// --- First Request ---
	balancer.ServeHTTP(httptest.NewRecorder(), req)
	if receivedKey != "key2" {
		t.Fatalf("Expected first key to be 'key2' (lowest usage), but got '%s'", receivedKey)
	}

	// --- Use key2 until its count surpasses key1's count ---
	// Initial count is 5. After first use, it's 6.
	// We need to use it 95 more times for its count to become 101 (5 + 1 + 95).
	for i := 0; i < 95; i++ {
		balancer.ServeHTTP(httptest.NewRecorder(), req)
		if receivedKey != "key2" {
			t.Fatalf("Expected to keep using 'key2' but got '%s' on iteration %d", receivedKey, i+1)
		}
	}

	// --- Next Request should use key1 ---
	balancer.ServeHTTP(httptest.NewRecorder(), req)
	if receivedKey != "key1" {
		t.Fatalf("Expected to switch to 'key1' after exhausting 'key2', but got '%s'", receivedKey)
	}

	// --- Verify DB counts after graceful shutdown ---
	// Close the balancer, which will wait for the usage updater to finish.
	balancer.Close()

	var key1DB, key2DB model.GeminiKey
	if err := gormDB.First(&key1DB, "key = ?", "key1").Error; err != nil {
		t.Fatalf("Failed to find key1 in db: %v", err)
	}
	if err := gormDB.First(&key2DB, "key = ?", "key2").Error; err != nil {
		t.Fatalf("Failed to find key2 in db: %v", err)
	}

	// key2 was used 1 (initial) + 95 (loop) = 96 times. Initial count was 5. Total = 101.
	assert.Equal(t, int64(101), key2DB.UsageCount, "Expected key2 usage count in DB to be 101")
	// key1 was used once. Initial count was 100. Total = 101.
	assert.Equal(t, int64(101), key1DB.UsageCount, "Expected key1 usage count in DB to be 101")
}

func TestBalancer_GracefulShutdown_EnsuresUpdates(t *testing.T) {
	dbService, gormDB := setupTestDB(t)
	gormDB.Create(&model.GeminiKey{Key: "key1", Status: "active", UsageCount: 0})

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	balancer, err := NewBalancer(dbService, testLogger)
	assert.NoError(t, err)

	// Point the balancer to a dummy server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()
	targetURL, _ := url.Parse(upstreamServer.URL)
	originalDirector := balancer.proxy.Director
	balancer.proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
	}

	// Make 5 requests to queue up 5 updates
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	for i := 0; i < 5; i++ {
		balancer.ServeHTTP(httptest.NewRecorder(), req)
	}

	// Immediately close the balancer. This should block until the queue is processed.
	balancer.Close()

	// Verify that the usage count in the database is correct.
	var key1DB model.GeminiKey
	err = gormDB.First(&key1DB, "key = ?", "key1").Error
	assert.NoError(t, err)
	assert.Equal(t, int64(5), key1DB.UsageCount, "Expected usage count to be 5 after graceful shutdown")
}

func TestBalancer_ProxyError(t *testing.T) {
	dbService, gormDB := setupTestDB(t)
	gormDB.Create(&model.GeminiKey{Key: "key1", Status: "active"})

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	upstreamServer.Close()

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	balancer, err := NewBalancer(dbService, testLogger)
	if err != nil {
		t.Fatalf("Failed to create balancer: %v", err)
	}
	defer balancer.Close()

	targetURL, err := url.Parse(upstreamServer.URL)
	if err != nil {
		t.Fatalf("Failed to parse upstream server URL: %v", err)
	}
	originalDirector := balancer.proxy.Director
	balancer.proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/models/gemini-pro:generateContent", nil)
	rr := httptest.NewRecorder()

	balancer.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("Expected status code %d for proxy error, got %d", http.StatusBadGateway, rr.Code)
	}
	expectedBody := "Proxy Error\n"
	if rr.Body.String() != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, rr.Body.String())
	}
}

func TestBalancer_keyReloader(t *testing.T) {
	dbService, gormDB := setupTestDB(t)

	gormDB.Create(&model.GeminiKey{Key: "key1", Status: "active"})

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	balancer, err := NewBalancer(dbService, testLogger)
	if err != nil {
		t.Fatalf("Failed to create balancer: %v", err)
	}
	defer balancer.Close()

	// Add a new key to the database
	gormDB.Create(&model.GeminiKey{Key: "key2", Status: "active"})

	// Manually call updateKeys
	balancer.updateKeys()

	// Check if the new key is loaded
	balancer.mutex.Lock()
	defer balancer.mutex.Unlock()
	assert.Len(t, balancer.keys, 2)
}
