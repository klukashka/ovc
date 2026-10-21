package domain_test

import (
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
