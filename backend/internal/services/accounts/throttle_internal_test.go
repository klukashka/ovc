package accounts

import (
	"net/netip"
	"testing"
)

func TestSignInAddressesAreBucketedByIPv4AddressOrIPv6Slash64(t *testing.T) {
	cases := map[string]string{
		"203.0.113.5":          "203.0.113.5",
		"::ffff:203.0.113.5":   "203.0.113.5",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1:2::",
		"2001:db8:1:2:ffff::1": "2001:db8:1:2::",
		"2001:db8:1:3::1":      "2001:db8:1:3::",
		"::1":                  "::",
		"fe80::1%en0":          "fe80::",
	}

	for address, want := range cases {
		got := addressBucket(netip.MustParseAddr(address))

		if got != want {
			t.Errorf("addressBucket(%s) = %s, want %s", address, got, want)
		}
	}
}
