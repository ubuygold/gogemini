package scheduler

import (
	"fmt"
	"gogemini/internal/config"
	"gogemini/internal/db"
	"gogemini/internal/model"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// setupTestDB creates a new in-memory SQLite database for testing.
func setupTestDB(t *testing.T) (db.Service, *gorm.DB) {
	t.Helper()
	// Use a temporary file-based database for each test to ensure isolation.
	dbPath := fmt.Sprintf("%s/gogemini_test.db", t.TempDir())
	dsn := dbPath
	service, err := db.NewService(config.DatabaseConfig{
		Type: "sqlite",
		DSN:  dsn,
	})
	if err != nil {
		t.Fatalf("Failed to create test db service: %v", err)
	}

	// We need to migrate both schemas as the scheduler might interact with either.
	err = service.GetDB().AutoMigrate(&model.APIKey{}, &model.GeminiKey{})
	if err != nil {
		t.Fatalf("Failed to auto-migrate schema: %v", err)
	}
	return service, service.GetDB()
}

func TestScheduler_ResetUsageJob(t *testing.T) {
	dbService, gormDB := setupTestDB(t)

	// 1. Add test data
	keys := []model.APIKey{
		{Key: "test-key-1", UsageCount: 100},
		{Key: "test-key-2", UsageCount: 250},
		{Key: "test-key-3", UsageCount: 0},
	}
	err := gormDB.Create(&keys).Error
	assert.NoError(t, err)

	// 2. Create and start the scheduler
	testConfig := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}
	scheduler := NewScheduler(dbService, testConfig)
	scheduler.Start()
	// Stop the scheduler when the test finishes to clean up the cron goroutine.
	defer scheduler.Stop()

	// 3. Manually trigger the job by calling the method directly
	scheduler.resetAPIKeyUsage()

	// 4. Verify that the usage counts have been reset
	var updatedKeys []model.APIKey
	err = gormDB.Find(&updatedKeys).Error
	assert.NoError(t, err)
	assert.Len(t, updatedKeys, 3)

	for _, key := range updatedKeys {
		assert.Equal(t, 0, key.UsageCount, "Expected usage count for key %s to be reset to 0", key.Key)
	}
}

func TestNewScheduler(t *testing.T) {
	dbService, _ := setupTestDB(t)
	testConfig := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}
	scheduler := NewScheduler(dbService, testConfig)
	assert.NotNil(t, scheduler)
	assert.NotNil(t, scheduler.c)
	assert.NotNil(t, scheduler.db)
	assert.Equal(t, dbService, scheduler.db)
}

func TestScheduler_CheckKeysJob(t *testing.T) {
	dbService, gormDB := setupTestDB(t)

	// --- Mock Gemini API Server ---
	mockServer := http.NewServeMux()
	mockServer.HandleFunc("/v1beta/models/gemini-pro", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "key-good" {
			w.WriteHeader(http.StatusOK)
		} else if key == "key-bad" {
			w.WriteHeader(http.StatusForbidden)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	testServer := httptest.NewServer(mockServer)
	defer testServer.Close()

	// --- Test Data ---
	gormDB.Create(&model.GeminiKey{Key: "key-good", Status: "active", FailureCount: 1})
	gormDB.Create(&model.GeminiKey{Key: "key-bad", Status: "active", FailureCount: 2})

	// --- Scheduler Setup & Run ---
	testConfig := &config.Config{Proxy: config.ProxyConfig{DisableKeyThreshold: 3}}
	scheduler := NewScheduler(dbService, testConfig)

	// Manually run the job, passing the mock server's URL
	scheduler.checkAllGeminiKeys(testServer.URL)

	// --- Verification ---
	var goodKey model.GeminiKey
	gormDB.Where("key = ?", "key-good").First(&goodKey)
	assert.Equal(t, 0, goodKey.FailureCount, "Failure count for good key should be reset")
	assert.Equal(t, "active", goodKey.Status, "Good key should remain active")

	var badKey model.GeminiKey
	gormDB.Where("key = ?", "key-bad").First(&badKey)
	assert.Equal(t, 3, badKey.FailureCount, "Failure count for bad key should be incremented")
	assert.Equal(t, "disabled", badKey.Status, "Bad key should be disabled")
}
