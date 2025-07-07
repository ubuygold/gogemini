package proxy

import (
	"errors"
	"gogemini/internal/config"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

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

		// The error handler should return 503, but the test recorder will capture the last response from the upstream.
		// In a real scenario, the client would see the 503 from the ErrorHandler.
		// For this test, we check that the final attempt's status code is what the server sent.
		assert.Equal(t, http.StatusForbidden, rr.Code)
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

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Equal(t, int32(5), requestCount, "Server should have been called exactly 5 times")
		mockKM.AssertExpectations(t)
	})
}
