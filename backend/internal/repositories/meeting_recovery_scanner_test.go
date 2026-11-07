package repositories_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/meetings"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/worker"
)

func recoveryScanner(f meetingFixture, logger *slog.Logger) *meetings.RecoveryScanner {
	entropy := make([]byte, 16*200)
	for i := range 200 {
		entropy[16*i] = byte(i + 1)
	}
	return meetings.NewRecoveryScanner(f.store, bytes.NewReader(entropy), logger, clock.System{}.Now)
}

func TestMeetingRecoveryScannerCursor(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	for i := byte(1); i <= 101; i++ {
		recoveryClose(t, f, f.create(t, i))
	}
	scanner := recoveryScanner(f, logger)

	first, firstErr := scanner.Run(t.Context(), instant)
	middle, middleErr := f.repos.MeetingRecovery.Cursor(t.Context())
	reconstructed := meetings.NewRecoveryScanner(repositories.NewStore(f.pool), bytes.NewReader(bytes.Repeat([]byte{202}, 32)), logger, clock.System{}.Now)
	second, secondErr := reconstructed.Run(t.Context(), instant.Add(time.Minute))
	after, afterErr := f.repos.MeetingRecovery.Cursor(t.Context())
	third, thirdErr := scanner.Run(t.Context(), instant.Add(2*time.Minute))

	meetingRequire(t, firstErr, middleErr, secondErr, afterErr, thirdErr)
	if first != 100 || second != 1 || third != 0 || middle.AfterProviderMeetingID != (domain.ProviderMeetingID{100}) || after.AfterProviderMeetingID != (domain.ProviderMeetingID{}) {
		t.Fatal("scanner lost bounded durable progress or wrap")
	}
	for _, private := range []string{"provider_meeting_id", "attempt_id", "create_time", "payload"} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("unsafe scanner diagnostics")
		}
	}
}

func TestMeetingRecoveryScannerFailedItemProgress(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	for _, id := range []byte{10, 11} {
		recoveryClose(t, f, f.create(t, id))
	}
	_, err := f.pool.Exec(t.Context(), `CREATE FUNCTION fail_first_recovery_cycle() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.provider_meeting_id = '0a000000-0000-0000-0000-000000000000' AND NEW.cycle_number = 1 THEN RAISE EXCEPTION 'synthetic recovery item failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER fail_first_recovery_cycle BEFORE UPDATE ON meeting_recovery FOR EACH ROW EXECUTE FUNCTION fail_first_recovery_cycle()`)
	meetingRequire(t, err)
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	scanner := recoveryScanner(f, logger)

	count, scanErr := scanner.Run(t.Context(), instant)
	cursor, cursorErr := f.repos.MeetingRecovery.Cursor(t.Context())
	failed := f.recovery(t, domain.ProviderMeetingID{10})
	progressed := f.recovery(t, domain.ProviderMeetingID{11})

	meetingRequire(t, cursorErr)
	if scanErr == nil || count != 1 || failed.CycleNumber != 0 || progressed.CycleNumber != 1 || cursor.Revision != 4 || cursor.AfterProviderMeetingID != (domain.ProviderMeetingID{}) {
		t.Fatal("failed item pinned progress or was replayed in the same pass")
	}
}

func TestMeetingRecoveryScannerWorkerAndRetention(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateStarted, true)
	second := f.create(t, 11)
	recoveryClose(t, f.meetingFixture, second)
	scanner := recoveryScanner(f.meetingFixture, f.logger)
	job := worker.Job{Name: domain.JobMeetingRecovery, Every: time.Minute, Run: scanner.Run}
	active := worker.New(f.logger, clock.NewShared(f.pool), f.queue, []worker.Lane{{Name: domain.LaneProviderCall, Concurrency: 4, PerformMeetingRecovery: f.service.Cleanup}}, []worker.Job{job}, worker.DefaultTiming())

	firstErr := active.Tick(t.Context(), instant)
	secondErr := active.Tick(t.Context(), instant.Add(time.Minute))
	retained, retainedErr := scanner.Run(t.Context(), instant.Add(7*24*time.Hour))
	boundary := f.recovery(t, second.ProviderMeetingID)
	removed, removedErr := scanner.Run(t.Context(), instant.Add(7*24*time.Hour+time.Minute))
	_, absentErr := f.repos.MeetingRecovery.ByProviderID(t.Context(), second.ProviderMeetingID)
	uncertain := f.recovery(t, f.identity.ProviderMeetingID)

	meetingRequire(t, firstErr, secondErr, retainedErr, removedErr)
	if retained != 1 || removed != 0 || boundary.SettledAt == nil || !errors.Is(absentErr, repositories.ErrNotFound) || uncertain.CycleNumber != 2 || !uncertain.ReconcileCycleUsed || uncertain.SettledAt != nil || uncertain.CycleResult != domain.MeetingRecoveryCyclePending {
		t.Fatal("scanner did not queue exactly one reconciliation cycle after the window or lost the retention boundary")
	}
	if !strings.Contains(f.logs.String(), "meeting.recovery.scanned") {
		t.Fatal("scanner did not emit safe pass diagnostics")
	}
}

