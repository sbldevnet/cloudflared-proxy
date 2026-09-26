package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/sbldevnet/cloudflared-proxy/cloudflared"
	"github.com/sbldevnet/cloudflared-proxy/config"
	"github.com/sbldevnet/cloudflared-proxy/logger"
)

type ProxyService interface {
	CloudflareAccessTokenForApp(url string) (string, error)
	StartMultipleProxies(ctx context.Context, configs []CFAccessProxyConfig) error
}

type LiveProxyService struct{}

func NewLiveProxyService() *LiveProxyService {
	return &LiveProxyService{}
}

func (s *LiveProxyService) CloudflareAccessTokenForApp(url string) (string, error) {
	return cloudflared.CloudflareAccessTokenForApp(url)
}

func (s *LiveProxyService) StartMultipleProxies(ctx context.Context, configs []CFAccessProxyConfig) error {
	return StartMultipleProxies(ctx, configs)
}

func ProxyCFAccess(ctx context.Context, configs []config.ProxyConfig, service ProxyService) error {
	proxyConfigs := make([]CFAccessProxyConfig, len(configs))
	for i, config := range configs {
		token, err := service.CloudflareAccessTokenForApp(config.Address())
		if err != nil {
			if errors.Is(err, cloudflared.ErrAccessAppNotFound) {
				logger.Warn("proxy.ProxyCFAccess", "Access application not found at %s, continuing without authentication", config.Address())
			} else {
				return err
			}
		}

		url, err := url.Parse(fmt.Sprintf("https://%s", config.Address()))
		if err != nil {
			return fmt.Errorf("error parsing target URL for %s: %w", config.Address(), err)
		}

		proxyConfigs[i] = CFAccessProxyConfig{
			Url:       url,
			LocalPort: config.LocalPort,
			Token:     token,
			SkipTLS:   config.SkipTLS,
		}
	}

	return service.StartMultipleProxies(ctx, proxyConfigs)
}
