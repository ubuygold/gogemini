package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCustomRecovery_ErrAbortHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Capture log output
	var logBuf bytes.Buffer
	testLogger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	router := gin.New()
	router.Use(customRecovery(testLogger))
	router.GET("/panic-abort", func(c *gin.Context) {
		panic(http.ErrAbortHandler)
	})

	req, _ := http.NewRequest(http.MethodGet, "/panic-abort", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// With ErrAbortHandler, the connection is considered closed.
	// The recovery middleware should catch it, log a specific message, and not write a 500.
	assert.NotEqual(t, http.StatusInternalServerError, rr.Code)

	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "Client connection aborted")
	assert.Contains(t, logOutput, `"path":"/panic-abort"`)
	assert.NotContains(t, logOutput, "Panic recovered")
}

func TestCustomRecovery_OtherPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Capture log output
	var logBuf bytes.Buffer
	testLogger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	router := gin.New()
	router.Use(customRecovery(testLogger))
	router.GET("/panic-other", func(c *gin.Context) {
		panic("some other error")
	})

	req, _ := http.NewRequest(http.MethodGet, "/panic-other", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	// For other panics, we expect a 500 Internal Server Error
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	logOutput := logBuf.String()
	assert.NotContains(t, logOutput, "Client connection aborted")
	assert.Contains(t, logOutput, "Panic recovered")
	assert.Contains(t, logOutput, `"error":"some other error"`)
}
