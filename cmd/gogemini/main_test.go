package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"gogemini/internal/admin"
	"gogemini/internal/config"
	"gogemini/internal/db"
	"os"

	"github.com/gin-gonic/gin"

	"encoding/json"
	"gogemini/internal/auth"
	"gogemini/internal/balancer"
	"gogemini/internal/logger"
	"gogemini/internal/model"
	"gogemini/internal/proxy"
	"gogemini/internal/scheduler"

	"github.com/stretchr/testify/assert"
)

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
  dsn: "file::memory:"
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
	admin.SetupRoutes(router, dbService, cfg)

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
  dsn: "file::memory:"
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
	admin.SetupRoutes(router, dbService, cfg)

	geminiHandler, err := balancer.NewBalancer(dbService, log)
	assert.NoError(t, err)
	defer geminiHandler.Close()
	openaiProxy, err := proxy.NewOpenAIProxy(dbService, cfg, log)
	assert.NoError(t, err)
	defer openaiProxy.Close()
	s := scheduler.NewScheduler(dbService, cfg)
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
