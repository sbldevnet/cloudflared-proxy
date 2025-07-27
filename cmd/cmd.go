package cmd

import (
	"fmt"
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig(cfgFile)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(endpoints) == 0 && !viper.IsSet("proxies") {
				cmd.Help()
				return nil
			}

			if len(endpoints) > 0 && viper.IsSet("proxies") {
				return fmt.Errorf("cannot specify endpoints via flags when a config file is used")
			}

			var proxyConfigs []config.ProxyConfig

			if len(endpoints) > 0 {
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
				var cfg config.Config
				if err := viper.Unmarshal(&cfg); err != nil {
					return fmt.Errorf("unable to decode into struct, %v", err)
				}
				proxyConfigs = cfg.Proxies
				config.SetDefaults(proxyConfigs)
			}

			logger.Debug("cmd.Run", "Starting %d proxies", len(proxyConfigs))
			logger.Debug("cmd.Run", "Proxy configs: %v", proxyConfigs)

			err := internal.ProxyCFAccess(cmd.Context(), proxyConfigs, internal.NewLiveProxyService())
			if err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.config/cloudflared-proxy/config.yaml)")
	cmd.Flags().StringSliceVarP(&endpoints, "endpoints", "e", []string{}, "List of endpoints to proxy in format [LOCAL_PORT:]HOSTNAME[:DEST_PORT]")
	cmd.Flags().BoolVarP(&skipTLS, "skip-tls", "s", false, "Skip TLS verification")

	return cmd
}

func initConfig(cfgFile string) error {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		viper.AddConfigPath(filepath.Join(home, ".config", "cloudflared-proxy"))
		viper.SetConfigName("config")
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error if not explicitly provided
			if cfgFile != "" {
				return err
			}
		} else {
			return err
		}
	}

	if cfgFile != "" {
		logger.Debug("cmd.initConfig", "Config file path: %s", viper.ConfigFileUsed())
	}

	return nil
}
