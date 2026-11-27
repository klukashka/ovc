package testclock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/testclock"
)

func TestSystemAndHoldingClocksKeepTheContract(t *testing.T) {
	for name, source := range map[string]clock.Source{"system": clock.System{}, "holding": testclock.NewHolding(clock.System{})} {
		for operation, read := range map[string]func(context.Context) (time.Time, error){
			"instant": source.Instant,
			"within":  func(ctx context.Context) (time.Time, error) { return source.InstantWithin(ctx, nil) },
		} {
			t.Run(name+"/"+operation, func(t *testing.T) {
				before := clock.System{}.Now()

				instant, err := read(t.Context())

				if err != nil || instant.Before(before) || instant.After(clock.System{}.Now()) || instant.Location() != time.UTC {
					t.Fatalf("instant = %v, error = %v", instant, err)
				}
			})
		}
	}
}

type clockResult struct {
	instant time.Time
	err     error
}

func awaitClock(t *testing.T, result <-chan clockResult) clockResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatal("the clock did not finish")
		return clockResult{}
	}
}

func pausedInstant(t *testing.T, ctx context.Context, source *testclock.Holding) (*testclock.Pause, <-chan clockResult) {
	t.Helper()
	pause := source.PauseNextInstant()
	finished := make(chan struct{})
	t.Cleanup(func() {
		pause.Resume()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Error("the paused clock did not stop during cleanup")
		}
	})
	result := make(chan clockResult, 1)
	go func() {
		instant, err := source.Instant(ctx)
		result <- clockResult{instant: instant, err: err}
		close(finished)
	}()
	select {
	case <-pause.Reached():
	case <-time.After(time.Second):
		t.Fatal("the clock did not reach its pause")
	}
	return pause, result
}

func TestHoldingClockWaitsUntilReleased(t *testing.T) {
	source := testclock.NewHolding(clock.System{})
	pause, result := pausedInstant(t, t.Context(), source)
	before := clock.System{}.Now()

	pause.Resume()
	value := awaitClock(t, result)

	if value.err != nil || value.instant.Before(before) || value.instant.After(clock.System{}.Now()) {
		t.Fatalf("the clock did not read its source after release: %+v", value)
	}
}

func TestHoldingClockEndsItsWaitWhenCancelled(t *testing.T) {
	source := testclock.NewHolding(clock.System{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, result := pausedInstant(t, ctx, source)

	cancel()
	value := awaitClock(t, result)

	if !errors.Is(value.err, context.Canceled) || !value.instant.IsZero() {
		t.Fatalf("the cancelled clock returned %+v", value)
	}
}

func TestHoldingClockPausesOnlyTheNextInstant(t *testing.T) {
	source := testclock.NewHolding(clock.System{})
	pausedInstant(t, t.Context(), source)
	before := clock.System{}.Now()

	instant, err := source.Instant(t.Context())

	if err != nil || instant.Before(before) || instant.After(clock.System{}.Now()) {
		t.Fatalf("the next read did not return the source instant: %v, %v", instant, err)
	}
}
