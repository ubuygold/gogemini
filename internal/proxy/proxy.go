package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/ubuygold/gogemini/internal/config"
)

// Manager defines the interface for a key manager that the proxy can use.
type Manager interface {
	GetNextKey() (string, error)
	HandleKeyFailure(key string)
	HandleKeySuccess(key string)
	GetAvailableKeyCount() int
}

// retryingTransport is a custom http.RoundTripper that implements retry logic.
type retryingTransport struct {
	keyManager Manager
	logger     *slog.Logger
	transport  http.RoundTripper
}

const maxRetryAttempts = 5

// RoundTrip executes a single HTTP transaction, but adds retry logic.
func (rt *retryingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The first key is already attached to the request by the Director.
	if _, ok := req.Context().Value(geminiKeyContextKey).(string); !ok {
		return nil, errors.New("gemini key not found in request context for transport")
	}

	numAvailableKeys := rt.keyManager.GetAvailableKeyCount()
	numAttempts := numAvailableKeys
	if numAttempts > maxRetryAttempts {
		numAttempts = maxRetryAttempts
	}
	var lastErr error

	for i := 0; i < numAttempts; i++ {
		currentKey := req.Context().Value(geminiKeyContextKey).(string)
		rt.logger.Debug("Attempting request", "attempt", i+1, "key_suffix", safeKeySuffix(currentKey))

		resp, err := rt.transport.RoundTrip(req)

		// Check if the response is successful or a non-retryable error.
		if err == nil && resp.StatusCode < 400 {
			rt.keyManager.HandleKeySuccess(currentKey)
			return resp, nil // Success
		}
		if err == nil && !isRetryableStatusCode(resp.StatusCode) {
			// Not a key-related failure (e.g., 400 Bad Request), so don't retry.
			rt.logger.Warn("Received non-retryable error status", "status", resp.StatusCode, "key_suffix", safeKeySuffix(currentKey))
			return resp, nil
		}

		// It's a retryable error (either transport error or HTTP status), so handle the failure.
		if err != nil {
			lastErr = err
			rt.logger.Warn("Request failed with transport error, will retry", "key_suffix", safeKeySuffix(currentKey), "error", err)
		} else {
			lastErr = fmt.Errorf("received status code %d", resp.StatusCode)
			rt.logger.Warn("Request failed with retryable status, will retry", "status", resp.StatusCode, "key_suffix", safeKeySuffix(currentKey))
		}
		rt.keyManager.HandleKeyFailure(currentKey)

		// If this was the last retry, return the last known response/error, wrapping the error for context.
		if i == numAttempts-1 {
			return resp, fmt.Errorf("last attempt failed: %w", lastErr)
		}

		// Get the next key for the retry.
		nextKey, keyErr := rt.keyManager.GetNextKey()
		if keyErr != nil {
			rt.logger.Error("Failed to get next key for retry", "error", keyErr)
			return resp, lastErr // Return the last response and error
		}

		// Update the request with the new key for the next iteration.
		req = req.WithContext(context.WithValue(req.Context(), geminiKeyContextKey, nextKey))
		req.Header.Set("Authorization", "Bearer "+nextKey)
	}

	return nil, fmt.Errorf("all retries failed; last error: %w", lastErr)
}

func isRetryableStatusCode(code int) bool {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	// Also retry on server-side errors
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return true
	default:
		return false
	}
}

type OpenAIProxy struct {
	keyManager   Manager
	reverseProxy *httputil.ReverseProxy
	targetURL    *url.URL
	debug        bool
	logger       *slog.Logger
}

type contextKey string

const geminiKeyContextKey = contextKey("geminiKey")

