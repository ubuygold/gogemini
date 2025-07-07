package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ubuygold/gogemini/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockKeyManager is a mock implementation of the keymanager.Manager interface.
type MockKeyManager struct {
	mock.Mock
}

func (m *MockKeyManager) GetNextKey() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *MockKeyManager) HandleKeyFailure(key string) {
	m.Called(key)
}

func (m *MockKeyManager) HandleKeySuccess(key string) {
	m.Called(key)
}

func (m *MockKeyManager) GetAvailableKeyCount() int {
	args := m.Called()
	return args.Int(0)
}

func TestOpenAIProxy_RetryLogic(t *testing.T) {
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	testConfig := &config.Config{Debug: false}

	t.Run("successfully proxies on first attempt", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer key-good", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		}))
		defer server.Close()

		mockKM := new(MockKeyManager)
		mockKM.On("GetAvailableKeyCount").Return(1) // Only one key, so max 1 attempt
		mockKM.On("GetNextKey").Return("key-good", nil).Once()
		mockKM.On("HandleKeySuccess", "key-good").Return().Once()

		proxy, err := newOpenAIProxyWithURL(mockKM, testConfig, server.URL, testLogger)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rr := httptest.NewRecorder()
		proxy.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "OK", rr.Body.String())
		mockKM.AssertExpectations(t)
	})

	t.Run("retries on failure and succeeds on second attempt", func(t *testing.T) {
		var requestCount int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&requestCount, 1)
			if count == 1 {
				assert.Equal(t, "Bearer key-bad-1", r.Header.Get("Authorization"))
				w.WriteHeader(http.StatusTooManyRequests) // Retryable error
			} else {
				assert.Equal(t, "Bearer key-good-2", r.Header.Get("Authorization"))
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}
		}))
		defer server.Close()

		mockKM := new(MockKeyManager)
		mockKM.On("GetAvailableKeyCount").Return(2)
		// First call in ServeHTTP
		mockKM.On("GetNextKey").Return("key-bad-1", nil).Once()
		// Second call for retry
		mockKM.On("GetNextKey").Return("key-good-2", nil).Once()

		mockKM.On("HandleKeyFailure", "key-bad-1").Return().Once()
		mockKM.On("HandleKeySuccess", "key-good-2").Return().Once()

		proxy, err := newOpenAIProxyWithURL(mockKM, testConfig, server.URL, testLogger)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rr := httptest.NewRecorder()
		proxy.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "OK", rr.Body.String())
		assert.Equal(t, int32(2), requestCount, "Server should have been called twice")
		mockKM.AssertExpectations(t)
	})

	t.Run("fails after all keys are exhausted", func(t *testing.T) {
		var requestCount int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			w.WriteHeader(http.StatusForbidden) // Always fail
		}))
		defer server.Close()

		mockKM := new(MockKeyManager)
		mockKM.On("GetAvailableKeyCount").Return(2)
		mockKM.On("GetNextKey").Return("key-bad-1", nil).Once()
		mockKM.On("GetNextKey").Return("key-bad-2", nil).Once()
		mockKM.On("HandleKeyFailure", "key-bad-1").Return().Once()
		mockKM.On("HandleKeyFailure", "key-bad-2").Return().Once()

		proxy, err := newOpenAIProxyWithURL(mockKM, testConfig, server.URL, testLogger)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rr := httptest.NewRecorder()
		proxy.ServeHTTP(rr, req)

		// After all retries fail, the proxy's ErrorHandler should be called, which returns a 503.
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		assert.Contains(t, rr.Body.String(), "Service unavailable after multiple retries")
		assert.Equal(t, int32(2), requestCount, "Server should have been called twice")
		mockKM.AssertExpectations(t)
	})

	t.Run("does not retry on non-retryable error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest) // 400 should not be retried
			w.Write([]byte("Bad Request"))
		}))
		defer server.Close()

		mockKM := new(MockKeyManager)
		mockKM.On("GetAvailableKeyCount").Return(1)
		mockKM.On("GetNextKey").Return("key-good", nil).Once()
		// HandleKeyFailure should NOT be called
		// HandleKeySuccess should NOT be called

		proxy, err := newOpenAIProxyWithURL(mockKM, testConfig, server.URL, testLogger)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rr := httptest.NewRecorder()
		proxy.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "Bad Request", rr.Body.String())
		mockKM.AssertExpectations(t)
	})

	t.Run("handles key manager error on first attempt", func(t *testing.T) {
		mockKM := new(MockKeyManager)
		mockKM.On("GetNextKey").Return("", errors.New("no keys available")).Once()

		proxy, err := newOpenAIProxyWithURL(mockKM, testConfig, "http://dummy.url", testLogger)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rr := httptest.NewRecorder()
		proxy.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		mockKM.AssertExpectations(t)
	})

	t.Run("stops after 5 retries even if more keys are available", func(t *testing.T) {
		var requestCount int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			w.WriteHeader(http.StatusForbidden) // Always fail
		}))
		defer server.Close()

		mockKM := new(MockKeyManager)
		// We have 10 keys, but should only try 5 times.
		mockKM.On("GetAvailableKeyCount").Return(10)
		// Initial key + 4 retries = 5 attempts
		mockKM.On("GetNextKey").Return("key-1", nil).Times(1)
		mockKM.On("GetNextKey").Return("key-2", nil).Times(1)
		mockKM.On("GetNextKey").Return("key-3", nil).Times(1)
		mockKM.On("GetNextKey").Return("key-4", nil).Times(1)
		mockKM.On("GetNextKey").Return("key-5", nil).Times(1)

		mockKM.On("HandleKeyFailure", mock.Anything).Times(5)

		proxy, err := newOpenAIProxyWithURL(mockKM, testConfig, server.URL, testLogger)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rr := httptest.NewRecorder()
		proxy.ServeHTTP(rr, req)

		// After all retries fail, the proxy's ErrorHandler should be called, which returns a 503.
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		assert.Contains(t, rr.Body.String(), "Service unavailable after multiple retries")
		assert.Equal(t, int32(5), requestCount, "Server should have been called exactly 5 times")
		mockKM.AssertExpectations(t)
	})
}

