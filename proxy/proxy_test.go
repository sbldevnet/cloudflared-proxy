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
			{url: u1, localPort: 8080},
			{url: u2, localPort: 8082},
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
			{url: u, localPort: 8080},
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
			{url: u, localPort: 8080},
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
			{url: u, localPort: 8080},
		}

		genericError := errors.New("a generic error")
		mockSrvr.On("ListenAndServe").Return(genericError).Once()
		mockSrvr.On("Shutdown", mock.Anything).Return(nil).Once()

		assert.NoError(t, serveUntilListening(t, r, configs, listening, 1))

		mockSrvr.AssertExpectations(t)
	})
}
