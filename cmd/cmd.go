package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/sbldevnet/cloudflared-proxy/internal"
	"github.com/sbldevnet/cloudflared-proxy/internal/config"
	"github.com/sbldevnet/cloudflared-proxy/pkg/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Execute() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "cloudflared-proxy",
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
		cfgFile   string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start reverse proxies",
		Long:  "Start reverse proxies to Cloudflare Access applications",
		RunE: func(cmd *cobra.Command, args []string) error {
			hasEndpoints := cmd.Flags().Changed("endpoints")
			hasConfig := cmd.Flags().Changed("config")

			// Check for conflicting flags
			if hasConfig && hasEndpoints {
				return fmt.Errorf("cannot specify both --config and --endpoints flags")
			}

			var proxyConfigs []config.ProxyConfig

			log := logger.New()

			// If endpoints provided
			if hasEndpoints {
				proxyConfigs = make([]config.ProxyConfig, len(endpoints))
				for i, endpoint := range endpoints {
					proxy, err := config.ParseEndpointString(endpoint)
					if err != nil {
						return err
					}
					proxy.SkipTLS = skipTLS
					proxyConfigs[i] = *proxy
				}
			} else {
				// If config or default provided
				if err := initConfig(log, cfgFile); err != nil {
					return err
				}

				if !viper.IsSet("proxies") {
					return fmt.Errorf("no proxies defined in config file")
				}

				var cfg config.Config
				if err := viper.Unmarshal(&cfg); err != nil {
					return fmt.Errorf("unable to decode into struct, %v", err)
				}
				config.SetDefaults(cfg.Proxies)
				proxyConfigs = cfg.Proxies
			}

			log.Debug("starting proxies", "count", len(proxyConfigs))
			log.Debug("proxy configs", "configs", proxyConfigs)

			return internal.ProxyCFAccess(cmd.Context(), log, proxyConfigs, internal.NewLiveProxyService())
		},
	}

	cmd.Flags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.config/cloudflared-proxy/config.yaml)")
	cmd.Flags().StringSliceVarP(&endpoints, "endpoints", "e", []string{}, "List of endpoints to proxy in format [LOCAL_PORT:]HOSTNAME[:DEST_PORT]")
	cmd.Flags().BoolVarP(&skipTLS, "skip-tls", "s", false, "Skip TLS verification")

	return cmd
}

func initConfig(log *slog.Logger, cfgFile string) error {
	if cfgFile != "" {
		// Explicit config file
		log.Debug("using explicit config file", "path", cfgFile)
		viper.SetConfigFile(cfgFile)
	} else {
		// Try default config location
		log.Debug("no explicit config file, using default location")
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		viper.AddConfigPath(filepath.Join(home, ".config", "cloudflared-proxy"))
		viper.SetConfigName("config")
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			if cfgFile != "" {
				return fmt.Errorf("config file not found: %s", cfgFile)
			}
			log.Debug("default config file not found")
			return fmt.Errorf("no config file or endpoints provided")
		}
		return err
	}

	log.Debug("config file loaded", "path", viper.ConfigFileUsed())
	return nil
}
