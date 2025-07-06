package db

import (
	"fmt"
	"gogemini/internal/config"
	"gogemini/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Service defines the interface for database operations.
type Service interface {
	// Gemini Key Management
	CreateGeminiKey(key *model.GeminiKey) error
	BatchAddGeminiKeys(keys []string) error
	BatchDeleteGeminiKeys(keys []string) error
	ListGeminiKeys() ([]model.GeminiKey, error)
	GetGeminiKey(id uint) (*model.GeminiKey, error)
	UpdateGeminiKey(key *model.GeminiKey) error
	DeleteGeminiKey(id uint) error
	LoadActiveGeminiKeys() ([]model.GeminiKey, error)
	HandleGeminiKeyFailure(key string, disableThreshold int) (bool, error)
	ResetGeminiKeyFailureCount(key string) error
	IncrementGeminiKeyUsageCount(key string) error

	// Client API Key Management
	CreateAPIKey(key *model.APIKey) error
	ListAPIKeys() ([]model.APIKey, error)
	GetAPIKey(id uint) (*model.APIKey, error)
	UpdateAPIKey(key *model.APIKey) error
	DeleteAPIKey(id uint) error
	IncrementAPIKeyUsageCount(key string) error
	ResetAllAPIKeyUsage() error

	GetDB() *gorm.DB
}

// gormService is an implementation of the Service interface that uses GORM.
type gormService struct {
	db *gorm.DB
}

// NewService creates a new Service with a database connection.
func NewService(cfg config.DatabaseConfig) (Service, error) {
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

	return &gormService{db: db}, nil
}

// GetDB returns the underlying GORM DB instance.
// This is useful for components that need direct DB access, like the scheduler.
func (s *gormService) GetDB() *gorm.DB {
	return s.db
}

// LoadActiveGeminiKeys retrieves all active Gemini keys from the database.
func (s *gormService) LoadActiveGeminiKeys() ([]model.GeminiKey, error) {
	var keys []model.GeminiKey
	result := s.db.Model(&model.GeminiKey{}).
		Where("status = ?", "active").
		Order("usage_count asc").
		Find(&keys)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to load gemini keys: %w", result.Error)
	}
	return keys, nil
}

// HandleGeminiKeyFailure increments the failure count for a key and disables it if the threshold is met.
func (s *gormService) HandleGeminiKeyFailure(key string, disableThreshold int) (bool, error) {
	var disabled bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.GeminiKey{}).Where("key = ?", key).UpdateColumn("failure_count", gorm.Expr("failure_count + 1"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("key not found during failure count update: %s", key)
		}

		var geminiKey model.GeminiKey
		if err := tx.Where("key = ?", key).First(&geminiKey).Error; err != nil {
			return err
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
func (s *gormService) ResetGeminiKeyFailureCount(key string) error {
	result := s.db.Model(&model.GeminiKey{}).Where("key = ?", key).Update("failure_count", 0)
	if result.Error != nil {
		return fmt.Errorf("failed to reset failure count for key %s: %w", key, result.Error)
	}
	return nil
}

// IncrementGeminiKeyUsageCount atomically increments the usage count for a given key.
func (s *gormService) IncrementGeminiKeyUsageCount(key string) error {
	result := s.db.Model(&model.GeminiKey{}).Where("key = ?", key).UpdateColumn("usage_count", gorm.Expr("usage_count + 1"))
	if result.Error != nil {
		return fmt.Errorf("failed to increment usage count for key %s: %w", key, result.Error)
	}
	return nil
}

// BatchAddGeminiKeys adds multiple Gemini keys to the database in a single transaction.
func (s *gormService) BatchAddGeminiKeys(keys []string) error {
	if s.db.Error != nil {
		return s.db.Error
	}
	if len(keys) == 0 {
		return nil
	}

	var keyModels []model.GeminiKey
	for _, key := range keys {
		keyModels = append(keyModels, model.GeminiKey{Key: key, Status: "active"})
	}

	result := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&keyModels)
	if result.Error != nil {
		return fmt.Errorf("failed to batch add gemini keys: %w", result.Error)
	}
	return nil
}

// BatchDeleteGeminiKeys removes multiple Gemini keys from the database.
func (s *gormService) BatchDeleteGeminiKeys(keys []string) error {
	if s.db.Error != nil {
		return s.db.Error
	}
	if len(keys) == 0 {
		return nil
	}
	result := s.db.Unscoped().Where("key IN ?", keys).Delete(&model.GeminiKey{})
	if result.Error != nil {
		return fmt.Errorf("failed to batch delete gemini keys: %w", result.Error)
	}
	return nil
}

func (s *gormService) CreateGeminiKey(key *model.GeminiKey) error {
	result := s.db.Create(key)
	if result.Error != nil {
		return fmt.Errorf("failed to create gemini key: %w", result.Error)
	}
	return nil
}

func (s *gormService) ListGeminiKeys() ([]model.GeminiKey, error) {
	var keys []model.GeminiKey
	result := s.db.Find(&keys)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list gemini keys: %w", result.Error)
	}
	return keys, nil
}

