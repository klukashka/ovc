package listener_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
)

var instant = time.Date(2026, time.October, 25, 9, 0, 0, 0, time.UTC)

const waitLimit = 15 * time.Second

type event struct {
	connected listener.Generation
	lost      listener.Generation
	lossKind  listener.LossKind
	signal    domain.Signal
	isSignal  bool
	isLoss    bool
	isConnect bool
}

type recorder struct {
	mu        sync.Mutex
	events    []event
	changed   chan struct{}
	intercept func(domain.Signal)
	onConnect func()
	onLost    func()
}

func newRecorder() *recorder {
	return &recorder{changed: make(chan struct{})}
}

func (r *recorder) add(entry event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, entry)
	close(r.changed)
	r.changed = make(chan struct{})
}

func (r *recorder) ListenerConnected(generation listener.Generation) {
	if r.onConnect != nil {
		r.onConnect()
	}
	r.add(event{connected: generation, isConnect: true})
}

func (r *recorder) ListenerLost(generation listener.Generation, kind listener.LossKind) {
	if r.onLost != nil {
		r.onLost()
	}
	r.add(event{lost: generation, lossKind: kind, isLoss: true})
}

func (r *recorder) Signal(signal domain.Signal) {
	if r.intercept != nil {
		r.intercept(signal)
	}
	r.add(event{signal: signal, isSignal: true})
}

func (r *recorder) snapshot() ([]event, <-chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]event(nil), r.events...), r.changed
}

func (r *recorder) waitFor(t *testing.T, what string, found func([]event) bool) []event {
	t.Helper()
	deadline := time.NewTimer(waitLimit)
	defer deadline.Stop()
	for {
		events, changed := r.snapshot()
		if found(events) {
			return events
		}
		select {
		case <-changed:
		case <-deadline.C:
			t.Fatalf("waited %s for %s; recorded %d events", waitLimit, what, len(events))
		}
	}
}

func (r *recorder) waitConnected(t *testing.T, generation listener.Generation) {
	t.Helper()
	r.waitFor(t, fmt.Sprintf("generation %d to connect", generation), func(events []event) bool {
		for _, entry := range events {
			if entry.isConnect && entry.connected == generation {
				return true
			}
		}
		return false
	})
}

func signalsOf(events []event) []domain.Signal {
	var signals []domain.Signal
	for _, entry := range events {
		if entry.isSignal {
			signals = append(signals, entry.signal)
		}
	}
	return signals
}

type logBuffer struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	changed chan struct{}
}

func newLogBuffer() *logBuffer {
	return &logBuffer{changed: make(chan struct{})}
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	close(b.changed)
	b.changed = make(chan struct{})
	return b.buffer.Write(p)
}

func (b *logBuffer) written() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.changed
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (b *logBuffer) records(t *testing.T, message string) []map[string]json.RawMessage {
	t.Helper()
	var found []map[string]json.RawMessage
	for _, line := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("a log line is not JSON: %v", err)
		}
		if string(record["msg"]) == `"`+message+`"` {
			found = append(found, record)
		}
	}
	return found
}

type fixture struct {
	database *testdb.Database
	store    *repositories.Store
	listener *listener.Listener
	recorder *recorder
	logs     *logBuffer
	instance domain.InstanceID
}