// newOpenAIProxyWithURL is the internal constructor that allows for custom target URLs, making it testable.
func newOpenAIProxyWithURL(km Manager, cfg *config.Config, target string, logger *slog.Logger) (*OpenAIProxy, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	proxy := &OpenAIProxy{
		keyManager: km,
		targetURL:  targetURL,
		debug:      cfg.Debug,
		logger:     logger.With("component", "proxy"),
	}

	proxy.reverseProxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = proxy.targetURL.Scheme
			req.URL.Host = proxy.targetURL.Host
			req.Host = proxy.targetURL.Host

			// Manually construct the full path to avoid issues with url.ResolveReference.
			trimmedPath := strings.TrimPrefix(req.URL.Path, "/v1")
			req.URL.Path = "/v1beta/openai" + trimmedPath

			// The key is retrieved in ServeHTTP and attached to the context.
			// The transport will use this key for the first attempt.
			key := req.Context().Value(geminiKeyContextKey).(string)
			req.Header.Set("Authorization", "Bearer "+key)

			// Sanitize the request body to remove OpenAI-specific fields.
			if err := proxy.ModifyRequestBody(req); err != nil {
				proxy.logger.Error("Failed to modify request body", "error", err)
				// We can't easily fail the request here, but logging is important.
			}

			if proxy.debug {
				proxy.logger.Debug("Proxying request", "path", req.URL.Path)
			}
		},
		Transport: &retryingTransport{
			keyManager: km,
			logger:     logger.With("component", "transport"),
			transport:  http.DefaultTransport,
		},
		// ModifyResponse is no longer needed as success/failure is handled in the transport.
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.Canceled) || errors.Is(err, http.ErrAbortHandler) {
				proxy.logger.Warn("Client disconnected", "error", err)
				return
			}
			proxy.logger.Error("Proxy error after all retries", "error", err)
			http.Error(w, "Service unavailable after multiple retries", http.StatusServiceUnavailable)
		},
	}

	return proxy, nil
}

// NewOpenAIProxy creates a new OpenAIProxy with the default Google API target.
func NewOpenAIProxy(km Manager, cfg *config.Config, logger *slog.Logger) (*OpenAIProxy, error) {
	return newOpenAIProxyWithURL(km, cfg, "https://generativelanguage.googleapis.com", logger)
}

func (p *OpenAIProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key, err := p.keyManager.GetNextKey()
	if err != nil {
		p.logger.Error("Failed to get next available key for proxy", "error", err)
		http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
		return
	}

	// Store the key in the request context to access it in Director and ModifyResponse
	ctx := context.WithValue(r.Context(), geminiKeyContextKey, key)
	req := r.WithContext(ctx)

	p.reverseProxy.ServeHTTP(w, req)
}

// safeKeySuffix returns the last 4 characters of a key for logging.
func safeKeySuffix(key string) string {
	if len(key) > 4 {
		return key[len(key)-4:]
	}
	return key
}

// Close is a no-op because the KeyManager's lifecycle is managed centrally.
func (p *OpenAIProxy) Close() {
	p.logger.Info("Proxy shutdown.")
}

// ModifyRequestBody reads the request body, removes OpenAI-specific fields,
// and replaces the request body with the modified version.
func (p *OpenAIProxy) ModifyRequestBody(req *http.Request) error {
	if req.Body == nil {
		return nil
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	// Restore the body so it can be read again if needed
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if len(bodyBytes) == 0 {
		return nil
	}

	var bodyJSON map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &bodyJSON); err != nil {
		// If it's not valid JSON, we don't touch it.
		p.logger.Debug("Request body is not valid JSON, skipping modification", "error", err)
		return nil
	}

	// List of OpenAI-specific fields to remove.
	// Sourced from OpenAI API documentation and common client libraries.
	fieldsToRemove := []string{
		"frequency_penalty",
		"presence_penalty",
		"logit_bias",
		"logprobs",
		"top_logprobs",
		"reasoning_effort",
		"max_completion_tokens",
		"n",
		"tools",
		"function_call",
		"functions",
	}

	modified := false
	for _, field := range fieldsToRemove {
		if _, ok := bodyJSON[field]; ok {
			delete(bodyJSON, field)
			modified = true
		}
	}

	// 1. Unconditionally remove "top_k"
	if _, ok := bodyJSON["top_k"]; ok {
		delete(bodyJSON, "top_k")
		modified = true
	}

	// 2. Remove any field that has a null value.
	for key, value := range bodyJSON {
		if value == nil {
			delete(bodyJSON, key)
			modified = true
		}
	}

	// Also, remove "models/" prefix from the model name if it exists.
	if model, ok := bodyJSON["model"].(string); ok {
		if strings.HasPrefix(model, "models/") {
			bodyJSON["model"] = strings.TrimPrefix(model, "models/")
			modified = true
		}
	}

	if modified {
		p.logger.Debug("Removed OpenAI-specific fields from request body", "fields", fieldsToRemove)
		newBodyBytes, err := json.Marshal(bodyJSON)
		if err != nil {
			return fmt.Errorf("failed to marshal modified request body: %w", err)
		}
		p.logger.Debug("Modified request body for proxying", "body", string(newBodyBytes))
		req.Body = io.NopCloser(bytes.NewBuffer(newBodyBytes))
		req.ContentLength = int64(len(newBodyBytes))
	}

	return nil
}
