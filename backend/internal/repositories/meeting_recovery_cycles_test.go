package repositories_test

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
)

func recoveryClose(t *testing.T, f meetingFixture, identity domain.MeetingIdentity) {
	t.Helper()
	outcome := f.mutate(t, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
		return repo.CloseUnderLessonLock(t.Context(), domain.MeetingExpectation{Identity: identity, State: domain.MeetingCreating}, domain.MeetingCloseProviderError, instant)
	})
	if outcome != domain.MeetingMutationApplied {
		t.Fatal("fixture close refused")
	}
}

func recoveryQueue(t *testing.T, f meetingFixture, id domain.ProviderMeetingID, outbox domain.OutboxID, mode domain.MeetingRecoveryCycleMode) domain.MeetingMutation {
	t.Helper()
	row := f.recovery(t, id)
	outcome, err := f.repos.MeetingRecovery.QueueCycle(t.Context(), repositories.MeetingRecoveryQueue{ProviderMeetingID: id, Revision: row.Revision, OutboxID: outbox, Mode: mode, Now: instant})
	meetingRequire(t, err)
	return outcome
}

func TestMeetingRecoveryCapacity(t *testing.T) {
	t.Parallel()
	t.Run("admission race and rollback", testRecoveryCapacityAdmission)
	t.Run("settlement fences dispatch and releases once", testRecoverySettlement)
	t.Run("uncertain evidence never releases", testRecoveryUncertainSettlement)
}

