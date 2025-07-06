package scheduler

import (
	"fmt"
	"gogemini/internal/config"
	"gogemini/internal/db"
	"gogemini/internal/model"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// setupTestDB creates a new in-memory SQLite database for testing.
func setupTestDB(t *testing.T) (db.Service, *gorm.DB) {
	t.Helper()
	// Use the test name to ensure a unique database for each test, preventing interference.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
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
	scheduler := NewScheduler(dbService)
	scheduler.Start()
	// Stop the scheduler when the test finishes to clean up the cron goroutine.
	defer scheduler.Stop()

	// 3. Manually trigger the job
	// We expect exactly one job to be scheduled.
	assert.Len(t, scheduler.c.Entries(), 1, "Expected one cron job to be scheduled")
	jobEntry := scheduler.c.Entries()[0]
	jobEntry.Job.Run() // Run the job immediately

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
	scheduler := NewScheduler(dbService)
	assert.NotNil(t, scheduler)
	assert.NotNil(t, scheduler.c)
	assert.NotNil(t, scheduler.db)
	assert.Equal(t, dbService, scheduler.db)
}
