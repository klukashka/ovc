package listener_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/testdb/forwarder"
)

func hasLoss(events []event, generation listener.Generation, kind listener.LossKind) bool {
	for _, entry := range events {
		if entry.isLoss && entry.lost == generation && entry.lossKind == kind {
			return true
		}
	}
	return false
}

func anyLoss(events []event) bool {
	for _, entry := range events {
		if entry.isLoss {
			return true
		}
	}
	return false
}

func (b *logBuffer) waitForLoss(t *testing.T, kind listener.LossKind) {
	t.Helper()
	deadline := time.NewTimer(waitLimit)
	defer deadline.Stop()
	for {
		written := b.written()
		for _, record := range b.records(t, "realtime.listener.lost") {
			if string(record["kind"]) == `"`+string(kind)+`"` && string(record["level"]) == `"ERROR"` {
				return
			}
		}
		select {
		case <-written:
		case <-deadline.C:
			t.Fatalf("no realtime.listener.lost record of kind %s within %s", kind, waitLimit)
		}
	}
}

func TestListenerReconnectsAfterItsConnectionIsTerminated(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	f := startOn(t, database, database.URL, nil, fastTiming())

	var terminated bool
	err := database.Pool.QueryRow(context.Background(),
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name = $1 AND datname = current_database()",
		listener.ApplicationName).Scan(&terminated)
	f.recorder.waitFor(t, "the loss of generation 1", func(events []event) bool {
		return hasLoss(events, 1, listener.LossConnection)
	})
	f.recorder.waitConnected(t, 2)
	f.sentinel(t)

	if err != nil || !terminated {
		t.Fatalf("terminating the listener's backend answered %v, %s", terminated, core.ErrorKind(err))
	}
	if generation, live := f.listener.State(); generation != 2 || !live {
		t.Fatalf("after reconnecting the listener is at generation %d, live %v", generation, live)
	}
	f.logs.waitForLoss(t, listener.LossConnection)
	if connected := f.logs.records(t, "realtime.listener.connected"); len(connected) != 2 {
		t.Fatalf("%d realtime.listener.connected records, want 2", len(connected))
	}
}

type pausableHeartbeats struct {
	sends     listener.ConnectionHeartbeats
	mu        sync.Mutex
	paused    bool
	failing   bool
	alternate bool
	calls     int
	sent      int
	changed   chan struct{}
}

var errHeartbeatRefused = errors.New("the heartbeat write is refused on purpose")

func newPausableHeartbeats() *pausableHeartbeats {
	return &pausableHeartbeats{changed: make(chan struct{})}
}

func (h *pausableHeartbeats) Heartbeat(ctx context.Context, connection *pgx.Conn, instance domain.InstanceID) error {
	h.mu.Lock()
	h.calls++
	paused, refused := h.paused, h.failing || (h.alternate && h.calls%2 == 0)
	h.mu.Unlock()
	if paused {
		return nil
	}
	if refused {
		return errHeartbeatRefused
	}
	if err := h.sends.Heartbeat(ctx, connection, instance); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sent++
	close(h.changed)
	h.changed = make(chan struct{})
	return nil
}

func (h *pausableHeartbeats) pause() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.paused = true
}

func (h *pausableHeartbeats) resume() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.paused = false
}

func (h *pausableHeartbeats) failEverySecondWrite() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.alternate = true
}

func (h *pausableHeartbeats) failEveryWrite() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failing = true
}

func (h *pausableHeartbeats) sentCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sent
}

func (h *pausableHeartbeats) waitSent(t *testing.T, count int) {
	t.Helper()
	deadline := time.NewTimer(waitLimit)
	defer deadline.Stop()
	for {
		h.mu.Lock()
		sent, changed := h.sent, h.changed
		h.mu.Unlock()
		if sent >= count {
			return
		}
		select {
		case <-changed:
		case <-deadline.C:
			t.Fatalf("only %d heartbeats were sent within %s", sent, waitLimit)
		}
	}
}

