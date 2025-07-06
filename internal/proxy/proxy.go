package proxy

import (
	"context"
	"errors"
	"fmt"
	"gogemini/internal/config"
	"gogemini/internal/db"
	"gogemini/internal/model"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

type OpenAIProxy struct {
	geminiKeys              []string
	nextKeyIndex            int
	mutex                   sync.Mutex
	reverseProxy            *httputil.ReverseProxy
	targetURL               *url.URL
	debug                   bool
	logger                  *slog.Logger
	database                db.Service
	stopChan                chan struct{}
	disableThreshold        int
	temporarilyDisabledKeys map[string]struct{}
	tempDisableMutex        sync.Mutex
}

type contextKey string

const geminiKeyContextKey = contextKey("geminiKey")

// newOpenAIProxyWithURL is the internal constructor that allows for custom target URLs, making it testable.
func newOpenAIProxyWithURL(dbService db.Service, cfg *config.Config, target string, logger *slog.Logger) (*OpenAIProxy, error) {
	initialKeys, err := dbService.LoadActiveGeminiKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to perform initial load of Gemini keys for proxy: %w", err)
	}
	if len(initialKeys) == 0 {
		logger.Warn("No active Gemini API keys found in the database for proxy. It will start but return 503 until keys are added.")
	}

	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	proxy := &OpenAIProxy{
		geminiKeys:              extractKeys(initialKeys),
		targetURL:               targetURL,
		debug:                   cfg.Debug,
		logger:                  logger.With("component", "proxy"),
		database:                dbService,
		stopChan:                make(chan struct{}),
		disableThreshold:        cfg.Proxy.DisableKeyThreshold,
		temporarilyDisabledKeys: make(map[string]struct{}),
	}

	// Start a background goroutine to periodically update the keys
	go proxy.keyUpdater()

	proxy.reverseProxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = proxy.targetURL.Scheme
			req.URL.Host = proxy.targetURL.Host
			req.Host = proxy.targetURL.Host

			// Manually construct the full path to avoid issues with url.ResolveReference.
			trimmedPath := strings.TrimPrefix(req.URL.Path, "/v1")
			req.URL.Path = "/v1beta/openai" + trimmedPath

			// Use the next key in a round-robin fashion, skipping temporarily disabled keys
			key, err := proxy.getNextAvailableKey()
			if err != nil {
				proxy.logger.Error("Failed to get next available key", "error", err)
				// We don't set the Authorization header, which will cause an upstream auth error.
				return
			}

			if proxy.debug {
				proxy.logger.Debug("Proxying request", "path", req.URL.Path, "key_suffix", safeKeySuffix(key))
			}

			// Store the key in the request context to access it in ModifyResponse
			ctx := context.WithValue(req.Context(), geminiKeyContextKey, key)
			*req = *req.WithContext(ctx)

			// Set the Authorization header for the upstream request.
			req.Header.Set("Authorization", "Bearer "+key)
		},
		ModifyResponse: proxy.modifyResponse,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.Canceled) || errors.Is(err, http.ErrAbortHandler) {
				proxy.logger.Warn("Client disconnected", "error", err)
				return
			}
			proxy.logger.Error("Proxy error", "error", err)
			http.Error(w, "Proxy Error", http.StatusBadGateway)
		},
	}

	return proxy, nil
}

// NewOpenAIProxy creates a new OpenAIProxy with the default Google API target.
func NewOpenAIProxy(dbService db.Service, cfg *config.Config, logger *slog.Logger) (*OpenAIProxy, error) {
	return newOpenAIProxyWithURL(dbService, cfg, "https://generativelanguage.googleapis.com", logger)
}

func (p *OpenAIProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mutex.Lock()
	noKeys := len(p.geminiKeys) == 0
	p.mutex.Unlock()

	if noKeys {
		p.logger.Error("Service Unavailable: No active Gemini API keys for proxy")
		http.Error(w, "Service Unavailable: No active API keys for proxy", http.StatusServiceUnavailable)
		return
	}

	p.reverseProxy.ServeHTTP(w, r)
}

// keyUpdater periodically reloads the keys from the database.
func (p *OpenAIProxy) keyUpdater() {
	// Wait for a minute before the first update
	time.Sleep(1 * time.Minute)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.updateKeys()
		case <-p.stopChan:
			p.logger.Info("Stopping proxy key updater.")
			return
		}
	}
}

