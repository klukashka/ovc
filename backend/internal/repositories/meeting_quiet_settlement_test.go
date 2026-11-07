package repositories_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/meetings"
	"ovc/backend/internal/testdb"
)

const quietWindow = domain.MeetingCreateQuietWindow

func TestMeetingQuietSettlement(t *testing.T) {
	t.Parallel()
	t.Run("uncertain attempt settles after the window", testQuietSettlesAfterWindow)
	t.Run("late room is ended then settled", testQuietLateRoomEnded)
	t.Run("unavailable provider keeps it until an operator cycle", testQuietUnavailableThenOperator)
	t.Run("operator cycle before the window stays unresolved", testQuietOperatorBeforeWindow)
	t.Run("crashed started attempt settles after the window", testQuietStartedSettles)
	t.Run("legacy uncertain evidence settles after the window", testQuietLegacySettles)
	t.Run("clock stepped back before claim spends the cycle once", testQuietClockSteppedBack)
	t.Run("dispatch instant in the future never settles", testQuietFutureDispatch)
	t.Run("late acknowledgement after settlement changes nothing", testQuietLateAcknowledgement)
	t.Run("repository refuses early evaluation", testQuietRepositoryFence)
	t.Run("constraint refuses forged early settlement", testQuietConstraint)
	t.Run("create returning after settlement is ended", testQuietCreateAfterSettlement)
	t.Run("expired permit dispatches no create", testQuietExpiredPermit)
}

func (f recoveryWorkerFixture) unsettledOfTutor(t *testing.T) int64 {
	t.Helper()
	count, err := f.repos.Meetings.UnsettledOfTutor(t.Context(), f.tutor)
	meetingRequire(t, err)
	return count
}

func (f recoveryWorkerFixture) capacityNow(t *testing.T) int64 {
	t.Helper()
	count, err := f.repos.MeetingRecovery.Capacity(t.Context())
	meetingRequire(t, err)
	return count
}

func (f recoveryWorkerFixture) scan(t *testing.T, scanner *meetings.RecoveryScanner, now time.Time) int64 {
	t.Helper()
	queued, err := scanner.Run(t.Context(), now)
	meetingRequire(t, err)
	return queued
}

func (f recoveryWorkerFixture) tick(t *testing.T, now time.Time) {
	t.Helper()
	meetingRequire(t, f.worker.Tick(t.Context(), now))
}

func (f recoveryWorkerFixture) operator(t *testing.T, now time.Time) *meetings.Operator {
	t.Helper()
	source := clock.NewShared(f.pool)
	_, err := source.Set(t.Context(), now)
	meetingRequire(t, err)
	return meetings.NewOperator(f.store, source, bytes.NewReader(bytes.Repeat([]byte{77}, 64)), f.logger)
}

func (f recoveryWorkerFixture) assertQuietSettled(t *testing.T) repositories.StoredMeetingRecovery {
	t.Helper()
	row := f.recovery(t, f.identity.ProviderMeetingID)
	if row.SettledAt == nil || row.Settlement == nil || *row.Settlement != domain.MeetingRecoverySettledQuietAbsence || row.CycleResult != domain.MeetingRecoveryCycleSettled {
		t.Fatalf("not settled by quiet absence: %+v", row)
	}
	if f.capacityNow(t) != 0 || f.unsettledOfTutor(t) != 0 {
		t.Fatalf("capacity %d and tutor count %d after quiet settlement", f.capacityNow(t), f.unsettledOfTutor(t))
	}
	logs := f.logs.String()
	if !strings.Contains(logs, `"msg":"meeting.recovery.settled"`) || !strings.Contains(logs, `"settlement":"quiet_absence"`) {
		t.Fatal("quiet settlement emitted no safe info event")
	}
	for _, private := range []string{"Private lesson sentinel", f.identity.ProviderMeetingID.String(), "create_time", "attempt_id"} {
		if strings.Contains(logs, private) {
			t.Fatalf("unsafe diagnostic field %s", private)
		}
	}
	return row
}

