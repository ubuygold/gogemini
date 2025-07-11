package scheduler

import (
	"log"

	"github.com/ubuygold/gogemini/internal/config"
	"github.com/ubuygold/gogemini/internal/db"

	"github.com/robfig/cron/v3"
)

// Manager defines the interface for a key manager that the scheduler can use.
type Manager interface {
	ReviveDisabledKeys()
	CheckAllKeysHealth()
}

type Scheduler struct {
	db         db.Service
	c          *cron.Cron
	config     *config.Config
	keyManager Manager
}

func NewScheduler(db db.Service, cfg *config.Config, keyManager Manager) *Scheduler {
	return &Scheduler{
		db:         db,
		c:          cron.New(),
		config:     cfg,
		keyManager: keyManager,
	}
}

func (s *Scheduler) Start() {
	// Schedule periodic check to revive disabled Gemini keys
	revivalInterval := "@every 10m" // Default to every 10 minutes
	if s.config.Scheduler.KeyRevivalInterval != "" {
		revivalInterval = s.config.Scheduler.KeyRevivalInterval
	}
	_, err := s.c.AddFunc(revivalInterval, s.runKeyRevivalJob)
	if err != nil {
		log.Fatalf("Error scheduling gemini key revival job: %v", err)
	}

	// Schedule daily health check for all keys
	_, err = s.c.AddFunc("@daily", s.runDailyHealthCheckJob)
	if err != nil {
		log.Fatalf("Error scheduling daily health check job: %v", err)
	}

	s.c.Start()
}

func (s *Scheduler) runKeyRevivalJob() {
	log.Println("Running scheduled job: Checking for disabled keys to revive.")
	s.keyManager.ReviveDisabledKeys()
}

func (s *Scheduler) runDailyHealthCheckJob() {
	log.Println("Running daily job: Performing health check on all keys.")
	s.keyManager.CheckAllKeysHealth()
}

func (s *Scheduler) Stop() {
	s.c.Stop()
}
