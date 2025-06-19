package proxy

import (
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

type ProxyConfig struct {
	Hostname string
	Token    string
	Port     uint16
	SkipTLS  bool
}

func StartMultipleProxies(configs []ProxyConfig) error {
	if len(configs) == 0 {
		return errors.New("no proxy configurations provided")
	}

	// Multiple proxies, run them concurrently
	var wg sync.WaitGroup
	errChan := make(chan error, len(configs))

	for _, cfg := range configs {
		wg.Add(1)
		go func(config ProxyConfig) {
			defer wg.Done()
			if err := startReverseProxy(config.Hostname, config.Token, config.Port, config.SkipTLS); err != nil {
				errChan <- fmt.Errorf("proxy for %s:%d failed: %w", config.Hostname, config.Port, err)
			}
		}(cfg)
	}

	// Wait for all proxies to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("some proxies failed: %v", errors)
	}

	return nil
}

func startReverseProxy(hostname, token string, port uint16, skipTLS bool) error {
	target, err := url.Parse(fmt.Sprintf("https://%s", hostname))
	if err != nil {
		logger.Error("proxy.Proxy", err, "Error parsing target URL")
		return err
	}

	// Create custom transport with TLS config
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLS,
			MinVersion:         tls.VersionTLS12,
		},
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transport
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		req.Header.Add("cf-access-token", token)

		// Log the final headers
		logger.Debug("proxy.Proxy", "URL: %s", req.URL.String())
		for name, values := range req.Header {
			for _, value := range values {
				logger.Debug("proxy.Proxy", "Header: %s: %s", name, value)
			}
		}
	}

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: proxy,
	}

	logger.Info("proxy.Proxy", "Starting proxy server on http://localhost%s, forwarding to %s", server.Addr, target.String())

	err = server.ListenAndServe()
	if err != nil && errors.Is(err, syscall.EADDRINUSE) {
		randomPort := getRandomPort()
		server.Addr = fmt.Sprintf(":%d", randomPort)
		logger.Warn("proxy.Proxy", "Port %d in use, retrying with random port http://localhost%s", port, server.Addr)
		return server.ListenAndServe()
	}
	logger.Error("proxy.Proxy", err, "Error starting proxy server")
	return err
}

func getRandomPort() int {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return r.Intn(1000) + 8000
}
