package keymanager

import (
	"errors"
	"gogemini/internal/config"
	"gogemini/internal/model"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDBService is a mock implementation of the db.Service interface.
type MockDBService struct {
	mock.Mock
}

func (m *MockDBService) LoadActiveGeminiKeys() ([]model.GeminiKey, error) {
	args := m.Called()
	return args.Get(0).([]model.GeminiKey), args.Error(1)
}

func (m *MockDBService) IncrementGeminiKeyUsageCount(key string) error {
	args := m.Called(key)
	return args.Error(0)
}

func (m *MockDBService) HandleGeminiKeyFailure(key string, threshold int) (bool, error) {
	args := m.Called(key, threshold)
	return args.Bool(0), args.Error(1)
}

func (m *MockDBService) ResetGeminiKeyFailureCount(key string) error {
	args := m.Called(key)
	return args.Error(0)
}

// Implement other db.Service methods if needed for tests, returning nil or zero values.
func (m *MockDBService) CreateGeminiKey(key *model.GeminiKey) error { return nil }
func (m *MockDBService) BatchAddGeminiKeys(keys []string) error     { return nil }
func (m *MockDBService) BatchDeleteGeminiKeys(ids []uint) error     { return nil }
func (m *MockDBService) ListGeminiKeys(page, limit int, statusFilter string, minFailureCount int) ([]model.GeminiKey, int64, error) {
	return nil, 0, nil
}
func (m *MockDBService) GetGeminiKey(id uint) (*model.GeminiKey, error) { return nil, nil }
func (m *MockDBService) UpdateGeminiKey(key *model.GeminiKey) error {
	args := m.Called(key)
	return args.Error(0)
}
func (m *MockDBService) DeleteGeminiKey(id uint) error                     { return nil }
func (m *MockDBService) UpdateGeminiKeyStatus(key, status string) error    { return nil }
func (m *MockDBService) CreateAPIKey(key *model.APIKey) error              { return nil }
func (m *MockDBService) ListAPIKeys() ([]model.APIKey, error)              { return nil, nil }
func (m *MockDBService) GetAPIKey(id uint) (*model.APIKey, error)          { return nil, nil }
func (m *MockDBService) UpdateAPIKey(key *model.APIKey) error              { return nil }
func (m *MockDBService) DeleteAPIKey(id uint) error                        { return nil }
func (m *MockDBService) IncrementAPIKeyUsageCount(key string) error        { return nil }
func (m *MockDBService) ResetAllAPIKeyUsage() error                        { return nil }
func (m *MockDBService) FindAPIKeyByKey(key string) (*model.APIKey, error) { return nil, nil }

func TestNewKeyManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}

	t.Run("successful initialization", func(t *testing.T) {
		mockDB := new(MockDBService)
		keys := []model.GeminiKey{{Key: "key1"}, {Key: "key2"}}
		mockDB.On("LoadActiveGeminiKeys").Return(keys, nil).Once()

		km, err := NewKeyManager(mockDB, cfg, logger)
		assert.NoError(t, err)
		assert.NotNil(t, km)
		assert.Equal(t, 2, len(km.keys))
		mockDB.AssertExpectations(t)
		km.Close() // Shutdown background goroutines
	})

	t.Run("db error on initial load", func(t *testing.T) {
		mockDB := new(MockDBService)
		mockDB.On("LoadActiveGeminiKeys").Return(([]model.GeminiKey)(nil), errors.New("db error")).Once()

		km, err := NewKeyManager(mockDB, cfg, logger)
		assert.Error(t, err)
		assert.Nil(t, km)
		mockDB.AssertExpectations(t)
	})

	t.Run("no active keys found", func(t *testing.T) {
		mockDB := new(MockDBService)
		mockDB.On("LoadActiveGeminiKeys").Return(([]model.GeminiKey)(nil), nil).Once()

		km, err := NewKeyManager(mockDB, cfg, logger)
		assert.NoError(t, err)
		assert.NotNil(t, km)
		assert.Empty(t, km.keys)
		mockDB.AssertExpectations(t)
		km.Close()
	})
}

func TestGetNextKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("selects key with lowest usage", func(t *testing.T) {
		mockDB := new(MockDBService)
		keys := []model.GeminiKey{
			{Key: "key1", UsageCount: 10},
			{Key: "key2", UsageCount: 5}, // Should be picked first
			{Key: "key3", UsageCount: 15},
		}
		// We don't need to mock LoadActiveGeminiKeys here since we are setting keys manually
		managedKeys := make([]*managedKey, len(keys))
		for i, k := range keys {
			managedKeys[i] = &managedKey{GeminiKey: k}
		}
		km := &KeyManager{
			keys:        managedKeys,
			logger:      logger,
			db:          mockDB,
			updateQueue: make(chan string, 10),
		}
		km.sortKeys() // Ensure keys are sorted initially

		mockDB.On("IncrementGeminiKeyUsageCount", "key2").Return(nil).Once()

		key, err := km.GetNextKey()
		assert.NoError(t, err)
		assert.Equal(t, "key2", key)

		// Verify that the usage count was incremented in memory and re-sorted
		assert.Equal(t, int64(6), km.keys[0].UsageCount) // key2 is now at the front
		assert.Equal(t, "key1", km.keys[1].Key)

		// Drain the queue to allow shutdown
		close(km.updateQueue)
	})

	t.Run("no available keys", func(t *testing.T) {
		mockDB := new(MockDBService)
		km := &KeyManager{
			keys:   []*managedKey{},
			logger: logger,
			db:     mockDB,
		}

		key, err := km.GetNextKey()
		assert.Error(t, err)
		assert.Equal(t, "", key)
	})
}
func TestHandleKeyFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}

	t.Run("increments failure count and disables key on threshold", func(t *testing.T) {
		mockDB := new(MockDBService)
		keys := []*managedKey{
			{GeminiKey: model.GeminiKey{Key: "key1", Status: "active", FailureCount: 2}},
		}
		km := &KeyManager{
			keys:             keys,
			logger:           logger,
			db:               mockDB,
			disableThreshold: cfg.Proxy.DisableKeyThreshold,
			updateQueue:      make(chan string, 10),
		}

		// Expect UpdateGeminiKey to be called with the updated key data
		mockDB.On("UpdateGeminiKey", mock.MatchedBy(func(k *model.GeminiKey) bool {
			return k.Key == "key1" && k.FailureCount == 3 && k.Status == "disabled"
		})).Return(nil).Once()

		km.HandleKeyFailure("key1")

		// Check internal state
		assert.Equal(t, 3, km.keys[0].FailureCount)
		assert.True(t, km.keys[0].Disabled)
		assert.NotNil(t, km.keys[0].DisabledAt)

		// Allow time for the async DB update to be called
		time.Sleep(50 * time.Millisecond)
		mockDB.AssertExpectations(t)
	})

	t.Run("does not disable key below threshold", func(t *testing.T) {
		mockDB := new(MockDBService)
		keys := []*managedKey{
			{GeminiKey: model.GeminiKey{Key: "key1", Status: "active", FailureCount: 1}},
		}
		km := &KeyManager{
			keys:             keys,
			logger:           logger,
			db:               mockDB,
			disableThreshold: cfg.Proxy.DisableKeyThreshold,
			updateQueue:      make(chan string, 10),
		}

		// No DB call is expected
		km.HandleKeyFailure("key1")

		assert.Equal(t, 2, km.keys[0].FailureCount)
		assert.False(t, km.keys[0].Disabled)
		// We still expect a DB call to update the failure count
		mockDB.On("UpdateGeminiKey", mock.MatchedBy(func(k *model.GeminiKey) bool {
			return k.Key == "key1" && k.FailureCount == 2
		})).Return(nil).Once()

		// Allow time for the async DB update to be called
		time.Sleep(50 * time.Millisecond)
		mockDB.AssertExpectations(t)
	})
}

func TestHandleKeySuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("resets failure count and re-enables key", func(t *testing.T) {
		mockDB := new(MockDBService)
		keys := []*managedKey{
			{
				GeminiKey:  model.GeminiKey{Key: "key1", Status: "disabled", FailureCount: 3},
				Disabled:   true,
				DisabledAt: time.Now(),
			},
		}
		km := &KeyManager{
			keys:        keys,
			logger:      logger,
			db:          mockDB,
			updateQueue: make(chan string, 10),
		}

		// Expect UpdateGeminiKey to be called with the reset key data
		mockDB.On("UpdateGeminiKey", mock.MatchedBy(func(k *model.GeminiKey) bool {
			return k.Key == "key1" && k.FailureCount == 0 && k.Status == "active"
		})).Return(nil).Once()

		km.HandleKeySuccess("key1")

		// Check internal state
		assert.Equal(t, 0, km.keys[0].FailureCount)
		assert.False(t, km.keys[0].Disabled)

		// Allow time for the async DB update to be called
		time.Sleep(50 * time.Millisecond)
		mockDB.AssertExpectations(t)
	})

	t.Run("does nothing for a healthy key", func(t *testing.T) {
		mockDB := new(MockDBService)
		keys := []*managedKey{
			{GeminiKey: model.GeminiKey{Key: "key1", Status: "active", FailureCount: 0}},
		}
		km := &KeyManager{
			keys:        keys,
			logger:      logger,
			db:          mockDB,
			updateQueue: make(chan string, 10),
		}

		// No DB call is expected
		km.HandleKeySuccess("key1")

		assert.Equal(t, 0, km.keys[0].FailureCount)
		assert.False(t, km.keys[0].Disabled)
		t.Run("returns error when all keys are disabled", func(t *testing.T) {
			mockDB := new(MockDBService)
			keys := []*managedKey{
				{GeminiKey: model.GeminiKey{Key: "key1"}, Disabled: true},
				{GeminiKey: model.GeminiKey{Key: "key2"}, Disabled: true},
			}
			km := &KeyManager{
				keys:   keys,
				logger: logger,
				db:     mockDB,
			}

			key, err := km.GetNextKey()
			assert.Error(t, err)
			assert.Equal(t, "all available Gemini keys are temporarily disabled", err.Error())
			assert.Equal(t, "", key)
		})

		mockDB.AssertExpectations(t)
	})
}

func TestReviveDisabledKeys(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := &config.Config{}

	t.Run("revives a valid key", func(t *testing.T) {
		mockDB := new(MockDBService)
		// This key will be revived by a real API call, so it needs to be a valid one.
		// Note: This test requires network access and a valid Google API key.
		// You should replace "your-real-valid-google-api-key" with a key for testing.
		// For CI/CD, this might need to be handled via secrets.
		validTestKey := os.Getenv("VALID_GEMINI_API_KEY")
		if validTestKey == "" {
			t.Skip("Skipping test: VALID_GEMINI_API_KEY environment variable not set.")
		}

		keys := []*managedKey{
			{
				GeminiKey:  model.GeminiKey{Key: validTestKey, Status: "disabled"},
				Disabled:   true,
				DisabledAt: time.Now().Add(-1 * time.Minute), // Disabled a minute ago
			},
		}
		mockDB.On("LoadActiveGeminiKeys").Return(([]model.GeminiKey)(nil), nil).Once()
		km, err := NewKeyManager(mockDB, cfg, logger)
		assert.NoError(t, err)
		km.keys = keys // Manually set the keys for the test

		// Expect the key to be marked as active in the DB after successful test
		mockDB.On("UpdateGeminiKey", mock.MatchedBy(func(k *model.GeminiKey) bool {
			return k.Key == validTestKey && k.FailureCount == 0 && k.Status == "active"
		})).Return(nil).Once()

		km.ReviveDisabledKeys()

		// Check internal state
		assert.False(t, km.keys[0].Disabled)
		assert.Equal(t, 0, km.keys[0].FailureCount)

		mockDB.AssertExpectations(t)
	})

	t.Run("does not revive an invalid key", func(t *testing.T) {
		mockDB := new(MockDBService)
		invalidKey := "AIzaSyDS-jJkLryClJOym8AfAJtkBUoKbC4AN_4"
		keys := []*managedKey{
			{
				GeminiKey:  model.GeminiKey{Key: invalidKey, Status: "disabled"},
				Disabled:   true,
				DisabledAt: time.Now().Add(-1 * time.Minute),
			},
		}
		mockDB.On("LoadActiveGeminiKeys").Return(([]model.GeminiKey)(nil), nil).Once()
		km, err := NewKeyManager(mockDB, cfg, logger)
		assert.NoError(t, err)
		km.keys = keys // Manually set the keys for the test

		// We do NOT expect any DB calls because the key is invalid and should not be revived.
		km.ReviveDisabledKeys()

		// Check internal state - key should remain disabled
		assert.True(t, km.keys[0].Disabled)

		mockDB.AssertExpectations(t)
	})
}
