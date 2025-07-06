package main

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"log"
	"os"
)

// Config holds the configuration for the load balancer.
type Config struct {
	ClientKeys []string `yaml:"client_keys"`
	GeminiKeys []string `yaml:"gemini_keys"`
	Port       int      `yaml:"port"`
	Debug      bool     `yaml:"debug"`
}

// loadConfig reads and parses the configuration file.
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if len(config.GeminiKeys) == 0 {
		return nil, fmt.Errorf("no gemini_keys found in config file")
	}

	if len(config.ClientKeys) == 0 {
		log.Println("Warning: No client_keys configured. The proxy will not authorize any requests.")
	}

	return &config, nil
}
