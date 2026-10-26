package domain

import (
	"net/netip"
	"time"
)

const ipv6BucketBits = 64

func FixedWindow(now time.Time, size time.Duration) (time.Time, time.Duration) {
	start := now.UTC().Truncate(size)
	return start, start.Add(size).Sub(now)
}

func AddressBucket(address netip.Addr) netip.Addr {
	bucket := address.Unmap()
	if bucket.Is6() {
		bucket = netip.PrefixFrom(bucket, ipv6BucketBits).Masked().Addr()
	}
	return bucket
}
