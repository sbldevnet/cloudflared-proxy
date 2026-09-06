package logger

import (
	"fmt"
	"log/slog"
	"os"
)

// New returns a logger writing to stderr: human-readable text by default, or structured JSON
// when LOG_FORMAT=json. Its level is controlled by LOG_LEVEL (default info); an invalid value
// falls back to info with a warning. Each record carries a "source" attribute (file:line) of
// its call site.
func New() *slog.Logger {
	level, warning := parseLevel(os.Getenv("LOG_LEVEL"))

	opts := &slog.HandlerOptions{Level: level, AddSource: true}
	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	log := slog.New(handler)
	if warning != "" {
		log.Warn(warning)
	}
	return log
}

func parseLevel(raw string) (slog.Level, string) {
	if raw == "" {
		return slog.LevelInfo, ""
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(raw)); err != nil {
		return slog.LevelInfo, fmt.Sprintf("invalid LOG_LEVEL %q, falling back to info", raw)
	}
	return level, ""
}
