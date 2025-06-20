package cmd

import (
	"fmt"
	"strings"

	"github.com/sbldevnet/cloudflared-proxy/internal"
	"github.com/spf13/cobra"
)

func Execute() *cobra.Command {
	var (
		endpoints []string
		skipTLS   bool
	)

	cmd := &cobra.Command{
		Use:   "cfproxy",
		Short: "Start a reverse proxy",
		Long:  "Start a reverse proxy to a Cloudflare Access application",
		RunE: func(cmd *cobra.Command, args []string) error {
			var proxyConfigs []internal.ProxyConfig

			for _, endpoint := range endpoints {
				parts := strings.Split(endpoint, ":")
				if len(parts) == 0 || len(parts) > 3 {
					return fmt.Errorf("invalid endpoint format '%s'. Expected format: [LOCAL_PORT:]HOSTNAME[:DEST_PORT]", endpoint)
				}

				var hostname string
				var localPort uint16 = internal.DefaultLocalPort
				var destPort uint16 = internal.DefaultDestinationPort

				switch len(parts) {
				// Only hostname provided
				case 1:
					hostname = parts[0]

				// Two parts could be either LOCAL_PORT:HOSTNAME or HOSTNAME:DEST_PORT
				case 2:
					// Try to parse first part as local port
					if _, err := fmt.Sscanf(parts[0], "%d", &localPort); err == nil {
						// First part was a valid port number, so it's LOCAL_PORT:HOSTNAME
						hostname = parts[1]
					} else {
						// First part wasn't a valid port, assume HOSTNAME:DEST_PORT format
						if _, err := fmt.Sscanf(parts[1], "%d", &destPort); err != nil {
							return fmt.Errorf("invalid destination port '%s': %v", parts[1], err)
						}
						hostname = parts[0]
					}

				// Full format: LOCAL_PORT:HOSTNAME:DEST_PORT
				case 3:
					if _, err := fmt.Sscanf(parts[0], "%d", &localPort); err != nil {
						return fmt.Errorf("invalid local port '%s': %v", parts[0], err)
					}
					hostname = parts[1]
					if _, err := fmt.Sscanf(parts[2], "%d", &destPort); err != nil {
						return fmt.Errorf("invalid destination port '%s': %v", parts[2], err)
					}
				}

				proxyConfigs = append(proxyConfigs, internal.ProxyConfig{
					Hostname:        hostname,
					DestinationPort: destPort,
					LocalPort:       localPort,
					SkipTLS:         skipTLS,
				})
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
