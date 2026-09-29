// Command cloudflared-proxy runs local reverse proxies to Cloudflare Access
// applications, authenticating through cloudflared.
//
// Usage:
//
//	cloudflared-proxy [flags]
//
// To use it as a library, see the packages
// [github.com/sbldevnet/cloudflared-proxy/proxy] and
// [github.com/sbldevnet/cloudflared-proxy/config].
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sbldevnet/cloudflared-proxy/internal/cmd"
)

func main() {
	rootCmd := cmd.Execute()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
