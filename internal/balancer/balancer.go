package balancer

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// Manager defines the interface for a key manager that the balancer can use.
type Manager interface {
	GetNextKey() (string, error)
}

type contextKey string

const geminiKey contextKey = "geminiKey"

// Balancer holds the state of our load balancer.
type Balancer struct {
	keyManager Manager
	proxy      *httputil.ReverseProxy
	logger     *slog.Logger
}

// NewBalancer creates a new Balancer that acts as a reverse proxy.
func NewBalancer(km Manager, logger *slog.Logger) (*Balancer, error) {
	targetURL, err := url.Parse("https://generativelanguage.googleapis.com")
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	balancer := &Balancer{
		keyManager: km,
		proxy:      proxy,
		logger:     logger.With("component", "balancer"),
	}

	proxy.Director = func(req *http.Request) {
		// Retrieve the key from the context.
		key, ok := req.Context().Value(geminiKey).(string)
		if !ok {
			// This should not happen if ServeHTTP is used, but as a safeguard:
			balancer.logger.Error("Gemini key not found in request context")
			return
		}

		// This is the key part: we are REPLACING the client's key with one from our pool.
		req.Header.Set("x-goog-api-key", key)
		req.Header.Del("Authorization") // Not needed by Gemini

		// Set the host and scheme to the target's
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host

		// The original path from the client request is already in req.URL.Path
		// The original path from the client request is already in req.URL.Path.
		// We need to ensure the "models/" prefix exists for the target API.
		// e.g., /v1beta/gemini-pro:generateContent -> /v1beta/models/gemini-pro:generateContent
		if !strings.Contains(req.URL.Path, "/models/") {
			req.URL.Path = strings.Replace(req.URL.Path, "/v1beta/", "/v1beta/models/", 1)
		}
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		// Check if the error is a context cancellation from the client.
		if errors.Is(err, context.Canceled) || errors.Is(err, http.ErrAbortHandler) {
			// This happens when the client closes the connection, which is normal for streaming.
			balancer.logger.Warn("Client disconnected", "error", err)
			return // Stop further processing.
		}

		// For all other errors, log them and return a bad gateway status.
		balancer.logger.Error("Proxy error", "error", err)
		http.Error(w, "Proxy Error", http.StatusBadGateway)
	}

	return balancer, nil
}

// ServeHTTP is the handler for all incoming requests.
func (b *Balancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key, err := b.keyManager.GetNextKey()
	if err != nil {
		b.logger.Error("Aborting request, no available Gemini key", "error", err)
		http.Error(w, "Service Unavailable: No active API keys", http.StatusServiceUnavailable)
		return
	}

	// Store the key in the request context to pass it to the director.
	ctx := context.WithValue(r.Context(), geminiKey, key)
	reqWithContext := r.WithContext(ctx)

	// The ReverseProxy handles everything else, including streaming.
	b.proxy.ServeHTTP(w, reqWithContext)
}

// Close gracefully shuts down the balancer's background tasks.
func (b *Balancer) Close() {
	// No-op since the keyManager is now responsible for its own lifecycle.
	b.logger.Info("Balancer shutdown.")
}
