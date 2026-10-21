package domain_test

import (
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func TestOutboxRetryDelayDoublesWithJitterBetween30SecondsAndOneHour(t *testing.T) {
	cases := []struct {
		attempts int32
		fraction float64
		want     time.Duration
	}{
		{1, 0, 30 * time.Second},
		{1, 1, 45 * time.Second},
		{2, 0, time.Minute},
		{3, 0.5, 2*time.Minute + 30*time.Second},
		{7, 0, 32 * time.Minute},
		{7, 1, 48 * time.Minute},
		{8, 0, time.Hour},
		{40, 0.3, time.Hour},
		{1, -5, 30 * time.Second},
		{1, 7, 45 * time.Second},
	}

	for _, testCase := range cases {
		got := domain.OutboxRetryDelay(testCase.attempts, testCase.fraction)

		if got != testCase.want {
			t.Errorf("OutboxRetryDelay(%d, %v) = %v, want %v", testCase.attempts, testCase.fraction, got, testCase.want)
		}
	}
}