func testQuietSettlesAfterWindow(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateUncertain, false)
	scanner := recoveryScanner(f.meetingFixture, f.logger)
	f.tick(t, instant)
	first := f.recovery(t, f.identity.ProviderMeetingID)
	capacityBefore, tutorBefore := f.capacityNow(t), f.unsettledOfTutor(t)

	early := f.scan(t, scanner, instant.Add(quietWindow-time.Second))
	due := f.scan(t, scanner, instant.Add(quietWindow))
	f.tick(t, instant.Add(quietWindow))
	again := f.scan(t, scanner, instant.Add(quietWindow+time.Minute))

	if first.CycleResult != domain.MeetingRecoveryCycleUnresolved || first.SettledAt != nil || first.DispatchedAt == nil || !first.DispatchedAt.Equal(instant) {
		t.Fatalf("first cycle before the window: %+v", first)
	}
	if capacityBefore != 1 || tutorBefore != 1 || early != 0 || due != 1 || again != 0 {
		t.Fatalf("capacity=%d tutor=%d early=%d due=%d again=%d", capacityBefore, tutorBefore, early, due, again)
	}
	settled := f.assertQuietSettled(t)
	if settled.CycleNumber != 2 || !settled.ReconcileCycleUsed {
		t.Fatalf("settlement did not use exactly one reconciliation cycle: %+v", settled)
	}
}

func testQuietLateRoomEnded(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateUncertain, false)
	scanner := recoveryScanner(f.meetingFixture, f.logger)
	f.tick(t, instant)
	_, err := f.provider.CreateMeeting(t.Context(), domain.Meeting{ID: f.identity.ProviderMeetingID, Name: "Private lesson sentinel"})
	meetingRequire(t, err)

	due := f.scan(t, scanner, instant.Add(quietWindow))
	f.tick(t, instant.Add(quietWindow))
	info, infoErr := f.provider.InspectMeeting(t.Context(), f.identity.ProviderMeetingID)

	meetingRequire(t, infoErr)
	if due != 1 || info.Status != domain.MeetingNotFound {
		t.Fatalf("late room was not ended: due=%d status=%s", due, info.Status)
	}
	f.assertQuietSettled(t)
}

func testQuietUnavailableThenOperator(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateUncertain, false)
	scanner := recoveryScanner(f.meetingFixture, f.logger)
	f.tick(t, instant)
	meetingRequire(t, f.provider.SimulateUnavailable(t.Context(), true))
	due := f.scan(t, scanner, instant.Add(quietWindow))
	for attempt := range domain.OutboxMaxAttempts + 2 {
		f.tick(t, instant.Add(quietWindow+time.Duration(attempt)*2*time.Hour))
	}
	exhausted := f.recovery(t, f.identity.ProviderMeetingID)
	status, statusErr := f.operator(t, instant.Add(30*time.Hour)).Status(t.Context(), domain.ProviderMeetingID{})
	meetingRequire(t, statusErr, f.provider.SimulateUnavailable(t.Context(), false))

	operatorNow := instant.Add(30 * time.Hour)
	receipt, retryErr := f.operator(t, operatorNow).Retry(t.Context(), f.identity.ProviderMeetingID, exhausted.Revision)
	f.tick(t, operatorNow)

	meetingRequire(t, retryErr)
	if due != 1 || exhausted.SettledAt != nil || exhausted.CycleResult != domain.MeetingRecoveryCycleExhausted || f.capacityNow(t) != 0 {
		t.Fatalf("unavailable provider: due=%d %+v", due, exhausted)
	}
	if len(status.Rows) != 1 || !status.Rows[0].OperatorAction || status.Rows[0].Settled {
		t.Fatalf("status did not ask for operator action: %+v", status.Rows)
	}
	if receipt.Mode != domain.MeetingRecoveryOperator || receipt.CycleNumber != 3 {
		t.Fatalf("operator receipt: %+v", receipt)
	}
	f.assertQuietSettled(t)
}

