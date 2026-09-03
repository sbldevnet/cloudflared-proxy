package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sbldevnet/cloudflared-proxy/pkg/logger"
)

// Server defines the behavior of a server that can be started and shut down.
type Server interface {
	ListenAndServe() error
	Shutdown(ctx context.Context) error
	HTTPServer() *http.Server
}

// httpServer is a wrapper around http.Server that implements the Server interface.
type httpServer struct {
	*http.Server
}

// HTTPServer returns the underlying http.Server.
func (s *httpServer) HTTPServer() *http.Server {
	return s.Server
}

// newServer is a constructor that can be replaced in tests.
var newServer = func(addr string, handler http.Handler) Server {
	return &httpServer{
		&http.Server{
			Addr:    addr,
			Handler: handler,
		},
	}
}

const (
	randomPortRange = 1000
	randomPortStart = 8000
)

func newDirector(config CFAccessProxyConfig) func(*http.Request) {
	return func(req *http.Request) {
		req.URL.Scheme = config.Url.Scheme
		req.URL.Host = config.Url.Host
		req.Host = config.Url.Host
		req.Header.Add("cf-access-token", config.Token)

		// Debug requests through the proxy
		logger.Debug("proxy.Proxy", "Request to localhost:%d, URL: %s, Headers: %v", config.LocalPort, req.URL, req.Header)
	}
}

type CFAccessProxyConfig struct {
	Url       *url.URL
	Token     string
	LocalPort uint16 // change to local port
	SkipTLS   bool
}

// addr is tracked outside http.Server.Addr: the retry goroutine reassigns
// that field concurrently with the shutdown loop reading it for logging.
type serverEntry struct {
	server Server
	addr   atomic.Pointer[string]
}

func (e *serverEntry) setAddr(addr string) {
	e.server.HTTPServer().Addr = addr
	e.addr.Store(&addr)
}

func (e *serverEntry) getAddr() string {
	return *e.addr.Load()
}

func StartMultipleProxies(ctx context.Context, configs []CFAccessProxyConfig) error {
	if len(configs) == 0 {
		return errors.New("no proxy configurations provided")
	}

	var entries []*serverEntry
	var wg sync.WaitGroup

	for _, proxyConfig := range configs {

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: proxyConfig.SkipTLS, MinVersion: tls.VersionTLS12},
		}

		proxy := httputil.NewSingleHostReverseProxy(proxyConfig.Url)
		proxy.Transport = transport
		proxy.Director = newDirector(proxyConfig)

		addr := fmt.Sprintf(":%d", proxyConfig.LocalPort)
		entry := &serverEntry{server: newServer(addr, proxy)}
		entry.addr.Store(&addr)
		entries = append(entries, entry)

		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("proxy.Proxy", "Starting proxy server on http://localhost:%d, forwarding to %s", proxyConfig.LocalPort, proxyConfig.Url.String())

			err := entry.server.ListenAndServe()

			// If the error is that the port is in use, try again with a random port.
			if err != nil && errors.Is(err, syscall.EADDRINUSE) {
				randomPort := getRandomPort()
				logger.Warn("proxy.Proxy", "Port %d for target %s is in use. Retrying on port %d", proxyConfig.LocalPort, proxyConfig.Url.String(), randomPort)
				entry.setAddr(fmt.Sprintf(":%d", randomPort))
				err = entry.server.ListenAndServe() // Retry
			}

			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("proxy.Proxy", err, "Proxy for %s failed to start", proxyConfig.Url.String())
			}
		}()
	}

	logger.Info("proxy.Proxy", "Press CTRL+C to stop.")

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("proxy.Proxy", "Shutdown signal received, gracefully shutting down servers...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, e := range entries {
		if err := e.server.Shutdown(shutdownCtx); err != nil {
			logger.Error("proxy.Proxy", err, "Failed to gracefully shut down server at %s", e.getAddr())
		}
	}

	wg.Wait()
	logger.Info("proxy.Proxy", "All proxies have been shut down.")
	return nil
}

var getRandomPort = func() int {
	return rand.N(randomPortRange) + randomPortStart
}
