package proxy

import (
	"bytes"
	"fmt"
	"gogemini/internal/config"
	"gogemini/internal/db"
	"gogemini/internal/model"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupTestDB creates a new in-memory SQLite database for testing.
func setupTestDB(t *testing.T) (db.Service, *gorm.DB) {
	t.Helper()
	// Use a temporary file-based database for each test to ensure isolation.
	dbPath := fmt.Sprintf("%s/gogemini_test.db", t.TempDir())
	dsn := dbPath
	service, err := db.NewService(config.DatabaseConfig{
		Type: "sqlite",
		DSN:  dsn,
	})
	if err != nil {
		t.Fatalf("Failed to create test db service: %v", err)
	}
	// The NewService function already handles AutoMigrate for all necessary models.
	return service, service.GetDB()
}

// closeNotifierRecorder is a custom ResponseRecorder that implements http.CloseNotifier
type closeNotifierRecorder struct {
	*httptest.ResponseRecorder
	closed chan bool
}

func newCloseNotifierRecorder() *closeNotifierRecorder {
	return &closeNotifierRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closed:           make(chan bool, 1),
	}
}

func (r *closeNotifierRecorder) CloseNotify() <-chan bool {
	return r.closed
}

func TestOpenAIProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbService, gormDB := setupTestDB(t)

	gormDB.Create(&model.GeminiKey{Key: "test-key", Status: "active"})

	// Create a mock upstream server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedAuth := "Bearer test-key"
		if r.Header.Get("Authorization") != expectedAuth {
			t.Errorf("Expected Authorization header to be '%s', got '%s'", expectedAuth, r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "OK")
	}))
	defer upstreamServer.Close()

	// Create the proxy and point it to the mock upstream server
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	testConfig := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}
	proxy, err := newOpenAIProxyWithURL(dbService, testConfig, upstreamServer.URL, testLogger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}
	defer proxy.Close()

	// Create a Gin router and route to the proxy
	router := gin.New()
	router.Any("/*path", gin.WrapH(proxy))

	// Create a request to the proxy
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer client-key")
	rr := newCloseNotifierRecorder()

	// Serve the request
	router.ServeHTTP(rr, req)

	// Check the response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Body.String() != "OK" {
		t.Errorf("Expected body 'OK', got '%s'", rr.Body.String())
	}
}

func TestNewOpenAIProxy_NoKeysInDB(t *testing.T) {
	dbService, _ := setupTestDB(t)

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	testConfig := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}
	proxy, err := newOpenAIProxyWithURL(dbService, testConfig, "http://localhost", testLogger)
	if err != nil {
		t.Fatalf("newOpenAIProxyWithURL should not return an error when no keys are in the database, but got: %v", err)
	}
	if proxy == nil {
		t.Fatal("newOpenAIProxyWithURL should return a non-nil proxy instance")
	}
	if len(proxy.geminiKeys) != 0 {
		t.Errorf("Proxy should have no keys loaded initially, but has %d", len(proxy.geminiKeys))
	}
}

func TestOpenAIProxy_ServeHTTP_NoKeys(t *testing.T) {
	dbService, _ := setupTestDB(t)

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	testConfig := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}
	proxy, err := newOpenAIProxyWithURL(dbService, testConfig, "http://localhost", testLogger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Service Unavailable: No active API keys for proxy") {
		t.Errorf("Expected error message in response body, but got: %s", rr.Body.String())
	}
}

func TestOpenAIProxy_ServeHTTP_KeysRemoved(t *testing.T) {
	dbService, gormDB := setupTestDB(t)

	// 1. Start with a key
	key := model.GeminiKey{Key: "key1", Status: "active"}
	gormDB.Create(&key)

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	testConfig := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}
	proxy, err := newOpenAIProxyWithURL(dbService, testConfig, "http://localhost", testLogger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}
	if len(proxy.geminiKeys) != 1 {
		t.Fatalf("Expected 1 key initially, got %d", len(proxy.geminiKeys))
	}

	// 2. Remove the key from the database
	gormDB.Delete(&key)

	// 3. Manually trigger the key updater
	proxy.updateKeys()

	// 4. Assert that the proxy has no more keys
	proxy.mutex.Lock()
	if len(proxy.geminiKeys) != 0 {
		t.Errorf("Keys should be empty after update, but got %d", len(proxy.geminiKeys))
	}
	proxy.mutex.Unlock()

	// 5. Make a request and expect a 503 error
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rr := httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d after keys are removed, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestNewOpenAIProxy_UrlParseError(t *testing.T) {
	dbService, gormDB := setupTestDB(t)
	gormDB.Create(&model.GeminiKey{Key: "test-key", Status: "active"})

	// Pass an invalid URL with a control character to force a parse error
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	testConfig := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}
	_, err := newOpenAIProxyWithURL(dbService, testConfig, "http://\x7f.com", testLogger)
	if err == nil {
		t.Error("Expected an error from newOpenAIProxyWithURL when URL parsing fails, but got nil")
	}
}

