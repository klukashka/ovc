package repositories_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/meetings"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/worker"
)

type recoveryWorkerFixture struct {
	meetingFixture
	identity   domain.MeetingIdentity
	provider   *meeting.SharedFake
	service    *meetings.Service
	queue      *outbox.Service
	worker     *worker.Worker
	logs       *bytes.Buffer
	logger     *slog.Logger
	generation domain.ProviderCreateTime
}

func newRecoveryWorker(t *testing.T, state domain.MeetingCreateState, present bool) recoveryWorkerFixture {
	t.Helper()
	f := recoveryWorkerFixture{meetingFixture: newMeetingFixture(t, testdb.New(t)), logs: &bytes.Buffer{}}
	f.logger = slog.New(slog.NewJSONHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sharedClock := clock.NewShared(f.pool)
	_, err := sharedClock.Set(t.Context(), instant)
	meetingRequire(t, err)
	f.provider, err = meeting.NewSharedFake(t.Context(), f.pool, sharedClock, bytes.NewReader(make([]byte, 4096)), f.logger, meeting.FakeLimits{Meetings: 8, Links: 8, Participants: 8, Generations: 32})
	meetingRequire(t, err)
	f.identity = f.create(t, 10)
	f.generation = 100
	if present {
		f.generation, err = f.provider.CreateMeeting(t.Context(), domain.Meeting{ID: f.identity.ProviderMeetingID, Name: "Private lesson sentinel"})
		meetingRequire(t, err)
	}
	if state == domain.MeetingCreateReturned {
		f.acknowledge(t, f.identity, f.generation)
	} else if state != domain.MeetingCreatePrepared {
		if f.begin(t, f.identity, domain.MeetingCreateAttemptID(f.identity.ProviderMeetingID), 1) != domain.MeetingMutationApplied {
			t.Fatal("fixture begin refused")
		}
		if state == domain.MeetingCreateUncertain {
			_, err = f.repos.MeetingRecovery.MarkCreateUncertain(t.Context(), f.identity.ProviderMeetingID, domain.MeetingCreateAttemptID(f.identity.ProviderMeetingID), instant)
			meetingRequire(t, err)
		}
	}
	recoveryClose(t, f.meetingFixture, f.identity)
	if recoveryQueue(t, f.meetingFixture, f.identity.ProviderMeetingID, domain.OutboxID{30}, domain.MeetingRecoveryAutomatic) != domain.MeetingMutationApplied {
		t.Fatal("fixture queue refused")
	}
	f.service = meetings.New(f.store, f.provider, domain.MeetingProviderFake, f.logger)
	f.queue = outbox.New(f.store, bytes.NewReader(make([]byte, 8192)), f.logger)
	f.worker = worker.New(f.logger, sharedClock, f.queue, []worker.Lane{{Name: domain.LaneProviderCall, Concurrency: 4, PerformMeetingRecovery: f.service.Cleanup}}, nil, worker.DefaultTiming())
	return f
}

func (f recoveryWorkerFixture) assertResult(t *testing.T, result domain.MeetingRecoveryCycleResult, status domain.OutboxStatus, capacity int64) {
	t.Helper()
	row := f.recovery(t, f.identity.ProviderMeetingID)
	cycle, cycleErr := f.repos.MeetingRecovery.Cycle(t.Context(), domain.OutboxID{30})
	effect, effectErr := f.repos.Outbox.ByID(t.Context(), domain.OutboxID{30})
	count, countErr := f.repos.MeetingRecovery.Capacity(t.Context())
	meetingRequire(t, cycleErr, effectErr, countErr)
	if row.CycleResult != result || cycle.Result != result || effect.Status != status || count != capacity || (row.SettledAt != nil) != (result == domain.MeetingRecoveryCycleSettled) {
		t.Fatalf("worker result: ledger=%s receipt=%s effect=%s capacity=%d", row.CycleResult, cycle.Result, effect.Status, count)
	}
	for _, private := range []string{"Private lesson sentinel", "provider_meeting_id", "create_time", "attempt_id", "payload"} {
		if strings.Contains(f.logs.String(), private) {
			t.Fatalf("unsafe worker diagnostic field %s", private)
		}
	}
}

func TestMeetingRecoveryWorker(t *testing.T) {
	t.Run("terminal evidence", testRecoveryWorkerTerminal)
	t.Run("generation conflict", testRecoveryWorkerGenerationConflict)
	t.Run("finite unavailable attempts", testRecoveryWorkerUnavailable)
	t.Run("late acknowledgement fence", testRecoveryWorkerLateAcknowledgement)
	t.Run("cycle identity refusal", testRecoveryWorkerCycleIdentity)
	t.Run("single available connection", testRecoveryWorkerSingleConnection)
}

func testRecoveryWorkerTerminal(t *testing.T) {
	cases := []struct {
		name    string
		state   domain.MeetingCreateState
		present bool
		legacy  bool
	}{
		{"prepared revoked", domain.MeetingCreatePrepared, false, false},
		{"returned absent", domain.MeetingCreateReturned, false, false},
		{"returned present", domain.MeetingCreateReturned, true, false},
		{"started absent", domain.MeetingCreateStarted, false, false},
		{"started present", domain.MeetingCreateStarted, true, false},
		{"uncertain absent", domain.MeetingCreateUncertain, false, false},
		{"uncertain present", domain.MeetingCreateUncertain, true, false},
		{"legacy absent", domain.MeetingCreateUncertain, false, true},
		{"legacy present", domain.MeetingCreateUncertain, true, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := newRecoveryWorker(t, test.state, test.present)
			if test.legacy {
				_, err := f.pool.Exec(t.Context(), "UPDATE meeting_recovery SET origin = 'legacy', attempt_id = NULL WHERE provider_meeting_id = $1", f.identity.ProviderMeetingID)
				meetingRequire(t, err)
			}
			if test.state == domain.MeetingCreatePrepared {
				meetingRequire(t, f.provider.SimulateUnavailable(t.Context(), true))
			}

			err := f.worker.Tick(t.Context(), instant)
			repeatErr := f.worker.Tick(t.Context(), instant)

			meetingRequire(t, err, repeatErr)
			result, capacity := domain.MeetingRecoveryCycleUnresolved, int64(1)
			if test.state == domain.MeetingCreatePrepared || test.state == domain.MeetingCreateReturned {
				result, capacity = domain.MeetingRecoveryCycleSettled, 0
			}
			f.assertResult(t, result, domain.OutboxDone, capacity)
			if test.state != domain.MeetingCreatePrepared {
				info, infoErr := f.provider.InspectMeeting(t.Context(), f.identity.ProviderMeetingID)
				meetingRequire(t, infoErr)
				if info.Status != domain.MeetingNotFound {
					t.Fatal("worker left the owned provider generation present")
				}
			}
		})
	}
}

