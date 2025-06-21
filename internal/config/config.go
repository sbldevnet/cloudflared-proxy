package config

import (
	"fmt"
	"strings"
)

const (
	DefaultLocalPort       uint16 = 8888
	DefaultDestinationPort uint16 = 443
)

type ProxyConfig struct {
	Hostname        string `mapstructure:"hostname"`
	DestinationPort uint16 `mapstructure:"destinationPort"`
	LocalPort       uint16 `mapstructure:"localPort"`
	SkipTLS         bool   `mapstructure:"skipTLS"`
}

type Config struct {
	Proxies []ProxyConfig `mapstructure:"proxies"`
}

// Parses a string representation of a proxy endpoint
// into a ProxyConfig struct. The format is [LOCAL_PORT:]HOSTNAME[:DEST_PORT].
func ParseEndpointString(endpoint string) (*ProxyConfig, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint cannot be empty. Expected format: [LOCAL_PORT:]HOSTNAME[:DEST_PORT]")
	}

	parts := strings.Split(endpoint, ":")
	if len(parts) == 0 || len(parts) > 3 {
		return nil, fmt.Errorf("invalid endpoint format '%s'. Expected format: [LOCAL_PORT:]HOSTNAME[:DEST_PORT]", endpoint)
	}

	var hostname string
	var localPort = DefaultLocalPort
	var destPort = DefaultDestinationPort

	switch len(parts) {
	case 1: // Only hostname provided
		hostname = parts[0]
	case 2: // Two parts could be either LOCAL_PORT:HOSTNAME or HOSTNAME:DEST_PORT
		// Try to parse first part as local port
		if _, err := fmt.Sscanf(parts[0], "%d", &localPort); err == nil {
			hostname = parts[1]
		} else { // Assume HOSTNAME:DEST_PORT
			if _, err := fmt.Sscanf(parts[1], "%d", &destPort); err != nil {
				return nil, fmt.Errorf("invalid destination port '%s': %v", parts[1], err)
			}
			hostname = parts[0]
		}
	case 3: // Full format: LOCAL_PORT:HOSTNAME:DEST_PORT
		if _, err := fmt.Sscanf(parts[0], "%d", &localPort); err != nil {
			return nil, fmt.Errorf("invalid local port '%s': %v", parts[0], err)
		}
		hostname = parts[1]
		if _, err := fmt.Sscanf(parts[2], "%d", &destPort); err != nil {
			return nil, fmt.Errorf("invalid destination port '%s': %v", parts[2], err)
		}
	}

	if hostname == "" {
		return nil, fmt.Errorf("hostname cannot be empty")
	}

	return &ProxyConfig{
		Hostname:        hostname,
		LocalPort:       localPort,
		DestinationPort: destPort,
	}, nil
}

// Returns the full address of the target application.
func (c *ProxyConfig) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Hostname, c.DestinationPort)
}

// Sets the default values, if not provided, for the proxy configuration.
func SetDefaults(proxies []ProxyConfig) {
	for i := range proxies {
		if proxies[i].LocalPort == 0 {
			proxies[i].LocalPort = DefaultLocalPort
		}
		if proxies[i].DestinationPort == 0 {
			proxies[i].DestinationPort = DefaultDestinationPort
		}
	}
}