func testQuietOperatorBeforeWindow(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateUncertain, false)
	f.tick(t, instant)
	before := f.recovery(t, f.identity.ProviderMeetingID)
	operatorNow := instant.Add(quietWindow - time.Minute)

	_, retryErr := f.operator(t, operatorNow).Retry(t.Context(), f.identity.ProviderMeetingID, before.Revision)
	f.tick(t, operatorNow)
	after := f.recovery(t, f.identity.ProviderMeetingID)

	meetingRequire(t, retryErr)
	if after.SettledAt != nil || after.CycleResult != domain.MeetingRecoveryCycleUnresolved || after.CycleNumber != 2 || after.ReconcileCycleUsed || f.capacityNow(t) != 1 {
		t.Fatalf("operator cycle before the window: %+v", after)
	}
}

func testQuietStartedSettles(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateStarted, false)
	scanner := recoveryScanner(f.meetingFixture, f.logger)
	f.tick(t, instant)

	due := f.scan(t, scanner, instant.Add(quietWindow))
	f.tick(t, instant.Add(quietWindow))

	if due != 1 {
		t.Fatalf("started attempt was not reconciled: due=%d", due)
	}
	settled := f.assertQuietSettled(t)
	if settled.CreateState != domain.MeetingCreateStarted {
		t.Fatalf("settlement changed the create evidence: %+v", settled)
	}
}

func testQuietLegacySettles(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateUncertain, false)
	_, err := f.pool.Exec(t.Context(), "UPDATE meeting_recovery SET origin = 'legacy', attempt_id = NULL WHERE provider_meeting_id = $1", f.identity.ProviderMeetingID)
	meetingRequire(t, err)
	scanner := recoveryScanner(f.meetingFixture, f.logger)
	f.tick(t, instant)

	due := f.scan(t, scanner, instant.Add(quietWindow))
	f.tick(t, instant.Add(quietWindow))

	if due != 1 {
		t.Fatalf("legacy evidence was not reconciled: due=%d", due)
	}
	f.assertQuietSettled(t)
}

func testQuietClockSteppedBack(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateUncertain, false)
	scanner := recoveryScanner(f.meetingFixture, f.logger)
	f.tick(t, instant)

	due := f.scan(t, scanner, instant.Add(quietWindow))
	_, err := f.pool.Exec(t.Context(), "UPDATE outbox SET next_attempt_at = $1 WHERE kind = 'meeting_cleanup' AND status = 'pending'", instant.Add(quietWindow-time.Second))
	meetingRequire(t, err)

	f.tick(t, instant.Add(quietWindow-time.Second))
	spent := f.recovery(t, f.identity.ProviderMeetingID)
	again := f.scan(t, scanner, instant.Add(quietWindow+time.Minute))
	refused, refusedErr := f.repos.MeetingRecovery.QueueCycle(t.Context(), repositories.MeetingRecoveryQueue{ProviderMeetingID: f.identity.ProviderMeetingID, Revision: spent.Revision, OutboxID: domain.OutboxID{90}, Mode: domain.MeetingRecoveryAutomatic, Now: instant.Add(2 * quietWindow)})
	status, statusErr := f.operator(t, instant.Add(2*quietWindow)).Status(t.Context(), domain.ProviderMeetingID{})

	meetingRequire(t, refusedErr, statusErr)
	if due != 1 || spent.SettledAt != nil || spent.CycleResult != domain.MeetingRecoveryCycleUnresolved || !spent.ReconcileCycleUsed || f.capacityNow(t) != 1 {
		t.Fatalf("evaluation before the window settled or kept the cycle: due=%d %+v", due, spent)
	}
	if again != 0 || refused != domain.MeetingMutationRefused || len(status.Rows) != 1 || !status.Rows[0].OperatorAction {
		t.Fatalf("spent reconciliation was renewed: again=%d refused=%s status=%+v", again, refused, status.Rows)
	}
}

