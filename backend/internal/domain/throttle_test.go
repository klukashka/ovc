package domain_test

import (
	"net/netip"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func TestFixedWindowStartsOnTheBoundaryAndCountsDownToTheNext(t *testing.T) {
	cases := []struct {
		now        time.Time
		start      time.Time
		retryAfter time.Duration
	}{
		{time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC), time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC), 15 * time.Minute},
		{time.Date(2030, 3, 10, 9, 5, 30, 0, time.UTC), time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC), 9*time.Minute + 30*time.Second},
		{time.Date(2030, 3, 10, 9, 14, 59, 0, time.UTC), time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC), time.Second},
		{time.Date(2030, 3, 10, 12, 20, 0, 0, time.FixedZone("Moscow", 3*60*60)), time.Date(2030, 3, 10, 9, 15, 0, 0, time.UTC), 10 * time.Minute},
	}

	for _, testCase := range cases {
		start, retryAfter := domain.FixedWindow(testCase.now, 15*time.Minute)

		if !start.Equal(testCase.start) || retryAfter != testCase.retryAfter {
			t.Errorf("FixedWindow(%v) = %v, %v; want %v, %v", testCase.now, start, retryAfter, testCase.start, testCase.retryAfter)
		}
	}
}

func TestAddressesAreBucketedByIPv4AddressOrIPv6Slash64(t *testing.T) {
	cases := map[string]string{
		"203.0.113.5":          "203.0.113.5",
		"::ffff:203.0.113.5":   "203.0.113.5",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1:2::",
		"2001:db8:1:2:ffff::1": "2001:db8:1:2::",
		"2001:db8:1:3::1":      "2001:db8:1:3::",
		"::1":                  "::",
		"fe80::1%en0":          "fe80::",
	}

	buckets := map[string]netip.Addr{}
	for address := range cases {
		buckets[address] = domain.AddressBucket(netip.MustParseAddr(address))
	}

	for address, want := range cases {
		if got := buckets[address]; got != netip.MustParseAddr(want) {
			t.Errorf("AddressBucket(%s) = %s, want %s", address, got, want)
		}
	}
}
