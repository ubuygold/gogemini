package main

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	content := []byte(`
client_keys:
  - client1
gemini_keys:
  - gemini1
  - gemini2
port: 9999
debug: false
`)
	tmpfile, err := os.CreateTemp("", "config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Test loading the config
	config, err := loadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("Expected no error, but got %v", err)
	}
	if len(config.ClientKeys) != 1 || config.ClientKeys[0] != "client1" {
		t.Errorf("Expected [client1] ClientKeys, but got %v", config.ClientKeys)
	}
	if len(config.GeminiKeys) != 2 {
		t.Errorf("Expected 2 Gemini keys, but got %d", len(config.GeminiKeys))
	}
	if config.Port != 9999 {
		t.Errorf("Expected port 9999, but got %d", config.Port)
	}
	if config.Debug != false {
		t.Errorf("Expected debug to be false, but got %v", config.Debug)
	}

	// Test error on non-existent file
	_, err = loadConfig("non-existent-file.yaml")
	if err == nil {
		t.Error("Expected an error for non-existent file, but got nil")
	}

	// Test error on invalid yaml
	_, err = loadConfig("/dev/null") // Invalid yaml content
	if err == nil {
		t.Error("Expected an error for invalid yaml, but got nil")
	}

	// Test error on missing gemini keys
	content = []byte(`client_keys: [c1]`)
	tmpfileNoGemini, _ := os.CreateTemp("", "config.yaml")
	defer os.Remove(tmpfileNoGemini.Name())
	tmpfileNoGemini.Write(content)
	tmpfileNoGemini.Close()
	_, err = loadConfig(tmpfileNoGemini.Name())
	if err == nil {
		t.Error("Expected an error for missing gemini_keys, but got nil")
	}
}
