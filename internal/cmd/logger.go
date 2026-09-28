package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

var (
	logLevels  = []string{"debug", "info", "warn", "error"}
	logFormats = []string{"text", "json"}
)

// logOptions holds the resolved logger settings and whether a source with
// higher precedence than the config file (flag or environment variable)
// already provided each one.
type logOptions struct {
	level     slog.Level
	json      bool
	levelSet  bool
	formatSet bool

	// invalidEnvLevel is the rejected LOG_LEVEL value, reported once by the
	// first logger built from these options.
	invalidEnvLevel string
}

// resolveLogOptions applies the flag, then the LOG_LEVEL and LOG_FORMAT
// environment variables. Flag values are validated strictly; an invalid
// LOG_LEVEL falls back to info with a warning and any LOG_FORMAT other than
// json means text.
func resolveLogOptions(flagLevel, flagFormat string, levelFlagSet, formatFlagSet bool) (logOptions, error) {
	o := logOptions{level: slog.LevelInfo}

	switch v := os.Getenv("LOG_LEVEL"); {
	case levelFlagSet:
		level, err := parseLogLevel("--log-level", flagLevel)
		if err != nil {
			return o, err
		}
		o.level, o.levelSet = level, true
	case v != "":
		o.levelSet = true
		if err := o.level.UnmarshalText([]byte(v)); err != nil {
			o.level = slog.LevelInfo
			o.invalidEnvLevel = v
		}
	}

	switch v := os.Getenv("LOG_FORMAT"); {
	case formatFlagSet:
		json, err := parseLogFormat("--log-format", flagFormat)
		if err != nil {
			return o, err
		}
		o.json, o.formatSet = json, true
	case v != "":
		o.json, o.formatSet = v == "json", true
	}

	return o, nil
}

// withConfig fills in the settings that no flag or environment variable set
// from the logLevel and logFormat keys of the loaded config file. It reports
// whether anything changed.
func (o logOptions) withConfig() (logOptions, bool, error) {
	changed := false
	if !o.levelSet && viper.IsSet("logLevel") {
		level, err := parseLogLevel("logLevel", viper.GetString("logLevel"))
		if err != nil {
			return o, false, err
		}
		o.level, changed = level, true
	}
	if !o.formatSet && viper.IsSet("logFormat") {
		json, err := parseLogFormat("logFormat", viper.GetString("logFormat"))
		if err != nil {
			return o, false, err
		}
		o.json, changed = json, true
	}
	return o, changed, nil
}

func parseLogLevel(name, v string) (slog.Level, error) {
	switch strings.ToLower(v) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("invalid %s %q: allowed values are %s", name, v, strings.Join(logLevels, ", "))
}

func parseLogFormat(name, v string) (bool, error) {
	switch strings.ToLower(v) {
	case "text":
		return false, nil
	case "json":
		return true, nil
	}
	return false, fmt.Errorf("invalid %s %q: allowed values are %s", name, v, strings.Join(logFormats, ", "))
}

// logger returns a logger that writes to stderr, adding the source only at
// debug level.
func (o logOptions) logger() *slog.Logger {
	opts := &slog.HandlerOptions{AddSource: o.level <= slog.LevelDebug, Level: o.level}

	var handler slog.Handler
	if o.json {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		opts.ReplaceAttr = shortSource
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	log := slog.New(handler)
	if o.invalidEnvLevel != "" {
		log.Warn("invalid LOG_LEVEL, falling back to info", "value", o.invalidEnvLevel)
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
