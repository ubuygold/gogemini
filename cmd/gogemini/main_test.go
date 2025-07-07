package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"os"

	"github.com/ubuygold/gogemini/internal/admin"
	"github.com/ubuygold/gogemini/internal/config"
	"github.com/ubuygold/gogemini/internal/db"
	"github.com/ubuygold/gogemini/internal/keymanager"

	"github.com/gin-gonic/gin"

	"encoding/json"

	"github.com/ubuygold/gogemini/internal/auth"
	"github.com/ubuygold/gogemini/internal/balancer"
	"github.com/ubuygold/gogemini/internal/logger"
	"github.com/ubuygold/gogemini/internal/model"
	"github.com/ubuygold/gogemini/internal/proxy"
	"github.com/ubuygold/gogemini/internal/scheduler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDBService is a mock implementation of the db.Service interface.
type MockDBService struct {
	mock.Mock
}

func (m *MockDBService) LoadActiveGeminiKeys() ([]model.GeminiKey, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.GeminiKey), args.Error(1)
}
func (m *MockDBService) CreateGeminiKey(key *model.GeminiKey) error {
	args := m.Called(key)
	return args.Error(0)
}
func (m *MockDBService) BatchAddGeminiKeys(keys []string) error {
	args := m.Called(keys)
	return args.Error(0)
}
func (m *MockDBService) BatchDeleteGeminiKeys(ids []uint) error {
	args := m.Called(ids)
	return args.Error(0)
}
func (m *MockDBService) ListGeminiKeys(page, limit int, statusFilter string, minFailureCount int) ([]model.GeminiKey, int64, error) {
	args := m.Called(page, limit, statusFilter, minFailureCount)
	if args.Get(0) == nil {
		return nil, int64(args.Int(1)), args.Error(2)
	}
	return args.Get(0).([]model.GeminiKey), int64(args.Int(1)), args.Error(2)
}
func (m *MockDBService) GetGeminiKey(id uint) (*model.GeminiKey, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.GeminiKey), args.Error(1)
}
func (m *MockDBService) UpdateGeminiKey(key *model.GeminiKey) error {
	args := m.Called(key)
	return args.Error(0)
}
func (m *MockDBService) DeleteGeminiKey(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}
func (m *MockDBService) IncrementGeminiKeyUsageCount(key string) error {
	args := m.Called(key)
	return args.Error(0)
}
func (m *MockDBService) HandleGeminiKeyFailure(key string, threshold int) (bool, error) {
	args := m.Called(key, threshold)
	return args.Bool(0), args.Error(1)
}
func (m *MockDBService) ResetGeminiKeyFailureCount(key string) error {
	args := m.Called(key)
	return args.Error(0)
}
func (m *MockDBService) UpdateGeminiKeyStatus(key, status string) error {
	args := m.Called(key, status)
	return args.Error(0)
}
func (m *MockDBService) CreateAPIKey(key *model.APIKey) error {
	args := m.Called(key)
	return args.Error(0)
}
func (m *MockDBService) ListAPIKeys() ([]model.APIKey, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.APIKey), args.Error(1)
}
func (m *MockDBService) GetAPIKey(id uint) (*model.APIKey, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.APIKey), args.Error(1)
}
func (m *MockDBService) UpdateAPIKey(key *model.APIKey) error {
	args := m.Called(key)
	return args.Error(0)
}
func (m *MockDBService) DeleteAPIKey(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}
func (m *MockDBService) IncrementAPIKeyUsageCount(key string) error {
	args := m.Called(key)
	return args.Error(0)
}
func (m *MockDBService) ResetAllAPIKeyUsage() error {
	args := m.Called()
	return args.Error(0)
}
func (m *MockDBService) FindAPIKeyByKey(key string) (*model.APIKey, error) {
	args := m.Called(key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.APIKey), args.Error(1)
}

func TestCustomRecovery_Panic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logBuf bytes.Buffer
	testLogger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	router := gin.New()
	router.Use(customRecovery(testLogger))
	router.GET("/", func(c *gin.Context) {
		panic("test panic")
	})

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, logBuf.String(), "Panic recovered")
	assert.Contains(t, logBuf.String(), "test panic")
}

