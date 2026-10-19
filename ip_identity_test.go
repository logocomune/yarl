package yarl

import (
	"context"
	"net/netip"
	"testing"
	"testing/quick"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewWithConfiguration(t *testing.T) {
	for _, bits := range []int{-1, 129, 0, 1, 56, 64, 128} {
		limiter, err := NewWithConfiguration(newMockBackend(time.Minute, nil), Configuration{IPv6PrefixBits: bits})
		if bits < 0 || bits > 128 {
			require.Error(t, err)
			require.Nil(t, limiter)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, bits, limiter.ipv6PrefixBits)
	}
}

func TestNormalizeIPIdentity(t *testing.T) {
	tests := []struct {
		name, input, want string
		bits              int
	}{
		{"IPv4", "192.0.2.1", "192.0.2.1", 64},
		{"mapped IPv4", "::ffff:192.0.2.1", "192.0.2.1", 64},
		{"hex mapped IPv4", "::ffff:c000:201", "192.0.2.1", 56},
		{"IPv6 canonical", "2001:0DB8:ABCD:0012:0000:0000:0000:0001", "2001:db8:abcd:12::/64", 64},
		{"IPv6 /56", "2001:db8:abcd:12ff::1", "2001:db8:abcd:1200::/56", 56},
		{"IPv6 /128", "2001:db8::1", "2001:db8::1/128", 128},
		{"IPv6 /1", "ffff::1", "8000::/1", 1},
		{"unspecified", "::", "::/64", 64},
		{"loopback", "::1", "::/64", 64},
		{"empty", "", "", 64},
		{"user key", "user:42", "", 64},
		{"composite", "2001:db8::1:tenant-A", "", 64},
		{"IPv4 port", "192.0.2.1:80", "", 64},
		{"IPv6 port", "[2001:db8::1]:80", "", 64},
		{"prefix", "2001:db8::/64", "", 64},
		{"zone", "fe80::1%eth0", "", 64},
		{"whitespace", " 192.0.2.1", "", 64},
		{"hostname", "example.com", "", 64},
		{"leading zero", "192.000.2.1", "", 64},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeIPIdentity(test.input, test.bits)
			if test.want == "" {
				require.ErrorIs(t, err, ErrInvalidIPIdentity)
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestIPIdentitySerialAndBatch(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "serial"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			serial := newMockBackend(time.Minute, nil)
			batched := newBatchCapturingBackend()
			var backend Backend = serial
			counts := serial.counts
			if batch {
				backend, counts = batched, batched.counts
			}
			limiter, err := NewWithConfiguration(backend, Configuration{IPv6PrefixBits: 64},
				Rule{ID: "burst", TTL: time.Minute, MaxRequests: 1},
				Rule{ID: "daily", TTL: time.Hour, MaxRequests: 5})
			require.NoError(t, err)
			for _, invalid := range []string{"", "invalid", "fe80::1%eth0"} {
				results, err := limiter.Check(context.Background(), invalid)
				require.ErrorIs(t, err, ErrInvalidIPIdentity)
				require.Nil(t, results)
			}
			require.Empty(t, counts)
			require.Zero(t, batched.batchCalls)
			for _, address := range []string{"2001:db8:1:2::1", "2001:db8:1:2::ffff", "2001:db8:1:3::1", "192.0.2.1", "::ffff:192.0.2.1", "192.0.2.2"} {
				results, err := limiter.Check(context.Background(), address)
				require.NoError(t, err)
				require.Len(t, results, 2)
				require.Equal(t, results[0].Current <= 1, results[0].Allowed)
				require.True(t, results[1].Allowed)
			}
			for _, rule := range []string{"burst", "daily"} {
				require.Equal(t, int64(2), counts[rule+":2001:db8:1:2::/64"])
				require.Equal(t, int64(1), counts[rule+":2001:db8:1:3::/64"])
				require.Equal(t, int64(2), counts[rule+":192.0.2.1"])
				require.Equal(t, int64(1), counts[rule+":192.0.2.2"])
			}
		})
	}
}

func TestIPIdentityDisabled(t *testing.T) {
	backend := newMockBackend(time.Minute, nil)
	limiter, err := NewWithConfiguration(backend, Configuration{}, Rule{ID: "r", TTL: time.Minute, MaxRequests: 1})
	require.NoError(t, err)
	for _, identity := range []string{"", "user:42", "2001:db8::1"} {
		_, err := limiter.Check(context.Background(), identity)
		require.NoError(t, err)
		require.Equal(t, int64(1), backend.counts["r:"+identity])
	}
}

func TestIPIdentityProperties(t *testing.T) {
	property := func(raw [16]byte, other [16]byte, selector uint8) bool {
		bits := int(selector%128) + 1
		address := netip.AddrFrom16(raw)
		got, err := normalizeIPIdentity(address.String(), bits)
		if err != nil {
			return false
		}
		if address.Is4In6() {
			return got == address.Unmap().String()
		}
		prefix, err := netip.ParsePrefix(got)
		if err != nil || prefix.Bits() != bits || prefix != prefix.Masked() || !prefix.Contains(address) {
			return false
		}
		second, err := normalizeIPIdentity(prefix.Addr().String(), bits)
		if err != nil || got != second {
			return false
		}
		for index := 0; index < bits; index++ {
			mask := byte(1 << (7 - index%8))
			other[index/8] = (other[index/8] &^ mask) | (raw[index/8] & mask)
		}
		neighbor := netip.AddrFrom16(other)
		if neighbor.Is4In6() {
			return true
		}
		neighborKey, err := normalizeIPIdentity(neighbor.String(), bits)
		return err == nil && neighborKey == got
	}
	require.NoError(t, quick.Check(property, &quick.Config{MaxCount: 5000}))
}

func TestIPv4IdentityProperties(t *testing.T) {
	property := func(raw [4]byte, selector uint8) bool {
		bits := int(selector%128) + 1
		address := netip.AddrFrom4(raw)
		mapped := netip.AddrFrom16(address.As16())
		plainKey, plainErr := normalizeIPIdentity(address.String(), bits)
		mappedKey, mappedErr := normalizeIPIdentity(mapped.String(), bits)
		return plainErr == nil && mappedErr == nil && plainKey == address.String() && mappedKey == plainKey
	}
	require.NoError(t, quick.Check(property, &quick.Config{MaxCount: 5000}))
}

func FuzzIPIdentity(f *testing.F) {
	for _, seed := range []string{"", "user:42", "192.0.2.1", "::ffff:192.0.2.1", "2001:db8:abcd:12::1", "2001:0DB8::1", "fe80::1%eth0", "[::1]:80", "::/64", "\x00"} {
		f.Add(seed, uint8(64))
	}
	f.Fuzz(func(t *testing.T, input string, selector uint8) {
		bits := int(selector%128) + 1
		got, err := normalizeIPIdentity(input, bits)
		address, parseErr := netip.ParseAddr(input)
		if parseErr != nil || address.Zone() != "" {
			require.ErrorIs(t, err, ErrInvalidIPIdentity)
			require.Empty(t, got)
			return
		}
		require.NoError(t, err)
		address = address.Unmap()
		if address.Is4() {
			require.Equal(t, address.String(), got)
			return
		}
		prefix, err := netip.ParsePrefix(got)
		require.NoError(t, err)
		require.Equal(t, bits, prefix.Bits())
		require.Equal(t, prefix.Masked(), prefix)
		require.True(t, prefix.Contains(address))
		second, err := normalizeIPIdentity(prefix.Addr().String(), bits)
		require.NoError(t, err)
		require.Equal(t, got, second)
	})
}
