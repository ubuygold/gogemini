package logger

import (
	"io"
	"log/slog"
	"os"
)

// New creates a new slog.Logger instance that writes to os.Stdout.
// If debug is true, the log level is set to Debug. Otherwise, it's set to Info.
func New(debug bool) *slog.Logger {
	return NewWithWriter(os.Stdout, debug)
}

// NewWithWriter creates a new slog.Logger instance with a specific writer.
func NewWithWriter(w io.Writer, debug bool) *slog.Logger {
	var level slog.Level
	if debug {
		level = slog.LevelDebug
	} else {
		level = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	}))
}