func TestCustomRecovery_AbortHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logBuf bytes.Buffer
	testLogger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	router := gin.New()
	router.Use(customRecovery(testLogger))
	router.GET("/", func(c *gin.Context) {
		panic(http.ErrAbortHandler)
	})

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	// Use a custom response writer that can be closed to simulate the client disconnecting
	rr := httptest.NewRecorder()
	closedChan := make(chan bool, 1)
	closeNotifier := &closeNotifier{rr, closedChan}

	// Simulate client disconnecting
	go func() {
		// This is a bit of a hack to make the test work.
		// In a real scenario, the http server would handle this.
		// We can't easily simulate the client closing the connection here,
		// so we just panic with ErrAbortHandler.
	}()

	router.ServeHTTP(closeNotifier, req)

	// The status code is not set when aborting, so we check the log
	assert.Contains(t, logBuf.String(), "Client connection aborted")
}

func TestAdminRoutesE2E(t *testing.T) {
	// Create a temporary config file for the test
	const tempConfig = `
port: 8081
debug: false
database:
  type: "sqlite"
  dsn: "file:admin_e2e?mode=memory&cache=shared"
admin:
  password: "e2e-test-password"
`
	configPath := "config_test.yaml"
	err := os.WriteFile(configPath, []byte(tempConfig), 0644)
	assert.NoError(t, err)
	defer os.Remove(configPath)

	// Load config
	cfg, _, err := config.LoadConfig(configPath)
	assert.NoError(t, err)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	dbService, err := db.NewService(cfg.Database)
	assert.NoError(t, err)
	// For this test, we don't need a real key manager, so we can use a mock.
	mockKM := &mockKeyManager{}
	admin.SetupRoutes(router, dbService, mockKM, cfg)

	// --- Test Cases ---

	// 1. No Auth
	req, _ := http.NewRequest(http.MethodGet, "/admin/gemini-keys", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	// 2. Wrong Password
	req, _ = http.NewRequest(http.MethodGet, "/admin/gemini-keys", nil)
	req.SetBasicAuth("admin", "wrong-password")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	// 3. Correct Password
	req, _ = http.NewRequest(http.MethodGet, "/admin/gemini-keys", nil)
	req.SetBasicAuth("admin", "e2e-test-password")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
}

// closeNotifier is a custom ResponseWriter that implements http.CloseNotifier
type closeNotifier struct {
	*httptest.ResponseRecorder
	closed chan bool
}

func (cn *closeNotifier) CloseNotify() <-chan bool {
	return cn.closed
}

func (cn *closeNotifier) Write(b []byte) (int, error) {
	// Simulate the connection being closed by the client
	if cn.closed != nil {
		cn.closed <- true
		close(cn.closed)
		cn.closed = nil
	}
	return cn.ResponseRecorder.Write(b)
}

func (cn *closeNotifier) WriteHeader(statusCode int) {
	cn.ResponseRecorder.WriteHeader(statusCode)
}

func (cn *closeNotifier) Header() http.Header {
	return cn.ResponseRecorder.Header()
}

func (cn *closeNotifier) Body() *bytes.Buffer {
	return cn.ResponseRecorder.Body
}

func (cn *closeNotifier) Flush() {
	// no-op
}

func TestProxyRoutesE2E(t *testing.T) {
	// 1. Setup: Create config, start services, and setup router
	const tempConfig = `
port: 8082
debug: false
database:
  type: "sqlite"
  dsn: "file:proxy_e2e?mode=memory&cache=shared&_pragma=busy_timeout(5000)"
admin:
  password: "proxy-e2e-password"
proxy:
  disable_key_threshold: 3
`
	configPath := "config_proxy_test.yaml"
	err := os.WriteFile(configPath, []byte(tempConfig), 0644)
	assert.NoError(t, err)
	defer os.Remove(configPath)

	cfg, _, err := config.LoadConfig(configPath)
	assert.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	log := logger.New(cfg.Debug)
	dbService, err := db.NewService(cfg.Database)
	assert.NoError(t, err)

	// Manually add a gemini key for the balancer to start
	err = dbService.CreateGeminiKey(&model.GeminiKey{Key: "fake-gemini-key-for-testing", Status: "active"})
	assert.NoError(t, err)

	// Setup all routes
	// We use the real keyManager for the proxy/balancer tests, but the admin routes can use a mock.
	keyManager, err := keymanager.NewKeyManager(dbService, cfg, log)
	assert.NoError(t, err)
	admin.SetupRoutes(router, dbService, keyManager, cfg)
	assert.NoError(t, err)
	defer keyManager.Close()

	geminiHandler, err := balancer.NewBalancer(keyManager, log)
	assert.NoError(t, err)
	// No need to close geminiHandler, as its lifecycle is tied to the keyManager
	openaiProxy, err := proxy.NewOpenAIProxy(keyManager, cfg, log)
	assert.NoError(t, err)
	// No need to close openaiProxy
	s := scheduler.NewScheduler(dbService, cfg, keyManager)
	s.Start() // Start scheduler for key checks
	defer s.Stop()

	geminiHandlerFunc := func(c *gin.Context) {
		http.StripPrefix("/gemini", geminiHandler).ServeHTTP(c.Writer, c.Request)
	}
	geminiGroup := router.Group("/gemini")
	geminiGroup.Use(auth.AuthMiddleware(dbService))
	geminiGroup.Any("/*path", geminiHandlerFunc)

	openaiHandlerFunc := func(c *gin.Context) {
		http.StripPrefix("/openai", openaiProxy).ServeHTTP(c.Writer, c.Request)
	}
	openaiGroup := router.Group("/openai")
	openaiGroup.Use(auth.AuthMiddleware(dbService))
	openaiGroup.Any("/*path", openaiHandlerFunc)

	// 2. Create a client API key via the admin endpoint
	createKeyBody := `{"key": "test-client-key-e2e"}`
	req, _ := http.NewRequest(http.MethodPost, "/admin/client-keys", bytes.NewBufferString(createKeyBody))
	req.SetBasicAuth("admin", "proxy-e2e-password")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusCreated, resp.Code)

	var createdAPIKey model.APIKey
	err = json.Unmarshal(resp.Body.Bytes(), &createdAPIKey)
	assert.NoError(t, err)
	assert.Equal(t, "test-client-key-e2e", createdAPIKey.Key)

	// 3. Test proxy endpoints with the new key
	testCases := []struct {
		name string
		path string
	}{
		{"Gemini Proxy", "/gemini/v1/models"},
		{"OpenAI Proxy", "/openai/v1/chat/completions"},
	}

	for _, tc := range testCases {
		t.Run(tc.name+" No Auth", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, tc.path, nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			assert.Equal(t, http.StatusUnauthorized, resp.Code)
		})

		t.Run(tc.name+" Wrong Auth", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, tc.path, nil)
			req.Header.Set("Authorization", "Bearer wrong-key")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			assert.Equal(t, http.StatusUnauthorized, resp.Code)
		})

		t.Run(tc.name+" Correct Auth", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+createdAPIKey.Key)
			// Use the closeNotifier to prevent panic in ReverseProxy
			rr := httptest.NewRecorder()
			closeNotifier := &closeNotifier{rr, make(chan bool, 1)}
			router.ServeHTTP(closeNotifier, req)

			// The request is expected to be proxied to the upstream API.
			// Since we are not providing a valid request body or using a real API key,
			// the upstream service will reject the request. We check for the specific
			// error codes returned by the respective services for our malformed requests.
			// This confirms that authentication passed and the request was proxied.
			expectedStatus := 0
			if tc.name == "Gemini Proxy" {
				// Google's API returns 404 for POST to /v1/models
				expectedStatus = http.StatusNotFound
			} else {
				// OpenAI's API returns 400 for an empty body on chat completions
				expectedStatus = http.StatusBadRequest
			}
			assert.Equal(t, expectedStatus, rr.Code)
		})
	}
}

