package main

import (
	"os"

	"github.com/sbldevnet/cloudflared-proxy/cmd"
)

func main() {
	rootCmd := cmd.Execute()

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
