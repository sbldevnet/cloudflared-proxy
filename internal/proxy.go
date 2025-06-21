package internal

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/sbldevnet/cloudflared-proxy/internal/config"
	"github.com/sbldevnet/cloudflared-proxy/pkg/cloudflared"
	"github.com/sbldevnet/cloudflared-proxy/pkg/logger"
	"github.com/sbldevnet/cloudflared-proxy/pkg/proxy"
)

func ProxyCFAccess(ctx context.Context, configs []config.ProxyConfig) error {
	proxyConfigs := make([]proxy.CFAccessProxyConfig, len(configs))
	for i, config := range configs {
		token, err := cloudflared.GetCloudflareAccessTokenForApp(config.GetAddress())
		if err != nil {
			if errors.Is(err, cloudflared.ErrAccessAppNotFound) {
				logger.Warn("proxy.ProxyCFAccess", "Access application not found at %s, continuing without authentication", config.GetAddress())
			} else {
				return err
			}
		}

		url, err := url.Parse(fmt.Sprintf("https://%s", config.GetAddress()))
		if err != nil {
			logger.Error("proxy.ProxyCFAccess", err, "Error parsing target URL for %s, skipping", config.GetAddress())
			return err
		}

		proxyConfigs[i] = proxy.CFAccessProxyConfig{
			Url:       url,
			LocalPort: config.LocalPort,
			Token:     token,
			SkipTLS:   config.SkipTLS,
		}
	}

	return proxy.StartMultipleProxies(ctx, proxyConfigs)
}
