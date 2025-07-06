package scheduler

import (
	"gogemini/internal/db"
	"log"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// StartScheduler initializes and starts the cron scheduler.
func StartScheduler(gormDB *gorm.DB) {
	c := cron.New()
	_, err := c.AddFunc("@daily", func() {
		log.Println("Running daily job: Resetting all API key usage counts.")
		if err := db.ResetAllAPIKeyUsage(gormDB); err != nil {
			log.Printf("Error resetting API key usage: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("Error scheduling daily job: %v", err)
	}
	c.Start()
}
