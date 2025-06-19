package internal

import (
	"errors"

	"github.com/sbldevnet/cloudflared-proxy/pkg/cloudflared"
	"github.com/sbldevnet/cloudflared-proxy/pkg/logger"
	"github.com/sbldevnet/cloudflared-proxy/pkg/proxy"
)

type ProxyConfig struct {
	Address string
	Port    uint16
	SkipTLS bool
}

func ProxyCFAccess(configs []ProxyConfig) error {

	proxyConfigs := make([]proxy.ProxyConfig, len(configs))
	for i, config := range configs {
		proxyConfigs[i] = proxy.ProxyConfig{
			Hostname: config.Address,
			Port:     config.Port,
			SkipTLS:  config.SkipTLS,
		}
	}

	for i, proxyConfig := range proxyConfigs {
		token, err := cloudflared.GetCloudflareAccessTokenForApp(proxyConfig.Hostname)
		if err != nil {
			if errors.Is(err, cloudflared.ErrAccessAppNotFound) {
				logger.Warn("proxy.ProxyCFAccess", "Access application not found at %s, continuing without authentication", proxyConfig.Hostname)
				continue
			}
			return err
		}
		proxyConfigs[i].Token = token
	}

	err := proxy.StartMultipleProxies(proxyConfigs)
	if err != nil {
		return err
	}

	return nil
}
