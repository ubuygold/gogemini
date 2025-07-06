package db

import (
	"gogemini/internal/config"
	"gogemini/internal/model"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	err = db.AutoMigrate(&model.APIKey{}, &model.GeminiKey{})
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}
	return db
}

func TestInit(t *testing.T) {
	// Test with sqlite
	db, err := Init(config.DatabaseConfig{Type: "sqlite", DSN: "file::memory:?cache=shared"})
	assert.NoError(t, err)
	assert.NotNil(t, db)

	// Test with unsupported type
	_, err = Init(config.DatabaseConfig{Type: "unsupported"})
	assert.Error(t, err)
}

func TestIncrementAPIKeyUsageCount(t *testing.T) {
	db := setupTestDB(t)
	apiKey := model.APIKey{Key: "test-key", UsageCount: 0}
	db.Create(&apiKey)

	err := IncrementAPIKeyUsageCount(db, "test-key")
	assert.NoError(t, err)

	var updatedAPIKey model.APIKey
	db.First(&updatedAPIKey, "key = ?", "test-key")
	assert.Equal(t, 1, updatedAPIKey.UsageCount)
}

func TestResetAllAPIKeyUsage(t *testing.T) {
	db := setupTestDB(t)
	db.Create(&model.APIKey{Key: "key1", UsageCount: 10})
	db.Create(&model.APIKey{Key: "key2", UsageCount: 5})
	db.Create(&model.APIKey{Key: "key3", UsageCount: 0})

	err := ResetAllAPIKeyUsage(db)
	assert.NoError(t, err)

	var keys []model.APIKey
	db.Find(&keys)

	for _, key := range keys {
		assert.Equal(t, 0, key.UsageCount)
	}
}

func TestLoadActiveGeminiKeys(t *testing.T) {
	db := setupTestDB(t)
	db.Create(&model.GeminiKey{Key: "key1", Status: "active"})
	db.Create(&model.GeminiKey{Key: "key2", Status: "disabled"})
	db.Create(&model.GeminiKey{Key: "key3", Status: "active", UsageCount: 1})

	keys, err := LoadActiveGeminiKeys(db)
	assert.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Equal(t, "key1", keys[0].Key)
	assert.Equal(t, "key3", keys[1].Key)
}

func TestHandleGeminiKeyFailure(t *testing.T) {
	db := setupTestDB(t)
	db.Create(&model.GeminiKey{Key: "key1", Status: "active", FailureCount: 0})

	disabled, err := HandleGeminiKeyFailure(db, "key1", 3)
	assert.NoError(t, err)
	assert.False(t, disabled)

	var key model.GeminiKey
	db.First(&key, "key = ?", "key1")
	assert.Equal(t, 1, key.FailureCount)

	// Increment again
	disabled, err = HandleGeminiKeyFailure(db, "key1", 3)
	assert.NoError(t, err)
	assert.False(t, disabled)

	// Increment to threshold
	disabled, err = HandleGeminiKeyFailure(db, "key1", 3)
	assert.NoError(t, err)
	assert.True(t, disabled)

	db.First(&key, "key = ?", "key1")
	assert.Equal(t, "disabled", key.Status)
}

func TestResetGeminiKeyFailureCount(t *testing.T) {
	db := setupTestDB(t)
	db.Create(&model.GeminiKey{Key: "key1", FailureCount: 5})

	err := ResetGeminiKeyFailureCount(db, "key1")
	assert.NoError(t, err)

	var key model.GeminiKey
	db.First(&key, "key = ?", "key1")
	assert.Equal(t, 0, key.FailureCount)
}

func TestIncrementGeminiKeyUsageCount(t *testing.T) {
	db := setupTestDB(t)
	db.Create(&model.GeminiKey{Key: "key1", UsageCount: 0})

	err := IncrementGeminiKeyUsageCount(db, "key1")
	assert.NoError(t, err)

	var key model.GeminiKey
	db.First(&key, "key = ?", "key1")
	assert.Equal(t, int64(1), key.UsageCount)
}
