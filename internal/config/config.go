package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// DatabaseConfig holds the database connection information.
type DatabaseConfig struct {
	Type string `yaml:"type"`
	DSN  string `yaml:"dsn"`
}

// ProxyConfig holds configuration specific to the proxy.
type ProxyConfig struct {
	DisableKeyThreshold int `yaml:"disable_key_threshold"`
}

// AdminConfig holds configuration for the admin panel.
type AdminConfig struct {
	Password string `yaml:"password"`
}

// SchedulerConfig holds configuration for the scheduler.
type SchedulerConfig struct {
	KeyRevivalInterval string `yaml:"key_revival_interval"`
}

// Config holds the configuration for the load balancer.
type Config struct {
	Database  DatabaseConfig  `yaml:"database"`
	Proxy     ProxyConfig     `yaml:"proxy"`
	Admin     AdminConfig     `yaml:"admin"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	Port      int             `yaml:"port"`
	Debug     bool            `yaml:"debug"`
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

	// Set default values
	if config.Proxy.DisableKeyThreshold == 0 {
		config.Proxy.DisableKeyThreshold = 3
		warning = "proxy.disable_key_threshold not set, using default value of 3"
	}

	if config.Database.Type == "" || config.Database.DSN == "" {
		return nil, "", fmt.Errorf("database type and dsn must be configured")
	}

	return &config, warning, nil
}
