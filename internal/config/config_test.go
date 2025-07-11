package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		content := []byte(
			"database:\n" +
				"  type: \"sqlite\"\n" +
				"  dsn: \"gogemini.db\"\n" +
				"proxy:\n" +
				"  disable_key_threshold: 5\n" +
				"port: 8080\n" +
				"debug: true\n")
		tmpfile, _ := os.CreateTemp("", "config.yaml")
		defer os.Remove(tmpfile.Name())
		tmpfile.Write(content)
		tmpfile.Close()

		config, warning, err := LoadConfig(tmpfile.Name())
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}
		if warning != "" {
			t.Errorf("Expected no warning, but got '%s'", warning)
		}
		if config.Database.Type != "sqlite" {
			t.Errorf("Expected database type sqlite, got %s", config.Database.Type)
		}
		if config.Database.DSN != "gogemini.db" {
			t.Errorf("Expected database dsn gogemini.db, got %s", config.Database.DSN)
		}
		if config.Port != 8080 {
			t.Errorf("Expected port 8080, got %d", config.Port)
		}
		if !config.Debug {
			t.Error("Expected debug to be true")
		}
		if config.Proxy.DisableKeyThreshold != 5 {
			t.Errorf("Expected disable_key_threshold to be 5, got %d", config.Proxy.DisableKeyThreshold)
		}
	})

	t.Run("non-existent file without env vars", func(t *testing.T) {
		_, _, err := LoadConfig("non-existent-file.yaml")
		if err == nil {
			t.Error("Expected an error for missing database config, but got nil")
		}
	})

	t.Run("non-existent file with env vars", func(t *testing.T) {
		os.Setenv("GOGEMINI_DATABASE_TYPE", "postgres")
		os.Setenv("GOGEMINI_DATABASE_DSN", "some-dsn")
		defer os.Unsetenv("GOGEMINI_DATABASE_TYPE")
		defer os.Unsetenv("GOGEMINI_DATABASE_DSN")

		config, _, err := LoadConfig("non-existent-file.yaml")
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}
		if config.Database.Type != "postgres" {
			t.Errorf("Expected database type postgres, got %s", config.Database.Type)
		}
		if config.Database.DSN != "some-dsn" {
			t.Errorf("Expected database dsn some-dsn, got %s", config.Database.DSN)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		tmpfile, _ := os.CreateTemp("", "config.yaml")
		defer os.Remove(tmpfile.Name())
		tmpfile.Write([]byte("port: 8080\n  debug: true\n invalid-indent: true")) // Invalid YAML
		tmpfile.Close()
		_, _, err := LoadConfig(tmpfile.Name())
		if err == nil {
			t.Error("Expected an error for invalid YAML, but got nil")
		}
	})

	t.Run("missing database config", func(t *testing.T) {
		content := []byte(
			"port: 8080\n" +
				"debug: true\n")
		tmpfile, _ := os.CreateTemp("", "config.yaml")
		defer os.Remove(tmpfile.Name())
		tmpfile.Write(content)
		tmpfile.Close()

		_, _, err := LoadConfig(tmpfile.Name())
		if err == nil {
			t.Error("Expected an error for missing database config, but got nil")
		}
	})
}
