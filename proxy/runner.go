package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/sbldevnet/cloudflared-proxy/cloudflared"
	"github.com/sbldevnet/cloudflared-proxy/config"
	"github.com/sbldevnet/cloudflared-proxy/internal/listen"
)

// Runner starts reverse proxies to Cloudflare Access applications.
type Runner struct {
	// tokenFetcher returns cloudflared.ErrAccessAppNotFound when the address has
	// no Access application; Run then continues without a token.
	tokenFetcher func(ctx context.Context, url string) (string, error)
	newServer    func(addr string, handler http.Handler) server
	log          *slog.Logger
}

// Option configures a Runner.
type Option func(*Runner)

// WithLogger sets the logger used by the Runner. A nil logger is ignored.
func WithLogger(l *slog.Logger) Option {
	return func(r *Runner) {
		if l != nil {
			r.log = l
		}
	}
}

// New returns a ready-to-use Runner; by default it uses the real cloudflared binary.
func New(opts ...Option) *Runner {
	r := &Runner{
		tokenFetcher: cloudflared.CloudflareAccessTokenForApp,
		newServer:    newHTTPServer,
		log:          slog.Default(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run fetches Cloudflare Access tokens and starts one server per config,
// blocking until ctx is cancelled.
func (r *Runner) Run(ctx context.Context, configs []config.ProxyConfig) error {
	listens := make([]string, len(configs))
	for i, cfg := range configs {
		listens[i] = cfg.Listen
		if listens[i] == "" {
			listens[i] = listen.Loopback
		}
		if err := listen.Validate(listens[i]); err != nil {
			return fmt.Errorf("proxy %s: %w", cfg.Hostname, err)
		}
	}

	proxyConfigs := make([]accessProxyConfig, len(configs))
	for i, cfg := range configs {
		r.log.Debug("fetching Access token", "address", cfg.Address())
		token, err := r.tokenFetcher(ctx, cfg.Address())
		if err != nil {
			if !errors.Is(err, cloudflared.ErrAccessAppNotFound) {
				if ctxErr := ctx.Err(); ctxErr != nil {
					err = ctxErr
				}
				return fmt.Errorf("fetching Access token for %s: %w", cfg.Address(), err)
			}
			r.log.Warn("Access application not found, continuing without authentication", "address", cfg.Address())
		}

		target, err := url.Parse(fmt.Sprintf("https://%s", cfg.Address()))
		if err != nil {
			return fmt.Errorf("error parsing target URL for %s: %w", cfg.Address(), err)
		}

		proxyConfigs[i] = accessProxyConfig{
			url:       target,
			localPort: cfg.LocalPort,
			listen:    listens[i],
			token:     token,
			skipTLS:   cfg.SkipTLS,
		}
	}

	return r.serve(ctx, proxyConfigs)
}
