package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/sbldevnet/cloudflared-proxy/config"
	"github.com/sbldevnet/cloudflared-proxy/internal/listen"
	"github.com/sbldevnet/cloudflared-proxy/proxy"

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
		endpoints  []string
		skipTLS    bool
		cfgFile    string
		listenAddr string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start reverse proxies",
		Long:  "Start reverse proxies to Cloudflare Access applications",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := newLogger()
			hasEndpoints := cmd.Flags().Changed("endpoints")
			hasConfig := cmd.Flags().Changed("config")

			// Check for conflicting flags
			if hasConfig && hasEndpoints {
				return fmt.Errorf("cannot specify both --config and --endpoints flags")
			}

			var proxyConfigs []config.ProxyConfig

			// If endpoints provided
			if hasEndpoints {
				proxyConfigs = make([]config.ProxyConfig, len(endpoints))
				for i, endpoint := range endpoints {
					proxy, err := config.ParseEndpointString(endpoint)
					if err != nil {
						return err
					}
					proxyConfigs[i] = *proxy
				}
			} else {
				// If config or default provided
				if err := initConfig(log, cfgFile); err != nil {
					return err
				}
				log.Debug("config file loaded", "path", viper.ConfigFileUsed())

				if !viper.IsSet("proxies") {
					return fmt.Errorf("no proxies defined in config file")
				}

				var cfg config.Config
				if err := viper.Unmarshal(&cfg); err != nil {
					return fmt.Errorf("unable to decode into struct, %v", err)
				}
				config.SetDefaults(cfg.Proxies)
				proxyConfigs = cfg.Proxies
				applyListenDefault(proxyConfigs, cfg.Listen)
			}

			if cmd.Flags().Changed("listen") {
				if err := listen.Validate(listenAddr); err != nil {
					return err
				}
				overrideListen(proxyConfigs, listenAddr)
			}
			if err := validateListen(proxyConfigs); err != nil {
				return err
			}

			if cmd.Flags().Changed("skip-tls") {
				overrideSkipTLS(proxyConfigs, skipTLS)
			}
			warnSkipTLS(log, proxyConfigs)

			// Arguments are valid from here on; runtime errors should not print usage.
			cmd.SilenceUsage = true

			log.Debug("starting proxies", "count", len(proxyConfigs), "configs", proxyConfigs)

			return proxy.New(proxy.WithLogger(log)).Run(cmd.Context(), proxyConfigs)
		},
	}

	cmd.Flags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.config/cloudflared-proxy/config.yaml)")
	cmd.Flags().StringSliceVarP(&endpoints, "endpoints", "e", []string{}, "List of endpoints to proxy in format [LOCAL_PORT:]HOSTNAME[:DEST_PORT]")
	cmd.Flags().BoolVarP(&skipTLS, "skip-tls", "s", false, "Skip TLS verification for every proxy, overriding the config file (--skip-tls=false forces verification on)")

	cmd.Flags().StringVar(&listenAddr, "listen", "", "IP address to listen on for every proxy, overriding the config file (default 127.0.0.1)")

	return cmd
}

// applyListenDefault sets the top-level listen address on proxies that do not
// define their own.
func applyListenDefault(proxies []config.ProxyConfig, def string) {
	for i := range proxies {
		if proxies[i].Listen == "" {
			proxies[i].Listen = def
		}
	}
}

// overrideListen sets addr on every proxy, taking precedence over any config
// file value.
func overrideListen(proxies []config.ProxyConfig, addr string) {
	for i := range proxies {
		proxies[i].Listen = addr
	}
}

// overrideSkipTLS sets skip on every proxy, taking precedence over any config
// file value.
func overrideSkipTLS(proxies []config.ProxyConfig, skip bool) {
	for i := range proxies {
		proxies[i].SkipTLS = skip
	}
}

// warnSkipTLS logs one warning for each proxy whose upstream certificate is
// not verified.
func warnSkipTLS(log *slog.Logger, proxies []config.ProxyConfig) {
	for _, p := range proxies {
		if p.SkipTLS {
			log.Warn("TLS certificate verification is disabled", "hostname", p.Hostname)
		}
	}
}

// validateListen checks every explicitly set listen address. Empty values are
// left for the runner to default to loopback.
func validateListen(proxies []config.ProxyConfig) error {
	for _, p := range proxies {
		if p.Listen == "" {
			continue
		}
		if err := listen.Validate(p.Listen); err != nil {
			return err
		}
	}
	return nil
}

func initConfig(log *slog.Logger, cfgFile string) error {
	var dir string
	if cfgFile != "" {
		log.Debug("using explicit config file", "path", cfgFile)
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".config", "cloudflared-proxy")
		log.Debug("using default config location", "path", dir)
		viper.AddConfigPath(dir)
		viper.SetConfigName("config")
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			if cfgFile != "" {
				return fmt.Errorf("config file not found: %s", cfgFile)
			}
			log.Debug("default config file not found", "path", dir)
			return fmt.Errorf("no config file or endpoints provided")
		}
		return err
	}

	return nil
}
