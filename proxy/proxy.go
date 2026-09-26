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

// server defines the behavior of a server that can be started and shut down.
type server interface {
	ListenAndServe() error
	Shutdown(ctx context.Context) error
	HTTPServer() *http.Server
}

// httpServer is a wrapper around http.Server that implements the server interface.
type httpServer struct {
	*http.Server
}

// HTTPServer returns the underlying http.Server.
func (s *httpServer) HTTPServer() *http.Server {
	return s.Server
}

func newHTTPServer(addr string, handler http.Handler) server {
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

func newDirector(log *slog.Logger, config accessProxyConfig) func(*http.Request) {
	return func(req *http.Request) {
		req.URL.Scheme = config.url.Scheme
		req.URL.Host = config.url.Host
		req.Host = config.url.Host
		req.Header.Add("cf-access-token", config.token)

		log.Debug("proxying request", "local_port", config.localPort, "method", req.Method, "url", req.URL.String())
	}
}

type accessProxyConfig struct {
	url       *url.URL
	token     string
	localPort uint16
	skipTLS   bool
}

// addr is tracked outside http.Server.Addr: the retry goroutine reassigns
// that field concurrently with the shutdown loop reading it for logging.
type serverEntry struct {
	server server
	addr   atomic.Pointer[string]
}

func (e *serverEntry) setAddr(addr string) {
	e.server.HTTPServer().Addr = addr
	e.addr.Store(&addr)
}

func (e *serverEntry) getAddr() string {
	return *e.addr.Load()
}

func (r *Runner) serve(ctx context.Context, configs []accessProxyConfig) error {
	if len(configs) == 0 {
		return errors.New("no proxy configurations provided")
	}

	var entries []*serverEntry
	var wg sync.WaitGroup

	for _, proxyConfig := range configs {

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: proxyConfig.skipTLS, MinVersion: tls.VersionTLS12},
		}

		proxy := httputil.NewSingleHostReverseProxy(proxyConfig.url)
		proxy.Transport = transport
		proxy.Director = newDirector(r.log, proxyConfig)

		addr := fmt.Sprintf(":%d", proxyConfig.localPort)
		entry := &serverEntry{server: r.newServer(addr, proxy)}
		entry.addr.Store(&addr)
		entries = append(entries, entry)

		wg.Add(1)
		go func() {
			defer wg.Done()
			r.log.Info("starting proxy server", "local_port", proxyConfig.localPort, "target", proxyConfig.url.String())

			err := entry.server.ListenAndServe()

			// If the error is that the port is in use, try again with a random port.
			if err != nil && errors.Is(err, syscall.EADDRINUSE) {
				randomPort := getRandomPort()
				r.log.Warn("port in use, retrying on random port", "local_port", proxyConfig.localPort, "target", proxyConfig.url.String(), "retry_port", randomPort)
				entry.setAddr(fmt.Sprintf(":%d", randomPort))
				err = entry.server.ListenAndServe() // Retry
			}

			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				r.log.Error("proxy failed to start", "target", proxyConfig.url.String(), "error", err)
			}
		}()
	}

	r.log.Info("press CTRL+C to stop")

	// Wait for shutdown signal
	<-ctx.Done()
	r.log.Info("shutdown signal received, shutting down servers")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, e := range entries {
		if err := e.server.Shutdown(shutdownCtx); err != nil {
			r.log.Error("graceful shutdown failed", "addr", e.getAddr(), "error", err)
		}
	}

	wg.Wait()
	r.log.Info("all proxies shut down")
	return nil
}

var getRandomPort = func() int {
	return rand.N(randomPortRange) + randomPortStart
}
