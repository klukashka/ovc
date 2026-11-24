package worker_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/worker"
)

var errClaimRefused = errors.New("the claim's database call failed")

type heldClaims struct {
	mutex    sync.Mutex
	calls    int
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (c *heldClaims) claim(_ context.Context, _ time.Time, _ int) (func(context.Context) error, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.calls++
	switch c.calls {
	case 1:
		return nil, errClaimRefused
	case 2:
		return func(context.Context) error {
			close(c.started)
			<-c.release
			close(c.finished)
			return nil
		}, nil
	}
	return nil, nil
}

func TestAClaimFailureDoesNotLetShutdownSkipARunningBuild(t *testing.T) {
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	store := repositories.NewStore(testdb.New(t).Pool)
	claims := &heldClaims{started: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	lane := worker.ClaimLane{Name: "export", Slots: 2, Poll: 10 * time.Millisecond, Claim: claims.claim}
	idle := []worker.Lane{{Name: domain.LaneEmail, Concurrency: 1, Perform: neverPerformed}}
	background := worker.New(logger, clock.System{}, outbox.New(store, &countingRandom{}, logger), idle, nil, worker.DefaultTiming(), lane)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan bool, 1)
	go func() {
		runErr := background.Run(ctx)
		select {
		case <-claims.finished:
			returned <- runErr == nil
		default:
			returned <- false
		}
	}()
	awaitBuildStarted(t, claims.started)

	cancel()
	early := returnedWithin(returned, 500*time.Millisecond)
	close(claims.release)
	waitedForTheBuild := early == nil && awaitRunReturned(t, returned)

	if early != nil || !waitedForTheBuild {
		t.Fatal("the run returned before its running build finished, or with an error")
	}
	if !strings.Contains(logs.String(), `"msg":"worker.claim_lane.failed"`) {
		t.Fatal("the failed claim was not logged")
	}
}

func neverPerformed(context.Context, repositories.ClaimedOutboxRow, time.Time) (domain.OutboxDoneReason, error) {
	return "", errClaimRefused
}

func awaitBuildStarted(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatal("the build never started")
	}
}

func returnedWithin(result <-chan bool, window time.Duration) *bool {
	select {
	case value := <-result:
		return &value
	case <-time.After(window):
		return nil
	}
}

func awaitRunReturned(t *testing.T, result <-chan bool) bool {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(10 * time.Second):
		t.Fatal("the run never returned")
		return false
	}
}
