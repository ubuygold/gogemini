package logger

import (
	"log/slog"
	"os"
)

// New creates a new slog.Logger instance.
// If debug is true, the log level is set to Debug. Otherwise, it's set to Info.
func New(debug bool) *slog.Logger {
	var level slog.Level
	if debug {
		level = slog.LevelDebug
	} else {
		level = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}