func TestSetupAndRunServer_Failure(t *testing.T) {
	// We can't easily test the full setupAndRunServer because it blocks on signal.
	// But we can test the initial setup steps by manipulating the conditions.

	t.Run("keymanager initialization failure", func(t *testing.T) {
		cfg := &config.Config{} // A minimal config is fine
		log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

		// Use a mock DB that returns an error on LoadActiveGeminiKeys
		mockDB := new(MockDBService)
		mockDB.On("LoadActiveGeminiKeys").Return(nil, assert.AnError).Once()

		err := setupAndRunServer(cfg, log, mockDB)
		assert.Error(t, err)
		// The error from keymanager.New should be wrapped, so we check for the underlying error.
		assert.ErrorIs(t, err, assert.AnError)

		mockDB.AssertExpectations(t)
	})

	t.Run("gin logger is used in debug mode", func(t *testing.T) {
		// This is tricky to assert directly without inspecting the router's middleware stack,
		// which Gin doesn't expose publicly. We'll test it by checking the log output.
		cfg := &config.Config{Debug: true, Port: 9999} // Use a different port
		log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		mockDB := new(MockDBService)
		mockDB.On("LoadActiveGeminiKeys").Return([]model.GeminiKey{}, nil)

		// We need to run the server briefly and capture its output
		var logBuf bytes.Buffer
		gin.DefaultWriter = &logBuf

		serverErrChan := make(chan error, 1)
		go func() {
			serverErrChan <- setupAndRunServer(cfg, log, mockDB)
		}()

		// Give the server a moment to start up
		time.Sleep(100 * time.Millisecond)

		// Send a request to trigger the logger
		http.Get("http://localhost:9999/")

		// Give it a moment to log
		time.Sleep(100 * time.Millisecond)

		// Send a shutdown signal
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)

		// Wait for the server to shut down
		err := <-serverErrChan
		assert.NoError(t, err)

		// Check if the Gin logger output is present
		assert.Contains(t, logBuf.String(), "[GIN]")
		assert.Contains(t, logBuf.String(), "GET")
		assert.Contains(t, logBuf.String(), "/")

		gin.DefaultWriter = os.Stdout // Reset to default
	})
}

