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
var LoadConfig = func(path string) (*Config, string, error) {
	var config Config
	var warning string

	data, err := os.ReadFile(path)
	if err == nil {
		// File exists, so unmarshal it
		err = yaml.Unmarshal(data, &config)
		if err != nil {
			return nil, "", fmt.Errorf("failed to parse config file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		// An error other than "not found" occurred
		return nil, "", fmt.Errorf("failed to read config file: %w", err)
	}
	// If file does not exist, we continue with an empty config and rely on environment variables.

	// Set default values
	if config.Proxy.DisableKeyThreshold == 0 {
		config.Proxy.DisableKeyThreshold = 3
		warning = "proxy.disable_key_threshold not set, using default value of 3"
	}

	// Override with environment variables if they exist
	if dsn := os.Getenv("GOGEMINI_DATABASE_DSN"); dsn != "" {
		config.Database.DSN = dsn
	}
	if dbType := os.Getenv("GOGEMINI_DATABASE_TYPE"); dbType != "" {
		config.Database.Type = dbType
	}
	if port := os.Getenv("GOGEMINI_PORT"); port != "" {
		// Ignoring potential error for simplicity in this context
		if p, err := fmt.Sscanf(port, "%d", &config.Port); err == nil && p == 1 {
			// Value was updated
		}
	}
	if password := os.Getenv("GOGEMINI_ADMIN_PASSWORD"); password != "" {
		config.Admin.Password = password
	}
	if debug := os.Getenv("GOGEMINI_DEBUG"); debug != "" {
		config.Debug = (debug == "true")
	}

	// Final validation after overrides
	if config.Database.Type == "" || config.Database.DSN == "" {
		return nil, "", fmt.Errorf("database type and dsn must be configured in config.yaml or via environment variables")
	}

	return &config, warning, nil
}
