package yarl

import (
	"errors"
	"net/netip"
)

// ErrInvalidIPIdentity means an IP-mode key is not an unzoned IP address.
var ErrInvalidIPIdentity = errors.New("yarl: identity must be an IP address without a zone")

// Configuration controls optional IP identity normalization.
type Configuration struct {
	// IPv6PrefixBits enables IP-only identities when set to 1..128.
	// Use 64 to group IPv6 clients by /64. Zero preserves generic keys.
	// IPv4 identities always use their canonical individual address.
	IPv6PrefixBits int
}

// NewWithConfiguration creates a limiter with optional IP-only identities.
// Configuration is copied at construction. New retains generic-key behavior.
func NewWithConfiguration(b Backend, configuration Configuration, rules ...Rule) (*Limiter, error) {
	if configuration.IPv6PrefixBits < 0 || configuration.IPv6PrefixBits > 128 {
		return nil, errors.New("yarl: IPv6PrefixBits must be between 0 and 128")
	}
	limiter := New(b, rules...)
	limiter.ipv6PrefixBits = configuration.IPv6PrefixBits
	return limiter, nil
}

func normalizeIPIdentity(identity string, prefixBits int) (string, error) {
	address, err := netip.ParseAddr(identity)
	if err != nil || address.Zone() != "" {
		return "", ErrInvalidIPIdentity
	}
	address = address.Unmap()
	if address.Is4() {
		return address.String(), nil
	}
	return netip.PrefixFrom(address, prefixBits).Masked().String(), nil
}
