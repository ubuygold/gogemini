package keymanager

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ubuygold/gogemini/internal/config"
	"github.com/ubuygold/gogemini/internal/db"
	"github.com/ubuygold/gogemini/internal/model"
)

// HTTPClient defines the interface for making HTTP requests.
// This allows for mocking in tests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Manager defines the interface for managing Gemini API keys.
// This allows for mocking in tests and decouples the admin handler from the concrete implementation.
type Manager interface {
	GetNextKey() (string, error)
	HandleKeyFailure(key string)
	HandleKeySuccess(key string)
	ReviveDisabledKeys()
	CheckAllKeysHealth()
	GetAvailableKeyCount() int
	TestKeyByID(id uint) error
	TestAllKeysAsync()
	Close()
}

// KeyManager holds the state of our load balancer.
// managedKey wraps a GeminiKey with additional in-memory state for the manager.
type managedKey struct {
	model.GeminiKey
	// Disabled marks the key as temporarily out of service.
	Disabled bool
	// DisabledAt records when the key was disabled.
	DisabledAt time.Time
}

// GetKey returns the key string.
func (mk *managedKey) GetKey() string {
	return mk.Key
}

// GetUsageCount returns the usage count.
func (mk *managedKey) GetUsageCount() int64 {
	return mk.UsageCount
}

// GetFailureCount returns the failure count.
func (mk *managedKey) GetFailureCount() int {
	return mk.FailureCount
}

type KeyManager struct {
	mutex            sync.Mutex
	keys             []*managedKey
	logger           *slog.Logger
	db               db.Service
	stopChan         chan struct{}
	updateQueue      chan string
	wg               sync.WaitGroup
	disableThreshold int
	httpClient       HTTPClient
	revivalInterval  time.Duration
	syncDBUpdates    bool // For testing purposes
}

// NewKeyManager creates a new KeyManager.
func NewKeyManager(dbService db.Service, cfg *config.Config, logger *slog.Logger) (*KeyManager, error) {
	initialKeys, err := dbService.LoadActiveGeminiKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to perform initial load of Gemini keys: %w", err)
	}
	if len(initialKeys) == 0 {
		logger.Warn("No active Gemini API keys found in the database. KeyManager will start but return no keys until they are added.")
	}

	managedKeys := make([]*managedKey, len(initialKeys))
	for i, key := range initialKeys {
		managedKeys[i] = &managedKey{GeminiKey: key}
	}

	km := &KeyManager{
		keys:             managedKeys,
		logger:           logger.With("component", "keymanager"),
		db:               dbService,
		stopChan:         make(chan struct{}),
		updateQueue:      make(chan string, 100), // Buffered channel
		disableThreshold: cfg.Proxy.DisableKeyThreshold,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Generous timeout for the check
		},
		revivalInterval: 5 * time.Minute, // Cooldown before a key can be revived
	}

	// Start a background goroutine to periodically update the keys from DB
	go km.keyReloader()

	// Start a background goroutine to process usage updates
	km.wg.Add(1)
	go km.usageUpdater()

	return km, nil
}

// GetNextKey selects the key with the lowest usage count.
func (km *KeyManager) GetNextKey() (string, error) {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	if len(km.keys) == 0 {
		return "", fmt.Errorf("no active Gemini keys available")
	}

	// Find the first key that is not disabled
	var keyToUse *managedKey
	var keyIndex int = -1
	for i, k := range km.keys {
		if !k.Disabled {
			keyToUse = k
			keyIndex = i
			break
		}
	}

	if keyIndex == -1 {
		return "", fmt.Errorf("all available Gemini keys are temporarily disabled")
	}

	keyStr := keyToUse.Key

	// Increment the usage count for the selected key in memory immediately.
	km.keys[keyIndex].UsageCount++

	// Re-sort the slice to maintain the order for the next call.
	km.sortKeys()

	// Asynchronously update the usage count in the database by sending it to the queue.
	select {
	case km.updateQueue <- keyStr:
		// Successfully queued
	default:
		// This case should be rare if the buffer is large enough and the worker is healthy.
		km.logger.Error("Failed to queue usage count update: queue is full")
	}

	return keyStr, nil
}

// sortKeys sorts the keys slice by UsageCount in ascending order.
func (km *KeyManager) sortKeys() {
	// This is an internal helper, so we assume the lock is already held.
	sort.Slice(km.keys, func(i, j int) bool {
		return km.keys[i].UsageCount < km.keys[j].UsageCount
	})
}

// keyReloader periodically reloads the keys from the database.
func (km *KeyManager) keyReloader() {
	// Wait for a minute before the first update
	time.Sleep(1 * time.Minute)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			km.updateKeys()
		case <-km.stopChan:
			km.logger.Info("Stopping key reloader.")
			return
		}
	}
}

