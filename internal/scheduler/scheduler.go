package scheduler

import (
	"fmt"
	"gogemini/internal/config"
	"gogemini/internal/db"
	"log"
	"net/http"
	"time"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	db     db.Service
	c      *cron.Cron
	config *config.Config
}

func NewScheduler(db db.Service, cfg *config.Config) *Scheduler {
	return &Scheduler{
		db:     db,
		c:      cron.New(),
		config: cfg,
	}
}

func (s *Scheduler) Start() {
	// Schedule daily reset of API key usage
	_, err := s.c.AddFunc("@daily", s.resetAPIKeyUsage)
	if err != nil {
		log.Fatalf("Error scheduling daily api key reset job: %v", err)
	}

	// Schedule hourly check of Gemini keys
	_, err = s.c.AddFunc("@hourly", func() { s.checkAllGeminiKeys("https://generativelanguage.googleapis.com") })
	if err != nil {
		log.Fatalf("Error scheduling hourly gemini key check job: %v", err)
	}

	s.c.Start()
}

func (s *Scheduler) resetAPIKeyUsage() {
	log.Println("Running daily job: Resetting all API key usage counts.")
	if err := s.db.ResetAllAPIKeyUsage(); err != nil {
		log.Printf("Error resetting API key usage: %v", err)
	}
}

func (s *Scheduler) checkAllGeminiKeys(baseURL string) {
	log.Println("Running hourly job: Checking all active Gemini keys for validity.")
	keys, err := s.db.LoadActiveGeminiKeys()
	if err != nil {
		log.Printf("Error loading gemini keys for check: %v", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	disableThreshold := s.config.Proxy.DisableKeyThreshold

	for _, key := range keys {
		url := fmt.Sprintf("%s/v1beta/models/gemini-pro?key=%s", baseURL, key.Key)
		resp, err := client.Get(url)
		if err != nil {
			log.Printf("Error checking key %s: %v", key.Key, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			// Key is valid, reset its failure count if it has any
			if key.FailureCount > 0 {
				log.Printf("Key %s is valid again. Resetting failure count.", key.Key)
				if err := s.db.ResetGeminiKeyFailureCount(key.Key); err != nil {
					log.Printf("Error resetting failure count for key %s: %v", key.Key, err)
				}
			}
		} else if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			// Key is invalid, increment its failure count
			log.Printf("Key %s is invalid (status %d). Incrementing failure count.", key.Key, resp.StatusCode)
			disabled, err := s.db.HandleGeminiKeyFailure(key.Key, disableThreshold)
			if err != nil {
				log.Printf("Error handling failure for key %s: %v", key.Key, err)
			}
			if disabled {
				log.Printf("Key %s has been disabled after reaching failure threshold.", key.Key)
			}
		}
	}
}

func (s *Scheduler) Stop() {
	s.c.Stop()
}
