package model

import "gorm.io/gorm"

// GeminiKey represents a Google Gemini API key stored in the database.
type GeminiKey struct {
	gorm.Model
	Key          string `gorm:"type:varchar(255);uniqueIndex;not null"`
	Status       string `gorm:"type:varchar(50);default:'active';not null"`
	FailureCount int    `gorm:"default:0;not null"`
	UsageCount   int64  `gorm:"default:0;not null"`
}