// usageUpdater is a worker that processes key usage updates from a channel.
func (km *KeyManager) usageUpdater() {
	defer km.wg.Done()
	km.logger.Info("Starting usage updater worker.")

	for keyStr := range km.updateQueue {
		err := km.db.IncrementGeminiKeyUsageCount(keyStr)
		if err != nil {
			keySuffix := ""
			if len(keyStr) > 4 {
				keySuffix = keyStr[len(keyStr)-4:]
			} else {
				keySuffix = keyStr
			}
			km.logger.Warn("Failed to increment usage count in DB", "key_suffix", keySuffix, "error", err)
		}
	}
	km.logger.Info("Usage updater worker stopped.")
}

// updateKeys fetches the latest set of active keys from the database.
func (km *KeyManager) updateKeys() {
	km.logger.Info("Updating Gemini API keys from database...")
	keys, err := km.db.LoadActiveGeminiKeys()
	if err != nil {
		km.logger.Error("Failed to update Gemini keys from database", "error", err)
		return
	}

	km.mutex.Lock()
	defer km.mutex.Unlock()

	if len(keys) == 0 {
		km.logger.Warn("No active Gemini keys found in database during update.")
	}

	managedKeys := make([]*managedKey, len(keys))
	for i, key := range keys {
		managedKeys[i] = &managedKey{GeminiKey: key}
	}

	km.keys = managedKeys
	if len(keys) > 0 {
		km.logger.Info("Successfully updated Gemini API keys", "count", len(keys))
	}
}

// Close gracefully shuts down the KeyManager's background tasks.
func (km *KeyManager) Close() {
	close(km.stopChan)
	close(km.updateQueue)
	km.wg.Wait()
	km.logger.Info("KeyManager shutdown complete.")
}

// HandleKeyFailure is called when a key fails a request.
func (km *KeyManager) HandleKeyFailure(key string) {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	for _, k := range km.keys {
		if k.Key == key {
			k.FailureCount++
			if k.FailureCount >= km.disableThreshold {
				if !k.Disabled { // Only log and update status on the transition
					k.Disabled = true
					k.DisabledAt = time.Now()
					k.Status = "disabled"
					km.logger.Warn("Disabling key due to reaching failure threshold", "key_suffix", safeKeySuffix(key), "failures", k.FailureCount)
				}
			}

			// Persist the updated failure count and status to the database in the background.
			// We make a copy to avoid data races in the goroutine.
			keyToUpdate := k.GeminiKey
			if km.syncDBUpdates {
				if err := km.db.UpdateGeminiKey(&keyToUpdate); err != nil {
					km.logger.Error("Failed to update key failure count in DB", "key_id", keyToUpdate.ID, "error", err)
				}
			} else {
				go func() {
					if err := km.db.UpdateGeminiKey(&keyToUpdate); err != nil {
						km.logger.Error("Failed to update key failure count in DB", "key_id", keyToUpdate.ID, "error", err)
					}
				}()
			}
			break
		}
	}
}

// HandleKeySuccess is called when a key succeeds in a request.
func (km *KeyManager) HandleKeySuccess(key string) {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	for _, k := range km.keys {
		if k.Key == key {
			if k.FailureCount > 0 || k.Disabled {
				km.logger.Info("Re-activating key after successful request", "key_suffix", safeKeySuffix(key), "old_failures", k.FailureCount)
				k.FailureCount = 0
				k.Disabled = false
				k.Status = "active"

				// Persist the updated failure count and status to the database in the background.
				// We make a copy to avoid data races in the goroutine.
				keyToUpdate := k.GeminiKey
				if km.syncDBUpdates {
					if err := km.db.UpdateGeminiKey(&keyToUpdate); err != nil {
						km.logger.Error("Failed to update key success status in DB", "key_id", keyToUpdate.ID, "error", err)
					}
				} else {
					go func() {
						if err := km.db.UpdateGeminiKey(&keyToUpdate); err != nil {
							km.logger.Error("Failed to update key success status in DB", "key_id", keyToUpdate.ID, "error", err)
						}
					}()
				}
			}
			break
		}
	}
}

// safeKeySuffix returns the last 4 characters of a key, or the full key if it's shorter.
func safeKeySuffix(key string) string {
	if len(key) > 4 {
		return key[len(key)-4:]
	}
	return key
}

// ReviveDisabledKeys attempts to reactivate keys that were previously disabled.
func (km *KeyManager) ReviveDisabledKeys() {
	km.mutex.Lock()
	disabledKeys := make([]*managedKey, 0)
	for _, k := range km.keys {
		// Check if the key is disabled and if enough time has passed since it was disabled.
		if k.Disabled && time.Since(k.DisabledAt) > km.revivalInterval {
			disabledKeys = append(disabledKeys, k)
		}
	}
	km.mutex.Unlock()

	if len(disabledKeys) == 0 {
		return
	}

	km.logger.Info("Starting check to revive disabled keys", "count", len(disabledKeys))

	var wg sync.WaitGroup
	for _, k := range disabledKeys {
		wg.Add(1)
		go func(key *managedKey) {
			defer wg.Done()
			err := km.testAPIKey(key.Key)
			if err == nil {
				km.logger.Info("Successfully revived key", "key_suffix", safeKeySuffix(key.Key))
				km.HandleKeySuccess(key.Key)
			} else {
				km.logger.Debug("Key still failing check", "key_suffix", safeKeySuffix(key.Key), "error", err)
				// We need to update the DisabledAt time to reset the revival timer,
				// otherwise we'll keep checking it on every scheduler run.
				km.mutex.Lock()
				key.DisabledAt = time.Now()
				km.mutex.Unlock()
			}
		}(k)
	}
	wg.Wait()
	km.logger.Info("Finished checking disabled keys.")
}

