package proxy

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockServer struct {
	mock.Mock
	// listening, when set, receives one value after each ListenAndServe call is recorded.
	listening chan<- struct{}
}

func (m *MockServer) ListenAndServe() error {
	args := m.Called()
	if m.listening != nil {
		m.listening <- struct{}{}
	}
	return args.Error(0)
}

func (m *MockServer) Shutdown(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockServer) HTTPServer() *http.Server {
	args := m.Called()
	if server, ok := args.Get(0).(*http.Server); ok {
		// Return a copy to avoid race conditions on the Addr field
		return &http.Server{Addr: server.Addr}
	}
	return nil
}

// serveUntilListening runs r.serve, waits for wantListens signals on listening,
// then cancels the context and returns serve's error once it has returned.
func serveUntilListening(t *testing.T, r *Runner, configs []accessProxyConfig, listening <-chan struct{}, wantListens int) error {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errc := make(chan error, 1)
	go func() { errc <- r.serve(ctx, configs) }()

	timeout := time.After(5 * time.Second)
	for range wantListens {
		select {
		case <-listening:
		case <-timeout:
			t.Fatal("timed out waiting for servers to start listening")
		}
	}
	cancel()

	select {
	case err := <-errc:
		return err
	case <-timeout:
		t.Fatal("timed out waiting for serve to return")
		return nil
	}
}

// serveWithTimeout runs r.serve with a context that is never cancelled by the
// test, so serve can only return on its own or fail the test by timing out.
func serveWithTimeout(t *testing.T, r *Runner, configs []accessProxyConfig) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := r.serve(ctx, configs)
	require.NoError(t, ctx.Err(), "serve did not return before the timeout")
	return err
}

// addrRecordingServer records the address of each ListenAndServe call and
// reports the port as in use on the first one.
type addrRecordingServer struct {
	http.Server
	listening chan<- struct{}
	addrs     []string
}

func (s *addrRecordingServer) ListenAndServe() error {
	s.addrs = append(s.addrs, s.Server.Addr)
	s.listening <- struct{}{}
	if len(s.addrs) == 1 {
		return syscall.EADDRINUSE
	}
	return http.ErrServerClosed
}

func (s *addrRecordingServer) Shutdown(context.Context) error { return nil }
func (s *addrRecordingServer) HTTPServer() *http.Server       { return &s.Server }

const (
	allInterfacesWarning   = "proxy listens on every network interface, so it is reachable from other machines, and forwards requests with your Cloudflare Access token"
	specificAddressWarning = "proxy is reachable from other machines and forwards requests with your Cloudflare Access token"
)

// TestNewDirector validates that the director function is configured correctly.
func TestNewDirector(t *testing.T) {
	targetURL, _ := url.Parse("https://app.example.com")
	config := accessProxyConfig{
		url:   targetURL,
		token: "test-token",
	}

	director := newDirector(slog.New(slog.DiscardHandler), config)

	// Create a sample request to test the director
	req := httptest.NewRequest("GET", "http://localhost:8080/", nil)
	director(req) // Apply the director logic to the request

	// Assert that the request was modified as expected
	assert.Equal(t, "https://app.example.com/", req.URL.String())
	assert.Equal(t, "app.example.com", req.Host)
	assert.Equal(t, "test-token", req.Header.Get("cf-access-token"))
}

func TestNewDirectorReplacesClientAccessToken(t *testing.T) {
	targetURL, _ := url.Parse("https://app.example.com")
	director := newDirector(slog.New(slog.DiscardHandler), accessProxyConfig{url: targetURL, token: "proxy-token"})

	req := httptest.NewRequest("GET", "http://localhost:8080/", nil)
	req.Header.Set("cf-access-token", "client-token")
	director(req)

	assert.Equal(t, []string{"proxy-token"}, req.Header.Values("cf-access-token"))
}