func fastTiming() listener.Timing {
	return listener.Timing{
		HeartbeatInterval: 100 * time.Millisecond,
		LossAfter:         1500 * time.Millisecond,
		MinBackoff:        50 * time.Millisecond,
		MaxBackoff:        200 * time.Millisecond,
		ConnectTimeout:    2 * time.Second,
		CloseTimeout:      500 * time.Millisecond,
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	database := testdb.New(t)
	return startOn(t, database, database.URL, nil, listener.DefaultTiming())
}

func startOn(t *testing.T, database *testdb.Database, url core.Secret, heartbeats listener.Heartbeats, timing listener.Timing) *fixture {
	t.Helper()
	f := startWith(t, database, url, heartbeats, timing, newRecorder())
	f.recorder.waitConnected(t, 1)
	return f
}

func startWith(
	t *testing.T, database *testdb.Database, url core.Secret, heartbeats listener.Heartbeats, timing listener.Timing, consumer *recorder,
) *fixture {
	t.Helper()
	store := repositories.NewStore(database.Pool)
	if heartbeats == nil {
		heartbeats = listener.ConnectionHeartbeats{}
	}
	instance, err := core.NewID[domain.InstanceID](core.CryptoRandom())
	if err != nil {
		t.Fatal(err)
	}
	logs := newLogBuffer()
	adapter, err := listener.New(listener.Dependencies{
		DatabaseURL: url, Instance: instance, Heartbeats: heartbeats, Consumer: consumer,
		Logger: slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), Timing: timing,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		adapter.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	return &fixture{database: database, store: store, listener: adapter, recorder: consumer, logs: logs, instance: instance}
}

func (f *fixture) sentinel(t *testing.T) []event {
	t.Helper()
	id, err := core.NewID[domain.EntityID](core.CryptoRandom())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Repositories().Signals.Entity(context.Background(), []domain.UserID{{0xff}}, domain.EntityNotification, id); err != nil {
		t.Fatal(err)
	}
	isSentinel := func(entry event) bool {
		entity, ok := entry.signal.(domain.EntitySignal)
		return entry.isSignal && ok && entity.ID == id
	}
	events := f.recorder.waitFor(t, "the sentinel signal", func(events []event) bool {
		return slices.ContainsFunc(events, isSentinel)
	})
	return events[:slices.IndexFunc(events, isSentinel)]
}

func (f *fixture) createUser(t *testing.T, id domain.UserID, state domain.AccountState) {
	t.Helper()
	sum := sha256.Sum256([]byte("fixture"))
	var verifiedAt *time.Time
	if state != domain.AccountUnverified {
		verifiedAt = &instant
	}
	err := f.store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: id, Role: domain.RoleTutor, State: state, Email: fmt.Sprintf("person-%x@example.org", id[:4]), EmailVerifiedAt: verifiedAt,
		PasswordHash: fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%x", sum), Name: "A language tutor",
		Language: domain.LanguageEnglish, Timezone: "Europe/Moscow", Theme: domain.ThemeSystem,
		ConsentVersion: "2026-10", ConsentAt: instant, CreatedAt: instant,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) createSession(t *testing.T, id domain.SessionID, user domain.UserID) {
	t.Helper()
	sum := sha256.Sum256(id[:])
	err := f.store.Repositories().Sessions.Create(context.Background(), repositories.NewSession{
		ID: id, UserID: user, RefreshHash: sum[:], ExpiresAt: instant.Add(30 * 24 * time.Hour), CreatedAt: instant,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) exec(t *testing.T, statement string, argument string) {
	t.Helper()
	tag, err := f.database.Pool.Exec(context.Background(), statement, argument)
	if err != nil {
		t.Fatalf("%s: %s", statement, core.ErrorKind(err))
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("%s affected %d rows", statement, tag.RowsAffected())
	}
}

func rechecksOf(signals []domain.Signal) [][]domain.UserID {
	var rechecks [][]domain.UserID
	for _, signal := range signals {
		if recheck, ok := signal.(domain.RecheckSignal); ok {
			rechecks = append(rechecks, recheck.Users)
		}
	}
	return rechecks
}

type rawListener struct {
	connection *pgx.Conn
	skipped    domain.InstanceID
}

func listenRaw(t *testing.T, f *fixture) *rawListener {
	t.Helper()
	connection, err := pgx.Connect(context.Background(), f.database.URL.Reveal())
	if err != nil {
		t.Fatalf("connect the raw listener: %s", core.ErrorKind(err))
	}
	t.Cleanup(func() {
		if err := connection.Close(context.Background()); err != nil {
			t.Errorf("close the raw listener: %s", core.ErrorKind(err))
		}
	})
	if _, err := connection.Exec(context.Background(), "LISTEN "+domain.SignalChannel); err != nil {
		t.Fatalf("listen: %s", core.ErrorKind(err))
	}
	return &rawListener{connection: connection, skipped: f.instance}
}

func (l *rawListener) next(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitLimit)
	defer cancel()
	for {
		notification, err := l.connection.WaitForNotification(ctx)
		if err != nil {
			t.Fatalf("wait for a raw notification: %s", core.ErrorKind(err))
		}
		if !l.isSkippedHeartbeat(notification.Payload) {
			return notification.Payload
		}
	}
}

func (l *rawListener) isSkippedHeartbeat(payload string) bool {
	parsed, err := domain.ParseSignal(payload)
	if err != nil {
		return false
	}
	heartbeat, ok := parsed.(domain.HeartbeatSignal)
	return ok && heartbeat.Instance == l.skipped
}