func testRecoveryCapacityAdmission(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	other := independentMeetingFixture(t, f)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO meeting_recovery (provider_meeting_id, provider, origin, create_state, dispatched_at, created_at, updated_at)
 SELECT md5('capacity-' || item::text)::uuid, 'fake', 'legacy', 'uncertain', $1, $1, $1 FROM generate_series(1, 4095) AS item`, instant)
	meetingRequire(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE meeting_recovery_capacity SET unsettled = 4095")
	meetingRequire(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	firstInput, secondInput := meetingInput(10, f.lesson), meetingInput(11, f.otherLesson)
	release, first := holdRecoveryTransaction(t, ctx, f, func(repos repositories.Repositories) error {
		return repos.Meetings.CreateUnderLessonLock(ctx, firstInput)
	})

	second := recoveryRun(t, ctx, "capacity-contender", func() error { return other.repos.Meetings.CreateUnderLessonLock(ctx, secondInput) })
	waitMeetingLock(t, ctx, f)
	release()
	firstErr, secondErr := meetingAwait(t, ctx, first), meetingAwait(t, ctx, second)
	count, countErr := f.repos.MeetingRecovery.Capacity(ctx)
	_, missing := f.repos.Meetings.ByID(ctx, secondInput.LessonID, secondInput.ID)

	meetingRequire(t, firstErr, countErr)
	if !errors.Is(secondErr, repositories.ErrMeetingRecoveryCapacity) || !errors.Is(missing, repositories.ErrNotFound) || count != 4096 {
		t.Fatal("admission race exceeded capacity or leaked a business row")
	}
}

func testRecoverySettlement(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	recoveryClose(t, f, identity)
	before := f.recovery(t, identity.ProviderMeetingID)
	input := repositories.MeetingRecoverySettlement{ProviderMeetingID: identity.ProviderMeetingID, Revision: before.Revision, Now: instant}

	settled, err := f.repos.MeetingRecovery.Settle(t.Context(), input)
	repeated, repeatErr := f.repos.MeetingRecovery.Settle(t.Context(), input)
	row := f.recovery(t, identity.ProviderMeetingID)
	began := f.begin(t, identity, domain.MeetingCreateAttemptID{20}, row.Revision)
	count, countErr := f.repos.MeetingRecovery.Capacity(t.Context())
	early, earlyErr := f.repos.MeetingRecovery.CleanupReceipts(t.Context(), instant.Add(7*24*time.Hour), 100)
	removed, removeErr := f.repos.MeetingRecovery.CleanupReceipts(t.Context(), instant.Add(7*24*time.Hour+time.Second), 100)
	opened, openErr := f.repos.Meetings.OpenUnderLessonLock(t.Context(), identity, 100, instant)
	business, businessErr := f.repos.Meetings.ByID(t.Context(), identity.LessonID, identity.ID)

	meetingRequire(t, err, repeatErr, countErr, earlyErr, removeErr, openErr, businessErr)
	if settled != domain.MeetingMutationApplied || repeated != domain.MeetingMutationRefused || began != domain.MeetingMutationRefused || row.SettledAt == nil || !row.CleanupRequested || count != 0 || early != 0 || removed != 1 || opened != domain.MeetingMutationRefused || business.State != domain.MeetingClosed {
		t.Fatal("settlement failed fencing, exact release, retention boundary or business preservation")
	}
}

func testRecoveryUncertainSettlement(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	attempt := domain.MeetingCreateAttemptID{20}
	if f.begin(t, identity, attempt, 1) != domain.MeetingMutationApplied {
		t.Fatal("begin refused")
	}
	recoveryClose(t, f, identity)
	before := f.recovery(t, identity.ProviderMeetingID)
	observed := domain.ProviderCreateTime(100)

	absent, absentErr := f.repos.MeetingRecovery.Settle(t.Context(), repositories.MeetingRecoverySettlement{ProviderMeetingID: identity.ProviderMeetingID, Revision: before.Revision, Now: instant})
	ended, endedErr := f.repos.MeetingRecovery.Settle(t.Context(), repositories.MeetingRecoverySettlement{ProviderMeetingID: identity.ProviderMeetingID, Revision: before.Revision, ObservedCreateTime: &observed, Now: instant})
	removed, removeErr := f.repos.MeetingRecovery.CleanupReceipts(t.Context(), instant.Add(365*24*time.Hour), 100)
	count, countErr := f.repos.MeetingRecovery.Capacity(t.Context())

	meetingRequire(t, absentErr, endedErr, removeErr, countErr)
	if absent != domain.MeetingMutationRefused || ended != domain.MeetingMutationRefused || removed != 0 || count != 1 {
		t.Fatal("uncertain remote outcome was forgotten or released")
	}
}

func TestMeetingRecoveryFiniteCycles(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	attempt := domain.MeetingCreateAttemptID{20}
	if f.begin(t, identity, attempt, 1) != domain.MeetingMutationApplied {
		t.Fatal("begin refused")
	}
	recoveryClose(t, f, identity)
	firstID, finalID, operatorID := domain.OutboxID{31}, domain.OutboxID{32}, domain.OutboxID{33}
	first := recoveryQueue(t, f, identity.ProviderMeetingID, firstID, domain.MeetingRecoveryAutomatic)

	completed, completeErr := f.repos.MeetingRecovery.CompleteCycle(t.Context(), repositories.MeetingRecoveryCompletion{ProviderMeetingID: identity.ProviderMeetingID, OutboxID: firstID, Result: domain.MeetingRecoveryCycleExhausted, Now: instant})
	_, deleteErr := f.pool.Exec(t.Context(), "DELETE FROM outbox WHERE id = $1", firstID)
	spent := recoveryQueue(t, f, identity.ProviderMeetingID, finalID, domain.MeetingRecoveryAutomatic)
	acknowledged, ackErr := f.repos.MeetingRecovery.RecordCreateReturned(t.Context(), identity.ProviderMeetingID, attempt, 100, instant)
	final := recoveryQueue(t, f, identity.ProviderMeetingID, finalID, domain.MeetingRecoveryAutomatic)
	duplicate := recoveryQueue(t, f, identity.ProviderMeetingID, finalID, domain.MeetingRecoveryAutomatic)
	stale, staleErr := f.repos.MeetingRecovery.CompleteCycle(t.Context(), repositories.MeetingRecoveryCompletion{ProviderMeetingID: identity.ProviderMeetingID, OutboxID: firstID, Result: domain.MeetingRecoveryCycleUnresolved, Now: instant})
	finalResult, finalErr := f.repos.MeetingRecovery.CompleteCycle(t.Context(), repositories.MeetingRecoveryCompletion{ProviderMeetingID: identity.ProviderMeetingID, OutboxID: finalID, Result: domain.MeetingRecoveryCycleConflict, Now: instant})
	_, repeatErr := f.repos.MeetingRecovery.RecordCreateReturned(t.Context(), identity.ProviderMeetingID, attempt, 100, instant)
	third := recoveryQueue(t, f, identity.ProviderMeetingID, operatorID, domain.MeetingRecoveryAutomatic)
	operator := recoveryQueue(t, f, identity.ProviderMeetingID, operatorID, domain.MeetingRecoveryOperator)
	history, historyErr := f.repos.MeetingRecovery.Cycles(t.Context(), repositories.MeetingRecoveryCyclePage{ProviderMeetingID: identity.ProviderMeetingID, Limit: 100})
	row := f.recovery(t, identity.ProviderMeetingID)

	meetingRequire(t, completeErr, deleteErr, ackErr, staleErr, finalErr, repeatErr, historyErr)
	for _, outcome := range []domain.MeetingMutation{first, completed, acknowledged, final, duplicate, finalResult, operator} {
		if outcome != domain.MeetingMutationApplied {
			t.Fatal("valid cycle transition refused")
		}
	}
	if spent != domain.MeetingMutationRefused || stale != domain.MeetingMutationRefused || third != domain.MeetingMutationRefused || len(history) != 3 || history[0].Result != domain.MeetingRecoveryCycleExhausted || history[1].Result != domain.MeetingRecoveryCycleConflict || row.CycleNumber != 3 || !row.FinalCycleUsed || row.AttemptID == nil || *row.AttemptID != attempt {
		t.Fatal("finite cycles or retained evidence failed")
	}
}

func TestMeetingRecoveryCursor(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	repo := f.repos.MeetingRecovery
	before, err := repo.Cursor(t.Context())
	meetingRequire(t, err)

	advanced, advanceErr := repo.AdvanceCursor(t.Context(), before, domain.ProviderMeetingID{10})
	stale, staleErr := repo.AdvanceCursor(t.Context(), before, domain.ProviderMeetingID{11})
	reconstructed := repositories.NewStore(f.pool).Repositories().MeetingRecovery
	next, nextErr := reconstructed.Cursor(t.Context())
	wrapped, wrapErr := reconstructed.AdvanceCursor(t.Context(), next, domain.ProviderMeetingID{})
	after, afterErr := reconstructed.Cursor(t.Context())

	meetingRequire(t, advanceErr, staleErr, nextErr, wrapErr, afterErr)
	if advanced != domain.MeetingMutationApplied || stale != domain.MeetingMutationRefused || wrapped != domain.MeetingMutationApplied || next.AfterProviderMeetingID != (domain.ProviderMeetingID{10}) || after.AfterProviderMeetingID != (domain.ProviderMeetingID{}) || after.Revision != before.Revision+2 {
		t.Fatal("durable cursor lost compare-and-swap progress or wrap")
	}
}

func TestMeetingRecoveryCycleAtomicity(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	recoveryClose(t, f, identity)
	before := f.recovery(t, identity.ProviderMeetingID)
	aborted := errors.New("abort recovery cycle")
	input := repositories.MeetingRecoveryQueue{ProviderMeetingID: identity.ProviderMeetingID, Revision: before.Revision, OutboxID: domain.OutboxID{30}, Mode: domain.MeetingRecoveryAutomatic, Now: instant}

	rollbackErr := f.store.WithTx(t.Context(), func(repos repositories.Repositories) error {
		outcome, err := repos.MeetingRecovery.QueueCycle(t.Context(), input)
		if err != nil {
			return err
		}
		if outcome != domain.MeetingMutationApplied {
			return errors.New("queue refused")
		}
		return aborted
	})
	_, missingCycle := f.repos.MeetingRecovery.Cycle(t.Context(), input.OutboxID)
	_, missingOutbox := f.repos.Outbox.ByID(t.Context(), input.OutboxID)
	afterRollback := f.recovery(t, identity.ProviderMeetingID)
	committed, commitErr := f.repos.MeetingRecovery.QueueCycle(t.Context(), input)
	row, outboxErr := f.repos.Outbox.ByID(t.Context(), input.OutboxID)
	receipt, receiptErr := f.repos.MeetingRecovery.Cycle(t.Context(), input.OutboxID)
	count, countErr := f.repos.MeetingRecovery.Capacity(t.Context())

	meetingRequire(t, commitErr, outboxErr, receiptErr, countErr)
	if !errors.Is(rollbackErr, aborted) || !errors.Is(missingCycle, repositories.ErrNotFound) || !errors.Is(missingOutbox, repositories.ErrNotFound) || afterRollback.Revision != before.Revision || afterRollback.CycleNumber != 0 || committed != domain.MeetingMutationApplied || row.Lane != domain.LaneProviderCall || row.Priority != domain.PriorityNormal || row.Kind != domain.KindMeetingCleanup || row.ID != receipt.OutboxID || row.Attempts != 0 || count != 1 {
		t.Fatal("cycle queue was not atomic with its outbox and durable receipt")
	}
}

func TestMeetingRecoveryAdmissionRollback(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	input := meetingInput(10, f.lesson)
	aborted := errors.New("abort recovery admission")

	rollbackErr := f.store.WithTx(t.Context(), func(repos repositories.Repositories) error {
		if err := repos.Meetings.CreateUnderLessonLock(t.Context(), input); err != nil {
			return err
		}
		return aborted
	})
	afterRollback, rollbackCountErr := f.repos.MeetingRecovery.Capacity(t.Context())
	createdErr := f.repos.Meetings.CreateUnderLessonLock(t.Context(), input)
	duplicateErr := f.repos.Meetings.CreateUnderLessonLock(t.Context(), input)
	afterDuplicate, duplicateCountErr := f.repos.MeetingRecovery.Capacity(t.Context())

	meetingRequire(t, rollbackCountErr, createdErr, duplicateCountErr)
	if !errors.Is(rollbackErr, aborted) || afterRollback != 0 || afterDuplicate != 1 {
		t.Fatal("admission leaked capacity on rollback or duplicate")
	}
	if code, _ := violation(duplicateErr); code != uniqueViolation {
		t.Fatal("duplicate admission did not preserve original uniqueness guard")
	}
}

func TestMeetingRecoveryPurgeSettlement(t *testing.T) {
	t.Parallel()
	for _, returned := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncertain", true: "returned"}[returned], func(t *testing.T) {
			f := newMeetingFixture(t, testdb.New(t))
			identity := f.create(t, 10)
			attempt := domain.MeetingCreateAttemptID{20}
			if f.begin(t, identity, attempt, 1) != domain.MeetingMutationApplied {
				t.Fatal("begin refused")
			}
			if returned {
				_, err := f.repos.MeetingRecovery.RecordCreateReturned(t.Context(), identity.ProviderMeetingID, attempt, 100, instant)
				meetingRequire(t, err)
			}
			_, err := f.repos.Lessons.DeleteBatchOfTutor(t.Context(), f.tutor, 10)
			meetingRequire(t, err)
			before := f.recovery(t, identity.ProviderMeetingID)
			wrong, correct := domain.ProviderCreateTime(101), domain.ProviderCreateTime(100)

			refused, refusedErr := f.repos.MeetingRecovery.Settle(t.Context(), repositories.MeetingRecoverySettlement{ProviderMeetingID: identity.ProviderMeetingID, Revision: before.Revision, ObservedCreateTime: &wrong, Now: instant})
			outcome, settleErr := f.repos.MeetingRecovery.Settle(t.Context(), repositories.MeetingRecoverySettlement{ProviderMeetingID: identity.ProviderMeetingID, Revision: before.Revision, ObservedCreateTime: &correct, Now: instant})
			after := f.recovery(t, identity.ProviderMeetingID)
			count, countErr := f.repos.MeetingRecovery.Capacity(t.Context())

			meetingRequire(t, refusedErr, settleErr, countErr)
			if refused != domain.MeetingMutationRefused || after.MeetingID != nil || after.ProviderMeetingID != identity.ProviderMeetingID {
				t.Fatal("purge changed provider identity or accepted another generation")
			}
			if returned && (outcome != domain.MeetingMutationApplied || after.SettledAt == nil || count != 0) {
				t.Fatal("returned orphan did not settle its exact generation")
			}
			if !returned && (outcome != domain.MeetingMutationRefused || after.SettledAt != nil || count != 1) {
				t.Fatal("uncertain orphan was settled")
			}
		})
	}
}

func TestMeetingRecoveryCapacityMigration(t *testing.T) {
	t.Parallel()
	t.Run("overcapacity backfill refuses admission without erasure", func(t *testing.T) {
		f := newMeetingFixture(t, testdb.NewMigratedTo(t, 24))
		_, err := f.pool.Exec(t.Context(), `INSERT INTO meeting_recovery (provider_meeting_id, provider, origin, create_state, created_at, updated_at)
   SELECT md5('migration-' || item::text)::uuid, 'fake', 'legacy', 'uncertain', $1, $1 FROM generate_series(1, 4097) AS item`, instant)
		meetingRequire(t, err)

		upErr := f.database.MigrateUpTo(t.Context(), 25)
		count, countErr := f.repos.MeetingRecovery.Capacity(t.Context())
		admissionErr := f.repos.Meetings.CreateUnderLessonLock(t.Context(), meetingInput(10, f.lesson))
		downErr := f.database.MigrateDownTo(t.Context(), 24)
		after, afterErr := f.repos.MeetingRecovery.Capacity(t.Context())
		var preserved int64
		preserveErr := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM meeting_recovery WHERE settled_at IS NULL AND origin = 'legacy' AND create_state = 'uncertain'").Scan(&preserved)

		meetingRequire(t, upErr, countErr, afterErr, preserveErr)
		if count != 4097 || after != count || preserved != count || !errors.Is(admissionErr, repositories.ErrMeetingRecoveryCapacity) {
			t.Fatal("migration dropped obligations or admitted above capacity")
		}
		if code, _ := violation(downErr); code != checkViolation {
			t.Fatal("downgrade erased durable evidence")
		}
	})
	t.Run("empty schema round trip", func(t *testing.T) {
		db := testdb.NewMigratedTo(t, 24)

		upErr := db.MigrateUpTo(t.Context(), 25)
		downErr := db.MigrateDownTo(t.Context(), 24)
		secondErr := db.MigrateUpTo(t.Context(), 25)
		count, countErr := repositories.NewStore(db.Pool).Repositories().MeetingRecovery.Capacity(t.Context())

		meetingRequire(t, upErr, downErr, secondErr, countErr)
		if count != 0 {
			t.Fatal("empty schema roundtrip changed capacity")
		}
	})
}

func TestMeetingRecoveryOutboxConflict(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	recoveryClose(t, f, identity)
	before := f.recovery(t, identity.ProviderMeetingID)
	id := domain.OutboxID{30}
	_, err := f.repos.Outbox.Insert(t.Context(), repositories.NewOutboxRow{ID: id, Lane: domain.LaneProviderCall, Priority: domain.PriorityHigh, Kind: domain.KindMeetingCleanup, Payload: []byte("{}"), IdempotencyKey: "conflicting-work", NextAttemptAt: instant, CreatedAt: instant})
	meetingRequire(t, err)

	_, queueErr := f.repos.MeetingRecovery.QueueCycle(t.Context(), repositories.MeetingRecoveryQueue{ProviderMeetingID: identity.ProviderMeetingID, Revision: before.Revision, OutboxID: id, Mode: domain.MeetingRecoveryAutomatic, Now: instant})
	after := f.recovery(t, identity.ProviderMeetingID)
	_, receiptErr := f.repos.MeetingRecovery.Cycle(t.Context(), id)

	if code, _ := violation(queueErr); code != uniqueViolation {
		t.Fatal("outbox identity collision was not refused")
	}
	if after.Revision != before.Revision || after.CycleNumber != 0 || !errors.Is(receiptErr, repositories.ErrNotFound) {
		t.Fatal("outbox failure leaked a spent cycle or receipt")
	}
}

func awaitNoAcquiredConnections(ctx context.Context, pool *pgxpool.Pool) int32 {
	acquired := pool.Stat().AcquiredConns()
	for acquired != 0 && ctx.Err() == nil {
		runtime.Gosched()
		acquired = pool.Stat().AcquiredConns()
	}
	return acquired
}

func TestMeetingRecoveryCapacityCancellation(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	other := independentMeetingFixture(t, f)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	release, first := holdRecoveryTransaction(t, ctx, f, func(repos repositories.Repositories) error {
		return repos.Meetings.CreateUnderLessonLock(ctx, meetingInput(10, f.lesson))
	})
	waiting, cancelWait := context.WithCancel(ctx)
	defer cancelWait()

	second := recoveryRun(t, ctx, "cancelled-capacity-contender", func() error {
		return other.store.WithTx(waiting, func(repos repositories.Repositories) error {
			return repos.Meetings.CreateUnderLessonLock(waiting, meetingInput(11, f.otherLesson))
		})
	})
	waitMeetingLock(t, ctx, f)
	cancelWait()
	cancelledErr := meetingAwait(t, ctx, second)
	release()
	firstErr := meetingAwait(t, ctx, first)
	count, countErr := other.repos.MeetingRecovery.Capacity(ctx)
	acquired := awaitNoAcquiredConnections(ctx, other.pool)

	meetingRequire(t, firstErr, countErr)
	if !errors.Is(cancelledErr, context.Canceled) || count != 1 || acquired != 0 {
		t.Fatalf("cancelled capacity reservation leaked a slot or connection: %v, count %d, acquired %d", cancelledErr, count, acquired)
	}
}