func testRecoveryWorkerGenerationConflict(t *testing.T) {
	f := newRecoveryWorker(t, domain.MeetingCreateReturned, true)
	meetingRequire(t, f.provider.EndMeeting(t.Context(), domain.Meeting{ID: f.identity.ProviderMeetingID, CreateTime: f.generation}))
	replacement, createErr := f.provider.CreateMeeting(t.Context(), domain.Meeting{ID: f.identity.ProviderMeetingID, Name: "Replacement sentinel"})
	meetingRequire(t, createErr)

	err := f.worker.Tick(t.Context(), instant)
	info, infoErr := f.provider.InspectMeeting(t.Context(), f.identity.ProviderMeetingID)

	meetingRequire(t, err, infoErr)
	f.assertResult(t, domain.MeetingRecoveryCycleConflict, domain.OutboxDone, 1)
	if replacement == f.generation || info.CreateTime != replacement || info.Status != domain.MeetingCreated {
		t.Fatal("cleanup destroyed an unrelated replacement generation")
	}
}

func testRecoveryWorkerUnavailable(t *testing.T) {
	f := newRecoveryWorker(t, domain.MeetingCreateReturned, true)
	meetingRequire(t, f.provider.SimulateUnavailable(t.Context(), true))

	for attempt := range domain.OutboxMaxAttempts + 2 {
		meetingRequire(t, f.worker.Tick(t.Context(), instant.Add(time.Duration(attempt)*2*time.Hour)))
	}
	row, err := f.repos.Outbox.ByID(t.Context(), domain.OutboxID{30})

	meetingRequire(t, err)
	f.assertResult(t, domain.MeetingRecoveryCycleExhausted, domain.OutboxDead, 1)
	if row.Attempts != domain.OutboxMaxAttempts || !strings.Contains(f.logs.String(), "meeting.cleanup.failed") || !strings.Contains(f.logs.String(), "outbox.row.retried") || !strings.Contains(f.logs.String(), "outbox.row.dead") {
		t.Fatal("provider failure did not use finite worker retries")
	}
}

