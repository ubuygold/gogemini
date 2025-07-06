package balancer

import (
	"gogemini/internal/config"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestNewBalancer_NoKeys(t *testing.T) {
	cfg := &config.Config{
		GeminiKeys: []string{},
	}
	_, err := NewBalancer(cfg)
	if err == nil {
		t.Error("Expected an error when no Gemini keys are provided, but got nil")
	}
}

func TestBalancer_Proxy(t *testing.T) {
	// 1. Create a mock upstream server
	var receivedKey string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("x-goog-api-key")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "OK from upstream")
	}))
	defer upstreamServer.Close()

	// 2. Create a config with test keys
	cfg := &config.Config{
		GeminiKeys: []string{"key1", "key2"},
	}

	// 3. Create the balancer
	balancer, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("Failed to create balancer: %v", err)
	}
	// No need to defer balancer.Close() as it does nothing now

	// 4. Point the balancer to the mock upstream server
	targetURL, err := url.Parse(upstreamServer.URL)
	if err != nil {
		t.Fatalf("Failed to parse upstream server URL: %v", err)
	}
	balancer.proxy.Director = func(req *http.Request) {
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host
		req.Header.Set("x-goog-api-key", balancer.getNextKey())
	}

	// 5. Create a request to the balancer
	req := httptest.NewRequest(http.MethodPost, "/gemini-pro:generateContent", nil)
	rr := httptest.NewRecorder()

	// 6. Serve the request
	balancer.ServeHTTP(rr, req)

	// 7. Check the response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Body.String() != "OK from upstream" {
		t.Errorf("Expected body 'OK from upstream', got '%s'", rr.Body.String())
	}

	// 8. Check if the key was received by the upstream server
	if receivedKey != "key1" {
		t.Errorf("Expected upstream to receive key 'key1', got '%s'", receivedKey)
	}

	// 9. Test round-robin key selection
	balancer.ServeHTTP(rr, req)
	if receivedKey != "key2" {
		t.Errorf("Expected upstream to receive key 'key2' on second request, got '%s'", receivedKey)
	}
}
func TestBalancer_Close(t *testing.T) {
	cfg := &config.Config{
		GeminiKeys: []string{"key1"},
	}
	balancer, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("Failed to create balancer: %v", err)
	}
	// Simply call Close to ensure it doesn't panic.
	balancer.Close()
}

func TestBalancer_ProxyError(t *testing.T) {
	// 1. Create a mock upstream server and close it immediately to trigger an error
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This should not be called
		w.WriteHeader(http.StatusOK)
	}))
	upstreamServer.Close() // Close the server to cause a connection error

	// 2. Create a config with test keys
	cfg := &config.Config{
		GeminiKeys: []string{"key1"},
	}

	// 3. Create the balancer
	balancer, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("Failed to create balancer: %v", err)
	}

	// 4. Point the balancer to the now-closed mock upstream server
	targetURL, err := url.Parse(upstreamServer.URL)
	if err != nil {
		t.Fatalf("Failed to parse upstream server URL: %v", err)
	}
	balancer.proxy.Transport = &http.Transport{
		Proxy: http.ProxyURL(targetURL),
	}
	// We need to manually set the director because NewSingleHostReverseProxy's default director
	// would be overwritten if we just re-assign it. Instead, we'll just point to the closed server.
	balancer.proxy.Director = func(req *http.Request) {
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host
		req.Header.Set("x-goog-api-key", balancer.getNextKey())
	}

	// 5. Create a request to the balancer
	req := httptest.NewRequest(http.MethodPost, "/v1/models/gemini-pro:generateContent", nil)
	rr := httptest.NewRecorder()

	// 6. Serve the request, which should trigger the ErrorHandler
	balancer.ServeHTTP(rr, req)

	// 7. Check the response
	if rr.Code != http.StatusBadGateway {
		t.Errorf("Expected status code %d for proxy error, got %d", http.StatusBadGateway, rr.Code)
	}
	expectedBody := "Proxy Error\n"
	if rr.Body.String() != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, rr.Body.String())
	}
}
