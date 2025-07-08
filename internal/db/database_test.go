package db

import (
	"testing"

	"github.com/ubuygold/gogemini/internal/config"
	"github.com/ubuygold/gogemini/internal/model"

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
	t.Run("ListGeminiKeys", func(t *testing.T) {
		db.CreateGeminiKey(&model.GeminiKey{Key: "disabled-key", Status: "disabled", FailureCount: 5})
		// Test no filters
		keys, total, err := db.ListGeminiKeys(1, 10, "all", 0)
		assert.NoError(t, err)
		assert.Len(t, keys, 2)
		assert.Equal(t, int64(2), total)

		// Test status filter
		keys, total, err = db.ListGeminiKeys(1, 10, "disabled", 0)
		assert.NoError(t, err)
		assert.Len(t, keys, 1)
		assert.Equal(t, int64(1), total)
		assert.Equal(t, "disabled-key", keys[0].Key)

		// Test failure count filter
		keys, total, err = db.ListGeminiKeys(1, 10, "all", 3)
		assert.NoError(t, err)
		assert.Len(t, keys, 1)
		assert.Equal(t, int64(1), total)
		assert.Equal(t, "disabled-key", keys[0].Key)
	})

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

	// Test failure for a non-existent key
	disabled, err = db.HandleGeminiKeyFailure("non-existent-key", 3)
	assert.Error(t, err)
	assert.False(t, disabled)
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
	allKeys, total, _ := db.ListGeminiKeys(1, 10, "all", 0)
	assert.Len(t, allKeys, 2)
	assert.Equal(t, int64(2), total)

	// Test adding empty slice
	err = db.BatchAddGeminiKeys([]string{})
	assert.NoError(t, err)

	// Batch Delete
	var idsToDelete []uint
	for _, k := range allKeys {
		idsToDelete = append(idsToDelete, k.ID)
	}
	err = db.BatchDeleteGeminiKeys(idsToDelete)
	assert.NoError(t, err)
	allKeys, total, _ = db.ListGeminiKeys(1, 10, "all", 0)
	assert.Len(t, allKeys, 0)
	assert.Equal(t, int64(0), total)

	// Test deleting empty slice
	err = db.BatchDeleteGeminiKeys([]uint{})
	assert.NoError(t, err)
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

func TestUpdateGeminiKeyStatus(t *testing.T) {
	db := setupTestDB(t)
	key := &model.GeminiKey{Key: "status-key", Status: "active"}
	db.CreateGeminiKey(key)

	err := db.UpdateGeminiKeyStatus("status-key", "disabled")
	assert.NoError(t, err)

	fetchedKey, _ := db.GetGeminiKey(key.ID)
	assert.Equal(t, "disabled", fetchedKey.Status)
}

func TestFindAPIKeyByKey(t *testing.T) {
	db := setupTestDB(t)
	key := &model.APIKey{Key: "find-me"}
	db.CreateAPIKey(key)

	foundKey, err := db.FindAPIKeyByKey("find-me")
	assert.NoError(t, err)
	assert.Equal(t, key.ID, foundKey.ID)

	_, err = db.FindAPIKeyByKey("not-found")
	assert.Error(t, err)
	assert.Equal(t, ErrAPIKeyNotFound, err)
}

func TestNewService_InvalidDSN(t *testing.T) {
	_, err := NewService(config.DatabaseConfig{
		Type: "sqlite",
		DSN:  "/invalid/path/to/db",
	})
	assert.Error(t, err)
}

func TestUpdateGeminiKeyStatus_NotFound(t *testing.T) {
	db := setupTestDB(t)
	err := db.UpdateGeminiKeyStatus("non-existent-key", "disabled")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key not found for status update")
}

func TestGetAPIKey_NotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := db.GetAPIKey(9999)
	assert.Error(t, err)
	assert.Equal(t, ErrAPIKeyNotFound, err)
}

func TestGetGeminiKey_NotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := db.GetGeminiKey(9999)
	assert.Error(t, err)
	assert.Equal(t, ErrGeminiKeyNotFound, err)
}

func TestBatchAddGeminiKeys_Conflict(t *testing.T) {
	db := setupTestDB(t)
	keys := []string{"conflict-key", "conflict-key"}

	err := db.BatchAddGeminiKeys(keys)
	assert.NoError(t, err)

	allKeys, total, _ := db.ListGeminiKeys(1, 10, "all", 0)
	assert.Len(t, allKeys, 1)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "conflict-key", allKeys[0].Key)
}

func TestListGeminiKeys_EmptyFilter(t *testing.T) {
	db := setupTestDB(t)
	db.CreateGeminiKey(&model.GeminiKey{Key: "key-1", Status: "active"})
	db.CreateGeminiKey(&model.GeminiKey{Key: "key-2", Status: "disabled"})

	keys, total, err := db.ListGeminiKeys(1, 10, "", 0)
	assert.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Equal(t, int64(2), total)
}
