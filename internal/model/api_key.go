package model

import (
	"time"

	"gorm.io/gorm"
)

// APIKey represents a client's API key for accessing the service.
type APIKey struct {
	gorm.Model
	Key         string    `gorm:"type:varchar(255);uniqueIndex;not null"`
	UsageCount  int       `gorm:"default:0;not null"`
	Status      string    `gorm:"type:varchar(50);default:'active';not null"`
	Permissions string    `gorm:"type:varchar(255);not null"`
	RateLimit   int       `gorm:"default:0"`
	ExpiresAt   time.Time `gorm:"default:null"`
}