func TestMissingHeartbeatsAreDetectedAndFailClosed(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	heartbeats := newPausableHeartbeats()
	timing := fastTiming()
	f := startOn(t, database, database.URL, heartbeats, timing)
	heartbeats.waitSent(t, int(timing.LossAfter/timing.HeartbeatInterval)+3)
	aliveWhileBeating, _ := f.recorder.snapshot()

	heartbeats.pause()
	f.recorder.waitFor(t, "the loss of generation 1 by heartbeat", func(events []event) bool {
		return hasLoss(events, 1, listener.LossHeartbeat)
	})
	f.recorder.waitFor(t, "the loss of generation 2 by heartbeat", func(events []event) bool {
		return hasLoss(events, 2, listener.LossHeartbeat)
	})

	if anyLoss(aliveWhileBeating) {
		t.Fatal("the listener was lost while its heartbeats arrived")
	}
	f.logs.waitForLoss(t, listener.LossHeartbeat)
}

func (f *fixture) holdEveryPoolConnection(t *testing.T) func() {
	t.Helper()
	acquiring, cancel := context.WithTimeout(context.Background(), waitLimit)
	defer cancel()
	var held []*pgxpool.Conn
	for range f.database.Pool.Config().MaxConns {
		connection, err := f.database.Pool.Acquire(acquiring)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, connection)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		for _, connection := range held {
			connection.Release()
		}
	}
	t.Cleanup(release)
	return release
}

func TestAHeldPoolDoesNotDelayTheHeartbeat(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	heartbeats := newPausableHeartbeats()
	timing := fastTiming()
	f := startOn(t, database, database.URL, heartbeats, timing)
	heartbeats.waitSent(t, 1)
	release := f.holdEveryPoolConnection(t)
	beforeHolding := heartbeats.sentCount()

	heartbeats.waitSent(t, beforeHolding+int(timing.LossAfter/timing.HeartbeatInterval)+3)
	whileHeld, _ := f.recorder.snapshot()
	_, liveWhileHeld := f.listener.State()
	release()

	if anyLoss(whileHeld) || !liveWhileHeld {
		t.Fatal("the listener was lost while every pool connection was held")
	}
	if generation, live := f.listener.State(); generation != 1 || !live {
		t.Fatalf("after the pool was held the listener is at generation %d, live %v", generation, live)
	}
}

func TestASilentListenerIsDetectedAndFailsClosed(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	relay := forwarder.New(t, database)
	f := startOn(t, database, relay.URL(), nil, fastTiming())
	f.sentinel(t)

	relay.Stop()
	f.recorder.waitFor(t, "the loss of generation 1 by heartbeat", func(events []event) bool {
		return hasLoss(events, 1, listener.LossHeartbeat)
	})
	f.logs.waitForLoss(t, listener.LossConnect)

	if generation, live := f.listener.State(); generation != 1 || live {
		t.Fatalf("a listener behind a silent connection is at generation %d, live %v", generation, live)
	}
	f.logs.waitForLoss(t, listener.LossHeartbeat)
}

func TestFailingHeartbeatWritesLoseTheListenerOnlyBySilence(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	heartbeats := newPausableHeartbeats()
	heartbeats.failEverySecondWrite()
	timing := fastTiming()
	f := startOn(t, database, database.URL, heartbeats, timing)
	heartbeats.waitSent(t, int(timing.LossAfter/timing.HeartbeatInterval)+3)
	aliveWhileSomeWritesFail, _ := f.recorder.snapshot()

	heartbeats.failEveryWrite()
	f.recorder.waitFor(t, "the loss of generation 1 by heartbeat", func(events []event) bool {
		return hasLoss(events, 1, listener.LossHeartbeat)
	})

	if anyLoss(aliveWhileSomeWritesFail) {
		t.Fatal("a single failed heartbeat write lost the listener")
	}
	f.logs.waitForLoss(t, listener.LossHeartbeat)
}