func TestMeetingRecoveryScannerCancellation(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	recoveryClose(t, f, f.create(t, 10))
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	scanner := recoveryScanner(f, logger)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tx, err := f.pool.Begin(ctx)
	meetingRequire(t, err)
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	_, err = tx.Exec(ctx, "SELECT singleton FROM meeting_recovery_cursor FOR UPDATE")
	meetingRequire(t, err)
	waiting, cancelWait := context.WithCancel(ctx)
	defer cancelWait()

	running := recoveryRun(t, ctx, "meeting-scanner-cancel", func() error { _, err := scanner.Run(waiting, instant); return err })
	backendPID := waitScannerBlockedBackend(t, ctx, f)
	cancelWait()
	scanErr := meetingAwait(t, ctx, running)
	waitScannerBackendFinished(t, ctx, f, backendPID)
	commitErr := tx.Commit(ctx)
	row := f.recovery(t, domain.ProviderMeetingID{10})
	cursor, cursorErr := f.repos.MeetingRecovery.Cursor(ctx)

	meetingRequire(t, commitErr, cursorErr)
	if !errors.Is(scanErr, context.Canceled) {
		t.Fatal("cancelled scanner did not return cancellation")
	}
	if row.CycleNumber != 0 || cursor.Revision != 1 {
		t.Fatalf("cancelled cursor changed state: cycle=%d revision=%d", row.CycleNumber, cursor.Revision)
	}
	waitScannerConnectionsReleased(t, ctx, f)
}

func TestMeetingRecoveryScannerCursorConflict(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	recoveryClose(t, f, f.create(t, 10))
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	selected, resume := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	calls := 0
	monotonic := func() time.Time {
		calls++
		if calls == 2 {
			close(selected)
			select {
			case <-resume:
			case <-ctx.Done():
			}
		}
		return instant
	}
	losingEntropy := bytes.NewReader(bytes.Repeat([]byte{201}, 16))
	losing := meetings.NewRecoveryScanner(f.store, losingEntropy, logger, monotonic)
	winning := recoveryScanner(f, logger)

	running := recoveryRun(t, ctx, "meeting-scanner-cas", func() error { _, err := losing.Run(ctx, instant); return err })
	select {
	case <-selected:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	count, winnerErr := winning.Run(ctx, instant)
	close(resume)
	loserErr := meetingAwait(t, ctx, running)
	row := f.recovery(t, domain.ProviderMeetingID{10})

	meetingRequire(t, winnerErr, loserErr)
	if count != 1 || row.CycleNumber != 1 || losingEntropy.Len() != 16 {
		t.Fatal("lost cursor ownership still queued a cycle or consumed entropy")
	}
}

func TestMeetingRecoveryScannerDatabaseDeadline(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	recoveryClose(t, f, f.create(t, 10))
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	scanner := recoveryScanner(f, logger)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tx, err := f.pool.Begin(ctx)
	meetingRequire(t, err)
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	_, err = tx.Exec(ctx, "SELECT singleton FROM meeting_recovery_cursor FOR UPDATE")
	meetingRequire(t, err)

	running := recoveryRun(t, ctx, "meeting-scanner-deadline", func() error { _, err := scanner.Run(ctx, instant); return err })
	backendPID := waitScannerBlockedBackend(t, ctx, f)
	scanErr := meetingAwait(t, ctx, running)
	waitScannerBackendFinished(t, ctx, f, backendPID)
	commitErr := tx.Commit(ctx)
	row := f.recovery(t, domain.ProviderMeetingID{10})

	meetingRequire(t, commitErr)
	if !errors.Is(scanErr, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatal("whole-pass deadline did not cancel SQL lock wait before the outer deadline")
	}
	if row.CycleNumber != 0 {
		t.Fatalf("expired scanner admitted cycle %d", row.CycleNumber)
	}
	waitScannerConnectionsReleased(t, ctx, f)
}

func TestMeetingRecoveryScannerFinalCycle(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateStarted, true)
	meetingRequire(t, f.worker.Tick(t.Context(), instant))
	acknowledged, err := f.repos.MeetingRecovery.RecordCreateReturned(t.Context(), f.identity.ProviderMeetingID, domain.MeetingCreateAttemptID(f.identity.ProviderMeetingID), f.generation, instant.Add(time.Minute))
	meetingRequire(t, err)
	if acknowledged != domain.MeetingMutationApplied {
		t.Fatal("late acknowledgement refused")
	}
	scanner := recoveryScanner(f.meetingFixture, f.logger)

	first, firstErr := scanner.Run(t.Context(), instant.Add(time.Minute))
	tickErr := f.worker.Tick(t.Context(), instant.Add(time.Minute))
	second, secondErr := scanner.Run(t.Context(), instant.Add(2*time.Minute))
	row := f.recovery(t, f.identity.ProviderMeetingID)

	meetingRequire(t, firstErr, tickErr, secondErr)
	if first != 1 || second != 0 || row.CycleNumber != 2 || !row.FinalCycleUsed || row.SettledAt == nil {
		t.Fatal("late acknowledgement did not receive exactly one final bounded cycle")
	}
}