func (s *gormService) GetGeminiKey(id uint) (*model.GeminiKey, error) {
	var key model.GeminiKey
	result := s.db.First(&key, id)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get gemini key %d: %w", id, result.Error)
	}
	return &key, nil
}

func (s *gormService) UpdateGeminiKey(key *model.GeminiKey) error {
	result := s.db.Save(key)
	if result.Error != nil {
		return fmt.Errorf("failed to update gemini key %d: %w", key.ID, result.Error)
	}
	return nil
}

func (s *gormService) DeleteGeminiKey(id uint) error {
	result := s.db.Unscoped().Delete(&model.GeminiKey{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete gemini key %d: %w", id, result.Error)
	}
	return nil
}

func (s *gormService) CreateAPIKey(key *model.APIKey) error {
	result := s.db.Create(key)
	if result.Error != nil {
		return fmt.Errorf("failed to create api key: %w", result.Error)
	}
	return nil
}

func (s *gormService) ListAPIKeys() ([]model.APIKey, error) {
	var keys []model.APIKey
	result := s.db.Find(&keys)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list api keys: %w", result.Error)
	}
	return keys, nil
}

func (s *gormService) GetAPIKey(id uint) (*model.APIKey, error) {
	var key model.APIKey
	result := s.db.First(&key, id)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get api key %d: %w", id, result.Error)
	}
	return &key, nil
}

func (s *gormService) UpdateAPIKey(key *model.APIKey) error {
	result := s.db.Save(key)
	if result.Error != nil {
		return fmt.Errorf("failed to update api key %d: %w", key.ID, result.Error)
	}
	return nil
}

func (s *gormService) DeleteAPIKey(id uint) error {
	result := s.db.Unscoped().Delete(&model.APIKey{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete api key %d: %w", id, result.Error)
	}
	return nil
}

// IncrementAPIKeyUsageCount atomically increments the usage count for a given API key.
func (s *gormService) IncrementAPIKeyUsageCount(key string) error {
	result := s.db.Model(&model.APIKey{}).Where("key = ?", key).UpdateColumn("usage_count", gorm.Expr("usage_count + 1"))
	if result.Error != nil {
		return fmt.Errorf("failed to increment usage count for api key %s: %w", key, result.Error)
	}
	return nil
}

// ResetAllAPIKeyUsage resets the usage count of all API keys to 0.
func (s *gormService) ResetAllAPIKeyUsage() error {
	result := s.db.Model(&model.APIKey{}).Where("usage_count > 0").Update("usage_count", 0)
	if result.Error != nil {
		return fmt.Errorf("failed to reset all api key usage: %w", result.Error)
	}
	return nil
}
