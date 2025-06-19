package cmd

import (
	"github.com/sbldevnet/cloudflared-proxy/internal"

	"github.com/spf13/cobra"
)

func Execute() *cobra.Command {
	var (
		hostname string
		port     uint16
		skipTLS  bool
	)

	cmd := &cobra.Command{
		Use:   "cfproxy",
		Short: "Start a reverse proxy",
		Long:  "Start a reverse proxy to a Cloudflare Access application",
		RunE: func(cmd *cobra.Command, args []string) error {

			// From this code I will pass the application to the internal package.

			proxyConfig := internal.ProxyConfig{
				Address: hostname,
				Port:    port,
				SkipTLS: skipTLS,
			}

			err := internal.ProxyCFAccess([]internal.ProxyConfig{proxyConfig})
			if err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&hostname, "hostname", "n", "", "Destination hostname to proxy")
	cmd.Flags().Uint16VarP(&port, "port", "p", 8888, "Local port to proxy")
	cmd.Flags().BoolVarP(&skipTLS, "skip-tls", "s", false, "Skip TLS verification")

	cmd.MarkFlagRequired("hostname")

	return cmd
}
