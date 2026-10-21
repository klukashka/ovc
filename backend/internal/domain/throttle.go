package domain

import "time"

func FixedWindow(now time.Time, size time.Duration) (time.Time, time.Duration) {
	start := now.UTC().Truncate(size)
	return start, start.Add(size).Sub(now)
}
