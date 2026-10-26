package tutoring_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/tutoring"
)

const (
	insertActiveStudents = `INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme,
    consent_version, consent_at, unverified_since, created_at, updated_at)
SELECT gen_random_uuid(), 'student', 'active', 'student-' || n || '@example.org', $1, $2, 'A student', 'en', 'Europe/Moscow', 'system',
    '2026-10', $1, $1, $1, $1
FROM generate_series(1, $3) AS n`
	insertActiveRelationships = `INSERT INTO relationships (id, tutor_id, student_id, state, display_name, colour, linked_at, created_at, updated_at)
SELECT gen_random_uuid(), $1, id, 'active', 'A student', 'clay', $2, $2, $2 FROM users WHERE role = 'student'`
	relationshipStudents = `SELECT id::text, student_id::text FROM relationships WHERE tutor_id = $1`
	queryEventPrefix     = "db.query."
)

type statementCounter struct {
	statements atomic.Int64
}

func (c *statementCounter) Enabled(context.Context, slog.Level) bool { return true }

func (c *statementCounter) Handle(_ context.Context, record slog.Record) error {
	if strings.HasPrefix(record.Message, queryEventPrefix) {
		c.statements.Add(1)
	}
	return nil
}

func (c *statementCounter) WithAttrs([]slog.Attr) slog.Handler { return c }

func (c *statementCounter) WithGroup(string) slog.Handler { return c }

type signalCollector struct {
	connected chan struct{}
	changed   chan struct{}
	once      sync.Once
	mu        sync.Mutex
	received  map[domain.EntityID]domain.UserID
	unusual   []domain.Signal
}

func newSignalCollector() *signalCollector {
	return &signalCollector{connected: make(chan struct{}), changed: make(chan struct{}, 1), received: map[domain.EntityID]domain.UserID{}}
}

func (c *signalCollector) ListenerConnected(listener.Generation) {
	c.once.Do(func() { close(c.connected) })
}

func (c *signalCollector) ListenerLost(listener.Generation, listener.LossKind) {}

func (c *signalCollector) Signal(signal domain.Signal) {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.notifyChanged()
	switch typed := signal.(type) {
	case domain.HeartbeatSignal:
	case domain.EntitySignal:
		if typed.Entity != domain.EntityRelationship || len(typed.Users) != 1 {
			c.unusual = append(c.unusual, signal)
			return
		}
		c.received[typed.ID] = typed.Users[0]
	default:
		c.unusual = append(c.unusual, signal)
	}
}

func (c *signalCollector) notifyChanged() {
	select {
	case c.changed <- struct{}{}:
	default:
	}
}

func (c *signalCollector) snapshot() (map[domain.EntityID]domain.UserID, []domain.Signal) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.received), append([]domain.Signal{}, c.unusual...)
}

type deletionRun struct {
	statements int64
	expected   map[domain.EntityID]domain.UserID
	received   map[domain.EntityID]domain.UserID
	unusual    []domain.Signal
}

type countedDeletion struct {
	fixture   cardFixture
	counter   *statementCounter
	store     *repositories.Store
	service   *tutoring.Service
	collector *signalCollector
	expected  map[domain.EntityID]domain.UserID
}

func newCountedDeletion(t *testing.T, students int) countedDeletion {
	t.Helper()
	f := newCardFixture(t, subMicrosecond)
	ctx := context.Background()
	if _, err := f.database.Pool.Exec(ctx, insertActiveStudents, subMicrosecond, fixturePasswordHash(), students); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.Pool.Exec(ctx, insertActiveRelationships, f.tutor, subMicrosecond); err != nil {
		t.Fatal(err)
	}
	counter := &statementCounter{}
	pool, err := core.OpenPool(ctx, core.Settings{DatabaseURL: f.database.URL, DBPoolSize: 2}, slog.New(counter), func() time.Time { return subMicrosecond })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := repositories.NewStore(pool)
	return countedDeletion{
		fixture: f, counter: counter, store: store, service: countedService(t, store),
		collector: listenTo(t, f), expected: seededRelationships(t, f),
	}
}

