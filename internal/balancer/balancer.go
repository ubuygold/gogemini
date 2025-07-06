package balancer

import (
	"context"
	"errors"
	"fmt"
	"gogemini/internal/config"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

// Balancer holds the state of our load balancer.
type Balancer struct {
	mutex   sync.Mutex
	nextKey int
	keys    []string
	proxy   *httputil.ReverseProxy
	logger  *slog.Logger
}

// NewBalancer creates a new Balancer that acts as a reverse proxy.
func NewBalancer(cfg *config.Config, logger *slog.Logger) (*Balancer, error) {
	if len(cfg.GeminiKeys) == 0 {
		return nil, fmt.Errorf("no Gemini API keys provided in configuration")
	}

	targetURL, err := url.Parse("https://generativelanguage.googleapis.com")
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	balancer := &Balancer{
		keys:   cfg.GeminiKeys,
		proxy:  proxy,
		logger: logger.With("component", "balancer"),
	}

	proxy.Director = func(req *http.Request) {
		// This is the key part: we are REPLACING the client's key with one from our pool.
		req.Header.Set("x-goog-api-key", balancer.getNextKey())
		req.Header.Del("Authorization") // Not needed by Gemini

		// Set the host and scheme to the target's
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host

		// The original path from the client request is already in req.URL.Path
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

// getNextKey gets the next key in a round-robin fashion.
func (b *Balancer) getNextKey() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	key := b.keys[b.nextKey]
	b.nextKey = (b.nextKey + 1) % len(b.keys)
	return key
}

// ServeHTTP is the handler for all incoming requests.
func (b *Balancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The ReverseProxy handles everything, including streaming.
	b.proxy.ServeHTTP(w, r)
}

// Close is a no-op, but included for potential future use
// and to satisfy any interfaces that might expect it.
func (b *Balancer) Close() {}
