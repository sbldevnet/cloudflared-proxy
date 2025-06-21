package cmd

import (
	"github.com/sbldevnet/cloudflared-proxy/internal"
	"github.com/sbldevnet/cloudflared-proxy/internal/config"
	"github.com/spf13/cobra"
)

func Execute() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "cfproxy",
		Short: "A reverse proxy tool for Cloudflare Access applications.",
	}

	cmd.AddCommand(Run())
	cmd.AddCommand(Version())

	return cmd
}

func Run() *cobra.Command {
	var (
		endpoints []string
		skipTLS   bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start reverse proxies",
		Long:  "Start reverse proxies to Cloudflare Access applications",
		RunE: func(cmd *cobra.Command, args []string) error {
			proxyConfigs := make([]config.ProxyConfig, len(endpoints))

			for i, endpoint := range endpoints {
				proxy, err := config.ParseProxyString(endpoint)
				if err != nil {
					return err
				}
				proxy.SkipTLS = skipTLS
				proxyConfigs[i] = *proxy
			}

			err := internal.ProxyCFAccess(cmd.Context(), proxyConfigs)
			if err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringSliceVarP(&endpoints, "endpoints", "e", []string{}, "List of endpoints to proxy in format [LOCAL_PORT:]HOSTNAME[:DEST_PORT]")
	cmd.Flags().BoolVarP(&skipTLS, "skip-tls", "s", false, "Skip TLS verification")

	cmd.MarkFlagRequired("endpoints")

	return cmd
}