func seededRelationships(t *testing.T, f cardFixture) map[domain.EntityID]domain.UserID {
	t.Helper()
	expected := map[domain.EntityID]domain.UserID{}
	rows, err := f.database.Pool.Query(context.Background(), relationshipStudents, f.tutor)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var relationshipText, studentText string
		if err := rows.Scan(&relationshipText, &studentText); err != nil {
			t.Fatal(err)
		}
		relationship, relationshipParsed := domain.ParseUUID[domain.EntityID](relationshipText)
		student, studentParsed := domain.ParseUUID[domain.UserID](studentText)
		if !relationshipParsed || !studentParsed {
			t.Fatalf("the seeded relationship %q of student %q is not a UUID", relationshipText, studentText)
		}
		expected[relationship] = student
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return expected
}

func listenTo(t *testing.T, f cardFixture) *signalCollector {
	t.Helper()
	instance, err := core.NewID[domain.InstanceID](core.CryptoRandom())
	if err != nil {
		t.Fatal(err)
	}
	collector := newSignalCollector()
	adapter, err := listener.New(listener.Dependencies{
		DatabaseURL: f.database.URL, Instance: instance, Heartbeats: listener.ConnectionHeartbeats{},
		Consumer: collector, Logger: slog.New(slog.DiscardHandler), Timing: listener.DefaultTiming(),
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
	select {
	case <-collector.connected:
	case <-time.After(30 * time.Second):
		t.Fatal("the listener did not connect")
	}
	return collector
}

func countedService(t *testing.T, store *repositories.Store) *tutoring.Service {
	t.Helper()
	fieldKey := sha256.Sum256([]byte("test-field-key"))
	cipher, err := core.NewFieldCipher(core.NewSecret(base64.StdEncoding.EncodeToString(fieldKey[:])), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return tutoring.New(tutoring.Dependencies{
		Store: store, Clock: fixedClock{instant: subMicrosecond}, Random: rand.Reader, Keys: keys, Cipher: cipher,
		PublicURL: "https://ovc.example", Logger: slog.New(slog.DiscardHandler),
	})
}

func (d countedDeletion) run(t *testing.T) deletionRun {
	t.Helper()
	ctx := context.Background()
	before := d.counter.statements.Load()
	err := d.store.WithTx(ctx, func(repos repositories.Repositories) error {
		_, err := d.service.OnAccountDeleted(ctx, repos, d.fixture.tutor, domain.RoleTutor, subMicrosecond)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	statements := d.counter.statements.Load() - before
	received, unusual := d.awaitSignals(t)
	return deletionRun{statements: statements, expected: d.expected, received: received, unusual: unusual}
}

func (d countedDeletion) awaitSignals(t *testing.T) (map[domain.EntityID]domain.UserID, []domain.Signal) {
	t.Helper()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		received, unusual := d.collector.snapshot()
		if len(received) >= len(d.expected) || len(unusual) > 0 {
			return received, unusual
		}
		select {
		case <-d.collector.changed:
		case <-deadline.C:
			return received, unusual
		}
	}
}

func TestTutorDeletionStatementsDoNotGrowWithItsRelationships(t *testing.T) {
	t.Parallel()
	few := newCountedDeletion(t, 3)
	many := newCountedDeletion(t, 150)
	beyondOneStatement := newCountedDeletion(t, domain.MaxSignalsPerStatement+1)

	fewRun := few.run(t)
	manyRun := many.run(t)
	beyondRun := beyondOneStatement.run(t)

	for name, run := range map[string]deletionRun{"few": fewRun, "many": manyRun, "beyond one statement": beyondRun} {
		if !maps.Equal(run.received, run.expected) || len(run.unusual) != 0 {
			t.Fatalf("the %s run signalled %d of %d relationships to their students, and also %v", name, len(run.received), len(run.expected), run.unusual)
		}
	}
	if fewRun.statements != manyRun.statements || beyondRun.statements != fewRun.statements+1 {
		t.Fatalf("statements: %d for 3, %d for 150 and %d for %d relationships", fewRun.statements, manyRun.statements, beyondRun.statements,
			domain.MaxSignalsPerStatement+1)
	}
}
