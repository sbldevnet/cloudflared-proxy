// Package listen validates and classifies the address a proxy binds to.
package listen

import (
	"fmt"
	"net/netip"
)

// Loopback is the address used when none is configured.
const Loopback = "127.0.0.1"

// Validate checks that addr is an IP address literal without a port.
func Validate(addr string) error {
	if _, err := netip.ParseAddr(addr); err != nil {
		return fmt.Errorf("invalid listen address %q: must be an IP address literal such as 127.0.0.1 or ::1, not a hostname (use an IP instead of \"localhost\")", addr)
	}
	return nil
}

// IsLoopback reports whether addr is a loopback IP address. It returns false
// for addr values that are not valid IP literals.
func IsLoopback(addr string) bool {
	ip, err := netip.ParseAddr(addr)
	return err == nil && ip.IsLoopback()
}

// IsUnspecified reports whether addr is 0.0.0.0 or ::, which bind every
// network interface.
func IsUnspecified(addr string) bool {
	ip, err := netip.ParseAddr(addr)
	return err == nil && ip.IsUnspecified()
}
