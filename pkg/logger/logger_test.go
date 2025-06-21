package logger_test

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/sbldevnet/cloudflared-proxy/pkg/logger"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func captureLogOutput(f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	savedLevel := log.GetLevel()
	log.SetLevel(log.TraceLevel) // Ensure all levels are captured
	defer func() {
		log.SetOutput(os.Stderr) // Restore default output
		log.SetLevel(savedLevel) // Restore log level
	}()

	f()
	return buf.String()
}

func TestLoggerLevels(t *testing.T) {
	funcName := "TestFunction"
	const msg = "test message"
	testErr := errors.New("test error")

	t.Run("Trace", func(t *testing.T) {
		output := captureLogOutput(func() {
			logger.Trace(funcName, "%v", msg)
		})
		assert.Contains(t, output, "level=trace")
		assert.Contains(t, output, "function=TestFunction")
		assert.Contains(t, output, msg)
	})

	t.Run("Debug", func(t *testing.T) {
		output := captureLogOutput(func() {
			logger.Debug(funcName, "%v", msg)
		})
		assert.Contains(t, output, "level=debug")
		assert.Contains(t, output, "function=TestFunction")
		assert.Contains(t, output, msg)
	})

	t.Run("Info", func(t *testing.T) {
		output := captureLogOutput(func() {
			logger.Info(funcName, "%v", msg)
		})
		assert.Contains(t, output, "level=info")
		assert.Contains(t, output, "function=TestFunction")
		assert.Contains(t, output, msg)
	})

	t.Run("Warn", func(t *testing.T) {
		output := captureLogOutput(func() {
			logger.Warn(funcName, "%v", msg)
		})
		assert.Contains(t, output, "level=warn")
		assert.Contains(t, output, "function=TestFunction")
		assert.Contains(t, output, msg)
	})

	t.Run("Error", func(t *testing.T) {
		output := captureLogOutput(func() {
			logger.Error(funcName, testErr, msg)
		})
		assert.Contains(t, output, "level=error")
		assert.Contains(t, output, "function=TestFunction")
		assert.Contains(t, output, "error=\"test error\"")
		assert.Contains(t, output, msg)
	})
}
