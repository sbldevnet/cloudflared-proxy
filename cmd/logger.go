package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// newLogger returns a logger that writes to stderr. LOG_LEVEL selects the level
// (debug, info, warn or error; default info) and LOG_FORMAT=json switches from
// text to JSON output. An invalid LOG_LEVEL falls back to info with a warning.
func newLogger() *slog.Logger {
	level := slog.LevelInfo
	var levelErr error
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		if levelErr = level.UnmarshalText([]byte(v)); levelErr != nil {
			level = slog.LevelInfo
		}
	}

	opts := &slog.HandlerOptions{AddSource: true, Level: level}

	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		opts.ReplaceAttr = shortSource
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	log := slog.New(handler)
	if levelErr != nil {
		log.Warn("invalid LOG_LEVEL, falling back to info", "value", os.Getenv("LOG_LEVEL"))
	}
	return log
}

func shortSource(_ []string, a slog.Attr) slog.Attr {
	if src, ok := a.Value.Any().(*slog.Source); ok && a.Key == slog.SourceKey {
		fn := src.Function
		if i := strings.LastIndex(fn, "/"); i >= 0 {
			fn = fn[i+1:]
		}
		a.Value = slog.StringValue(fn + " " + filepath.Base(src.File) + ":" + strconv.Itoa(src.Line))
	}
	return a
}
