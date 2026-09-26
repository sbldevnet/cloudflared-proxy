package listen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	testCases := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{name: "ipv4", addr: "127.0.0.1"},
		{name: "wildcard ipv4", addr: "0.0.0.0"},
		{name: "ipv6", addr: "::1"},
		{name: "wildcard ipv6", addr: "::"},
		{name: "hostname", addr: "example.com", wantErr: true},
		{name: "localhost", addr: "localhost", wantErr: true},
		{name: "empty", addr: "", wantErr: true},
		{name: "invalid ip", addr: "999.1.1.1", wantErr: true},
		{name: "with port", addr: "127.0.0.1:8080", wantErr: true},
		{name: "bracketed ipv6", addr: "[::1]", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.addr)
			if tc.wantErr {
				assert.ErrorContains(t, err, "must be an IP address literal")
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestIsLoopback(t *testing.T) {
	assert.True(t, IsLoopback("127.0.0.1"))
	assert.True(t, IsLoopback("127.0.0.2"))
	assert.True(t, IsLoopback("::1"))
	assert.False(t, IsLoopback("0.0.0.0"))
	assert.False(t, IsLoopback("::"))
	assert.False(t, IsLoopback("192.168.1.10"))
	assert.False(t, IsLoopback("localhost"))
}

func TestIsUnspecified(t *testing.T) {
	assert.True(t, IsUnspecified("0.0.0.0"))
	assert.True(t, IsUnspecified("::"))
	assert.False(t, IsUnspecified("127.0.0.1"))
	assert.False(t, IsUnspecified("192.168.1.10"))
	assert.False(t, IsUnspecified("localhost"))
}
