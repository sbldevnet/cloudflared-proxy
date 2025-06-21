package proxy

import (
	"context"
	"errors"
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
	config := CFAccessProxyConfig{
		Url:   targetURL,
		Token: "test-token",
	}

	director := newDirector(config)

	// Create a sample request to test the director
	req := httptest.NewRequest("GET", "http://localhost:8080/", nil)
	director(req) // Apply the director logic to the request

	// Assert that the request was modified as expected
	assert.Equal(t, "https://app.example.com/", req.URL.String())
	assert.Equal(t, "app.example.com", req.Host)
	assert.Equal(t, "test-token", req.Header.Get("cf-access-token"))
}

func TestStartMultipleProxies(t *testing.T) {
	// Backup and restore original functions
	originalNewServer := newServer
	originalGetRandomPort := getRandomPort
	t.Cleanup(func() {
		newServer = originalNewServer
		getRandomPort = originalGetRandomPort
	})

	t.Run("no proxy configs", func(t *testing.T) {
		err := StartMultipleProxies(context.Background(), []CFAccessProxyConfig{})
		assert.EqualError(t, err, "no proxy configurations provided")
	})

	t.Run("invalid hostname with other valid hostnames", func(t *testing.T) {
		var serverCreationCount int
		newServer = func(addr string, handler http.Handler) Server {
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
		configs := []CFAccessProxyConfig{
			{Url: u1, LocalPort: 8080},
			{Url: u2, LocalPort: 8082},
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			StartMultipleProxies(ctx, configs)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()
		wg.Wait()

		assert.Equal(t, 2, serverCreationCount, "should create two servers for the valid hostnames")
	})

	t.Run("successful startup and shutdown", func(t *testing.T) {
		mockSrvr := new(MockServer)
		newServer = func(addr string, handler http.Handler) Server {
			return mockSrvr
		}

		ctx, cancel := context.WithCancel(context.Background())
		u, _ := url.Parse("https://app.example.com")
		configs := []CFAccessProxyConfig{
			{Url: u, LocalPort: 8080},
		}

		mockSrvr.On("ListenAndServe").Return(http.ErrServerClosed).Once()
		mockSrvr.On("Shutdown", mock.Anything).Return(nil).Once()
		// mockSrvr.On("HTTPServer").Return(&http.Server{Addr: ":8080"}).Maybe()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := StartMultipleProxies(ctx, configs)
			assert.NoError(t, err)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()
		wg.Wait()

		mockSrvr.AssertExpectations(t)
	})

	t.Run("port in use with successful retry", func(t *testing.T) {
		mockSrvr := new(MockServer)
		newServer = func(addr string, handler http.Handler) Server {
			return mockSrvr
		}
		getRandomPort = func() int { return 9090 }

		ctx, cancel := context.WithCancel(context.Background())
		u, _ := url.Parse("https://app.example.com")
		configs := []CFAccessProxyConfig{
			{Url: u, LocalPort: 8080},
		}

		mockSrvr.On("ListenAndServe").Return(syscall.EADDRINUSE).Once()
		mockSrvr.On("ListenAndServe").Return(http.ErrServerClosed).Once()
		mockSrvr.On("Shutdown", mock.Anything).Return(nil).Once()
		mockSrvr.On("HTTPServer").Return(&http.Server{Addr: ":8080"}).Maybe()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := StartMultipleProxies(ctx, configs)
			assert.NoError(t, err)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()
		wg.Wait()

		mockSrvr.AssertExpectations(t)
	})

	t.Run("listen and serve fails with generic error", func(t *testing.T) {
		mockSrvr := new(MockServer)
		newServer = func(addr string, handler http.Handler) Server {
			return mockSrvr
		}

		ctx, cancel := context.WithCancel(context.Background())
		u, _ := url.Parse("https://app.example.com")
		configs := []CFAccessProxyConfig{
			{Url: u, LocalPort: 8080},
		}

		genericError := errors.New("a generic error")
		mockSrvr.On("ListenAndServe").Return(genericError).Once()
		mockSrvr.On("Shutdown", mock.Anything).Return(nil).Once()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := StartMultipleProxies(ctx, configs)
			assert.NoError(t, err)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()
		wg.Wait()

		mockSrvr.AssertExpectations(t)
	})
}
