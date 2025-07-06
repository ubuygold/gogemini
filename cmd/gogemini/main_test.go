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

func TestCustomRecovery_Panic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logBuf bytes.Buffer
	testLogger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	router := gin.New()
	router.Use(customRecovery(testLogger))
	router.GET("/", func(c *gin.Context) {
		panic("test panic")
	})

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, logBuf.String(), "Panic recovered")
	assert.Contains(t, logBuf.String(), "test panic")
}

func TestCustomRecovery_AbortHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logBuf bytes.Buffer
	testLogger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	router := gin.New()
	router.Use(customRecovery(testLogger))
	router.GET("/", func(c *gin.Context) {
		panic(http.ErrAbortHandler)
	})

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	// Use a custom response writer that can be closed to simulate the client disconnecting
	rr := httptest.NewRecorder()
	closedChan := make(chan bool, 1)
	closeNotifier := &closeNotifier{rr, closedChan}

	// Simulate client disconnecting
	go func() {
		// This is a bit of a hack to make the test work.
		// In a real scenario, the http server would handle this.
		// We can't easily simulate the client closing the connection here,
		// so we just panic with ErrAbortHandler.
	}()

	router.ServeHTTP(closeNotifier, req)

	// The status code is not set when aborting, so we check the log
	assert.Contains(t, logBuf.String(), "Client connection aborted")
}

// closeNotifier is a custom ResponseWriter that implements http.CloseNotifier
type closeNotifier struct {
	*httptest.ResponseRecorder
	closed chan bool
}

func (cn *closeNotifier) CloseNotify() <-chan bool {
	return cn.closed
}

func (cn *closeNotifier) Write(b []byte) (int, error) {
	// Simulate the connection being closed by the client
	if cn.closed != nil {
		cn.closed <- true
		close(cn.closed)
		cn.closed = nil
	}
	return cn.ResponseRecorder.Write(b)
}

func (cn *closeNotifier) WriteHeader(statusCode int) {
	cn.ResponseRecorder.WriteHeader(statusCode)
}

func (cn *closeNotifier) Header() http.Header {
	return cn.ResponseRecorder.Header()
}

func (cn *closeNotifier) Body() *bytes.Buffer {
	return cn.ResponseRecorder.Body
}

func (cn *closeNotifier) Flush() {
	// no-op
}
