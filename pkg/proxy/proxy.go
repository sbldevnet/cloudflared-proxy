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

const (
	randomPortRange = 1000
	randomPortStart = 8000
)

type CFAccessProxyConfig struct {
	Hostname string
	Token    string
	Port     uint16
	SkipTLS  bool
}

func StartMultipleProxies(ctx context.Context, configs []CFAccessProxyConfig) error {
	if len(configs) == 0 {
		return errors.New("no proxy configurations provided")
	}

	var servers []*http.Server
	var wg sync.WaitGroup

	for _, cfg := range configs {
		proxyConfig := cfg // Capture range variable
		target, err := url.Parse(fmt.Sprintf("https://%s", proxyConfig.Hostname))
		if err != nil {
			logger.Error("proxy.Proxy", err, "Error parsing target URL for %s, skipping", proxyConfig.Hostname)
			continue
		}

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: proxyConfig.SkipTLS, MinVersion: tls.VersionTLS12},
		}

		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Transport = transport
		proxy.Director = func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			req.Header.Add("cf-access-token", proxyConfig.Token)
			logger.Debug("proxy.Proxy", "URL: %s", req.URL.String())
		}

		server := &http.Server{
			Addr:    fmt.Sprintf(":%d", proxyConfig.Port),
			Handler: proxy,
		}
		servers = append(servers, server)

		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("proxy.Proxy", "Starting proxy server on http://localhost%s, forwarding to %s", server.Addr, target.String())

			err := server.ListenAndServe()

			// If the error is that the port is in use, try again with a random port.
			if err != nil && errors.Is(err, syscall.EADDRINUSE) {
				randomPort := getRandomPort()
				logger.Warn("proxy.Proxy", "Port %d for target %s is in use. Retrying on port %d", proxyConfig.Port, target.String(), randomPort)
				server.Addr = fmt.Sprintf(":%d", randomPort)
				err = server.ListenAndServe() // Retry
			}

			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("proxy.Proxy", err, "Proxy for %s failed to start", proxyConfig.Hostname)
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
			logger.Error("proxy.Proxy", err, "Failed to gracefully shut down server at %s", s.Addr)
		}
	}

	wg.Wait()
	logger.Info("proxy.Proxy", "All proxies have been shut down.")
	return nil
}

func getRandomPort() int {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return r.Intn(randomPortRange) + randomPortStart
}
