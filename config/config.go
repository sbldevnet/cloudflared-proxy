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

// ParseEndpointString parses a string representation of a proxy endpoint
// into a ProxyConfig struct. The format is [LOCAL_PORT:]HOSTNAME[:DEST_PORT].
//
// With two colon-separated parts the input is ambiguous: it can be
// LOCAL_PORT:HOSTNAME or HOSTNAME:DEST_PORT. The first part is treated as a
// local port if it starts with a number, and as a hostname otherwise. As a
// consequence, a purely numeric hostname cannot be combined with only a
// destination port: "12345:443" yields local port 12345 and hostname "443",
// not hostname "12345" with destination port 443. A single part is always
// taken as the hostname, so "12345" is a valid numeric hostname.
//
// To use a numeric hostname with a destination port, always give the full
// three-part form, e.g. "8888:12345:443".
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
func (c *ProxyConfig) Address() string {
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
