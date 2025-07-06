package main

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		content := []byte(`
client_keys:
  - client1
gemini_keys:
  - gemini1
port: 8080
debug: true
`)
		tmpfile, _ := os.CreateTemp("", "config.yaml")
		defer os.Remove(tmpfile.Name())
		tmpfile.Write(content)
		tmpfile.Close()

		config, err := loadConfig(tmpfile.Name())
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}
		if len(config.ClientKeys) != 1 || config.ClientKeys[0] != "client1" {
			t.Errorf("Expected [client1] ClientKeys, got %v", config.ClientKeys)
		}
		if config.Port != 8080 {
			t.Errorf("Expected port 8080, got %d", config.Port)
		}
		if !config.Debug {
			t.Error("Expected debug to be true")
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, err := loadConfig("non-existent-file.yaml")
		if err == nil {
			t.Error("Expected an error, but got nil")
		}
	})

	t.Run("missing gemini_keys", func(t *testing.T) {
		tmpfile, _ := os.CreateTemp("", "config.yaml")
		defer os.Remove(tmpfile.Name())
		tmpfile.Write([]byte(`client_keys: [c1]`))
		tmpfile.Close()
		_, err := loadConfig(tmpfile.Name())
		if err == nil {
			t.Error("Expected an error, but got nil")
		}
	})
}