func TestFrontendServing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock reading index.html
	indexHTML = []byte("<html><body>Mock Index</body></html>")

	// This setup is simplified from setupAndRunServer, focusing only on frontend routes.
	handler := func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	}
	router.GET("/", handler)
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/api") &&
			!strings.HasPrefix(path, "/gemini") &&
			!strings.HasPrefix(path, "/openai") {
			handler(c)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"code": "PAGE_NOT_FOUND", "message": "Page not found"})
	})

	t.Run("serves index.html for root", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "<html><body>Mock Index</body></html>", resp.Body.String())
	})

	t.Run("serves index.html for unknown path", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/some/unknown/path", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "<html><body>Mock Index</body></html>", resp.Body.String())
	})

	t.Run("returns 404 for API-like unknown path", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNotFound, resp.Code)
		assert.JSONEq(t, `{"code": "PAGE_NOT_FOUND", "message": "Page not found"}`, resp.Body.String())
	})
}

func TestGracefulShutdown(t *testing.T) {
	// This test is a bit complex as it involves signals and goroutines.
	// It's a good example of how to test main application lifecycle.

	// Create a valid config file
	const tempConfig = `
port: 8088
debug: false
database:
  type: "sqlite"
  dsn: "file:graceful_shutdown?mode=memory&cache=shared&_pragma=busy_timeout(5000)"
admin:
  password: "shutdown-test"
`
	configPath := "config_shutdown_test.yaml"
	err := os.WriteFile(configPath, []byte(tempConfig), 0644)
	assert.NoError(t, err)
	defer os.Remove(configPath)

	cfg, _, err := config.LoadConfig(configPath)
	assert.NoError(t, err)

	log := logger.New(cfg.Debug)
	dbService, err := db.NewService(cfg.Database)
	assert.NoError(t, err)

	// Run the server in a goroutine
	serverExited := make(chan struct{})
	go func() {
		err := setupAndRunServer(cfg, log, dbService)
		// We expect a "server closed" error on graceful shutdown, which is not a failure.
		if err != nil && err != http.ErrServerClosed {
			assert.NoError(t, err)
		}
		close(serverExited)
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Send the interrupt signal
	p, err := os.FindProcess(os.Getpid())
	assert.NoError(t, err)
	err = p.Signal(syscall.SIGINT)
	assert.NoError(t, err)

	// Wait for the server to exit gracefully
	select {
	case <-serverExited:
		// Success
	case <-time.After(6 * time.Second): // 5s timeout in main + 1s buffer
		t.Fatal("server did not shut down gracefully within the timeout")
	}
}

// mockKeyManager is a simple mock for tests that don't need key manager functionality.
type mockKeyManager struct{}

func (m *mockKeyManager) GetNextKey() (string, error) { return "", nil }
func (m *mockKeyManager) HandleKeyFailure(key string) {}
func (m *mockKeyManager) HandleKeySuccess(key string) {}
func (m *mockKeyManager) ReviveDisabledKeys()         {}
func (m *mockKeyManager) CheckAllKeysHealth()         {}
func (m *mockKeyManager) GetAvailableKeyCount() int   { return 0 }
func (m *mockKeyManager) TestKeyByID(id uint) error   { return nil }
func (m *mockKeyManager) TestAllKeysAsync()           {}
func (m *mockKeyManager) Close()                      {}
