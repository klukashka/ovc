package worker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/worker"
)

var start = time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)

const testJob domain.JobName = "test.hourly"

type zeroRandom struct{}

func (zeroRandom) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

type harness struct {
	logs     *bytes.Buffer
	store    *repositories.Store
	worker   *worker.Worker
	performs atomic.Int32
	jobRuns  atomic.Int32
	jobErr   error
}

func newHarness(t *testing.T, result func() error) *harness {
	t.Helper()
	return newHarnessWith(t, clock.System{}, worker.DefaultTiming(), func(context.Context) error { return result() })
}

func newHarnessWith(t *testing.T, source clock.Source, timing worker.Timing, result func(context.Context) error) *harness {
	t.Helper()
	pool := testdb.New(t).Pool
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := &harness{logs: logs, store: repositories.NewStore(pool)}
	queue := outbox.New(h.store, zeroRandom{}, logger)
	perform := func(ctx context.Context, _ repositories.ClaimedOutboxRow, _ time.Time) (domain.OutboxDoneReason, error) {
		h.performs.Add(1)
		return domain.DoneSent, result(ctx)
	}
	jobs := []worker.Job{{Name: testJob, Every: time.Hour, Run: func(context.Context, time.Time) (int64, error) {
		h.jobRuns.Add(1)
		return 0, h.jobErr
	}}}
	lanes := []worker.Lane{{Name: domain.LaneEmail, Concurrency: 2, Perform: perform}}
	h.worker = worker.New(logger, source, queue, lanes, jobs, timing)
	return h
}

func (h *harness) insert(t *testing.T) {
	t.Helper()
	h.insertDue(t, start)
}

func (h *harness) insertDue(t *testing.T, due time.Time) {
	t.Helper()
	inserted, err := h.store.Repositories().Outbox.Insert(context.Background(), repositories.NewOutboxRow{
		ID: domain.OutboxID{1}, Lane: domain.LaneEmail, Priority: domain.PriorityNormal, Kind: "test.effect",
		Payload: []byte(`{"to":"tutor@example.org"}`), IdempotencyKey: "effect:1", NextAttemptAt: due, CreatedAt: due,
	})
	if err != nil || !inserted {
		t.Fatalf("insert: %v, %v", inserted, err)
	}
}

