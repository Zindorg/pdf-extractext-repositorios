package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

const (
	// DefaultLevel is the default log level
	DefaultLevel = slog.LevelInfo
)

// ParseLevel parses a string into a slog.Level.
// Accepts: debug, info, warn, error (case-insensitive).
// Returns DefaultLevel if the input is empty or invalid.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return DefaultLevel
	}
}

// New creates a new slog.Logger with JSON handler writing to w.
// The level controls the minimum level to log.
func New(level slog.Level, w io.Writer) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(handler)
}

// Setup configures the global slog default logger with JSON output to stdout.
// Returns the created logger.
func Setup(level slog.Level) *slog.Logger {
	logger := New(level, os.Stdout)
	slog.SetDefault(logger)
	return logger
}