func TestOpenAIProxy_DebugLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbService, gormDB := setupTestDB(t)
	gormDB.Create(&model.GeminiKey{Key: "test-key-1234", Status: "active"})

	// Create a mock upstream server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	// Capture log output
	var logBuf bytes.Buffer
	testLogger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create the proxy with debug enabled
	testConfig := &config.Config{Debug: true, Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}
	proxy, err := newOpenAIProxyWithURL(dbService, testConfig, upstreamServer.URL, testLogger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}
	defer proxy.Close()

	router := gin.New()
	router.Any("/*path", gin.WrapH(proxy))

	req, _ := http.NewRequest(http.MethodPost, "/v1/some/path", nil)
	rr := newCloseNotifierRecorder()

	router.ServeHTTP(rr, req)

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, `"path":"/v1beta/openai/some/path"`) {
		t.Errorf("Expected log to contain proxying path, but it didn't. Log: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"key_suffix":"1234"`) {
		t.Errorf("Expected log to contain key suffix, but it didn't. Log: %s", logOutput)
	}
}

func TestOpenAIProxy_KeyDisablingAndReactivation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbService, gormDB := setupTestDB(t)

	// Setup initial keys
	gormDB.Create(&model.GeminiKey{Key: "key-good", Status: "active"})
	gormDB.Create(&model.GeminiKey{Key: "key-bad", Status: "active"})

	// --- Mock Upstream Server ---
	var lastUsedKey string
	var mu sync.Mutex
	requestCounts := make(map[string]int)

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		key := strings.TrimPrefix(authHeader, "Bearer ")

		mu.Lock()
		lastUsedKey = key
		requestCounts[key]++
		count := requestCounts[key]
		mu.Unlock()

		if key == "key-bad" && count <= 3 {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, "Forbidden")
			return
		}

		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "OK from "+key)
	}))
	defer upstreamServer.Close()

	// --- Proxy Setup ---
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	testConfig := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}
	proxy, err := newOpenAIProxyWithURL(dbService, testConfig, upstreamServer.URL, testLogger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}
	defer proxy.Close()

	router := gin.New()
	router.Any("/*path", gin.WrapH(proxy))

	// --- Test Requests ---
	makeRequest := func() *closeNotifierRecorder {
		req, _ := http.NewRequest(http.MethodPost, "/", nil)
		rr := newCloseNotifierRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}

	// Make requests until the 'key-bad' is disabled.
	// We'll set a limit to avoid infinite loops in case of a bug.
	var badKeyInDB model.GeminiKey
	var disabledInDB bool
	for i := 0; i < 10; i++ {
		makeRequest()
		gormDB.Where("key = ?", "key-bad").First(&badKeyInDB)
		if badKeyInDB.Status == "disabled" {
			disabledInDB = true
			break
		}
		// Short sleep to allow the proxy to process the response
		time.Sleep(20 * time.Millisecond)
	}

	if !disabledInDB {
		t.Fatalf("'key-bad' was not disabled in the database after multiple failures.")
	}

	// Now that key-bad is disabled, the next request MUST use key-good.
	makeRequest()
	mu.Lock()
	if lastUsedKey != "key-good" {
		t.Errorf("Expected request after disabling to use 'key-good', but got '%s'", lastUsedKey)
	}
	mu.Unlock()

	// Verify in DB
	var badKeyDB model.GeminiKey
	gormDB.Where("key = ?", "key-bad").First(&badKeyDB)
	if badKeyDB.Status != "disabled" {
		t.Errorf("Expected 'key-bad' to be 'disabled' in DB, but got '%s'", badKeyDB.Status)
	}
	if badKeyDB.FailureCount < 3 {
		t.Errorf("Expected 'key-bad' failure count to be at least 3, but got %d", badKeyDB.FailureCount)
	}

	// --- Test Re-activation ---
	// The key is already marked as disabled in the DB, so the proxy won't use it
	// until the keyUpdater runs. We simulate this by updating the key in the DB
	// and then calling the updateKeys function manually.
	gormDB.Model(&model.GeminiKey{}).Where("key = ?", "key-bad").Updates(map[string]interface{}{"status": "active", "failure_count": 0})

	// Manually trigger key update
	proxy.updateKeys()
	time.Sleep(100 * time.Millisecond) // allow update to propagate

	// Make a few more requests to see if key-bad is back in rotation
	foundKeyBad := false
	for i := 0; i < 4; i++ {
		makeRequest()
		mu.Lock()
		if lastUsedKey == "key-bad" {
			foundKeyBad = true
		}
		mu.Unlock()
	}

	if !foundKeyBad {
		t.Error("Expected to find 'key-bad' being used again after manual DB update and key refresh, but it was not.")
	}
}

func TestOpenAIProxy_keyUpdater(t *testing.T) {
	dbService, gormDB := setupTestDB(t)

	gormDB.Create(&model.GeminiKey{Key: "key1", Status: "active"})

	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	testConfig := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}
	proxy, err := newOpenAIProxyWithURL(dbService, testConfig, "http://localhost", testLogger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}
	defer proxy.Close()

	// Add a new key to the database
	gormDB.Create(&model.GeminiKey{Key: "key2", Status: "active"})

	// Manually call updateKeys
	proxy.updateKeys()

	// Check if the new key is loaded
	proxy.mutex.Lock()
	defer proxy.mutex.Unlock()
	if len(proxy.geminiKeys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(proxy.geminiKeys))
	}
}
