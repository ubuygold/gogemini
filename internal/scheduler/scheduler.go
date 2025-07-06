package scheduler

import (
	"gogemini/internal/db"
	"log"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	db db.Service
	c  *cron.Cron
}

func NewScheduler(db db.Service) *Scheduler {
	return &Scheduler{
		db: db,
		c:  cron.New(),
	}
}

func (s *Scheduler) Start() {
	_, err := s.c.AddFunc("@daily", func() {
		log.Println("Running daily job: Resetting all API key usage counts.")
		if err := s.db.ResetAllAPIKeyUsage(); err != nil {
			log.Printf("Error resetting API key usage: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("Error scheduling daily job: %v", err)
	}
	s.c.Start()
}

func (s *Scheduler) Stop() {
	s.c.Stop()
}
