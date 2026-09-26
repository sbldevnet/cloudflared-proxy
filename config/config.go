package config

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	DefaultLocalPort       uint16 = 8888
	DefaultDestinationPort uint16 = 443

	minPort = 1
	maxPort = 65535
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
// A part is a port only if the whole token is a number from 1 to 65535, so
// "8x8.com" is a hostname. With two parts, the first is the local port if it is
// a port and the second is the destination port otherwise. This leaves one
// ambiguity: "12345:443" is local port 12345 and hostname "443". Use the
// three-part form ("8888:12345:443") for a numeric hostname.
func ParseEndpointString(endpoint string) (*ProxyConfig, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint cannot be empty. Expected format: [LOCAL_PORT:]HOSTNAME[:DEST_PORT]")
	}

	parts := strings.Split(endpoint, ":")
	if len(parts) > 3 {
		return nil, fmt.Errorf("invalid endpoint format '%s'. Expected format: [LOCAL_PORT:]HOSTNAME[:DEST_PORT]", endpoint)
	}

	var hostname string
	var localPort = DefaultLocalPort
	var destPort = DefaultDestinationPort

	switch len(parts) {
	case 1:
		hostname = parts[0]
	case 2:
		if port, err := parsePort(parts[0]); err == nil {
			localPort = port
			hostname = parts[1]
		} else if port, err := parsePort(parts[1]); err == nil {
			destPort = port
			hostname = parts[0]
		} else {
			return nil, fmt.Errorf("invalid endpoint '%s': neither '%s' nor '%s' is a valid port (must be a number between %d and %d)", endpoint, parts[0], parts[1], minPort, maxPort)
		}
	case 3:
		var err error
		if localPort, err = parsePort(parts[0]); err != nil {
			return nil, fmt.Errorf("invalid local port '%s': must be a number between %d and %d", parts[0], minPort, maxPort)
		}
		hostname = parts[1]
		if destPort, err = parsePort(parts[2]); err != nil {
			return nil, fmt.Errorf("invalid destination port '%s': must be a number between %d and %d", parts[2], minPort, maxPort)
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

func parsePort(s string) (uint16, error) {
	port, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, err
	}
	if port < minPort {
		return 0, fmt.Errorf("port %d out of range", port)
	}
	return uint16(port), nil
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
