package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Config holds the configuration for the load balancer.
type Config struct {
	ClientKeys []string `yaml:"client_keys"`
	GeminiKeys []string `yaml:"gemini_keys"`
	Port       int      `yaml:"port"`
	Debug      bool     `yaml:"debug"`
}

// LoadConfig reads and parses the configuration file. It returns the config and a potential warning message.
func LoadConfig(path string) (*Config, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	var warning string
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse config file: %w", err)
	}

	if len(config.GeminiKeys) == 0 {
		return nil, "", fmt.Errorf("no gemini_keys found in config file")
	}

	if len(config.ClientKeys) == 0 {
		warning = "No client_keys configured. The proxy will not authorize any requests."
	}

	return &config, warning, nil
}
