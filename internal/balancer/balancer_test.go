package balancer

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockKeyManager is a mock implementation of the Manager interface.
type MockKeyManager struct {
	mock.Mock
}

func (m *MockKeyManager) GetNextKey() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestBalancer_ServeHTTP(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("successfully proxies request", func(t *testing.T) {
		// 1. Setup Mock upstream server
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Assert that the key from the key manager is now in the header
			assert.Equal(t, "test-key-123", r.Header.Get("x-goog-api-key"))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		}))
		defer upstreamServer.Close()

		// 2. Setup Mock KeyManager
		mockKM := new(MockKeyManager)
		mockKM.On("GetNextKey").Return("test-key-123", nil).Once()

		// 3. Create Balancer with Mocks
		balancer, err := NewBalancer(mockKM, testLogger)
		require.NoError(t, err)

		// Manually set the proxy target to our test server
		targetURL, _ := url.Parse(upstreamServer.URL)
		originalDirector := balancer.proxy.Director
		balancer.proxy.Director = func(req *http.Request) {
			originalDirector(req) // This will set the key from the context
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.Host = targetURL.Host
		}

		// 4. Perform Request
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		balancer.ServeHTTP(rr, req)

		// 5. Assertions
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "OK", rr.Body.String())
		mockKM.AssertExpectations(t)
	})

	t.Run("handles error from keymanager", func(t *testing.T) {
		// 1. Setup Mock KeyManager to return an error
		mockKM := new(MockKeyManager)
		mockKM.On("GetNextKey").Return("", assert.AnError).Once()

		// 2. Create Balancer
		balancer, err := NewBalancer(mockKM, testLogger)
		require.NoError(t, err)

		// 3. Perform Request
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		balancer.ServeHTTP(rr, req)

		// 4. Assertions
		// When the key manager fails, we expect a 503 Service Unavailable error.
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		assert.Contains(t, rr.Body.String(), "Service Unavailable: No active API keys")
		mockKM.AssertExpectations(t)
	})

	t.Run("director safeguard", func(t *testing.T) {
		mockKM := new(MockKeyManager)
		balancer, err := NewBalancer(mockKM, testLogger)
		require.NoError(t, err)

		// Create a request without the geminiKey in the context
		req := httptest.NewRequest("GET", "/", nil)
		// We expect an error log, but the function should not panic.
		// In a real scenario, we'd capture the log output to verify.
		// For this test, we're just ensuring it runs without crashing.
		assert.NotPanics(t, func() {
			balancer.proxy.Director(req)
		})
	})
}

func TestNewBalancer(t *testing.T) {
	mockKM := new(MockKeyManager)
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	balancer, err := NewBalancer(mockKM, testLogger)
	require.NoError(t, err)
	assert.NotNil(t, balancer)
	assert.NotNil(t, balancer.proxy)
	assert.Equal(t, mockKM, balancer.keyManager)
}

func TestBalancer_ErrorHandler(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockKM := new(MockKeyManager)
	balancer, err := NewBalancer(mockKM, testLogger)
	require.NoError(t, err)

	t.Run("handles context canceled error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		balancer.proxy.ErrorHandler(rr, req, context.Canceled)
		// We expect no response to be written, just a log message.
		assert.Equal(t, http.StatusOK, rr.Code) // Default recorder code is 200
		assert.Equal(t, "", rr.Body.String())
	})

	t.Run("handles generic proxy error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		balancer.proxy.ErrorHandler(rr, req, assert.AnError)
		assert.Equal(t, http.StatusBadGateway, rr.Code)
		assert.Contains(t, rr.Body.String(), "Proxy Error")
	})
}

func TestDirector_PathModification(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockKM := new(MockKeyManager)
	balancer, err := NewBalancer(mockKM, testLogger)
	require.NoError(t, err)

	testCases := []struct {
		name         string
		inputPath    string
		expectedPath string
	}{
		{
			name:         "with models prefix",
			inputPath:    "/v1beta/models/gemini-pro:generateContent",
			expectedPath: "/v1beta/gemini-pro:generateContent",
		},
		{
			name:         "without models prefix",
			inputPath:    "/v1beta/gemini-pro:generateContent",
			expectedPath: "/v1beta/gemini-pro:generateContent",
		},
		{
			name:         "irrelevant path",
			inputPath:    "/v1beta/some/other/path",
			expectedPath: "/v1beta/some/other/path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.inputPath, nil)
			ctx := context.WithValue(req.Context(), geminiKey, "test-key")
			req = req.WithContext(ctx)

			balancer.proxy.Director(req)

			assert.Equal(t, tc.expectedPath, req.URL.Path)
		})
	}
}
