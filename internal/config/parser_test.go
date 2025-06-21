package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseProxyString(t *testing.T) {
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
			name:        "invalid format - too many parts",
			endpoint:    "1:2:3:4",
			expectedErr: fmt.Errorf("invalid endpoint format '1:2:3:4'. Expected format: [LOCAL_PORT:]HOSTNAME[:DEST_PORT]"),
		},
		{
			name:        "invalid format - empty string",
			endpoint:    "",
			expectedErr: fmt.Errorf("invalid endpoint format ''. Expected format: [LOCAL_PORT:]HOSTNAME[:DEST_PORT]"),
		},
		{
			name:        "invalid local port",
			endpoint:    "abc:host:123",
			expectedErr: fmt.Errorf("invalid local port 'abc': input does not match format"),
		},
		{
			name:        "invalid destination port",
			endpoint:    "host:abc",
			expectedErr: fmt.Errorf("invalid destination port 'abc': input does not match format"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config, err := ParseProxyString(tc.endpoint)

			if tc.expectedErr != nil {
				assert.Error(t, err)
				// Check for a prefix of the error message because the Sscanf error can vary.
				assert.Contains(t, err.Error(), tc.expectedErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedConfig, config)
			}
		})
	}
}
