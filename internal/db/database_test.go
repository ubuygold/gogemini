package db

import (
	"gogemini/internal/config"
	"gogemini/internal/model"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setupTestDB(t *testing.T) Service {
	service, err := NewService(config.DatabaseConfig{
		Type: "sqlite",
		DSN:  "file::memory:",
	})
	assert.NoError(t, err)
	return service
}

func TestGeminiKeyCRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create
	geminiKey := &model.GeminiKey{Key: "test-key"}
	err := db.CreateGeminiKey(geminiKey)
	assert.NoError(t, err)
	assert.NotZero(t, geminiKey.ID)

	// Get
	fetchedKey, err := db.GetGeminiKey(geminiKey.ID)
	assert.NoError(t, err)
	assert.Equal(t, geminiKey.Key, fetchedKey.Key)

	// List
	keys, err := db.ListGeminiKeys()
	assert.NoError(t, err)
	assert.Len(t, keys, 1)

	// Update
	fetchedKey.Key = "updated-key"
	err = db.UpdateGeminiKey(fetchedKey)
	assert.NoError(t, err)
	updatedKey, err := db.GetGeminiKey(fetchedKey.ID)
	assert.NoError(t, err)
	assert.Equal(t, "updated-key", updatedKey.Key)

	// Delete
	err = db.DeleteGeminiKey(updatedKey.ID)
	assert.NoError(t, err)
	_, err = db.GetGeminiKey(updatedKey.ID)
	assert.Error(t, err)
}

func TestAPIKeyCRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create
	apiKey := &model.APIKey{Key: "test-api-key"}
	err := db.CreateAPIKey(apiKey)
	assert.NoError(t, err)
	assert.NotZero(t, apiKey.ID)

	// Get
	fetchedKey, err := db.GetAPIKey(apiKey.ID)
	assert.NoError(t, err)
	assert.Equal(t, apiKey.Key, fetchedKey.Key)

	// List
	keys, err := db.ListAPIKeys()
	assert.NoError(t, err)
	assert.Len(t, keys, 1)

	// Update
	fetchedKey.Key = "updated-api-key"
	err = db.UpdateAPIKey(fetchedKey)
	assert.NoError(t, err)
	updatedKey, err := db.GetAPIKey(fetchedKey.ID)
	assert.NoError(t, err)
	assert.Equal(t, "updated-api-key", updatedKey.Key)

	// Delete
	err = db.DeleteAPIKey(updatedKey.ID)
	assert.NoError(t, err)
	_, err = db.GetAPIKey(updatedKey.ID)
	assert.Error(t, err)
}

func TestNewService_UnsupportedDB(t *testing.T) {
	_, err := NewService(config.DatabaseConfig{
		Type: "unsupported",
		DSN:  "dummy",
	})
	assert.Error(t, err)
}

func TestLoadActiveGeminiKeys(t *testing.T) {
	db := setupTestDB(t)
	db.CreateGeminiKey(&model.GeminiKey{Key: "active-key", Status: "active"})
	db.CreateGeminiKey(&model.GeminiKey{Key: "inactive-key", Status: "inactive"})

	keys, err := db.LoadActiveGeminiKeys()
	assert.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, "active-key", keys[0].Key)
}

func TestHandleGeminiKeyFailure(t *testing.T) {
	db := setupTestDB(t)
	key := &model.GeminiKey{Key: "fail-key", Status: "active"}
	db.CreateGeminiKey(key)

	// First failure
	disabled, err := db.HandleGeminiKeyFailure("fail-key", 3)
	assert.NoError(t, err)
	assert.False(t, disabled)
	fetchedKey, _ := db.GetGeminiKey(key.ID)
	assert.Equal(t, 1, fetchedKey.FailureCount)

	// Second failure
	disabled, err = db.HandleGeminiKeyFailure("fail-key", 3)
	assert.NoError(t, err)
	assert.False(t, disabled)

	// Third failure should disable the key
	disabled, err = db.HandleGeminiKeyFailure("fail-key", 3)
	assert.NoError(t, err)
	assert.True(t, disabled)
	fetchedKey, _ = db.GetGeminiKey(key.ID)
	assert.Equal(t, "disabled", fetchedKey.Status)
}

func TestResetGeminiKeyFailureCount(t *testing.T) {
	db := setupTestDB(t)
	key := &model.GeminiKey{Key: "reset-key", Status: "active", FailureCount: 5}
	db.CreateGeminiKey(key)

	err := db.ResetGeminiKeyFailureCount("reset-key")
	assert.NoError(t, err)

	fetchedKey, _ := db.GetGeminiKey(key.ID)
	assert.Equal(t, 0, fetchedKey.FailureCount)
}

func TestIncrementGeminiKeyUsageCount(t *testing.T) {
	db := setupTestDB(t)
	key := &model.GeminiKey{Key: "usage-key"}
	db.CreateGeminiKey(key)

	err := db.IncrementGeminiKeyUsageCount("usage-key")
	assert.NoError(t, err)

	fetchedKey, _ := db.GetGeminiKey(key.ID)
	assert.Equal(t, int64(1), fetchedKey.UsageCount)
}

func TestBatchAddDeleteGeminiKeys(t *testing.T) {
	db := setupTestDB(t)
	keys := []string{"batch-key-1", "batch-key-2"}

	// Batch Add
	err := db.BatchAddGeminiKeys(keys)
	assert.NoError(t, err)
	allKeys, _ := db.ListGeminiKeys()
	assert.Len(t, allKeys, 2)

	// Batch Delete
	err = db.BatchDeleteGeminiKeys(keys)
	assert.NoError(t, err)
	allKeys, _ = db.ListGeminiKeys()
	assert.Len(t, allKeys, 0)
}

func TestIncrementAPIKeyUsageCount(t *testing.T) {
	db := setupTestDB(t)
	key := &model.APIKey{Key: "api-usage-key"}
	db.CreateAPIKey(key)

	err := db.IncrementAPIKeyUsageCount("api-usage-key")
	assert.NoError(t, err)

	fetchedKey, _ := db.GetAPIKey(key.ID)
	assert.Equal(t, 1, fetchedKey.UsageCount)
}

func TestResetAllAPIKeyUsage(t *testing.T) {
	db := setupTestDB(t)
	db.CreateAPIKey(&model.APIKey{Key: "reset-api-1", UsageCount: 10})
	db.CreateAPIKey(&model.APIKey{Key: "reset-api-2", UsageCount: 20})

	err := db.ResetAllAPIKeyUsage()
	assert.NoError(t, err)

	keys, _ := db.ListAPIKeys()
	for _, k := range keys {
		assert.Equal(t, 0, k.UsageCount)
	}
}
