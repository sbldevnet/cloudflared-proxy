package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
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

func StartMultipleProxies(ctx context.Context, configs []CFAccessProxyConfig) error {
	if len(configs) == 0 {
		return errors.New("no proxy configurations provided")
	}

	var servers []Server
	var wg sync.WaitGroup

	for _, proxyConfig := range configs {

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: proxyConfig.SkipTLS, MinVersion: tls.VersionTLS12},
		}

		proxy := httputil.NewSingleHostReverseProxy(proxyConfig.Url)
		proxy.Transport = transport
		proxy.Director = newDirector(proxyConfig)

		server := newServer(fmt.Sprintf(":%d", proxyConfig.LocalPort), proxy)
		servers = append(servers, server)

		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("proxy.Proxy", "Starting proxy server on http://localhost:%d, forwarding to %s", proxyConfig.LocalPort, proxyConfig.Url.String())

			err := server.ListenAndServe()

			// If the error is that the port is in use, try again with a random port.
			if err != nil && errors.Is(err, syscall.EADDRINUSE) {
				randomPort := getRandomPort()
				logger.Warn("proxy.Proxy", "Port %d for target %s is in use. Retrying on port %d", proxyConfig.LocalPort, proxyConfig.Url.String(), randomPort)
				server.HTTPServer().Addr = fmt.Sprintf(":%d", randomPort)
				err = server.ListenAndServe() // Retry
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

	for _, s := range servers {
		if err := s.Shutdown(shutdownCtx); err != nil {
			logger.Error("proxy.Proxy", err, "Failed to gracefully shut down server at %s", s.HTTPServer().Addr)
		}
	}

	wg.Wait()
	logger.Info("proxy.Proxy", "All proxies have been shut down.")
	return nil
}

var getRandomPort = func() int {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return r.Intn(randomPortRange) + randomPortStart
}