func TestNewOpenAIProxyWithURL_Error(t *testing.T) {
	mockKM := new(MockKeyManager)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	testConfig := &config.Config{Debug: false}

	// Invalid URL with a control character
	_, err := newOpenAIProxyWithURL(mockKM, testConfig, "http://\x7f.invalid", testLogger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid control character in URL")
}

func TestRetryingTransport_RoundTrip_ContextError(t *testing.T) {
	mockKM := new(MockKeyManager)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	transport := &retryingTransport{
		keyManager: mockKM,
		logger:     testLogger,
		transport:  http.DefaultTransport,
	}

	// Create a request without the geminiKey in the context
	req := httptest.NewRequest("GET", "/", nil)
	_, err := transport.RoundTrip(req)

	assert.Error(t, err)
	assert.EqualError(t, err, "gemini key not found in request context for transport")
}

func TestRetryingTransport_GetNextKeyError(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusTooManyRequests) // Always fail to trigger retry
	}))
	defer server.Close()

	mockKM := new(MockKeyManager)
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	testConfig := &config.Config{Debug: false}

	mockKM.On("GetAvailableKeyCount").Return(2)
	mockKM.On("GetNextKey").Return("key-1", nil).Once() // For ServeHTTP
	// This error occurs when trying to get a key for the retry
	mockKM.On("GetNextKey").Return("", errors.New("no more keys")).Once()
	mockKM.On("HandleKeyFailure", "key-1").Once()

	proxy, err := newOpenAIProxyWithURL(mockKM, testConfig, server.URL, testLogger)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	rr := httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)

	// When GetNextKey fails, the loop terminates and the ErrorHandler is triggered.
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), "Service unavailable after multiple retries")
	assert.Equal(t, int32(1), requestCount, "Server should have been called only once")
	mockKM.AssertExpectations(t)
}

