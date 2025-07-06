package scheduler

import (
	"gogemini/internal/db"
	"gogemini/internal/model"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestStartScheduler(t *testing.T) {
	// Use an in-memory SQLite database for testing
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	// Auto-migrate the schema
	err = gormDB.AutoMigrate(&model.APIKey{})
	if err != nil {
		t.Fatalf("failed to auto-migrate database: %v", err)
	}

	// Add some test data
	apiKey := model.APIKey{Key: "test-key", UsageCount: 100}
	if err := gormDB.Create(&apiKey).Error; err != nil {
		t.Fatalf("failed to create api key: %v", err)
	}

	// This will start the scheduler, but we can't easily test the cron functionality itself.
	// Instead, we will directly call the function that the cron job is supposed to run.
	StartScheduler(gormDB)

	// Manually run the job that the scheduler is supposed to run.
	if err := db.ResetAllAPIKeyUsage(gormDB); err != nil {
		t.Fatalf("Error resetting API key usage: %v", err)
	}

	// Verify that the usage count has been reset
	var updatedAPIKey model.APIKey
	if err := gormDB.First(&updatedAPIKey, "key = ?", "test-key").Error; err != nil {
		t.Fatalf("failed to find api key: %v", err)
	}

	if updatedAPIKey.UsageCount != 0 {
		t.Errorf("Expected usage count to be 0, but got %d", updatedAPIKey.UsageCount)
	}
}