func testRecoveryWorkerLateAcknowledgement(t *testing.T) {
	f := newRecoveryWorker(t, domain.MeetingCreateStarted, true)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tx, err := f.pool.Begin(ctx)
	meetingRequire(t, err)
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	_, err = tx.Exec(ctx, "SELECT id FROM fake_meeting_state WHERE id FOR UPDATE")
	meetingRequire(t, err)

	running := recoveryRun(t, ctx, "meeting-worker-late-ack", func() error { return f.worker.Tick(ctx, instant) })
	waitMeetingLock(t, ctx, f.meetingFixture)
	applied, ackErr := f.repos.MeetingRecovery.RecordCreateReturned(ctx, f.identity.ProviderMeetingID, domain.MeetingCreateAttemptID(f.identity.ProviderMeetingID), f.generation, instant)
	commitErr := tx.Commit(ctx)
	runErr := meetingAwait(t, ctx, running)

	meetingRequire(t, ackErr, commitErr, runErr)
	f.assertResult(t, domain.MeetingRecoveryCyclePending, domain.OutboxInProgress, 1)
	if applied != domain.MeetingMutationApplied || !strings.Contains(f.logs.String(), "outbox.lease.lost") || strings.Contains(f.logs.String(), "outbox.row.done") {
		t.Fatal("late acknowledgement was overwritten by stale worker evidence")
	}
}

func testRecoveryWorkerCycleIdentity(t *testing.T) {
	f := newRecoveryWorker(t, domain.MeetingCreateReturned, true)
	row := outbox.Row{ID: domain.OutboxID{31}, Kind: domain.KindMeetingCleanup}

	_, err := f.service.Cleanup(t.Context(), row, instant)
	info, infoErr := f.provider.InspectMeeting(t.Context(), f.identity.ProviderMeetingID)

	meetingRequire(t, infoErr)
	if !errors.Is(err, repositories.ErrNotFound) || info.CreateTime != f.generation {
		t.Fatal("missing cycle receipt reached the provider")
	}
}

func testRecoveryWorkerSingleConnection(t *testing.T) {
	f := newRecoveryWorker(t, domain.MeetingCreateReturned, true)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for range testdb.DefaultPoolConnections - 1 {
		conn, err := f.pool.Acquire(ctx)
		meetingRequire(t, err)
		defer conn.Release()
	}

	err := f.worker.Tick(ctx, instant)

	meetingRequire(t, err)
	f.assertResult(t, domain.MeetingRecoveryCycleSettled, domain.OutboxDone, 0)
}

func TestMeetingRecoveryWorkerDispatchRefusals(t *testing.T) {
	for _, name := range []string{"missing typed handler", "wrong lane"} {
		t.Run(name, func(t *testing.T) {
			f := newRecoveryWorker(t, domain.MeetingCreateReturned, true)
			lane := worker.Lane{Name: domain.LaneProviderCall, Concurrency: 4}
			if name == "wrong lane" {
				lane.Name = domain.LaneEmail
				lane.PerformMeetingRecovery = f.service.Cleanup
				_, err := f.pool.Exec(t.Context(), "UPDATE outbox SET lane = 'email' WHERE id = $1", domain.OutboxID{30})
				meetingRequire(t, err)
			}
			w := worker.New(f.logger, clock.System{}, f.queue, []worker.Lane{lane}, nil, worker.DefaultTiming())

			err := w.Tick(t.Context(), instant)
			info, infoErr := f.provider.InspectMeeting(t.Context(), f.identity.ProviderMeetingID)

			meetingRequire(t, err, infoErr)
			f.assertResult(t, domain.MeetingRecoveryCyclePending, domain.OutboxPending, 1)
			if info.CreateTime != f.generation || strings.Contains(f.logs.String(), "panic") || !strings.Contains(f.logs.String(), "outbox.row.retried") {
				t.Fatal("invalid dispatcher configuration called the provider or panicked")
			}
		})
	}
}