func testQuietFutureDispatch(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateUncertain, false)
	_, err := f.pool.Exec(t.Context(), "UPDATE meeting_recovery SET dispatched_at = $2, updated_at = $2 WHERE provider_meeting_id = $1", f.identity.ProviderMeetingID, instant.Add(3*quietWindow))
	meetingRequire(t, err)
	scanner := recoveryScanner(f.meetingFixture, f.logger)
	f.tick(t, instant.Add(2*quietWindow))

	due := f.scan(t, scanner, instant.Add(quietWindow))
	row := f.recovery(t, f.identity.ProviderMeetingID)
	_, retryErr := f.operator(t, instant.Add(quietWindow)).Retry(t.Context(), f.identity.ProviderMeetingID, row.Revision)
	f.tick(t, instant.Add(2*quietWindow))
	after := f.recovery(t, f.identity.ProviderMeetingID)

	meetingRequire(t, retryErr)
	if due != 0 || after.SettledAt != nil || after.CycleResult != domain.MeetingRecoveryCycleUnresolved || f.capacityNow(t) != 1 {
		t.Fatalf("a dispatch instant after the evaluation settled: due=%d %+v", due, after)
	}
}

func testQuietLateAcknowledgement(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateUncertain, false)
	scanner := recoveryScanner(f.meetingFixture, f.logger)
	f.tick(t, instant)
	f.scan(t, scanner, instant.Add(quietWindow))
	f.tick(t, instant.Add(quietWindow))
	settled := f.recovery(t, f.identity.ProviderMeetingID)

	late, lateErr := f.repos.MeetingRecovery.RecordCreateReturned(t.Context(), f.identity.ProviderMeetingID, domain.MeetingCreateAttemptID(f.identity.ProviderMeetingID), 500, instant.Add(2*quietWindow))
	after := f.recovery(t, f.identity.ProviderMeetingID)
	queued := f.scan(t, scanner, instant.Add(2*quietWindow))

	meetingRequire(t, lateErr)
	if late != domain.MeetingMutationRefused || after.Revision != settled.Revision || after.CreateState != domain.MeetingCreateUncertain || after.CreateTime != nil || *after.Settlement != domain.MeetingRecoverySettledQuietAbsence || queued != 0 || f.capacityNow(t) != 0 {
		t.Fatalf("late acknowledgement changed settled evidence: %s %+v", late, after)
	}
}

func testQuietRepositoryFence(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateUncertain, false)
	f.tick(t, instant)
	row := f.recovery(t, f.identity.ProviderMeetingID)
	early, late := instant.Add(quietWindow-time.Second), instant.Add(quietWindow)
	completion := instant.Add(2 * quietWindow)

	missing, missingErr := f.repos.MeetingRecovery.Settle(t.Context(), repositories.MeetingRecoverySettlement{ProviderMeetingID: row.ProviderMeetingID, Revision: row.Revision, Now: completion})
	beforeWindow, beforeErr := f.repos.MeetingRecovery.Settle(t.Context(), repositories.MeetingRecoverySettlement{ProviderMeetingID: row.ProviderMeetingID, Revision: row.Revision, EvaluatedAt: &early, Now: completion})
	future, futureErr := f.repos.MeetingRecovery.Settle(t.Context(), repositories.MeetingRecoverySettlement{ProviderMeetingID: row.ProviderMeetingID, Revision: row.Revision, EvaluatedAt: &completion, Now: late})
	generation := domain.ProviderCreateTime(100)
	observed, observedErr := f.repos.MeetingRecovery.Settle(t.Context(), repositories.MeetingRecoverySettlement{ProviderMeetingID: row.ProviderMeetingID, Revision: row.Revision, ObservedCreateTime: &generation, EvaluatedAt: &late, Now: completion})
	applied, appliedErr := f.repos.MeetingRecovery.Settle(t.Context(), repositories.MeetingRecoverySettlement{ProviderMeetingID: row.ProviderMeetingID, Revision: row.Revision, EvaluatedAt: &late, Now: completion})

	meetingRequire(t, missingErr, beforeErr, futureErr, observedErr, appliedErr)
	for _, refused := range []domain.MeetingMutation{missing, beforeWindow, future, observed} {
		if refused != domain.MeetingMutationRefused {
			t.Fatal("settlement accepted evaluation outside the quiet window")
		}
	}
	if applied != domain.MeetingMutationApplied || f.capacityNow(t) != 0 {
		t.Fatalf("settlement at the window: %s capacity=%d", applied, f.capacityNow(t))
	}
}