func TestNewDirectorDoesNotLogSecrets(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	targetURL, _ := url.Parse("https://app.example.com")
	director := newDirector(log, accessProxyConfig{url: targetURL, token: "secret-cf-token", localPort: 8080})

	req := httptest.NewRequest("GET", "http://localhost:8080/api?token=secret-query", nil)
	req.Header.Set("Authorization", "Bearer secret-bearer")
	req.Header.Set("Cookie", "session=secret-cookie")
	director(req)

	assert.NotEmpty(t, buf.String())
	for _, secret := range []string{"secret-cf-token", "secret-bearer", "secret-cookie", "secret-query"} {
		assert.NotContains(t, buf.String(), secret)
	}
	assert.Contains(t, buf.String(), "path=/api")
}

func TestRunnerServe(t *testing.T) {
	// Backup and restore original functions
	originalGetRandomPort := getRandomPort
	t.Cleanup(func() {
		getRandomPort = originalGetRandomPort
	})

	r := New(WithLogger(slog.New(slog.DiscardHandler)))

	t.Run("no proxy configs", func(t *testing.T) {
		err := r.serve(context.Background(), []accessProxyConfig{})
		assert.EqualError(t, err, "no proxy configurations provided")
	})

	t.Run("invalid hostname with other valid hostnames", func(t *testing.T) {
		var serverCreationCount int
		listening := make(chan struct{}, 2)
		r.newServer = func(addr string, handler http.Handler) server {
			serverCreationCount++
			mockSrvr := &MockServer{listening: listening}
			mockSrvr.On("ListenAndServe").Return(http.ErrServerClosed)
			mockSrvr.On("Shutdown", mock.Anything).Return(nil)
			mockSrvr.On("HTTPServer").Return(&http.Server{Addr: addr})
			return mockSrvr
		}

		u1, _ := url.Parse("https://app.example.com")
		u2, _ := url.Parse("https://app2.example.com")
		configs := []accessProxyConfig{
			{url: u1, localPort: 8080, listen: "127.0.0.1"},
			{url: u2, localPort: 8082, listen: "127.0.0.1"},
		}

		serveUntilListening(t, r, configs, listening, 2)

		assert.Equal(t, 2, serverCreationCount, "should create two servers for the valid hostnames")
	})

	t.Run("successful startup and shutdown", func(t *testing.T) {
		listening := make(chan struct{}, 2)
		mockSrvr := &MockServer{listening: listening}
		r.newServer = func(addr string, handler http.Handler) server {
			return mockSrvr
		}

		u, _ := url.Parse("https://app.example.com")
		configs := []accessProxyConfig{
			{url: u, localPort: 8080, listen: "127.0.0.1"},
		}

		mockSrvr.On("ListenAndServe").Return(http.ErrServerClosed).Once()
		mockSrvr.On("Shutdown", mock.Anything).Return(nil).Once()
		// mockSrvr.On("HTTPServer").Return(&http.Server{Addr: ":8080"}).Maybe()

		assert.NoError(t, serveUntilListening(t, r, configs, listening, 1))

		mockSrvr.AssertExpectations(t)
	})

	t.Run("port in use with successful retry", func(t *testing.T) {
		listening := make(chan struct{}, 2)
		mockSrvr := &MockServer{listening: listening}
		r.newServer = func(addr string, handler http.Handler) server {
			return mockSrvr
		}
		getRandomPort = func() int { return 9090 }

		u, _ := url.Parse("https://app.example.com")
		configs := []accessProxyConfig{
			{url: u, localPort: 8080, listen: "127.0.0.1"},
		}

		mockSrvr.On("ListenAndServe").Return(syscall.EADDRINUSE).Once()
		mockSrvr.On("ListenAndServe").Return(http.ErrServerClosed).Once()
		mockSrvr.On("Shutdown", mock.Anything).Return(nil).Once()
		mockSrvr.On("HTTPServer").Return(&http.Server{Addr: ":8080"}).Maybe()

		assert.NoError(t, serveUntilListening(t, r, configs, listening, 2))

		mockSrvr.AssertExpectations(t)
	})

	t.Run("listen and serve fails with generic error", func(t *testing.T) {
		listening := make(chan struct{}, 2)
		mockSrvr := &MockServer{listening: listening}
		r.newServer = func(addr string, handler http.Handler) server {
			return mockSrvr
		}

		u, _ := url.Parse("https://app.example.com")
		configs := []accessProxyConfig{
			{url: u, localPort: 8080, listen: "127.0.0.1"},
		}

		genericError := errors.New("a generic error")
		mockSrvr.On("ListenAndServe").Return(genericError).Once()

		err := serveWithTimeout(t, r, configs)

		assert.ErrorIs(t, err, genericError)
		mockSrvr.AssertExpectations(t)
	})

	t.Run("every proxy fails to start", func(t *testing.T) {
		listening := make(chan struct{}, 4)
		r.newServer = func(addr string, handler http.Handler) server {
			m := &MockServer{listening: listening}
			m.On("ListenAndServe").Return(syscall.EADDRINUSE)
			m.On("HTTPServer").Return(&http.Server{Addr: addr})
			return m
		}
		getRandomPort = func() int { return 9090 }

		u, _ := url.Parse("https://app.example.com")
		configs := []accessProxyConfig{
			{url: u, localPort: 8080, listen: "127.0.0.1"},
			{url: u, localPort: 8081, listen: "127.0.0.1"},
		}

		err := serveWithTimeout(t, r, configs)

		assert.ErrorIs(t, err, syscall.EADDRINUSE)
	})

	t.Run("one proxy fails while another keeps running", func(t *testing.T) {
		listening := make(chan struct{}, 2)
		var count int
		r.newServer = func(addr string, handler http.Handler) server {
			count++
			m := &MockServer{listening: listening}
			if count == 1 {
				m.On("ListenAndServe").Return(errors.New("bind failed"))
			} else {
				m.On("ListenAndServe").Return(http.ErrServerClosed)
			}
			m.On("Shutdown", mock.Anything).Return(nil).Maybe()
			return m
		}

		u, _ := url.Parse("https://app.example.com")
		configs := []accessProxyConfig{
			{url: u, localPort: 8080, listen: "127.0.0.1"},
			{url: u, localPort: 8081, listen: "127.0.0.1"},
		}

		assert.NoError(t, serveUntilListening(t, r, configs, listening, 2))
	})

	t.Run("retry keeps the listen host", func(t *testing.T) {
		listening := make(chan struct{}, 2)
		srv := &addrRecordingServer{listening: listening}
		r.newServer = func(addr string, handler http.Handler) server {
			srv.Server.Addr = addr
			return srv
		}
		getRandomPort = func() int { return 9090 }

		u, _ := url.Parse("https://app.example.com")
		configs := []accessProxyConfig{{url: u, localPort: 8080, listen: "::1"}}

		assert.NoError(t, serveUntilListening(t, r, configs, listening, 2))

		assert.Equal(t, []string{"[::1]:8080", "[::1]:9090"}, srv.addrs)
	})

	t.Run("warns once when not loopback", func(t *testing.T) {
		for _, tc := range []struct {
			listen  string
			wantMsg string
		}{
			{"127.0.0.1", ""},
			{"::1", ""},
			{"0.0.0.0", allInterfacesWarning},
			{"::", allInterfacesWarning},
			{"192.168.1.10", specificAddressWarning},
		} {
			logs := &recordingHandler{}
			lr := New(WithLogger(slog.New(logs)))
			listening := make(chan struct{}, 1)
			lr.newServer = func(addr string, handler http.Handler) server {
				m := &MockServer{listening: listening}
				m.On("ListenAndServe").Return(http.ErrServerClosed).Once()
				m.On("Shutdown", mock.Anything).Return(nil).Once()
				return m
			}
			u, _ := url.Parse("https://app.example.com")

			assert.NoError(t, serveUntilListening(t, lr, []accessProxyConfig{{url: u, localPort: 8080, listen: tc.listen}}, listening, 1))

			for _, msg := range []string{allInterfacesWarning, specificAddressWarning} {
				attrs, warned := logs.find(msg)
				assert.Equal(t, msg == tc.wantMsg, warned, "%s: %s", tc.listen, msg)
				if warned {
					assert.Equal(t, slog.LevelWarn, attrs["level"])
					assert.Equal(t, tc.listen, attrs["listen"])
				}
			}
			start, ok := logs.find("starting proxy server")
			require.True(t, ok)
			assert.Equal(t, tc.listen, start["listen"])
		}
	})
}
