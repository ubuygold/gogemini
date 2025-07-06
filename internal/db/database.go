package db

import (
	"fmt"
	"gogemini/internal/config"
	"gogemini/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Init initializes the database connection based on the provided configuration.
func Init(cfg config.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.Type {
	case "sqlite":
		dialector = sqlite.Open(cfg.DSN)
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto-migrate the schema
	err = db.AutoMigrate(&model.APIKey{}, &model.GeminiKey{})
	if err != nil {
		return nil, fmt.Errorf("failed to auto-migrate database: %w", err)
	}

	return db, nil
}

// LoadActiveGeminiKeys retrieves all active Gemini keys from the database,
// ordered by their usage count in ascending order.
func LoadActiveGeminiKeys(db *gorm.DB) ([]model.GeminiKey, error) {
	var keys []model.GeminiKey
	result := db.Model(&model.GeminiKey{}).
		Where("status = ?", "active").
		Order("usage_count asc").
		Find(&keys)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to load gemini keys: %w", result.Error)
	}
	return keys, nil
}

// HandleGeminiKeyFailure increments the failure count for a key and disables it if the threshold is met.
// It returns true if the key was disabled, false otherwise.
func HandleGeminiKeyFailure(db *gorm.DB, key string, disableThreshold int) (bool, error) {
	var disabled bool
	err := db.Transaction(func(tx *gorm.DB) error {
		// Atomically increment the failure count
		result := tx.Model(&model.GeminiKey{}).Where("key = ?", key).UpdateColumn("failure_count", gorm.Expr("failure_count + 1"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("key not found during failure count update: %s", key)
		}

		// Check if the key needs to be disabled
		var geminiKey model.GeminiKey
		if err := tx.Where("key = ?", key).First(&geminiKey).Error; err != nil {
			return err // Should not happen if the update succeeded
		}

		if geminiKey.FailureCount >= disableThreshold && geminiKey.Status == "active" {
			if err := tx.Model(&geminiKey).Update("status", "disabled").Error; err != nil {
				return err
			}
			disabled = true
		}

		return nil
	})

	return disabled, err
}

// ResetGeminiKeyFailureCount resets the failure count for a given key.
func ResetGeminiKeyFailureCount(db *gorm.DB, key string) error {
	result := db.Model(&model.GeminiKey{}).Where("key = ?", key).Update("failure_count", 0)
	if result.Error != nil {
		return fmt.Errorf("failed to reset failure count for key %s: %w", key, result.Error)
	}
	// It's okay if RowsAffected is 0, the key might have been removed.
	return nil
}

// IncrementGeminiKeyUsageCount atomically increments the usage count for a given key.
func IncrementGeminiKeyUsageCount(db *gorm.DB, key string) error {
	result := db.Model(&model.GeminiKey{}).Where("key = ?", key).UpdateColumn("usage_count", gorm.Expr("usage_count + 1"))
	if result.Error != nil {
		return fmt.Errorf("failed to increment usage count for key %s: %w", key, result.Error)
	}
	// It's okay if RowsAffected is 0, the key might have been removed or disabled in the meantime.
	return nil
}

// IncrementAPIKeyUsageCount atomically increments the usage count for a given API key.
func IncrementAPIKeyUsageCount(db *gorm.DB, key string) error {
	result := db.Model(&model.APIKey{}).Where("key = ?", key).UpdateColumn("usage_count", gorm.Expr("usage_count + 1"))
	if result.Error != nil {
		return fmt.Errorf("failed to increment usage count for api key %s: %w", key, result.Error)
	}
	return nil
}

// ResetAllAPIKeyUsage resets the usage count of all API keys to 0.
func ResetAllAPIKeyUsage(db *gorm.DB) error {
	result := db.Model(&model.APIKey{}).Where("usage_count > 0").Update("usage_count", 0)
	if result.Error != nil {
		return fmt.Errorf("failed to reset all api key usage: %w", result.Error)
	}
	return nil
}
