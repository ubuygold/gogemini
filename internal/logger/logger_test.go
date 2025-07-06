package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	// Test with debug true
	logger := New(true)
	if logger == nil {
		t.Fatal("Expected logger to not be nil")
	}

	// Test with debug false
	logger = New(false)
	if logger == nil {
		t.Fatal("Expected logger to not be nil")
	}
}

func TestNew_Debug(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, true)
	logger.Debug("test debug message")

	if !strings.Contains(buf.String(), "test debug message") {
		t.Errorf("Expected log output to contain 'test debug message', but it didn't")
	}
}

func TestNew_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, false)
	logger.Info("test info message")

	if !strings.Contains(buf.String(), "test info message") {
		t.Errorf("Expected log output to contain 'test info message', but it didn't")
	}
}

func TestNew_Info_With_Debug_False(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, false)
	logger.Debug("test debug message")

	if strings.Contains(buf.String(), "test debug message") {
		t.Errorf("Expected log output to not contain 'test debug message', but it did")
	}
}
