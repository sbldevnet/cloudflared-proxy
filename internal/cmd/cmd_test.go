package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sbldevnet/cloudflared-proxy/config"
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

func TestListenPrecedence(t *testing.T) {
	testCases := []struct {
		name     string
		topLevel string
		perProxy string
		flag     *string
		want     string
	}{
		{name: "nothing set leaves the runner default", want: ""},
		{name: "top-level applies", topLevel: "0.0.0.0", want: "0.0.0.0"},
		{name: "per-proxy over top-level", topLevel: "0.0.0.0", perProxy: "127.0.0.1", want: "127.0.0.1"},
		{name: "flag over per-proxy and top-level", topLevel: "0.0.0.0", perProxy: "192.168.1.5", flag: ptr("::1"), want: "::1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			proxies := []config.ProxyConfig{{Hostname: "a.example.com", Listen: tc.perProxy}}

			applyListenDefault(proxies, tc.topLevel)
			if tc.flag != nil {
				overrideListen(proxies, *tc.flag)
			}

			assert.Equal(t, tc.want, proxies[0].Listen)
		})
	}
}

func TestValidateListen(t *testing.T) {
	assert.NoError(t, validateListen([]config.ProxyConfig{{Listen: ""}, {Listen: "::1"}, {Listen: "10.0.0.1"}}))
	assert.ErrorContains(t, validateListen([]config.ProxyConfig{{Listen: "localhost"}}), "IP address literal")
	assert.ErrorContains(t, validateListen([]config.ProxyConfig{{Listen: "example.com"}}), "IP address literal")
}

func TestListenConfigKeys(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	file := filepath.Join(t.TempDir(), "cfg.yaml")
	yaml := `listen: 0.0.0.0
proxies:
  - hostname: app1.example.com
  - hostname: app2.example.com
    listen: 127.0.0.1
`
	require.NoError(t, os.WriteFile(file, []byte(yaml), 0o600))
	require.NoError(t, initConfig(slog.New(&recordingHandler{}), file))

	var cfg config.Config
	require.NoError(t, viper.Unmarshal(&cfg))
	applyListenDefault(cfg.Proxies, cfg.Listen)

	assert.Equal(t, "0.0.0.0", cfg.Listen)
	assert.Equal(t, "0.0.0.0", cfg.Proxies[0].Listen)
	assert.Equal(t, "127.0.0.1", cfg.Proxies[1].Listen)
}

func TestListenFlagRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "localhost", "example.com", "999.1.1.1"} {
		t.Run(value, func(t *testing.T) {
			cmd := Run()
			cmd.SetArgs([]string{"-e", "example.com", "--listen", value})
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			err := cmd.Execute()

			assert.ErrorContains(t, err, "IP address literal")
		})
	}
}

func TestSkipTLSPrecedence(t *testing.T) {
	testCases := []struct {
		name string
		file []bool
		flag *bool
		want []bool
	}{
		{name: "endpoints, flag unset", file: []bool{false, false}, want: []bool{false, false}},
		{name: "endpoints, flag true", file: []bool{false, false}, flag: ptr(true), want: []bool{true, true}},
		{name: "endpoints, flag false", file: []bool{false, false}, flag: ptr(false), want: []bool{false, false}},
		{name: "config file, flag unset keeps per-proxy values", file: []bool{true, false}, want: []bool{true, false}},
		{name: "config file, flag true applies to all", file: []bool{true, false}, flag: ptr(true), want: []bool{true, true}},
		{name: "config file, flag false forces verification", file: []bool{true, false}, flag: ptr(false), want: []bool{false, false}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			proxies := make([]config.ProxyConfig, len(tc.file))
			for i, skip := range tc.file {
				proxies[i] = config.ProxyConfig{Hostname: fmt.Sprintf("app%d.example.com", i), SkipTLS: skip}
			}
			h := &recordingHandler{}

			if tc.flag != nil {
				overrideSkipTLS(proxies, *tc.flag)
			}
			warnSkipTLS(slog.New(h), proxies)

			var got []bool
			var wantWarned []string
			for _, p := range proxies {
				got = append(got, p.SkipTLS)
				if p.SkipTLS {
					wantWarned = append(wantWarned, p.Hostname)
				}
			}
			assert.Equal(t, tc.want, got)

			var warned []string
			for _, r := range h.records {
				assert.Equal(t, slog.LevelWarn, r.Level)
				r.Attrs(func(a slog.Attr) bool {
					if a.Key == "hostname" {
						warned = append(warned, a.Value.String())
					}
					return true
				})
			}
			assert.Equal(t, wantWarned, warned)
		})
	}
}

func ptr[T any](v T) *T { return &v }
