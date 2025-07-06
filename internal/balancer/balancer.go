package balancer

import (
	"context"
	"errors"
	"fmt"
	"gogemini/internal/db"
	"gogemini/internal/model"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"sync"
	"time"
)

// Balancer holds the state of our load balancer.
type Balancer struct {
	mutex       sync.Mutex
	keys        []model.GeminiKey
	proxy       *httputil.ReverseProxy
	logger      *slog.Logger
	db          db.Service
	stopChan    chan struct{}
	updateQueue chan string
	wg          sync.WaitGroup
}

// NewBalancer creates a new Balancer that acts as a reverse proxy.
func NewBalancer(dbService db.Service, logger *slog.Logger) (*Balancer, error) {
	initialKeys, err := dbService.LoadActiveGeminiKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to perform initial load of Gemini keys: %w", err)
	}
	if len(initialKeys) == 0 {
		logger.Warn("No active Gemini API keys found in the database. Balancer will start but return 503 until keys are added.")
	}

	targetURL, err := url.Parse("https://generativelanguage.googleapis.com")
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	balancer := &Balancer{
		keys:        initialKeys,
		proxy:       proxy,
		logger:      logger.With("component", "balancer"),
		db:          dbService,
		stopChan:    make(chan struct{}),
		updateQueue: make(chan string, 100), // Buffered channel
	}

	// Start a background goroutine to periodically update the keys from DB
	go balancer.keyReloader()

	// Start a background goroutine to process usage updates
	balancer.wg.Add(1)
	go balancer.usageUpdater()

	proxy.Director = func(req *http.Request) {
		key := balancer.getNextKey()
		if key == "" {
			balancer.logger.Error("No available Gemini key for request")
			// Let the request proceed without a key, which will result in an authentication error from Google.
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

// getNextKey selects the key with the lowest usage count.
// This method is now responsible for both selecting the key and initiating the usage count update.
func (b *Balancer) getNextKey() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if len(b.keys) == 0 {
		b.logger.Error("No active Gemini keys available to serve request")
		return ""
	}

	// The keys are already sorted by usage_count from the database and after each use.
	// We just need to pick the first one.
	// The keys are already sorted, so the one with the lowest usage is at the front.
	keyToUse := b.keys[0]
	keyStr := keyToUse.Key

	// Increment the usage count for the selected key in memory immediately.
	// This prevents the same key from being picked by the next request before the list is resorted.
	b.keys[0].UsageCount++

	// Re-sort the slice to maintain the order for the next call.
	b.sortKeys()

	// Asynchronously update the usage count in the database by sending it to the queue.
	select {
	case b.updateQueue <- keyStr:
		// Successfully queued
	default:
		// This case should be rare if the buffer is large enough and the worker is healthy.
		b.logger.Error("Failed to queue usage count update: queue is full")
	}

	return keyStr
}

// sortKeys sorts the keys slice by UsageCount in ascending order.
func (b *Balancer) sortKeys() {
	// This is an internal helper, so we assume the lock is already held.
	sort.Slice(b.keys, func(i, j int) bool {
		return b.keys[i].UsageCount < b.keys[j].UsageCount
	})
}

// keyReloader periodically reloads the keys from the database.
func (b *Balancer) keyReloader() {
	// Wait for a minute before the first update
	time.Sleep(1 * time.Minute)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.updateKeys()
		case <-b.stopChan:
			b.logger.Info("Stopping key reloader.")
			return
		}
	}
}

// usageUpdater is a worker that processes key usage updates from a channel.
func (b *Balancer) usageUpdater() {
	defer b.wg.Done()
	b.logger.Info("Starting usage updater worker.")

	for keyStr := range b.updateQueue {
		err := b.db.IncrementGeminiKeyUsageCount(keyStr)
		if err != nil {
			// Use a safe suffix for logging to avoid exposing the full key.
			keySuffix := ""
			if len(keyStr) > 4 {
				keySuffix = keyStr[len(keyStr)-4:]
			} else {
				keySuffix = keyStr
			}
			b.logger.Warn("Failed to increment usage count in DB", "key_suffix", keySuffix, "error", err)
		}
	}
	b.logger.Info("Usage updater worker stopped.")
}

// updateKeys fetches the latest set of active keys from the database.
func (b *Balancer) updateKeys() {
	b.logger.Info("Updating Gemini API keys from database...")
	keys, err := b.db.LoadActiveGeminiKeys()
	if err != nil {
		b.logger.Error("Failed to update Gemini keys from database", "error", err)
		// Keep using the old set of keys if the update fails
		return
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	if len(keys) == 0 {
		b.logger.Warn("No active Gemini keys found in database during update. Balancer will now return 503s.")
	}

	b.keys = keys
	if len(keys) > 0 {
		b.logger.Info("Successfully updated Gemini API keys", "count", len(keys))
	}
}

// ServeHTTP is the handler for all incoming requests.
func (b *Balancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mutex.Lock()
	noKeys := len(b.keys) == 0
	b.mutex.Unlock()

	if noKeys {
		b.logger.Error("Service Unavailable: No active Gemini API keys")
		http.Error(w, "Service Unavailable: No active API keys", http.StatusServiceUnavailable)
		return
	}
	// The ReverseProxy handles everything, including streaming.
	b.proxy.ServeHTTP(w, r)
}

// Close gracefully shuts down the balancer's background tasks.
func (b *Balancer) Close() {
	// Stop the periodic key reloader
	close(b.stopChan)

	// Close the update queue, which signals the usageUpdater to finish processing.
	close(b.updateQueue)

	// Wait for the usageUpdater to finish processing all items in the queue.
	b.wg.Wait()
	b.logger.Info("Balancer shutdown complete.")
}