func testQuietConstraint(t *testing.T) {
	t.Parallel()
	f := newRecoveryWorker(t, domain.MeetingCreateUncertain, false)

	_, early := f.pool.Exec(t.Context(), "UPDATE meeting_recovery SET settled_at = $2, settlement = 'quiet_absence', cleanup_requested = true WHERE provider_meeting_id = $1", f.identity.ProviderMeetingID, instant.Add(quietWindow-time.Second))
	_, mismatched := f.pool.Exec(t.Context(), "UPDATE meeting_recovery SET settled_at = $2, settlement = 'confirmed_end', cleanup_requested = true WHERE provider_meeting_id = $1", f.identity.ProviderMeetingID, instant.Add(2*quietWindow))
	_, unmarked := f.pool.Exec(t.Context(), "UPDATE meeting_recovery SET settled_at = $2, cleanup_requested = true WHERE provider_meeting_id = $1", f.identity.ProviderMeetingID, instant.Add(2*quietWindow))
	_, undispatched := f.pool.Exec(t.Context(), "UPDATE meeting_recovery SET dispatched_at = NULL WHERE provider_meeting_id = $1", f.identity.ProviderMeetingID)

	for _, err := range []error{early, mismatched, unmarked, undispatched} {
		if code, _ := violation(err); code != checkViolation {
			t.Fatalf("constraint accepted forged settlement evidence: %v", err)
		}
	}
}

type offsetClock struct {
	clock.Source
	offset time.Duration
}

func (c offsetClock) Instant(ctx context.Context) (time.Time, error) {
	now, err := c.Source.Instant(ctx)
	return now.Add(c.offset), err
}

func testQuietCreateAfterSettlement(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	result, identity := f.inFlight(t, t.Context())
	meetingRequire(t, f.rooms.Retire(t.Context(), identity, domain.MeetingCloseTutorEnd))
	meetingRequire(t, f.worker.Tick(t.Context(), instant))
	_, err := f.scanner.Run(t.Context(), instant.Add(quietWindow))
	meetingRequire(t, err)
	meetingRequire(t, f.worker.Tick(t.Context(), instant.Add(quietWindow)))
	settled := f.recovery(t, identity.ProviderMeetingID)
	capacity := f.capacity(t)

	close(f.gate.release)
	replacementErr := meetingAwait(t, t.Context(), result)
	after := f.recovery(t, identity.ProviderMeetingID)
	info, infoErr := f.observer.InspectMeeting(t.Context(), identity.ProviderMeetingID)
	current, currentErr := f.repos.Meetings.Current(t.Context(), f.lesson)

	meetingRequire(t, replacementErr, infoErr, currentErr)
	if settled.SettledAt == nil || settled.Settlement == nil || *settled.Settlement != domain.MeetingRecoverySettledQuietAbsence || capacity != 0 {
		t.Fatalf("absence after the window did not settle: %+v capacity=%d", settled, capacity)
	}
	if after.Revision != settled.Revision || after.CreateTime != nil || f.read(t, identity).State != domain.MeetingClosed {
		t.Fatalf("late create changed settled evidence or reopened its room: %+v", after)
	}
	if current.ProviderMeetingID == identity.ProviderMeetingID || f.capacity(t) != 1 {
		t.Fatalf("the late create was published instead of a replacement: capacity=%d", f.capacity(t))
	}
	if info.Status != domain.MeetingNotFound || !strings.Contains(f.logs.String(), `"msg":"meeting.room.created_after_settlement","ended":true`) {
		t.Fatalf("the room created after settlement was not ended: %s", info.Status)
	}
	f.assertSafeDiagnostics(t, identity)
}