// testAPIKey performs a simple, low-cost request to the Gemini API to validate a key.
func (km *KeyManager) testAPIKey(key string) error {
	// To validate a key, we send a request to the OpenAI-compatible model listing endpoint.
	// This is the most accurate and lightweight way to check if a key is valid for the proxy's use case.
	const testURL = "https://generativelanguage.googleapis.com/v1beta/openai/models"
	req, err := http.NewRequestWithContext(context.Background(), "GET", testURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}
	// The key for the OpenAI-compatible endpoint is still a Google Cloud API key, used as a Bearer token.
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := km.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("test request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// We read the body to get more context on the error.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("test request returned non-200 status: %d, body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

// CheckAllKeysHealth performs a health check on all managed keys.
func (km *KeyManager) CheckAllKeysHealth() {
	km.mutex.Lock()
	allKeys := make([]*managedKey, len(km.keys))
	copy(allKeys, km.keys)
	km.mutex.Unlock()

	if len(allKeys) == 0 {
		return
	}

	km.logger.Info("Starting daily health check for all keys", "count", len(allKeys))

	var wg sync.WaitGroup
	for _, k := range allKeys {
		wg.Add(1)
		go func(key *managedKey) {
			defer wg.Done()
			err := km.testAPIKey(key.Key)
			if err != nil {
				// Key is failing, if it's currently active, disable it.
				if !key.Disabled {
					km.logger.Warn("Key failed daily health check, disabling it.", "key_suffix", safeKeySuffix(key.Key), "error", err)
					// We manually set it to be at the threshold to ensure it gets disabled.
					km.mutex.Lock()
					key.FailureCount = km.disableThreshold - 1
					km.mutex.Unlock()
					km.HandleKeyFailure(key.Key)
				}
			} else {
				// Key is working, if it's currently disabled, enable it.
				if key.Disabled {
					km.logger.Info("Key passed daily health check, re-activating it.", "key_suffix", safeKeySuffix(key.Key))
					km.HandleKeySuccess(key.Key)
				}
			}
		}(k)
	}
	wg.Wait()
	km.logger.Info("Finished daily health check for all keys.")
}

// GetAvailableKeyCount returns the number of keys that are not currently disabled.
func (km *KeyManager) GetAvailableKeyCount() int {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	count := 0
	for _, k := range km.keys {
		if !k.Disabled {
			count++
		}
	}
	return count
}

// findKeyByID finds a key in the manager's current list by its database ID.
// Note: This searches the in-memory list, not the database directly.
func (km *KeyManager) findKeyByID(id uint) (*managedKey, error) {
	km.mutex.Lock()
	defer km.mutex.Unlock()
	for _, k := range km.keys {
		if k.ID == id {
			return k, nil
		}
	}
	return nil, fmt.Errorf("key with ID %d not found in active list", id)
}

// TestKeyByID fetches a key by its ID and performs a health check.
// This is a synchronous operation.
func (km *KeyManager) TestKeyByID(id uint) error {
	// First, try to find the key in the in-memory list for efficiency.
	mKey, err := km.findKeyByID(id)
	if err != nil {
		// If not in memory (e.g., it's inactive), fetch from DB.
		dbKey, dbErr := km.db.GetGeminiKey(id)
		if dbErr != nil {
			return fmt.Errorf("failed to find key with ID %d in DB: %w", id, dbErr)
		}
		// Create a managedKey and add it to the list so it can be handled.
		mKey = &managedKey{GeminiKey: *dbKey}
		km.mutex.Lock()
		km.keys = append(km.keys, mKey)
		km.mutex.Unlock()
	}

	km.logger.Info("Performing manual health check for key", "key_id", id)
	err = km.testAPIKey(mKey.Key)
	if err != nil {
		km.logger.Warn("Manual health check failed for key", "key_id", id, "error", err)
		// We can also trigger the failure handler to ensure its state is updated.
		km.HandleKeyFailure(mKey.Key)
		return err
	}

	km.logger.Info("Manual health check succeeded for key", "key_id", id)
	// On success, ensure the key is marked as active.
	km.HandleKeySuccess(mKey.Key)
	return nil
}

// TestAllKeysAsync triggers a health check for all keys in the background.
func (km *KeyManager) TestAllKeysAsync() {
	km.logger.Info("Triggering asynchronous health check for all keys...")
	go km.CheckAllKeysHealth()
}
