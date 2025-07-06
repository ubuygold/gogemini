package proxy

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

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

	// Create a mock upstream server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the Authorization header was correctly set
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
	proxy, err := newOpenAIProxyWithURL([]string{"test-key"}, upstreamServer.URL, false, testLogger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

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

func TestNewOpenAIProxy_UrlParseError(t *testing.T) {
	// Pass an invalid URL with a control character to force a parse error
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	_, err := newOpenAIProxyWithURL([]string{"test-key"}, "http://\x7f.com", false, testLogger)
	if err == nil {
		t.Error("Expected an error from newOpenAIProxyWithURL when URL parsing fails, but got nil")
	}
}

func TestOpenAIProxy_DebugLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a mock upstream server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	// Capture log output
	var logBuf bytes.Buffer
	testLogger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create the proxy with debug enabled
	proxy, err := newOpenAIProxyWithURL([]string{"test-key-1234"}, upstreamServer.URL, true, testLogger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	router := gin.New()
	router.Any("/*path", gin.WrapH(proxy))

	req, _ := http.NewRequest(http.MethodPost, "/v1/some/path", nil)
	// Use the closeNotifierRecorder to prevent panics with the reverse proxy
	rr := newCloseNotifierRecorder()

	router.ServeHTTP(rr, req)

	logOutput := logBuf.String()
	// With structured logging, we check for specific key-value pairs.
	if !strings.Contains(logOutput, `"path":"/v1beta/openai/some/path"`) {
		t.Errorf("Expected log to contain proxying path, but it didn't. Log: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"key_suffix":"1234"`) {
		t.Errorf("Expected log to contain key suffix, but it didn't. Log: %s", logOutput)
	}
}
