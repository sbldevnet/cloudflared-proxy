package logger

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestNew_DefaultFormatIsText(t *testing.T) {
	out := captureStderr(t, func() {
		log := New()
		log.Info("hello", "key", "value")
	})

	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("expected plain text output by default, got what looks like JSON: %s", out)
	}
	if !strings.Contains(out, "msg=hello") {
		t.Errorf("expected slog text handler output (msg=hello), got: %s", out)
	}
}

func TestNew_JSONFormat(t *testing.T) {
	t.Setenv("LOG_FORMAT", "json")

	out := captureStderr(t, func() {
		log := New()
		log.Info("hello", "key", "value")
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON output, got %q: %v", out, err)
	}
	if parsed["msg"] != "hello" {
		t.Errorf(`parsed["msg"] = %v, want "hello"`, parsed["msg"])
	}
}

func TestNew_InvalidLevelWarns(t *testing.T) {
	t.Setenv("LOG_LEVEL", "bogus")

	out := captureStderr(t, func() {
		New()
	})

	if !strings.Contains(out, "invalid LOG_LEVEL") || !strings.Contains(out, "bogus") || !strings.Contains(out, "falling back to info") {
		t.Errorf("expected a warning about the invalid LOG_LEVEL, got: %s", out)
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantLevel   slog.Level
		wantWarning bool
	}{
		{name: "valid level", raw: "DEBUG", wantLevel: slog.LevelDebug},
		{name: "valid level, different case", raw: "warn", wantLevel: slog.LevelWarn},
		{name: "invalid value falls back to info", raw: "bogus", wantLevel: slog.LevelInfo, wantWarning: true},
		{name: "unset falls back to info", raw: "", wantLevel: slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, warning := parseLevel(tt.raw)
			if level != tt.wantLevel {
				t.Errorf("parseLevel(%q) level = %v, want %v", tt.raw, level, tt.wantLevel)
			}
			if (warning != "") != tt.wantWarning {
				t.Errorf("parseLevel(%q) warning = %q, want warning: %v", tt.raw, warning, tt.wantWarning)
			}
		})
	}
}

// captureStderr swaps os.Stderr for a pipe for the duration of fn, restoring it afterward, and
// returns everything fn wrote. New writes to os.Stderr directly (there's no io.Writer injection
// point for a CLI-only logger), so this is the only way to observe its output.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}

	original := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = original }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}

	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return strings.Join(lines, "\n")
}
