package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseEndpointString(t *testing.T) {
	testCases := []struct {
		name           string
		endpoint       string
		expectedConfig *ProxyConfig
		expectedErr    error
	}{
		{
			name:     "hostname only",
			endpoint: "myapp.example.com",
			expectedConfig: &ProxyConfig{
				Hostname:        "myapp.example.com",
				LocalPort:       DefaultLocalPort,
				DestinationPort: DefaultDestinationPort,
			},
		},
		{
			name:     "local port and hostname",
			endpoint: "9000:myapp.example.com",
			expectedConfig: &ProxyConfig{
				Hostname:        "myapp.example.com",
				LocalPort:       9000,
				DestinationPort: DefaultDestinationPort,
			},
		},
		{
			name:     "hostname and destination port",
			endpoint: "myapp.example.com:8443",
			expectedConfig: &ProxyConfig{
				Hostname:        "myapp.example.com",
				LocalPort:       DefaultLocalPort,
				DestinationPort: 8443,
			},
		},
		{
			name:     "full format",
			endpoint: "9000:myapp.example.com:8443",
			expectedConfig: &ProxyConfig{
				Hostname:        "myapp.example.com",
				LocalPort:       9000,
				DestinationPort: 8443,
			},
		},
		{
			name:     "numeric hostname alone is a hostname",
			endpoint: "12345",
			expectedConfig: &ProxyConfig{
				Hostname:        "12345",
				LocalPort:       DefaultLocalPort,
				DestinationPort: DefaultDestinationPort,
			},
		},
		{
			name:     "numeric hostname with destination port is read as local port and hostname",
			endpoint: "12345:443",
			expectedConfig: &ProxyConfig{
				Hostname:        "443",
				LocalPort:       12345,
				DestinationPort: DefaultDestinationPort,
			},
		},
		{
			name:     "numeric hostname in full format",
			endpoint: "8888:12345:443",
			expectedConfig: &ProxyConfig{
				Hostname:        "12345",
				LocalPort:       8888,
				DestinationPort: 443,
			},
		},
		{
			name:     "digit-leading hostname",
			endpoint: "8x8.com",
			expectedConfig: &ProxyConfig{
				Hostname:        "8x8.com",
				LocalPort:       DefaultLocalPort,
				DestinationPort: DefaultDestinationPort,
			},
		},
		{
			name:     "digit-leading hostname with destination port",
			endpoint: "8x8.com:443",
			expectedConfig: &ProxyConfig{
				Hostname:        "8x8.com",
				LocalPort:       DefaultLocalPort,
				DestinationPort: 443,
			},
		},
		{
			name:     "digit-and-letter hostname with destination port",
			endpoint: "3m.com:443",
			expectedConfig: &ProxyConfig{
				Hostname:        "3m.com",
				LocalPort:       DefaultLocalPort,
				DestinationPort: 443,
			},
		},
		{
			name:     "digit-leading hostname with alternate destination port",
			endpoint: "1password.com:8443",
			expectedConfig: &ProxyConfig{
				Hostname:        "1password.com",
				LocalPort:       DefaultLocalPort,
				DestinationPort: 8443,
			},
		},
		{
			name:     "digit-leading hostname without dots",
			endpoint: "9gag.com",
			expectedConfig: &ProxyConfig{
				Hostname:        "9gag.com",
				LocalPort:       DefaultLocalPort,
				DestinationPort: DefaultDestinationPort,
			},
		},
		{
			name:     "numeric hostname alone",
			endpoint: "12345",
			expectedConfig: &ProxyConfig{
				Hostname:        "12345",
				LocalPort:       DefaultLocalPort,
				DestinationPort: DefaultDestinationPort,
			},
		},
		{
			name:     "numeric hostname with destination port reads as local port and hostname",
			endpoint: "12345:443",
			expectedConfig: &ProxyConfig{
				Hostname:        "443",
				LocalPort:       12345,
				DestinationPort: DefaultDestinationPort,
			},
		},
		{
			name:     "numeric hostname in full format",
			endpoint: "8888:12345:443",
			expectedConfig: &ProxyConfig{
				Hostname:        "12345",
				LocalPort:       8888,
				DestinationPort: 443,
			},
		},
		{
			name:        "trailing junk on destination port",
			endpoint:    "example.com:443x",
			expectedErr: fmt.Errorf("invalid endpoint 'example.com:443x': neither 'example.com' nor '443x' is a valid port (must be a number between 1 and 65535)"),
		},
		{
			name:        "trailing junk on full-format destination port",
			endpoint:    "8888:example.com:443abc",
			expectedErr: fmt.Errorf("invalid destination port '443abc': must be a number between 1 and 65535"),
		},
		{
			name:        "destination port zero",
			endpoint:    "example.com:0",
			expectedErr: fmt.Errorf("invalid endpoint 'example.com:0': neither 'example.com' nor '0' is a valid port (must be a number between 1 and 65535)"),
		},
		{
			name:        "local port out of range",
			endpoint:    "70000:example.com",
			expectedErr: fmt.Errorf("invalid endpoint '70000:example.com': neither '70000' nor 'example.com' is a valid port (must be a number between 1 and 65535)"),
		},
		{
			name:        "full format local port out of range",
			endpoint:    "70000:example.com:443",
			expectedErr: fmt.Errorf("invalid local port '70000': must be a number between 1 and 65535"),
		},
		{
			name:        "invalid local port",
			endpoint:    "abc:host:123",
			expectedErr: fmt.Errorf("invalid local port 'abc': must be a number between 1 and 65535"),
		},
		{
			name:        "invalid destination port",
			endpoint:    "host:abc",
			expectedErr: fmt.Errorf("invalid endpoint 'host:abc': neither 'host' nor 'abc' is a valid port (must be a number between 1 and 65535)"),
		},
		{
			name:        "too many parts",
			endpoint:    "1:2:3:4",
			expectedErr: fmt.Errorf("invalid endpoint format '1:2:3:4'. Expected format: [LOCAL_PORT:]HOSTNAME[:DEST_PORT]"),
		},
		{
			name:        "empty string",
			endpoint:    "",
			expectedErr: fmt.Errorf("endpoint cannot be empty. Expected format: [LOCAL_PORT:]HOSTNAME[:DEST_PORT]"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config, err := ParseEndpointString(tc.endpoint)

			if tc.expectedErr != nil {
				assert.Error(t, err)
				assert.EqualError(t, err, tc.expectedErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedConfig, config)
			}
		})
	}
}

func TestAddress(t *testing.T) {
	config := &ProxyConfig{
		Hostname:        "app.example.com",
		DestinationPort: 8080,
	}
	expected := "app.example.com:8080"
	assert.Equal(t, expected, config.Address())
}

func TestSetDefaults(t *testing.T) {
	testCases := []struct {
		name     string
		input    []ProxyConfig
		expected []ProxyConfig
	}{
		{
			name:     "no defaults needed",
			input:    []ProxyConfig{{Hostname: "host1", LocalPort: 1000, DestinationPort: 2000}},
			expected: []ProxyConfig{{Hostname: "host1", LocalPort: 1000, DestinationPort: 2000}},
		},
		{
			name:     "set both defaults",
			input:    []ProxyConfig{{Hostname: "host1"}},
			expected: []ProxyConfig{{Hostname: "host1", LocalPort: DefaultLocalPort, DestinationPort: DefaultDestinationPort}},
		},
		{
			name:     "empty input",
			input:    []ProxyConfig{},
			expected: []ProxyConfig{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			SetDefaults(tc.input)
			assert.Equal(t, tc.expected, tc.input)
		})
	}
}
