package cmd

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

// messages returns the recorded messages in order, each paired with its "path" attribute.
func (h *recordingHandler) messages() [][2]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out [][2]string
	for _, r := range h.records {
		var path string
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "path" {
				path = a.Value.String()
			}
			return true
		})
		out = append(out, [2]string{r.Message, path})
	}
	return out
}

func TestInitConfigDiagnostics(t *testing.T) {
	setup := func(t *testing.T) (*slog.Logger, *recordingHandler) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		h := &recordingHandler{}
		return slog.New(h), h
	}

	t.Run("explicit config file", func(t *testing.T) {
		log, h := setup(t)
		file := filepath.Join(t.TempDir(), "cfg.yaml")
		require.NoError(t, os.WriteFile(file, []byte("proxies: []\n"), 0o600))

		require.NoError(t, initConfig(log, file))

		assert.Equal(t, [][2]string{{"using explicit config file", file}}, h.messages())
	})

	t.Run("default location with a config file", func(t *testing.T) {
		log, h := setup(t)
		home := t.TempDir()
		t.Setenv("HOME", home)
		dir := filepath.Join(home, ".config", "cloudflared-proxy")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("proxies: []\n"), 0o600))

		require.NoError(t, initConfig(log, ""))

		assert.Equal(t, [][2]string{{"using default config location", dir}}, h.messages())
	})

	t.Run("default config file not found", func(t *testing.T) {
		log, h := setup(t)
		home := t.TempDir()
		t.Setenv("HOME", home)
		dir := filepath.Join(home, ".config", "cloudflared-proxy")

		err := initConfig(log, "")

		assert.EqualError(t, err, "no config file or endpoints provided")
		assert.Equal(t, [][2]string{
			{"using default config location", dir},
			{"default config file not found", dir},
		}, h.messages())
	})
}