func TestABlockedConsumerDoesNotKeepTheListenerLive(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	release := make(chan struct{})
	consumer := newRecorder()
	consumer.intercept = func(domain.Signal) { <-release }
	f := startWith(t, database, database.URL, nil, fastTiming(), consumer)
	f.recorder.waitConnected(t, 1)
	t.Cleanup(func() { close(release) })

	err := f.store.Repositories().Signals.Entity(context.Background(), []domain.UserID{{1}}, domain.EntityRelationship, domain.EntityID{1})
	f.recorder.waitFor(t, "the loss of generation 1 by heartbeat", func(events []event) bool {
		return hasLoss(events, 1, listener.LossHeartbeat)
	})

	if err != nil {
		t.Fatal(err)
	}
	if generation, live := f.listener.State(); generation != 1 || live {
		t.Fatalf("a listener stuck in its consumer is at generation %d, live %v", generation, live)
	}
}

func TestAPanickingConsumerLosesTheListenerAndReconnects(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	var once sync.Once
	consumer := newRecorder()
	consumer.intercept = func(domain.Signal) { once.Do(func() { panic("a consumer bug") }) }
	f := startWith(t, database, database.URL, nil, fastTiming(), consumer)
	f.recorder.waitConnected(t, 1)

	err := f.store.Repositories().Signals.Entity(context.Background(), []domain.UserID{{1}}, domain.EntityRelationship, domain.EntityID{1})
	f.recorder.waitFor(t, "the loss of generation 1 by the panic", func(events []event) bool {
		return hasLoss(events, 1, listener.LossPanic)
	})
	f.recorder.waitConnected(t, 2)
	f.sentinel(t)

	if err != nil {
		t.Fatal(err)
	}
	if recovered := f.logs.records(t, "panic.recovered"); len(recovered) != 1 {
		t.Fatalf("%d panic.recovered records, want 1", len(recovered))
	}
}

func TestAConsumerBlockedWhenConnectedDoesNotKeepTheListenerLive(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	release := make(chan struct{})
	consumer := newRecorder()
	consumer.onConnect = func() { <-release }

	f := startWith(t, database, database.URL, nil, fastTiming(), consumer)
	t.Cleanup(func() { close(release) })
	f.recorder.waitFor(t, "the loss of generation 1 by heartbeat", func(events []event) bool {
		return hasLoss(events, 1, listener.LossHeartbeat)
	})

	if generation, live := f.listener.State(); generation != 1 || live {
		t.Fatalf("a listener stuck in its consumer's connect callback is at generation %d, live %v", generation, live)
	}
}

type panickingHeartbeats struct {
	sends listener.ConnectionHeartbeats
	once  sync.Once
}

func (h *panickingHeartbeats) Heartbeat(ctx context.Context, connection *pgx.Conn, instance domain.InstanceID) error {
	h.once.Do(func() { panic("a heartbeat bug") })
	return h.sends.Heartbeat(ctx, connection, instance)
}

func TestAPanickingHeartbeatLosesTheListenerAndReconnects(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	heartbeats := &panickingHeartbeats{}

	f := startOn(t, database, database.URL, heartbeats, fastTiming())
	f.recorder.waitFor(t, "the loss of generation 1 by the panic", func(events []event) bool {
		return hasLoss(events, 1, listener.LossPanic)
	})
	f.recorder.waitConnected(t, 2)
	f.sentinel(t)

	if recovered := f.logs.records(t, "panic.recovered"); len(recovered) != 1 {
		t.Fatalf("%d panic.recovered records, want 1", len(recovered))
	}
}

func TestAPanickingLossCallbackStillFailsClosedAndReconnects(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	heartbeats := newPausableHeartbeats()
	var once sync.Once
	consumer := newRecorder()
	consumer.onLost = func() { once.Do(func() { panic("a loss callback bug") }) }
	f := startWith(t, database, database.URL, heartbeats, fastTiming(), consumer)
	f.recorder.waitConnected(t, 1)

	heartbeats.pause()
	f.logs.waitForLoss(t, listener.LossHeartbeat)
	generation, live := f.listener.State()
	heartbeats.resume()
	f.recorder.waitConnected(t, 2)

	if generation != 1 || live {
		t.Fatalf("after its loss callback panicked the listener is at generation %d, live %v", generation, live)
	}
	if recovered := f.logs.records(t, "panic.recovered"); len(recovered) != 1 {
		t.Fatalf("%d panic.recovered records, want 1", len(recovered))
	}
}
