package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
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

func newDirector(log *slog.Logger, config CFAccessProxyConfig) func(*http.Request) {
	return func(req *http.Request) {
		req.URL.Scheme = config.Url.Scheme
		req.URL.Host = config.Url.Host
		req.Host = config.Url.Host
		req.Header.Add("cf-access-token", config.Token)

		log.Debug("proxied request", "local_port", config.LocalPort, "url", req.URL.String(), "headers", req.Header)
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

func StartMultipleProxies(ctx context.Context, log *slog.Logger, configs []CFAccessProxyConfig) error {
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
		proxy.Director = newDirector(log, proxyConfig)

		addr := fmt.Sprintf(":%d", proxyConfig.LocalPort)
		entry := &serverEntry{server: newServer(addr, proxy)}
		entry.addr.Store(&addr)
		entries = append(entries, entry)

		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Info("starting proxy server", "local_port", proxyConfig.LocalPort, "target", proxyConfig.Url.String())

			err := entry.server.ListenAndServe()

			// If the error is that the port is in use, try again with a random port.
			if err != nil && errors.Is(err, syscall.EADDRINUSE) {
				randomPort := getRandomPort()
				log.Warn("port in use, retrying on random port", "local_port", proxyConfig.LocalPort, "target", proxyConfig.Url.String(), "retry_port", randomPort)
				entry.setAddr(fmt.Sprintf(":%d", randomPort))
				err = entry.server.ListenAndServe() // Retry
			}

			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("proxy failed to start", "target", proxyConfig.Url.String(), "error", err)
			}
		}()
	}

	log.Info("press CTRL+C to stop")

	// Wait for shutdown signal
	<-ctx.Done()
	log.Info("shutdown signal received, gracefully shutting down servers")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, e := range entries {
		if err := e.server.Shutdown(shutdownCtx); err != nil {
			log.Error("failed to gracefully shut down server", "addr", e.getAddr(), "error", err)
		}
	}

	wg.Wait()
	log.Info("all proxies have been shut down")
	return nil
}

var getRandomPort = func() int {
	return rand.N(randomPortRange) + randomPortStart
}
