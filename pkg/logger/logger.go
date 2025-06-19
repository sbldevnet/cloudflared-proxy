package logger

import (
	"os"

	log "github.com/sirupsen/logrus"
)

func init() {
	lvl, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		lvl = "info" // default
	}

	ll, err := log.ParseLevel(lvl)
	if err != nil {
		ll = log.DebugLevel
	}
	log.SetLevel(ll)

	format, ok := os.LookupEnv("LOG_FORMAT")
	if ok {
		SetLoggerFormat(format)
	}
}

func SetLoggerFormat(logFormat string) {
	switch logFormat {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	default:
		log.SetFormatter(&log.TextFormatter{})
	}
}

// Wrap logrus with function name
func Trace(funcName, format string, args ...any) {
	log.WithField("function", funcName).Tracef(format, args...)
}

func Debug(funcName, format string, args ...any) {
	log.WithField("function", funcName).Debugf(format, args...)
}

func Info(funcName, format string, args ...any) {
	log.WithField("function", funcName).Infof(format, args...)
}

func Warn(funcName, format string, args ...any) {
	log.WithField("function", funcName).Warnf(format, args...)
}

func Error(funcName string, err error, format string, args ...any) {
	log.WithField("function", funcName).WithError(err).Errorf(format, args...)
}