func testQuietExpiredPermit(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	timing := meetings.DefaultRoomTiming()
	rooms := meetings.NewRooms(meetings.RoomDependencies{
		Store: f.store, Provider: f.provider, Kind: domain.MeetingProviderFake, Clock: offsetClock{Source: f.clock, offset: timing.ProviderCall + time.Second},
		Random: bytes.NewReader(bytes.Repeat([]byte{91}, 256)), Logger: f.logger, Monotonic: clock.System{}.Now, Timing: timing,
	})

	_, err := rooms.Ensure(t.Context(), f.guard, publishNothing)
	rows, rowsErr := f.repos.MeetingRecovery.List(t.Context(), repositories.MeetingRecoveryPage{Limit: 10})

	meetingRequire(t, rowsErr)
	if !errors.Is(err, meetings.ErrProviderUnavailable) || len(rows) != 1 || rows[0].CreateState != domain.MeetingCreateUncertain || f.count(t, "SELECT count(id) FROM fake_meeting_rooms") != 0 || !strings.Contains(f.logs.String(), `"msg":"meeting.room.dispatch_expired"`) {
		t.Fatalf("an expired permit dispatched or lost its evidence: %v %+v", err, rows)
	}
}

func TestMeetingQuietSettlementMigration(t *testing.T) {
	t.Parallel()
	t.Run("backfill keeps evidence and downgrade refuses obligations", testQuietMigrationBackfill)
	t.Run("empty ledger supports down and up", testQuietMigrationEmpty)
}

func testQuietMigrationBackfill(t *testing.T) {
	f := newMeetingFixture(t, testdb.NewMigratedTo(t, 30))
	updated := instant.Add(time.Minute)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO meeting_recovery (provider_meeting_id, provider, origin, create_state, attempt_id, create_time, cleanup_requested, settled_at, created_at, updated_at) VALUES
 ('00000000-0000-4000-8000-000000000001', 'fake', 'attempt', 'prepared', NULL, NULL, false, NULL, $1, $2),
 ('00000000-0000-4000-8000-000000000002', 'fake', 'attempt', 'uncertain', '00000000-0000-4000-8000-000000000012', NULL, true, NULL, $1, $2),
 ('00000000-0000-4000-8000-000000000003', 'fake', 'legacy', 'uncertain', NULL, NULL, true, NULL, $1, $2),
 ('00000000-0000-4000-8000-000000000004', 'fake', 'attempt', 'returned', '00000000-0000-4000-8000-000000000014', 100, true, $2, $1, $2),
 ('00000000-0000-4000-8000-000000000005', 'fake', 'attempt', 'prepared', NULL, NULL, true, $2, $1, $2)`, instant, updated)
	meetingRequire(t, err)

	upErr := f.database.MigrateUpTo(t.Context(), 31)
	rows, listErr := f.repos.MeetingRecovery.List(t.Context(), repositories.MeetingRecoveryPage{Limit: 10})
	downErr := f.database.MigrateDownTo(t.Context(), 30)

	meetingRequire(t, upErr, listErr)
	want := []struct {
		dispatched bool
		settlement *domain.MeetingRecoverySettlement
	}{{false, nil}, {true, nil}, {true, nil}, {true, settlementOf(domain.MeetingRecoverySettledConfirmedEnd)}, {false, settlementOf(domain.MeetingRecoverySettledRevoked)}}
	if len(rows) != len(want) {
		t.Fatalf("migration changed the ledger size to %d", len(rows))
	}
	for index, row := range rows {
		if (row.DispatchedAt != nil) != want[index].dispatched || (row.DispatchedAt != nil && !row.DispatchedAt.Equal(updated)) || row.ReconcileCycleUsed {
			t.Fatalf("row %d dispatch backfill: %+v", index, row)
		}
		if (row.Settlement == nil) != (want[index].settlement == nil) || (row.Settlement != nil && *row.Settlement != *want[index].settlement) {
			t.Fatalf("row %d settlement backfill: %+v", index, row)
		}
	}
	if code, _ := violation(downErr); code != checkViolation {
		t.Fatalf("nonempty downgrade did not refuse: %v", downErr)
	}
}

func settlementOf(value domain.MeetingRecoverySettlement) *domain.MeetingRecoverySettlement {
	return &value
}

func testQuietMigrationEmpty(t *testing.T) {
	db := testdb.NewMigratedTo(t, 30)

	upErr := db.MigrateUpTo(t.Context(), 31)
	downErr := db.MigrateDownTo(t.Context(), 30)
	secondErr := db.MigrateUpTo(t.Context(), 31)

	meetingRequire(t, upErr, downErr, secondErr)
}
