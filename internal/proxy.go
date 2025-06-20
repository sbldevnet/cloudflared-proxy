package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/sbldevnet/cloudflared-proxy/pkg/cloudflared"
	"github.com/sbldevnet/cloudflared-proxy/pkg/logger"
	"github.com/sbldevnet/cloudflared-proxy/pkg/proxy"
)

const (
	DefaultLocalPort       uint16 = 8888
	DefaultDestinationPort uint16 = 443
)

type ProxyConfig struct {
	Hostname        string
	DestinationPort uint16
	LocalPort       uint16
	SkipTLS         bool
}

func ProxyCFAccess(ctx context.Context, configs []ProxyConfig) error {
	proxyConfigs := make([]proxy.CFAccessProxyConfig, len(configs))
	for i, config := range configs {
		token, err := cloudflared.GetCloudflareAccessTokenForApp(config.getAddress())
		if err != nil {
			if errors.Is(err, cloudflared.ErrAccessAppNotFound) {
				logger.Warn("proxy.ProxyCFAccess", "Access application not found at %s, continuing without authentication", config.getAddress())
			} else {
				return err
			}
		}

		proxyConfigs[i] = proxy.CFAccessProxyConfig{
			Hostname: config.Hostname,
			Port:     config.LocalPort,
			Token:    token,
			SkipTLS:  config.SkipTLS,
		}
	}

	return proxy.StartMultipleProxies(ctx, proxyConfigs)
}

func (c ProxyConfig) getAddress() string {
	return fmt.Sprintf("%s:%d", c.Hostname, c.DestinationPort)
}