func (h *harness) tick(t *testing.T, now time.Time) {
	t.Helper()
	if err := h.worker.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) row(t *testing.T) repositories.OutboxRow {
	t.Helper()
	row, err := h.store.Repositories().Outbox.ByID(context.Background(), domain.OutboxID{1})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestDispatcherMarksRowDeadAfterEightAttempts(t *testing.T) {
	h := newHarness(t, func() error { return errors.New("unreachable provider") })
	h.insert(t)

	for attempt := range domain.OutboxMaxAttempts + 2 {
		h.tick(t, start.Add(time.Duration(attempt)*2*time.Hour))
	}

	row := h.row(t)
	if row.Status != domain.OutboxDead || row.Attempts != domain.OutboxMaxAttempts || h.performs.Load() != domain.OutboxMaxAttempts {
		t.Fatalf("status %s, attempts %d, performed %d times", row.Status, row.Attempts, h.performs.Load())
	}
}

func TestFailedRowWaitsForItsRetryDelay(t *testing.T) {
	h := newHarness(t, func() error { return errors.New("unreachable provider") })
	h.insert(t)

	h.tick(t, start)
	h.tick(t, start.Add(29*time.Second))

	row := h.row(t)
	if row.Status != domain.OutboxPending || h.performs.Load() != 1 || !row.NextAttemptAt.Equal(start.Add(domain.OutboxFirstRetryDelay)) {
		t.Fatalf("status %s, performed %d, next attempt %v", row.Status, h.performs.Load(), row.NextAttemptAt)
	}
}

func TestPermanentFailureIsDeadAfterOneAttempt(t *testing.T) {
	h := newHarness(t, func() error { return domain.PermanentEffect(errors.New("malformed message")) })
	h.insert(t)

	h.tick(t, start)
	h.tick(t, start.Add(2*time.Hour))

	row := h.row(t)
	if row.Status != domain.OutboxDead || h.performs.Load() != 1 || row.LastErrorCode == nil || *row.LastErrorCode != "permanent" {
		t.Fatalf("status %s, performed %d, code %v", row.Status, h.performs.Load(), row.LastErrorCode)
	}
}

func TestTickTwiceWithSameNowHasNoExtraEffect(t *testing.T) {
	h := newHarness(t, func() error { return nil })
	h.insert(t)

	h.tick(t, start)
	h.tick(t, start)

	row := h.row(t)
	if row.Status != domain.OutboxDone || h.performs.Load() != 1 || h.jobRuns.Load() != 1 {
		t.Fatalf("status %s, performed %d, job runs %d", row.Status, h.performs.Load(), h.jobRuns.Load())
	}
}

func TestHourlyJobCatchesUpOnceAfterDowntime(t *testing.T) {
	h := newHarness(t, func() error { return nil })

	h.tick(t, start)
	h.tick(t, start.Add(30*time.Minute))
	h.tick(t, start.Add(5*time.Hour))
	h.tick(t, start.Add(5*time.Hour+time.Minute))

	if h.jobRuns.Load() != 2 {
		t.Fatalf("the hourly job ran %d times, want 2", h.jobRuns.Load())
	}
}

func TestDeadRowIsReportedAtErrorWithoutItsPayload(t *testing.T) {
	h := newHarness(t, func() error { return domain.PermanentEffect(errors.New("malformed message")) })
	h.insert(t)

	h.tick(t, start)

	logs := h.logs.String()
	if !strings.Contains(logs, `"level":"ERROR","msg":"outbox.row.dead"`) || !strings.Contains(logs, `"error_code":"permanent"`) {
		t.Fatalf("no ERROR outbox.row.dead record in:\n%s", logs)
	}
	if strings.Contains(logs, "tutor@example.org") || strings.Contains(logs, "malformed message") {
		t.Fatalf("the records carry the payload or the raw error:\n%s", logs)
	}
}

func TestPanickingEffectCountsAsAFailedAttempt(t *testing.T) {
	h := newHarness(t, func() error { panic("effect bug") })
	h.insert(t)

	h.tick(t, start)

	row := h.row(t)
	if row.Status != domain.OutboxPending || row.Attempts != 1 || row.LastErrorCode == nil || !strings.Contains(h.logs.String(), "panic.recovered") {
		t.Fatalf("status %s, attempts %d, code %v", row.Status, row.Attempts, row.LastErrorCode)
	}
}

func TestShutdownLetsAnEffectInProgressFinish(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	h := newHarnessWith(t, clock.System{}, worker.DefaultTiming(), func(ctx context.Context) error {
		close(started)
		<-release
		return ctx.Err()
	})
	h.insertDue(t, time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := core.Run(ctx, slog.New(slog.DiscardHandler), "worker-under-test", func() error { return h.worker.Run(ctx) })
	waitFor(t, started)

	cancel()
	close(release)
	var runErr error
	select {
	case runErr = <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("the worker did not stop")
	}

	row := h.row(t)
	if runErr != nil || row.Status != domain.OutboxDone {
		t.Fatalf("run %v, status %s", runErr, row.Status)
	}
}

func waitFor(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatal("the effect never started")
	}
}

func TestPassPastItsClaimBudgetClaimsNothing(t *testing.T) {
	wall := start
	exhausted := worker.Timing{Wall: func() time.Time { return wall }, ClaimBudget: 0}
	h := newHarnessWith(t, clock.System{}, exhausted, func(context.Context) error { return nil })
	h.insert(t)

	h.tick(t, start)

	if row := h.row(t); row.Status != domain.OutboxPending || h.performs.Load() != 0 {
		t.Fatalf("status %s, performed %d", row.Status, h.performs.Load())
	}
}

type jobRun struct {
	Level   string `json:"level"`
	Message string `json:"msg"`
	Result  string `json:"result"`
	Kind    string `json:"kind"`
}

func jobRunRecord(t *testing.T, logs string) jobRun {
	t.Helper()
	for line := range strings.Lines(logs) {
		var record jobRun
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("not a JSON record: %s", line)
		}
		if record.Message == "job.run" {
			return record
		}
	}
	t.Fatal("no job.run record")
	return jobRun{}
}

func TestAFailingJobDoesNotStopDispatch(t *testing.T) {
	h := newHarness(t, func() error { return nil })
	h.jobErr = errors.New("one account could not be purged")
	h.insert(t)

	err := h.worker.Tick(context.Background(), start)

	if err != nil {
		t.Fatalf("a failing job stopped the pass: %v", err)
	}
	if h.performs.Load() != 1 || h.row(t).Status != domain.OutboxDone {
		t.Fatalf("the email lane did not run after the failing job: performs %d, state %s", h.performs.Load(), h.row(t).Status)
	}
	if record := jobRunRecord(t, h.logs.String()); record.Level != "ERROR" || record.Result != "failed" || record.Kind != core.ErrorKind(h.jobErr) {
		t.Fatalf("the failed job was not reported at ERROR with its kind: %+v", record)
	}
}
