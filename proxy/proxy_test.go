package proxy

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockServer struct {
	mock.Mock
}

func (m *MockServer) ListenAndServe() error {
	args := m.Called()
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
		r.newServer = func(addr string, handler http.Handler) server {
			serverCreationCount++
			mockSrvr := new(MockServer)
			mockSrvr.On("ListenAndServe").Return(http.ErrServerClosed)
			mockSrvr.On("Shutdown", mock.Anything).Return(nil)
			mockSrvr.On("HTTPServer").Return(&http.Server{Addr: addr})
			return mockSrvr
		}

		ctx, cancel := context.WithCancel(context.Background())
		u1, _ := url.Parse("https://app.example.com")
		u2, _ := url.Parse("https://app2.example.com")
		configs := []accessProxyConfig{
			{url: u1, localPort: 8080},
			{url: u2, localPort: 8082},
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.serve(ctx, configs)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()
		wg.Wait()

		assert.Equal(t, 2, serverCreationCount, "should create two servers for the valid hostnames")
	})

	t.Run("successful startup and shutdown", func(t *testing.T) {
		mockSrvr := new(MockServer)
		r.newServer = func(addr string, handler http.Handler) server {
			return mockSrvr
		}

		ctx, cancel := context.WithCancel(context.Background())
		u, _ := url.Parse("https://app.example.com")
		configs := []accessProxyConfig{
			{url: u, localPort: 8080},
		}

		mockSrvr.On("ListenAndServe").Return(http.ErrServerClosed).Once()
		mockSrvr.On("Shutdown", mock.Anything).Return(nil).Once()
		// mockSrvr.On("HTTPServer").Return(&http.Server{Addr: ":8080"}).Maybe()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := r.serve(ctx, configs)
			assert.NoError(t, err)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()
		wg.Wait()

		mockSrvr.AssertExpectations(t)
	})

	t.Run("port in use with successful retry", func(t *testing.T) {
		mockSrvr := new(MockServer)
		r.newServer = func(addr string, handler http.Handler) server {
			return mockSrvr
		}
		getRandomPort = func() int { return 9090 }

		ctx, cancel := context.WithCancel(context.Background())
		u, _ := url.Parse("https://app.example.com")
		configs := []accessProxyConfig{
			{url: u, localPort: 8080},
		}

		mockSrvr.On("ListenAndServe").Return(syscall.EADDRINUSE).Once()
		mockSrvr.On("ListenAndServe").Return(http.ErrServerClosed).Once()
		mockSrvr.On("Shutdown", mock.Anything).Return(nil).Once()
		mockSrvr.On("HTTPServer").Return(&http.Server{Addr: ":8080"}).Maybe()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := r.serve(ctx, configs)
			assert.NoError(t, err)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()
		wg.Wait()

		mockSrvr.AssertExpectations(t)
	})

	t.Run("listen and serve fails with generic error", func(t *testing.T) {
		mockSrvr := new(MockServer)
		r.newServer = func(addr string, handler http.Handler) server {
			return mockSrvr
		}

		ctx, cancel := context.WithCancel(context.Background())
		u, _ := url.Parse("https://app.example.com")
		configs := []accessProxyConfig{
			{url: u, localPort: 8080},
		}

		genericError := errors.New("a generic error")
		mockSrvr.On("ListenAndServe").Return(genericError).Once()
		mockSrvr.On("Shutdown", mock.Anything).Return(nil).Once()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := r.serve(ctx, configs)
			assert.NoError(t, err)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()
		wg.Wait()

		mockSrvr.AssertExpectations(t)
	})
}
