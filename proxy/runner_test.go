package proxy

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/sbldevnet/cloudflared-proxy/cloudflared"
	"github.com/sbldevnet/cloudflared-proxy/config"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubServer struct {
	http.Server
	stopped chan struct{}
}

func (s *stubServer) ListenAndServe() error {
	<-s.stopped
	return http.ErrServerClosed
}

func (s *stubServer) Shutdown(context.Context) error {
	close(s.stopped)
	return nil
}

func (s *stubServer) HTTPServer() *http.Server { return &s.Server }

// runWithCancelledContext runs r with an already-cancelled context so Run
// returns as soon as the servers have been created and shut down.
func runWithCancelledContext(r *Runner, configs []config.ProxyConfig) error {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return r.Run(ctx, configs)
}

var errFetch = errors.New("some-cf-error")

func TestRunnerRun(t *testing.T) {
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(nil) })

	cfgs := []config.ProxyConfig{
		{Hostname: "app1.example.com", DestinationPort: 443, LocalPort: 8080},
	}

	newRunner := func(fetch func(context.Context, string) (string, error)) (*Runner, *[]string, *[]http.Handler) {
		var addrs []string
		var handlers []http.Handler
		r := New()
		r.tokenFetcher = fetch
		r.newServer = func(addr string, handler http.Handler) server {
			addrs = append(addrs, addr)
			handlers = append(handlers, handler)
			return &stubServer{stopped: make(chan struct{})}
		}
		return r, &addrs, &handlers
	}

	t.Run("forwards requests with the fetched token", func(t *testing.T) {
		var gotToken string
		backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			gotToken = req.Header.Get("cf-access-token")
		}))
		t.Cleanup(backend.Close)
		backendURL, err := url.Parse(backend.URL)
		require.NoError(t, err)
		port, err := strconv.Atoi(backendURL.Port())
		require.NoError(t, err)

		var fetched []string
		r, addrs, handlers := newRunner(func(_ context.Context, u string) (string, error) {
			fetched = append(fetched, u)
			return "token123", nil
		})
		backendCfgs := []config.ProxyConfig{
			{Hostname: backendURL.Hostname(), DestinationPort: uint16(port), LocalPort: 8080, SkipTLS: true},
		}

		require.NoError(t, runWithCancelledContext(r, backendCfgs))

		assert.Equal(t, []string{backendURL.Host}, fetched)
		assert.Equal(t, []string{":8080"}, *addrs)
		require.Len(t, *handlers, 1)

		rec := httptest.NewRecorder()
		(*handlers)[0].ServeHTTP(rec, httptest.NewRequest("GET", "http://localhost:8080/", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "token123", gotToken)
	})

	t.Run("access app not found continues without token", func(t *testing.T) {
		logOutput.Reset()
		r, addrs, _ := newRunner(func(context.Context, string) (string, error) {
			return "", cloudflared.ErrAccessAppNotFound
		})

		require.NoError(t, runWithCancelledContext(r, cfgs))

		assert.Equal(t, []string{":8080"}, *addrs)
		assert.Contains(t, logOutput.String(), "Access application not found at app1.example.com:443, continuing without authentication")
	})

	t.Run("token error aborts before starting servers", func(t *testing.T) {
		r, addrs, _ := newRunner(func(context.Context, string) (string, error) {
			return "", errFetch
		})

		err := r.Run(context.Background(), cfgs)

		assert.ErrorContains(t, err, "app1.example.com:443")
		assert.ErrorIs(t, err, errFetch)
		assert.Empty(t, *addrs)
	})

	t.Run("cancellation during fetch returns the context error", func(t *testing.T) {
		r, addrs, _ := newRunner(func(ctx context.Context, _ string) (string, error) {
			<-ctx.Done()
			return "", errors.New("signal: killed")
		})
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(10*time.Millisecond, cancel)

		err := r.Run(ctx, cfgs)

		assert.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, *addrs)
	})

	t.Run("no configs", func(t *testing.T) {
		r, _, _ := newRunner(func(context.Context, string) (string, error) { return "", nil })

		err := runWithCancelledContext(r, nil)

		assert.EqualError(t, err, "no proxy configurations provided")
	})
}
