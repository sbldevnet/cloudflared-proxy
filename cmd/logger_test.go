package cmd

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
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
			out := captureStderr(t, func() { log = logFromEnv() })

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
		logFromEnv().Info("hello", "key", "value")
	})

	assert.Contains(t, out, "level=INFO")
	assert.Contains(t, out, "msg=hello")
	assert.Contains(t, out, "key=value")
	assert.Contains(t, out, `source="cmd.TestNewLoggerTextOutput.func1 logger_test.go:`)
}

func TestNewLoggerJSONOutput(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "json")

	out := captureStderr(t, func() {
		logFromEnv().Info("hello", "key", "value")
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

func TestNewLoggerSourceOnlyAtDebug(t *testing.T) {
	tests := []struct {
		name   string
		format string
		level  string
		source bool
	}{
		{"text default level", "", "", false},
		{"text debug", "", "debug", true},
		{"json default level", "json", "", false},
		{"json debug", "json", "debug", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", tt.level)
			t.Setenv("LOG_FORMAT", tt.format)

			out := captureStderr(t, func() {
				logFromEnv().Info("hello")
			})

			if tt.format == "json" {
				assert.Equal(t, tt.source, strings.Contains(out, `"source":`))
			} else {
				assert.Equal(t, tt.source, strings.Contains(out, "source="))
			}
		})
	}
}

func logFromEnv() *slog.Logger {
	o, _ := resolveLogOptions("", "", false, false)
	return o.logger()
}

// resolveWithConfig runs the full resolution: flag, environment, then the config file.
func resolveWithConfig(t *testing.T, flag, env, cfg string, key, envName string) (logOptions, error) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv(envName, env)
	if cfg != "" {
		viper.Set(key, cfg)
	}
	var o logOptions
	var err error
	if key == "logLevel" {
		o, err = resolveLogOptions(flag, "", flag != "", false)
	} else {
		o, err = resolveLogOptions("", flag, false, flag != "")
	}
	if err != nil {
		return o, err
	}
	o, _, err = o.withConfig()
	return o, err
}

func TestLogLevelPrecedence(t *testing.T) {
	tests := []struct {
		name            string
		flag, env, file string
		want            slog.Level
	}{
		{"default", "", "", "", slog.LevelInfo},
		{"file only", "", "", "debug", slog.LevelDebug},
		{"env over file", "", "warn", "debug", slog.LevelWarn},
		{"flag over file", "error", "", "debug", slog.LevelError},
		{"flag over env", "error", "warn", "", slog.LevelError},
		{"flag over env and file", "debug", "warn", "error", slog.LevelDebug},
		{"invalid env still beats file", "", "bogus", "debug", slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := resolveWithConfig(t, tt.flag, tt.env, tt.file, "logLevel", "LOG_LEVEL")
			require.NoError(t, err)
			assert.Equal(t, tt.want, o.level)
		})
	}
}

func TestLogFormatPrecedence(t *testing.T) {
	tests := []struct {
		name            string
		flag, env, file string
		wantJSON        bool
	}{
		{"default", "", "", "", false},
		{"file only", "", "", "json", true},
		{"env over file", "", "text", "json", false},
		{"flag over file", "text", "", "json", false},
		{"flag over env", "json", "text", "", true},
		{"flag over env and file", "json", "text", "text", true},
		{"unknown env is text and beats file", "", "xml", "json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := resolveWithConfig(t, tt.flag, tt.env, tt.file, "logFormat", "LOG_FORMAT")
			require.NoError(t, err)
			assert.Equal(t, tt.wantJSON, o.json)
		})
	}
}

func TestInvalidFlagAndConfigValues(t *testing.T) {
	_, err := resolveWithConfig(t, "trace", "", "", "logLevel", "LOG_LEVEL")
	require.ErrorContains(t, err, "--log-level")
	assert.ErrorContains(t, err, "debug, info, warn, error")

	_, err = resolveWithConfig(t, "xml", "", "", "logFormat", "LOG_FORMAT")
	require.ErrorContains(t, err, "--log-format")
	assert.ErrorContains(t, err, "text, json")

	_, err = resolveWithConfig(t, "", "", "trace", "logLevel", "LOG_LEVEL")
	require.ErrorContains(t, err, "logLevel")
	assert.ErrorContains(t, err, "debug, info, warn, error")

	_, err = resolveWithConfig(t, "", "", "xml", "logFormat", "LOG_FORMAT")
	require.ErrorContains(t, err, "logFormat")
	assert.ErrorContains(t, err, "text, json")

	// A higher source hides an invalid config value.
	_, err = resolveWithConfig(t, "debug", "", "trace", "logLevel", "LOG_LEVEL")
	assert.NoError(t, err)
}

func TestInvalidEnvLevelWarns(t *testing.T) {
	t.Setenv("LOG_LEVEL", "bogus")
	t.Setenv("LOG_FORMAT", "")
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("logFormat", "json")

	o, err := resolveLogOptions("", "", false, false)
	require.NoError(t, err)
	out := captureStderr(t, func() { o.logger() })
	assert.Contains(t, out, "invalid LOG_LEVEL, falling back to info")
}

func TestConfigFileLogSettingsApplyAfterLoading(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")
	viper.Reset()
	t.Cleanup(viper.Reset)
	file := filepath.Join(t.TempDir(), "cfg.yaml")
	require.NoError(t, os.WriteFile(file, []byte("logLevel: debug\nlogFormat: json\nproxies: []\n"), 0o600))

	o, err := resolveLogOptions("", "", false, false)
	require.NoError(t, err)
	assert.Equal(t, slog.LevelInfo, o.level)

	require.NoError(t, initConfig(slog.New(&recordingHandler{}), file))
	o, changed, err := o.withConfig()
	require.NoError(t, err)
	assert.True(t, changed)

	out := captureStderr(t, func() { o.logger().Debug("hello") })
	assert.Contains(t, out, `"level":"DEBUG"`)
}

func TestRunRejectsInvalidLogFlags(t *testing.T) {
	for _, args := range [][]string{{"--log-level", "trace"}, {"--log-format", "xml"}} {
		cmd := Run()
		cmd.SetArgs(append(args, "-e", "example.com"))
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		require.ErrorContains(t, cmd.Execute(), "allowed values are")
	}
}
