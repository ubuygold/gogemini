package config

import (
	"os"
	"testing"
)

func TestConfigPriority(t *testing.T) {
	t.Run("env vars should override file config", func(t *testing.T) {
		// 1. Create a temporary config file
		fileContent := []byte(
			"port: 8000\n" +
				"debug: false\n" +
				"database:\n" +
				"  type: \"file-db\"\n" +
				"  dsn: \"file-dsn\"\n" +
				"admin:\n" +
				"  password: \"file-password\"\n")
		tmpfile, err := os.CreateTemp("", "config-*.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpfile.Name())
		if _, err := tmpfile.Write(fileContent); err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}
		tmpfile.Close()

		// 2. Set environment variables that should take precedence
		os.Setenv("GOGEMINI_PORT", "9000")
		os.Setenv("GOGEMINI_DEBUG", "true")
		os.Setenv("GOGEMINI_DATABASE_TYPE", "env-db")
		os.Setenv("GOGEMINI_DATABASE_DSN", "env-dsn")
		os.Setenv("GOGEMINI_ADMIN_PASSWORD", "env-password")

		// 3. Defer unsetting environment variables
		defer os.Unsetenv("GOGEMINI_PORT")
		defer os.Unsetenv("GOGEMINI_DEBUG")
		defer os.Unsetenv("GOGEMINI_DATABASE_TYPE")
		defer os.Unsetenv("GOGEMINI_DATABASE_DSN")
		defer os.Unsetenv("GOGEMINI_ADMIN_PASSWORD")

		// 4. Load the config
		config, _, err := LoadConfig(tmpfile.Name())
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}

		// 5. Assert that environment variable values were used
		if config.Port != 9000 {
			t.Errorf("Expected port from env (9000), but got %d", config.Port)
		}
		if !config.Debug {
			t.Error("Expected debug from env (true), but got false")
		}
		if config.Database.Type != "env-db" {
			t.Errorf("Expected db type from env ('env-db'), but got %s", config.Database.Type)
		}
		if config.Database.DSN != "env-dsn" {
			t.Errorf("Expected db dsn from env ('env-dsn'), but got %s", config.Database.DSN)
		}
		if config.Admin.Password != "env-password" {
			t.Errorf("Expected admin password from env ('env-password'), but got %s", config.Admin.Password)
		}
	})
}
