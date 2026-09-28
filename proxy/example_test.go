package proxy_test

import (
	"context"
	"log/slog"

	"github.com/sbldevnet/cloudflared-proxy/config"
	"github.com/sbldevnet/cloudflared-proxy/proxy"
)

func ExampleRunner_Run() {
	ctx := context.Background()

	_ = proxy.New().Run(ctx, []config.ProxyConfig{
		{Hostname: "example.com", DestinationPort: 443, LocalPort: 8888},
	})
}

func ExampleWithLogger() {
	ctx := context.Background()
	logger := slog.Default()

	_ = proxy.New(proxy.WithLogger(logger)).Run(ctx, []config.ProxyConfig{
		{Hostname: "example.com", DestinationPort: 443, LocalPort: 8888},
	})
}