func TestMeetingRecoveryWorkerCancellation(t *testing.T) {
	f := newRecoveryWorker(t, domain.MeetingCreateReturned, true)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tx, err := f.pool.Begin(ctx)
	meetingRequire(t, err)
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	_, err = tx.Exec(ctx, "SELECT id FROM fake_meeting_state WHERE id FOR UPDATE")
	meetingRequire(t, err)
	waiting, cancelWait := context.WithCancel(ctx)
	defer cancelWait()

	running := recoveryRun(t, ctx, "meeting-cleanup-cancelled", func() error {
		_, cleanupErr := f.service.Cleanup(waiting, outbox.Row{ID: domain.OutboxID{30}, Kind: domain.KindMeetingCleanup}, instant)
		return cleanupErr
	})
	waitMeetingLock(t, ctx, f.meetingFixture)
	cancelWait()
	cleanupErr := meetingAwait(t, ctx, running)
	commitErr := tx.Commit(ctx)
	info, infoErr := f.provider.InspectMeeting(ctx, f.identity.ProviderMeetingID)

	meetingRequire(t, commitErr, infoErr)
	f.assertResult(t, domain.MeetingRecoveryCyclePending, domain.OutboxPending, 1)
	if !errors.Is(cleanupErr, context.Canceled) || info.CreateTime != f.generation || !strings.Contains(f.logs.String(), "meeting.cleanup.failed") || strings.Contains(f.logs.String(), "meeting.cleanup.evaluated") || f.pool.Stat().AcquiredConns() != 0 {
		t.Fatal("cancellation lost recovery evidence, emitted success or retained a connection")
	}
}

func TestMeetingRecoveryReclaimFailureIsolatesItem(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateReturned, true)
	second := f.create(t, 11)
	recoveryClose(t, f.meetingFixture, second)
	if recoveryQueue(t, f.meetingFixture, second.ProviderMeetingID, domain.OutboxID{31}, domain.MeetingRecoveryAutomatic) != domain.MeetingMutationApplied {
		t.Fatal("second queue refused")
	}
	claims, err := f.queue.Claim(t.Context(), domain.LaneProviderCall, 2, instant)
	meetingRequire(t, err)
	if len(claims) != 2 {
		t.Fatal("expected both cleanup claims")
	}
	_, err = f.pool.Exec(t.Context(), `CREATE FUNCTION fail_first_cleanup_reclaim() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.id = '1e000000-0000-0000-0000-000000000000' AND NEW.status = 'pending' THEN RAISE EXCEPTION 'synthetic reclaim item failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER fail_first_cleanup_reclaim BEFORE UPDATE ON outbox FOR EACH ROW EXECUTE FUNCTION fail_first_cleanup_reclaim()`)
	meetingRequire(t, err)

	tickErr := f.worker.Tick(t.Context(), instant.Add(outbox.LeaseDuration))
	failed, failedErr := f.repos.Outbox.ByID(t.Context(), domain.OutboxID{30})
	progressed, progressedErr := f.repos.Outbox.ByID(t.Context(), domain.OutboxID{31})
	output := f.logs.String()

	meetingRequire(t, tickErr, failedErr, progressedErr)
	if failed.Status != domain.OutboxInProgress || progressed.Status != domain.OutboxDone {
		t.Fatalf("failed reclaim item stopped other work: failed=%s progressed=%s", failed.Status, progressed.Status)
	}
	if !strings.Contains(output, `"msg":"outbox.lease.reclaim_failed","outbox_id":"1e000000000000000000000000000000","kind":"meeting_cleanup","error_code":"sqlstate_P0001"`) || strings.Contains(output, "synthetic reclaim item failure") {
		t.Fatal("reclaim item failure was not reported with safe structured diagnostics")
	}
}
