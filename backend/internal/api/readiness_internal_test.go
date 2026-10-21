package api

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
)

func advancingClock() func() time.Time {
	var calls atomic.Int64
	return func() time.Time {
		return clock.System{}.Now().Add(time.Duration(calls.Add(1)) * 2 * readinessReuseWindow)
	}
}

func waitForJoined(t *testing.T, shared *sharedReadiness, expected int) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for shared.joinedCount() < expected {
		select {
		case <-deadline.C:
			t.Fatalf("only %d of %d callers joined", shared.joinedCount(), expected)
		case <-ticker.C:
		}
	}
}

func TestConcurrentCallersShareOneInFlightCheck(t *testing.T) {
	release := make(chan struct{})
	var checks atomic.Int32
	shared := &sharedReadiness{
		check: func(ctx context.Context) error {
			checks.Add(1)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		now:    advancingClock(),
		logger: slog.New(slog.DiscardHandler),
	}
	const callers = 20
	results := make(chan error, callers)
	for range callers {
		go func() {
			results <- shared.run(context.Background())
		}()
	}
	waitForJoined(t, shared, callers)

	close(release)
	collected := make([]error, 0, callers)
	for range callers {
		collected = append(collected, <-results)
	}

	for _, result := range collected {
		if result != nil {
			t.Fatalf("unexpected results %v", collected)
		}
	}
	if checks.Load() != 1 {
		t.Fatalf("expected one shared check, got %d", checks.Load())
	}
}
