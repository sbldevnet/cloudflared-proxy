package cmd

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = orig })

	fn()

	require.NoError(t, w.Close())
	os.Stderr = orig
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func TestNewLoggerLevels(t *testing.T) {
	tests := []struct {
		level   string
		enabled slog.Level
		warns   bool
	}{
		{"", slog.LevelInfo, false},
		{"debug", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"trace", slog.LevelInfo, true},
		{"bogus", slog.LevelInfo, true},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", tt.level)
			t.Setenv("LOG_FORMAT", "")

			var log *slog.Logger
			out := captureStderr(t, func() { log = newLogger() })

			assert.True(t, log.Enabled(t.Context(), tt.enabled))
			assert.False(t, log.Enabled(t.Context(), tt.enabled-1))
			if tt.warns {
				assert.Contains(t, out, "invalid LOG_LEVEL, falling back to info")
				assert.Contains(t, out, tt.level)
			} else {
				assert.Empty(t, out)
			}
		})
	}
}

func TestNewLoggerTextOutput(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "")

	out := captureStderr(t, func() {
		newLogger().Info("hello", "key", "value")
	})

	assert.Contains(t, out, "level=INFO")
	assert.Contains(t, out, "msg=hello")
	assert.Contains(t, out, "key=value")
	assert.Contains(t, out, `source="cmd.TestNewLoggerTextOutput.func1 logger_test.go:`)
}

func TestNewLoggerJSONOutput(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "json")

	out := captureStderr(t, func() {
		newLogger().Info("hello", "key", "value")
	})

	var rec struct {
		Level  string `json:"level"`
		Msg    string `json:"msg"`
		Key    string `json:"key"`
		Source struct {
			Function string `json:"function"`
			File     string `json:"file"`
			Line     int    `json:"line"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &rec))
	assert.Equal(t, "INFO", rec.Level)
	assert.Equal(t, "hello", rec.Msg)
	assert.Equal(t, "value", rec.Key)
	assert.Contains(t, rec.Source.Function, "TestNewLoggerJSONOutput")
	assert.Contains(t, rec.Source.File, "logger_test.go")
	assert.NotZero(t, rec.Source.Line)
}
