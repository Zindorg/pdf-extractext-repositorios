package logging

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},
		{"invalid", slog.LevelInfo},
		{"  info  ", slog.LevelInfo},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := ParseLevel(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestNew_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := New(slog.LevelInfo, &buf)

	logger.Info("test message", "key", "value")

	output := buf.String()
	require.Contains(t, output, `"level":"INFO"`)
	require.Contains(t, output, `"msg":"test message"`)
	require.Contains(t, output, `"key":"value"`)
	require.Contains(t, output, `"time"`)
}

func TestNew_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := New(slog.LevelWarn, &buf)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	require.NotContains(t, output, "debug message")
	require.NotContains(t, output, "info message")
	require.Contains(t, output, "warn message")
	require.Contains(t, output, "error message")
}

func TestSetup_GlobalDefault(t *testing.T) {
	originalDefault := slog.Default()
	defer slog.SetDefault(originalDefault)

	logger := Setup(slog.LevelInfo)
	require.Equal(t, logger, slog.Default())
}
