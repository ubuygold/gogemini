package keymanager

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ubuygold/gogemini/internal/config"
	"github.com/ubuygold/gogemini/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MockDBService is a mock implementation of the db.Service interface.
type MockDBService struct {
	mock.Mock
}

// MockHTTPClient is a mock implementation of the HTTPClient interface.
type MockHTTPClient struct {
	mock.Mock
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
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
func (m *MockDBService) GetGeminiKey(id uint) (*model.GeminiKey, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.GeminiKey), args.Error(1)
}
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
		assert.Equal(t, int64(6), km.keys[0].GetUsageCount()) // key2 is now at the front
		assert.Equal(t, "key1", km.keys[1].GetKey())

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
		assert.Equal(t, 3, km.keys[0].GetFailureCount())
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

		assert.Equal(t, 2, km.keys[0].GetFailureCount())
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
		assert.Equal(t, 0, km.keys[0].GetFailureCount())
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

		assert.Equal(t, 0, km.keys[0].GetFailureCount())
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

	t.Run("revives a valid key", func(t *testing.T) {
		mockDB := new(MockDBService)
		mockHTTP := new(MockHTTPClient)
		validKey := "a-valid-key"

		keys := []*managedKey{
			{
				GeminiKey:  model.GeminiKey{Key: validKey, Status: "disabled"},
				Disabled:   true,
				DisabledAt: time.Now().Add(-1 * time.Minute), // Disabled a minute ago
			},
		}
		km := &KeyManager{
			keys:          keys,
			logger:        logger,
			db:            mockDB,
			httpClient:    mockHTTP,
			updateQueue:   make(chan string, 10),
			syncDBUpdates: true,
		}

		// Mock the HTTP call to succeed
		mockHTTP.On("Do", mock.Anything).Return(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("OK"))}, nil).Once()
		// Expect the key to be marked as active in the DB after successful test
		mockDB.On("UpdateGeminiKey", mock.MatchedBy(func(k *model.GeminiKey) bool {
			return k.Key == validKey && k.FailureCount == 0 && k.Status == "active"
		})).Return(nil).Once()

		km.ReviveDisabledKeys()

		// Check internal state
		assert.False(t, km.keys[0].Disabled)
		assert.Equal(t, 0, km.keys[0].GetFailureCount())

		mockHTTP.AssertExpectations(t)
		mockDB.AssertExpectations(t)
	})

	t.Run("does not revive an invalid key", func(t *testing.T) {
		mockDB := new(MockDBService)
		mockHTTP := new(MockHTTPClient)
		invalidKey := "an-invalid-key"
		keys := []*managedKey{
			{
				GeminiKey:  model.GeminiKey{Key: invalidKey, Status: "disabled"},
				Disabled:   true,
				DisabledAt: time.Now().Add(-1 * time.Minute),
			},
		}
		km := &KeyManager{
			keys:          keys,
			logger:        logger,
			db:            mockDB,
			httpClient:    mockHTTP,
			updateQueue:   make(chan string, 10),
			syncDBUpdates: true,
		}

		// Mock the HTTP call to fail
		mockHTTP.On("Do", mock.Anything).Return(&http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("Bad Request"))}, nil).Once()
		// We do NOT expect any DB calls because the key is invalid and should not be revived.
		km.ReviveDisabledKeys()

		// Check internal state - key should remain disabled
		assert.True(t, km.keys[0].Disabled)

		mockHTTP.AssertExpectations(t)
		mockDB.AssertExpectations(t)
	})
}

