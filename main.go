package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sbldevnet/cloudflared-proxy/cmd"
)

func main() {
	rootCmd := cmd.Execute()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