func TestErrorHandler_ContextCanceled(t *testing.T) {
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	testConfig := &config.Config{Debug: false}
	mockKM := new(MockKeyManager)

	// We need a fully initialized proxy to test the error handler
	proxy, err := newOpenAIProxyWithURL(mockKM, testConfig, "http://dummy.url", testLogger)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	// The ErrorHandler should not write to the response writer for a canceled context
	proxy.reverseProxy.ErrorHandler(rr, req, context.Canceled)
	assert.Equal(t, http.StatusOK, rr.Code) // Default status, nothing written
	assert.Equal(t, "", rr.Body.String())
}

func TestSafeKeySuffix(t *testing.T) {
	assert.Equal(t, "6789", safeKeySuffix("123456789"))
	assert.Equal(t, "key", safeKeySuffix("key"))
	assert.Equal(t, "", safeKeySuffix(""))
}

// mockTransport is a mock for http.RoundTripper
type mockTransport struct {
	mock.Mock
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	if res := args.Get(0); res != nil {
		return res.(*http.Response), args.Error(1)
	}
	return nil, args.Error(1)
}

func TestModifyRequestBody(t *testing.T) {
	testLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	proxy := &OpenAIProxy{logger: testLogger}

	t.Run("removes unsupported fields", func(t *testing.T) {
		// Original body with unsupported fields
		originalBody := `{
			"model": "gemini-pro",
			"messages": [{"role": "user", "content": "hello"}],
			"temperature": 0.5,
			"frequency_penalty": 0.7,
			"presence_penalty": 0.8,
			"logit_bias": {"123": 100},
			"logprobs": true,
			"top_logprobs": 5
		}`
		// Expected body after modification
		expectedBody := `{
			"model": "gemini-pro",
			"messages": [{"role": "user", "content": "hello"}],
			"temperature": 0.5
		}`

		req := httptest.NewRequest("POST", "/", strings.NewReader(originalBody))
		err := proxy.ModifyRequestBody(req)
		require.NoError(t, err)

		modifiedBodyBytes, err := io.ReadAll(req.Body)
		require.NoError(t, err)

		// Unmarshal both to compare them structurally, ignoring formatting differences
		var originalMap, expectedMap, modifiedMap map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(originalBody), &originalMap))
		require.NoError(t, json.Unmarshal([]byte(expectedBody), &expectedMap))
		require.NoError(t, json.Unmarshal(modifiedBodyBytes, &modifiedMap))

		assert.NotEqual(t, originalMap, modifiedMap, "Original and modified map should not be equal")
		assert.Equal(t, expectedMap, modifiedMap, "Modified body does not match expected body")
		assert.Equal(t, int64(len(modifiedBodyBytes)), req.ContentLength, "ContentLength was not updated correctly")
	})

	t.Run("does not modify clean body", func(t *testing.T) {
		cleanBody := `{"model": "gemini-pro", "messages": [{"role": "user", "content": "hello"}]}`
		req := httptest.NewRequest("POST", "/", strings.NewReader(cleanBody))
		originalContentLength := req.ContentLength

		err := proxy.ModifyRequestBody(req)
		require.NoError(t, err)

		modifiedBodyBytes, err := io.ReadAll(req.Body)
		require.NoError(t, err)

		assert.JSONEq(t, cleanBody, string(modifiedBodyBytes), "Body should not have been modified")
		assert.Equal(t, originalContentLength, req.ContentLength, "ContentLength should not have changed")
	})

	t.Run("handles nil body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", nil)
		req.Body = nil // Explicitly set body to nil
		err := proxy.ModifyRequestBody(req)
		require.NoError(t, err)
		assert.Nil(t, req.Body, "Body should remain nil")
	})

	t.Run("handles empty body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(""))
		err := proxy.ModifyRequestBody(req)
		require.NoError(t, err)
		bodyBytes, _ := io.ReadAll(req.Body)
		assert.Empty(t, bodyBytes, "Body should remain empty")
	})

	t.Run("handles non-json body", func(t *testing.T) {
		nonJsonBody := "this is not json"
		req := httptest.NewRequest("POST", "/", strings.NewReader(nonJsonBody))
		err := proxy.ModifyRequestBody(req)
		require.NoError(t, err)
		bodyBytes, _ := io.ReadAll(req.Body)
		assert.Equal(t, nonJsonBody, string(bodyBytes), "Non-JSON body should not be modified")
	})
}