// updateKeys fetches the latest set of active keys from the database.
func (p *OpenAIProxy) updateKeys() {
	p.logger.Info("Updating Gemini API keys for proxy from database...")
	keys, err := p.database.LoadActiveGeminiKeys()
	if err != nil {
		p.logger.Error("Failed to update Gemini keys for proxy from database", "error", err)
		// Keep using the old set of keys if the update fails
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(keys) == 0 {
		p.logger.Warn("No active Gemini keys found in database during proxy update. Proxy will now return 503s.")
	}

	p.geminiKeys = extractKeys(keys)
	p.nextKeyIndex = 0 // Reset the counter to be safe

	// Clear the temporary disabled list as we have a fresh list of active keys
	p.tempDisableMutex.Lock()
	p.temporarilyDisabledKeys = make(map[string]struct{})
	p.tempDisableMutex.Unlock()

	if len(keys) > 0 {
		p.logger.Info("Successfully updated Gemini API keys for proxy", "count", len(keys))
	}
}

// modifyResponse is called after the response from the target is received.
func (p *OpenAIProxy) modifyResponse(resp *http.Response) error {
	key, ok := resp.Request.Context().Value(geminiKeyContextKey).(string)
	if !ok {
		p.logger.Error("Gemini key not found in request context in modifyResponse")
		return nil
	}

	if resp.StatusCode == http.StatusForbidden {
		p.logger.Warn("Gemini key returned 403 Forbidden", "key_suffix", safeKeySuffix(key))
		// Run the database update in a goroutine to avoid blocking the response
		go func(failedKey string) {
			disabled, err := p.database.HandleGeminiKeyFailure(failedKey, p.disableThreshold)
			if err != nil {
				p.logger.Error("Failed to handle Gemini key failure", "key_suffix", safeKeySuffix(failedKey), "error", err)
				return
			}
			if disabled {
				p.logger.Error("Temporarily disabling Gemini key due to repeated failures", "key_suffix", safeKeySuffix(failedKey))
				p.tempDisableMutex.Lock()
				p.temporarilyDisabledKeys[failedKey] = struct{}{}
				p.tempDisableMutex.Unlock()
			}
		}(key)
	} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// On success, reset failure count and remove from temp disabled list if present.
		go func(succeededKey string) {
			// First, check if it was temporarily disabled.
			p.tempDisableMutex.Lock()
			_, wasTemporarilyDisabled := p.temporarilyDisabledKeys[succeededKey]
			p.tempDisableMutex.Unlock()

			// Then, reset the failure count in the database.
			if err := p.database.ResetGeminiKeyFailureCount(succeededKey); err != nil {
				// This is not a critical error, so we just log it.
				p.logger.Warn("Failed to reset failure count for key", "key_suffix", safeKeySuffix(succeededKey), "error", err)
			}

			// Also, increment the usage count.
			if err := p.database.IncrementGeminiKeyUsageCount(succeededKey); err != nil {
				p.logger.Warn("Failed to increment usage count for key", "key_suffix", safeKeySuffix(succeededKey), "error", err)
			}

			// If it was temporarily disabled, log its re-activation.
			if wasTemporarilyDisabled {
				p.logger.Info("Re-activating key after successful request", "key_suffix", safeKeySuffix(succeededKey))
				p.tempDisableMutex.Lock()
				delete(p.temporarilyDisabledKeys, succeededKey)
				p.tempDisableMutex.Unlock()
			}
		}(key)
	}

	return nil
}

// getNextAvailableKey finds the next key that is not in the temporarily disabled list.
func (p *OpenAIProxy) getNextAvailableKey() (string, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(p.geminiKeys) == 0 {
		return "", fmt.Errorf("no active Gemini keys available")
	}

	// Iterate through the keys starting from the current index to find an available one.
	for i := 0; i < len(p.geminiKeys); i++ {
		candidateIndex := (p.nextKeyIndex + i) % len(p.geminiKeys)
		candidateKey := p.geminiKeys[candidateIndex]

		p.tempDisableMutex.Lock()
		_, isDisabled := p.temporarilyDisabledKeys[candidateKey]
		p.tempDisableMutex.Unlock()

		if !isDisabled {
			// Found an available key. Update the index for the next call.
			p.nextKeyIndex = (candidateIndex + 1) % len(p.geminiKeys)
			return candidateKey, nil
		}
	}

	return "", fmt.Errorf("all available Gemini keys are temporarily disabled")
}

// safeKeySuffix returns the last 4 characters of a key, or the full key if it's shorter.
func safeKeySuffix(key string) string {
	if len(key) > 4 {
		return key[len(key)-4:]
	}
	return key
}

// extractKeys converts a slice of GeminiKey models to a slice of key strings.
func extractKeys(keys []model.GeminiKey) []string {
	stringKeys := make([]string, len(keys))
	for i, k := range keys {
		stringKeys[i] = k.Key
	}
	return stringKeys
}

// Close gracefully shuts down the proxy's background tasks.
func (p *OpenAIProxy) Close() {
	close(p.stopChan)
}
