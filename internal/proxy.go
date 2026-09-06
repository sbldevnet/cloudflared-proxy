package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/sbldevnet/cloudflared-proxy/internal/config"
	"github.com/sbldevnet/cloudflared-proxy/pkg/cloudflared"
	"github.com/sbldevnet/cloudflared-proxy/pkg/proxy"
)

type ProxyService interface {
	GetCloudflareAccessTokenForApp(log *slog.Logger, url string) (string, error)
	StartMultipleProxies(ctx context.Context, log *slog.Logger, configs []proxy.CFAccessProxyConfig) error
}

type LiveProxyService struct{}

func NewLiveProxyService() *LiveProxyService {
	return &LiveProxyService{}
}

func (s *LiveProxyService) GetCloudflareAccessTokenForApp(log *slog.Logger, url string) (string, error) {
	return cloudflared.GetCloudflareAccessTokenForApp(log, url)
}

func (s *LiveProxyService) StartMultipleProxies(ctx context.Context, log *slog.Logger, configs []proxy.CFAccessProxyConfig) error {
	return proxy.StartMultipleProxies(ctx, log, configs)
}

func ProxyCFAccess(ctx context.Context, log *slog.Logger, configs []config.ProxyConfig, service ProxyService) error {
	proxyConfigs := make([]proxy.CFAccessProxyConfig, len(configs))
	for i, config := range configs {
		token, err := service.GetCloudflareAccessTokenForApp(log, config.GetAddress())
		if err != nil {
			if errors.Is(err, cloudflared.ErrAccessAppNotFound) {
				log.Warn("access application not found, continuing without authentication", "address", config.GetAddress())
			} else {
				return err
			}
		}

		url, err := url.Parse(fmt.Sprintf("https://%s", config.GetAddress()))
		if err != nil {
			return fmt.Errorf("error parsing target URL for %s: %w", config.GetAddress(), err)
		}

		proxyConfigs[i] = proxy.CFAccessProxyConfig{
			Url:       url,
			LocalPort: config.LocalPort,
			Token:     token,
			SkipTLS:   config.SkipTLS,
		}
	}

	return service.StartMultipleProxies(ctx, log, proxyConfigs)
}