func TestMeetingRecoveryScannerReceiptBatch(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	for i := byte(1); i <= 101; i++ {
		identity := f.create(t, i)
		recoveryClose(t, f, identity)
		row := f.recovery(t, identity.ProviderMeetingID)
		applied, err := f.repos.MeetingRecovery.Settle(t.Context(), repositories.MeetingRecoverySettlement{ProviderMeetingID: identity.ProviderMeetingID, Revision: row.Revision, Now: instant})
		meetingRequire(t, err)
		if applied != domain.MeetingMutationApplied {
			t.Fatal("prepared settlement refused")
		}
	}
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	scanner := recoveryScanner(f, logger)

	_, firstErr := scanner.Run(t.Context(), instant.Add(8*24*time.Hour))
	middle, middleErr := f.repos.MeetingRecovery.List(t.Context(), repositories.MeetingRecoveryPage{Limit: 100})
	_, secondErr := scanner.Run(t.Context(), instant.Add(8*24*time.Hour+time.Minute))
	after, afterErr := f.repos.MeetingRecovery.List(t.Context(), repositories.MeetingRecoveryPage{Limit: 100})

	meetingRequire(t, firstErr, middleErr, secondErr, afterErr)
	if len(middle) != 1 || len(after) != 0 {
		t.Fatal("receipt collection exceeded its bounded batch or lost progress")
	}
}

func TestMeetingRecoveryScannerConcurrentAdmission(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	recoveryClose(t, f, f.create(t, 10))
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	selected, resume := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var calls atomic.Int32
	monotonic := func() time.Time {
		if calls.Add(1) == 2 {
			close(selected)
			select {
			case <-resume:
			case <-ctx.Done():
			}
		}
		return instant
	}
	entropy := bytes.NewReader(bytes.Repeat([]byte{201}, 16))
	scanner := meetings.NewRecoveryScanner(f.store, entropy, logger, monotonic)
	cancelled, cancelWaiting := context.WithCancel(ctx)
	cancelWaiting()

	running := recoveryRun(t, ctx, "meeting-scanner-permit", func() error { _, err := scanner.Run(ctx, instant); return err })
	select {
	case <-selected:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	count, waitErr := scanner.Run(cancelled, instant)
	close(resume)
	firstErr := meetingAwait(t, ctx, running)
	row := f.recovery(t, domain.ProviderMeetingID{10})

	meetingRequire(t, firstErr)
	if count != 0 || !errors.Is(waitErr, context.Canceled) || row.CycleNumber != 1 || entropy.Len() != 0 || !strings.Contains(logs.String(), "meeting.recovery.failed") {
		t.Fatal("concurrent admission bypassed cancellation or lost its diagnostics")
	}
}

func waitScannerConnectionsReleased(t *testing.T, ctx context.Context, f meetingFixture) {
	t.Helper()
	for f.pool.Stat().AcquiredConns() != 0 {
		if ctx.Err() != nil {
			t.Fatal("scanner connections were not released within the original test deadline")
		}
		runtime.Gosched()
	}
}

func waitScannerBlockedBackend(t *testing.T, ctx context.Context, f meetingFixture) int32 {
	t.Helper()
	for {
		var pid int32
		err := f.pool.QueryRow(ctx, "SELECT pid FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND cardinality(pg_blocking_pids(pid)) > 0 LIMIT 1").Scan(&pid)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		meetingRequire(t, err)
		return pid
	}
}

func waitScannerBackendFinished(t *testing.T, ctx context.Context, f meetingFixture, pid int32) {
	t.Helper()
	for {
		var active bool
		err := f.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = current_database() AND pid = $1 AND state = 'active')", pid).Scan(&active)
		meetingRequire(t, err)
		if !active {
			return
		}
	}
}
