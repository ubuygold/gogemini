package db

import (
	"gogemini/internal/config"
	"gogemini/internal/model"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// setupTestDB creates a new in-memory SQLite database and returns a Service and the raw *gorm.DB.
func setupTestDB(t *testing.T) (Service, *gorm.DB) {
	// We use a real service implementation for most tests.
	service, err := NewService(config.DatabaseConfig{
		Type: "sqlite",
		DSN:  "file::memory:",
	})
	if err != nil {
		t.Fatalf("Failed to create test db service: %v", err)
	}
	return service, service.GetDB()
}

func TestNewService(t *testing.T) {
	// Test with sqlite
	service, err := NewService(config.DatabaseConfig{Type: "sqlite", DSN: "file::memory:"})
	assert.NoError(t, err)
	assert.NotNil(t, service)

	// Test with unsupported type
	_, err = NewService(config.DatabaseConfig{Type: "unsupported"})
	assert.Error(t, err)
}

func TestIncrementAPIKeyUsageCount(t *testing.T) {
	service, db := setupTestDB(t)
	apiKey := model.APIKey{Key: "test-key", UsageCount: 0}
	db.Create(&apiKey)

	err := service.IncrementAPIKeyUsageCount("test-key")
	assert.NoError(t, err)

	var updatedAPIKey model.APIKey
	db.First(&updatedAPIKey, "key = ?", "test-key")
	assert.Equal(t, 1, updatedAPIKey.UsageCount)
}

func TestResetAllAPIKeyUsage(t *testing.T) {
	service, db := setupTestDB(t)
	db.Create(&model.APIKey{Key: "key1", UsageCount: 10})
	db.Create(&model.APIKey{Key: "key2", UsageCount: 5})
	db.Create(&model.APIKey{Key: "key3", UsageCount: 0})

	err := service.ResetAllAPIKeyUsage()
	assert.NoError(t, err)

	var keys []model.APIKey
	db.Find(&keys)

	for _, key := range keys {
		assert.Equal(t, 0, key.UsageCount)
	}
}

func TestLoadActiveGeminiKeys(t *testing.T) {
	service, db := setupTestDB(t)
	db.Create(&model.GeminiKey{Key: "key1", Status: "active"})
	db.Create(&model.GeminiKey{Key: "key2", Status: "disabled"})
	db.Create(&model.GeminiKey{Key: "key3", Status: "active", UsageCount: 1})

	keys, err := service.LoadActiveGeminiKeys()
	assert.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Equal(t, "key1", keys[0].Key)
	assert.Equal(t, "key3", keys[1].Key)
}

func TestHandleGeminiKeyFailure(t *testing.T) {
	service, db := setupTestDB(t)
	db.Create(&model.GeminiKey{Key: "key1", Status: "active", FailureCount: 0})

	disabled, err := service.HandleGeminiKeyFailure("key1", 3)
	assert.NoError(t, err)
	assert.False(t, disabled)

	var key model.GeminiKey
	db.First(&key, "key = ?", "key1")
	assert.Equal(t, 1, key.FailureCount)

	// Increment again
	disabled, err = service.HandleGeminiKeyFailure("key1", 3)
	assert.NoError(t, err)
	assert.False(t, disabled)

	// Increment to threshold
	disabled, err = service.HandleGeminiKeyFailure("key1", 3)
	assert.NoError(t, err)
	assert.True(t, disabled)

	db.First(&key, "key = ?", "key1")
	assert.Equal(t, "disabled", key.Status)

	// Test handling failure for a non-existent key
	disabled, err = service.HandleGeminiKeyFailure("non-existent-key", 3)
	assert.Error(t, err)
	assert.False(t, disabled)
	assert.Contains(t, err.Error(), "key not found during failure count update")
}

func TestResetGeminiKeyFailureCount(t *testing.T) {
	service, db := setupTestDB(t)
	db.Create(&model.GeminiKey{Key: "key1", FailureCount: 5})

	err := service.ResetGeminiKeyFailureCount("key1")
	assert.NoError(t, err)

	var key model.GeminiKey
	db.First(&key, "key = ?", "key1")
	assert.Equal(t, 0, key.FailureCount)
}

func TestIncrementGeminiKeyUsageCount(t *testing.T) {
	service, db := setupTestDB(t)
	db.Create(&model.GeminiKey{Key: "key1", UsageCount: 0})

	err := service.IncrementGeminiKeyUsageCount("key1")
	assert.NoError(t, err)

	var key model.GeminiKey
	db.First(&key, "key = ?", "key1")
	assert.Equal(t, int64(1), key.UsageCount)
}
func TestBatchAddGeminiKeys(t *testing.T) {
	service, db := setupTestDB(t)
	keys := []string{"key1", "key2", "key3"}

	err := service.BatchAddGeminiKeys(keys)
	assert.NoError(t, err)

	var geminiKeys []model.GeminiKey
	db.Find(&geminiKeys)
	assert.Len(t, geminiKeys, 3)

	// Test adding duplicate keys
	err = service.BatchAddGeminiKeys([]string{"key1", "key4"})
	assert.NoError(t, err) // Should not error, but should not insert duplicates

	db.Find(&geminiKeys)
	assert.Len(t, geminiKeys, 4)

	// Test adding empty slice
	err = service.BatchAddGeminiKeys([]string{})
	assert.NoError(t, err)
	db.Find(&geminiKeys)
	assert.Len(t, geminiKeys, 4) // Count should not change
}

func TestBatchDeleteGeminiKeys(t *testing.T) {
	service, db := setupTestDB(t)
	db.Create(&model.GeminiKey{Key: "key1"})
	db.Create(&model.GeminiKey{Key: "key2"})
	db.Create(&model.GeminiKey{Key: "key3"})

	err := service.BatchDeleteGeminiKeys([]string{"key1", "key3"})
	assert.NoError(t, err)

	var geminiKeys []model.GeminiKey
	db.Find(&geminiKeys)
	assert.Len(t, geminiKeys, 1)
	if len(geminiKeys) > 0 {
		assert.Equal(t, "key2", geminiKeys[0].Key)
	}

	// Test deleting non-existent key
	err = service.BatchDeleteGeminiKeys([]string{"key4"})
	assert.NoError(t, err)

	var remainingKeys []model.GeminiKey
	db.Find(&remainingKeys)
	assert.Len(t, remainingKeys, 1)

	// Test deleting empty slice
	err = service.BatchDeleteGeminiKeys([]string{})
	assert.NoError(t, err)
	db.Find(&remainingKeys)
	assert.Len(t, remainingKeys, 1) // Count should not change
}
