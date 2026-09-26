package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/sbldevnet/cloudflared-proxy/cloudflared"
	"github.com/sbldevnet/cloudflared-proxy/config"
	"github.com/sbldevnet/cloudflared-proxy/logger"
)

// Runner starts reverse proxies to Cloudflare Access applications.
type Runner struct {
	tokenFetcher func(ctx context.Context, url string) (string, error)
	newServer    func(addr string, handler http.Handler) server
}

// Option configures a Runner.
type Option func(*Runner)

// WithTokenFetcher replaces how Cloudflare Access tokens are obtained.
// A nil f is ignored and the default fetcher is kept.
//
// A fetcher receives the context of Run and the target address (host:port), and
// returns its token. It should abort when the context is cancelled. It must return cloudflared.ErrAccessAppNotFound to mean "no Access application
// exists at this address": the Runner then continues without a token.
func WithTokenFetcher(f func(ctx context.Context, url string) (string, error)) Option {
	return func(r *Runner) {
		if f != nil {
			r.tokenFetcher = f
		}
	}
}

// New returns a ready-to-use Runner; by default it uses the real cloudflared binary.
func New(opts ...Option) *Runner {
	r := &Runner{
		tokenFetcher: cloudflared.CloudflareAccessTokenForApp,
		newServer:    newHTTPServer,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run fetches Cloudflare Access tokens and starts one server per config,
// blocking until ctx is cancelled.
func (r *Runner) Run(ctx context.Context, configs []config.ProxyConfig) error {
	proxyConfigs := make([]accessProxyConfig, len(configs))
	for i, cfg := range configs {
		token, err := r.tokenFetcher(ctx, cfg.Address())
		if err != nil {
			if !errors.Is(err, cloudflared.ErrAccessAppNotFound) {
				if ctxErr := ctx.Err(); ctxErr != nil {
					err = ctxErr
				}
				return fmt.Errorf("fetching Access token for %s: %w", cfg.Address(), err)
			}
			logger.Warn("proxy.Runner", "Access application not found at %s, continuing without authentication", cfg.Address())
		}

		target, err := url.Parse(fmt.Sprintf("https://%s", cfg.Address()))
		if err != nil {
			return fmt.Errorf("error parsing target URL for %s: %w", cfg.Address(), err)
		}

		proxyConfigs[i] = accessProxyConfig{
			url:       target,
			localPort: cfg.LocalPort,
			token:     token,
			skipTLS:   cfg.SkipTLS,
		}
	}

	return r.serve(ctx, proxyConfigs)
}