func TestKeyManager_Misc(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockDB := new(MockDBService)

	keys := []*managedKey{
		{GeminiKey: model.GeminiKey{Model: gorm.Model{ID: 1}, Key: "key1"}, Disabled: false},
		{GeminiKey: model.GeminiKey{Model: gorm.Model{ID: 2}, Key: "key2"}, Disabled: true},
		{GeminiKey: model.GeminiKey{Model: gorm.Model{ID: 3}, Key: "key3"}, Disabled: false},
	}

	km := &KeyManager{
		keys:   keys,
		logger: logger,
		db:     mockDB,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	t.Run("GetAvailableKeyCount", func(t *testing.T) {
		assert.Equal(t, 2, km.GetAvailableKeyCount())
	})

	t.Run("findKeyByID", func(t *testing.T) {
		foundKey, err := km.findKeyByID(2)
		assert.NoError(t, err)
		assert.Equal(t, "key2", foundKey.Key)

		_, err = km.findKeyByID(99)
		assert.Error(t, err)
	})

	t.Run("TestAllKeysAsync", func(t *testing.T) {
		// This is hard to test without a more complex setup,
		// but we can at least call it and ensure it doesn't panic.
		// A more thorough test would involve mocks for the http client.
		km.TestAllKeysAsync()
	})
}

func TestUpdateKeys(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockDB := new(MockDBService)

	km := &KeyManager{
		keys:   []*managedKey{{GeminiKey: model.GeminiKey{Key: "initial-key"}}},
		logger: logger,
		db:     mockDB,
	}

	// Define the new set of keys to be returned by the mock DB
	newKeys := []model.GeminiKey{
		{Key: "new-key-1", UsageCount: 1},
		{Key: "new-key-2", UsageCount: 2},
	}
	mockDB.On("LoadActiveGeminiKeys").Return(newKeys, nil).Once()

	km.updateKeys()

	// Verify that the keys in the manager have been updated
	assert.Equal(t, 2, len(km.keys))
	assert.Equal(t, "new-key-1", km.keys[0].Key)
	assert.Equal(t, "new-key-2", km.keys[1].Key)

	mockDB.AssertExpectations(t)
}

func TestTestKeyByID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("key found in memory, test fails", func(t *testing.T) {
		mockDB := new(MockDBService)
		mockHTTP := new(MockHTTPClient)
		km := &KeyManager{
			keys:             []*managedKey{{GeminiKey: model.GeminiKey{Model: gorm.Model{ID: 1}, Key: "in-memory-key"}}},
			logger:           logger,
			db:               mockDB,
			httpClient:       mockHTTP,
			disableThreshold: 1,
			updateQueue:      make(chan string, 10),
			syncDBUpdates:    true,
		}

		// Mock the HTTP call to fail
		mockHTTP.On("Do", mock.Anything).Return(&http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("Bad Request"))}, nil).Once()
		// Expect the DB to be updated with the failure
		mockDB.On("UpdateGeminiKey", mock.MatchedBy(func(k *model.GeminiKey) bool {
			return k.ID == 1 && k.Status == "disabled"
		})).Return(nil).Once()

		err := km.TestKeyByID(1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "test request returned non-200 status: 400")

		// No need for sleep, as we can now deterministically check mock calls
		mockHTTP.AssertExpectations(t)
		mockDB.AssertExpectations(t)
	})

	t.Run("key found in memory, test succeeds", func(t *testing.T) {
		mockDB := new(MockDBService)
		mockHTTP := new(MockHTTPClient)
		km := &KeyManager{
			keys:          []*managedKey{{GeminiKey: model.GeminiKey{Model: gorm.Model{ID: 1}, Key: "in-memory-key", Status: "disabled"}, Disabled: true}},
			logger:        logger,
			db:            mockDB,
			httpClient:    mockHTTP,
			updateQueue:   make(chan string, 10),
			syncDBUpdates: true,
		}

		// Mock the HTTP call to succeed
		mockHTTP.On("Do", mock.Anything).Return(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("OK"))}, nil).Once()
		// Expect the DB to be updated with the success (re-activation)
		mockDB.On("UpdateGeminiKey", mock.MatchedBy(func(k *model.GeminiKey) bool {
			return k.ID == 1 && k.Status == "active"
		})).Return(nil).Once()

		err := km.TestKeyByID(1)
		assert.NoError(t, err)

		mockHTTP.AssertExpectations(t)
		mockDB.AssertExpectations(t)
	})

	t.Run("key not in memory, fetched from DB, test fails", func(t *testing.T) {
		mockDB := new(MockDBService)
		mockHTTP := new(MockHTTPClient)
		km := &KeyManager{
			keys:             []*managedKey{},
			logger:           logger,
			db:               mockDB,
			httpClient:       mockHTTP,
			disableThreshold: 1,
			updateQueue:      make(chan string, 10),
			syncDBUpdates:    true,
		}

		dbKey := &model.GeminiKey{Model: gorm.Model{ID: 3}, Key: "db-key"}
		mockDB.On("GetGeminiKey", uint(3)).Return(dbKey, nil).Once()
		mockHTTP.On("Do", mock.Anything).Return(&http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("Server Error"))}, nil).Once()
		mockDB.On("UpdateGeminiKey", mock.Anything).Return(nil).Once()

		err := km.TestKeyByID(3)
		assert.Error(t, err)

		mockHTTP.AssertExpectations(t)
		mockDB.AssertExpectations(t)
	})

	t.Run("key not in memory, not in DB", func(t *testing.T) {
		mockDB := new(MockDBService)
		mockHTTP := new(MockHTTPClient)
		km := &KeyManager{
			keys:       []*managedKey{},
			logger:     logger,
			db:         mockDB,
			httpClient: mockHTTP,
		}

		mockDB.On("GetGeminiKey", uint(99)).Return(nil, gorm.ErrRecordNotFound).Once()

		err := km.TestKeyByID(99)
		assert.Error(t, err)
		assert.Equal(t, "failed to find key with ID 99 in DB: record not found", err.Error())

		mockHTTP.AssertNotCalled(t, "Do", mock.Anything)
		mockDB.AssertExpectations(t)
	})
}
