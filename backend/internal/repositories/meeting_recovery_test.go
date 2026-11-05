package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
)

func (f meetingFixture) recoveryUnderLock(ctx context.Context, work func(repositories.MeetingRecovery) error) error {
	return f.store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := repos.Lessons.LockForEdit(ctx, f.tutor, f.lesson, nil, domain.LocalDate{})
		if err != nil {
			return err
		}
		if len(locked) != 1 {
			return errors.New("the recovery fixture lesson was not locked")
		}
		return work(repos.MeetingRecovery)
	})
}

func (f meetingFixture) begin(t *testing.T, identity domain.MeetingIdentity, attempt domain.MeetingCreateAttemptID, revision domain.MeetingRecoveryRevision) domain.MeetingMutation {
	t.Helper()
	var outcome domain.MeetingMutation
	meetingRequire(t, f.recoveryUnderLock(t.Context(), func(repo repositories.MeetingRecovery) error {
		var err error
		outcome, err = repo.BeginCreateUnderLessonLock(t.Context(), identity, attempt, revision, instant)
		return err
	}))
	return outcome
}

func (f meetingFixture) acknowledge(t *testing.T, identity domain.MeetingIdentity, created domain.ProviderCreateTime) {
	t.Helper()
	attempt := domain.MeetingCreateAttemptID(identity.ProviderMeetingID)
	if f.begin(t, identity, attempt, 1) != domain.MeetingMutationApplied {
		t.Fatal("fixture create attempt did not start")
	}
	outcome, err := f.repos.MeetingRecovery.RecordCreateReturned(t.Context(), identity.ProviderMeetingID, attempt, created, instant)
	meetingRequire(t, err)
	if outcome != domain.MeetingMutationApplied {
		t.Fatal("fixture create acknowledgement did not persist")
	}
}

func (f meetingFixture) recovery(t *testing.T, id domain.ProviderMeetingID) repositories.StoredMeetingRecovery {
	t.Helper()
	row, err := f.repos.MeetingRecovery.ByProviderID(t.Context(), id)
	meetingRequire(t, err)
	return row
}

func insertLegacyMeeting(ctx context.Context, f meetingFixture, input repositories.NewMeeting) error {
	_, err := f.pool.Exec(ctx, `INSERT INTO meetings (id, lesson_id, provider, provider_meeting_id, routing_key, state, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, 'creating', $6, $6)`, input.ID, input.LessonID, input.Provider, input.ProviderMeetingID, input.RoutingKey, input.Now)
	return err
}

func TestMeetingRecoveryAtomicCreation(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	first := meetingInput(10, f.lesson)
	rolledBack := meetingInput(11, f.otherLesson)
	aborted := errors.New("abort meeting creation")

	commitErr := f.underLock(t.Context(), func(repo repositories.Meetings) error { return repo.CreateUnderLessonLock(t.Context(), first) })
	rollbackErr := f.store.WithTx(t.Context(), func(repos repositories.Repositories) error {
		if err := repos.Meetings.CreateUnderLessonLock(t.Context(), rolledBack); err != nil {
			return err
		}
		return aborted
	})
	duplicateErr := f.repos.Meetings.CreateUnderLessonLock(t.Context(), meetingInput(12, f.lesson))
	retained := f.recovery(t, first.ProviderMeetingID)
	_, missingBusiness := f.repos.Meetings.ByID(t.Context(), f.otherLesson, rolledBack.ID)
	_, missingRecovery := f.repos.MeetingRecovery.ByProviderID(t.Context(), rolledBack.ProviderMeetingID)
	_, duplicateRecovery := f.repos.MeetingRecovery.ByProviderID(t.Context(), domain.ProviderMeetingID{12})
	var counts [2]int
	countErr := f.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM meetings), (SELECT count(*) FROM meeting_recovery)").Scan(&counts[0], &counts[1])

	meetingRequire(t, commitErr, countErr)
	if !errors.Is(rollbackErr, aborted) || !errors.Is(missingBusiness, repositories.ErrNotFound) || !errors.Is(missingRecovery, repositories.ErrNotFound) || !errors.Is(duplicateRecovery, repositories.ErrNotFound) {
		t.Fatal("atomic creation leaked a rolled back or refused row")
	}
	if code, _ := violation(duplicateErr); code != uniqueViolation || counts != [2]int{1, 1} {
		t.Fatalf("duplicate creation: %v, counts: %v", duplicateErr, counts)
	}
	if retained.MeetingID == nil || *retained.MeetingID != first.ID || retained.Provider != first.Provider || retained.Origin != domain.MeetingRecoveryAttempt || retained.CreateState != domain.MeetingCreatePrepared || retained.Revision != 1 || retained.AttemptID != nil || retained.CreateTime != nil || retained.CleanupRequested {
		t.Fatal("new meeting has invalid recovery identity or evidence")
	}
}

func TestMeetingRecoveryTransitions(t *testing.T) {
	t.Parallel()
	t.Run("conditional single attempt and exact idempotent evidence", testRecoveryTransitions)
	t.Run("provenance state and finite timestamp constraints", testRecoveryConstraints)
	t.Run("independent stores dispatch at most one attempt", testRecoveryConcurrentBegin)
	t.Run("publication requires returned generation without cleanup", testRecoveryPublication)
	t.Run("cancelled blocked transition leaves authoritative state", testRecoveryCancellation)
}

func testRecoveryTransitions(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	attempt := domain.MeetingCreateAttemptID{20}
	other := domain.MeetingCreateAttemptID{21}
	id := identity.ProviderMeetingID

	zero := f.begin(t, identity, domain.MeetingCreateAttemptID{}, 1)
	stale := f.begin(t, identity, attempt, 2)
	began := f.begin(t, identity, attempt, 1)
	second := f.begin(t, identity, other, 2)
	wrong, wrongErr := f.repos.MeetingRecovery.MarkCreateUncertain(t.Context(), id, other, instant)
	uncertain, uncertainErr := f.repos.MeetingRecovery.MarkCreateUncertain(t.Context(), id, attempt, instant)
	reconstructed := repositories.NewStore(f.database.Pool).Repositories().MeetingRecovery
	before, beforeErr := reconstructed.ByProviderID(t.Context(), id)
	retry := f.begin(t, identity, other, before.Revision)
	zeroTime, zeroErr := reconstructed.RecordCreateReturned(t.Context(), id, attempt, 0, instant)
	wrongReturned, wrongReturnedErr := reconstructed.RecordCreateReturned(t.Context(), id, other, 100, instant)
	returned, returnedErr := reconstructed.RecordCreateReturned(t.Context(), id, attempt, 100, instant)
	acknowledged := f.recovery(t, id)
	repeated, repeatedErr := reconstructed.RecordCreateReturned(t.Context(), id, attempt, 100, instant.Add(time.Second))
	changed, changedErr := reconstructed.RecordCreateReturned(t.Context(), id, attempt, 101, instant)
	lateUncertain, lateErr := reconstructed.MarkCreateUncertain(t.Context(), id, attempt, instant)
	after := f.recovery(t, id)

	meetingRequire(t, wrongErr, uncertainErr, beforeErr, zeroErr, wrongReturnedErr, returnedErr, repeatedErr, changedErr, lateErr)
	for _, outcome := range []domain.MeetingMutation{zero, stale, second, wrong, retry, zeroTime, wrongReturned, changed, lateUncertain} {
		if outcome != domain.MeetingMutationRefused {
			t.Fatalf("invalid recovery transition applied: %s", outcome)
		}
	}
	if began != domain.MeetingMutationApplied || uncertain != domain.MeetingMutationApplied || returned != domain.MeetingMutationApplied || repeated != domain.MeetingMutationApplied {
		t.Fatal("valid evidence transition refused")
	}
	if before.CreateState != domain.MeetingCreateUncertain || before.Revision != 3 || before.CreateTime != nil || after.CreateState != domain.MeetingCreateReturned || after.CreateTime == nil || *after.CreateTime != 100 || after.Revision != 4 || after.Revision != acknowledged.Revision || !after.UpdatedAt.Equal(acknowledged.UpdatedAt) || after.Origin != domain.MeetingRecoveryAttempt || after.ProviderMeetingID != id || after.Provider != domain.MeetingProviderFake || after.AttemptID == nil || *after.AttemptID != attempt {
		t.Fatal("reconstruction, provenance or immutable acknowledgement failed")
	}
}

func testRecoveryConstraints(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	cases := []struct{ name, assignment string }{
		{"zero provider", "provider_meeting_id = '00000000-0000-0000-0000-000000000000'"},
		{"unknown provider", "provider = 'unknown'"},
		{"unknown origin", "origin = 'unknown'"},
		{"unknown state", "create_state = 'unknown'"},
		{"started without attempt", "create_state = 'started'"},
		{"uncertain without attempt", "create_state = 'uncertain'"},
		{"prepared with attempt", "attempt_id = '00000000-0000-0000-0000-000000000001'"},
		{"zero attempt", "create_state = 'started', attempt_id = '00000000-0000-0000-0000-000000000000'"},
		{"prepared with generation", "create_time = 1"},
		{"started with generation", "create_state = 'started', attempt_id = '00000000-0000-0000-0000-000000000001', create_time = 1"},
		{"returned without generation", "create_state = 'returned', attempt_id = '00000000-0000-0000-0000-000000000001'"},
		{"returned without attempt", "create_state = 'returned', create_time = 1"},
		{"zero generation", "create_state = 'returned', attempt_id = '00000000-0000-0000-0000-000000000001', create_time = 0"},
		{"negative generation", "create_state = 'returned', attempt_id = '00000000-0000-0000-0000-000000000001', create_time = -1"},
		{"legacy prepared", "origin = 'legacy'"},
		{"legacy started", "origin = 'legacy', create_state = 'started'"},
		{"legacy uncertain attempt", "origin = 'legacy', create_state = 'uncertain', attempt_id = '00000000-0000-0000-0000-000000000001'"},
		{"legacy returned attempt", "origin = 'legacy', create_state = 'returned', create_time = 1, attempt_id = '00000000-0000-0000-0000-000000000001'"},
		{"legacy uncertain generation", "origin = 'legacy', create_state = 'uncertain', create_time = 1"},
		{"legacy returned no generation", "origin = 'legacy', create_state = 'returned'"},
		{"zero revision", "revision = 0"},
		{"negative revision", "revision = -1"},
		{"infinite creation", "created_at = '-infinity'"},
		{"infinite update", "updated_at = 'infinity'"},
		{"backwards update", "updated_at = created_at - interval '1 second'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statement := "UPDATE meeting_recovery SET " + tc.assignment + " WHERE provider_meeting_id = $1"

			_, err := f.pool.Exec(t.Context(), statement, identity.ProviderMeetingID)

			if code, _ := violation(err); code != checkViolation {
				t.Fatalf("constraint accepted invalid evidence: %v", err)
			}
		})
	}
}

func independentMeetingFixture(t *testing.T, f meetingFixture) meetingFixture {
	t.Helper()
	pool, err := pgxpool.NewWithConfig(t.Context(), f.pool.Config().Copy())
	meetingRequire(t, err)
	t.Cleanup(pool.Close)
	f.pool = pool
	f.store = repositories.NewStore(pool)
	f.repos = f.store.Repositories()
	return f
}

func holdRecoveryTransaction(t *testing.T, ctx context.Context, f meetingFixture, work func(repositories.Repositories) error) (func(), <-chan error) {
	t.Helper()
	entered := make(chan error, 1)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	finished := recoveryRun(t, ctx, "recovery-transaction-holder", func() error {
		return f.store.WithTx(ctx, func(repos repositories.Repositories) error {
			err := work(repos)
			entered <- err
			if err != nil {
				return err
			}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	})
	t.Cleanup(unblock)
	meetingRequire(t, meetingAwait(t, ctx, entered))
	return unblock, finished
}

func testRecoveryConcurrentBegin(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	other := independentMeetingFixture(t, f)
	identity := f.create(t, 10)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var firstOutcome, secondOutcome domain.MeetingMutation
	release, first := holdRecoveryTransaction(t, ctx, f, func(repos repositories.Repositories) error {
		if _, err := repos.Lessons.LockForEdit(ctx, f.tutor, f.lesson, nil, domain.LocalDate{}); err != nil {
			return err
		}
		var err error
		firstOutcome, err = repos.MeetingRecovery.BeginCreateUnderLessonLock(ctx, identity, domain.MeetingCreateAttemptID{20}, 1, instant)
		return err
	})

	second := recoveryRun(t, ctx, "recovery-begin-contender", func() error {
		return other.recoveryUnderLock(ctx, func(repo repositories.MeetingRecovery) error {
			var err error
			secondOutcome, err = repo.BeginCreateUnderLessonLock(ctx, identity, domain.MeetingCreateAttemptID{21}, 1, instant)
			return err
		})
	})
	waitMeetingLock(t, ctx, f)
	release()
	firstErr, secondErr := meetingAwait(t, ctx, first), meetingAwait(t, ctx, second)
	row := f.recovery(t, identity.ProviderMeetingID)

	meetingRequire(t, firstErr, secondErr)
	if firstOutcome != domain.MeetingMutationApplied || secondOutcome != domain.MeetingMutationRefused || row.Revision != 2 || row.AttemptID == nil || *row.AttemptID != (domain.MeetingCreateAttemptID{20}) {
		t.Fatal("concurrent stores dispatched more than one create attempt")
	}
}

func testRecoveryPublication(t *testing.T) {
	for _, state := range []string{"prepared", "started", "uncertain", "wrong_generation", "cleanup"} {
		t.Run(state, func(t *testing.T) {
			f := newMeetingFixture(t, testdb.New(t))
			identity := f.create(t, 10)
			attempt := domain.MeetingCreateAttemptID{20}
			if state != "prepared" {
				if f.begin(t, identity, attempt, 1) != domain.MeetingMutationApplied {
					t.Fatal("fixture begin refused")
				}
			}
			if state == "uncertain" {
				_, err := f.repos.MeetingRecovery.MarkCreateUncertain(t.Context(), identity.ProviderMeetingID, attempt, instant)
				meetingRequire(t, err)
			}
			if state == "wrong_generation" || state == "cleanup" {
				_, err := f.repos.MeetingRecovery.RecordCreateReturned(t.Context(), identity.ProviderMeetingID, attempt, 101, instant)
				meetingRequire(t, err)
			}
			created := domain.ProviderCreateTime(100)
			if state == "cleanup" {
				created = 101
				_, err := f.pool.Exec(t.Context(), "UPDATE meeting_recovery SET cleanup_requested = true WHERE provider_meeting_id = $1", identity.ProviderMeetingID)
				meetingRequire(t, err)
			}

			outcome := f.mutate(t, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
				return repo.OpenUnderLessonLock(t.Context(), identity, created, instant)
			})
			row := f.read(t, identity)

			if outcome != domain.MeetingMutationRefused || row.State != domain.MeetingCreating || row.CreateTime != nil {
				t.Fatal("unproven or cleanup generation published")
			}
		})
	}
}

func testRecoveryCancellation(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	other := independentMeetingFixture(t, f)
	identity := f.create(t, 10)
	attempt := domain.MeetingCreateAttemptID{20}
	if f.begin(t, identity, attempt, 1) != domain.MeetingMutationApplied {
		t.Fatal("fixture begin refused")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	release, first := holdRecoveryTransaction(t, ctx, f, func(repos repositories.Repositories) error {
		_, err := repos.MeetingRecovery.MarkCreateUncertain(ctx, identity.ProviderMeetingID, attempt, instant)
		return err
	})
	waiting, cancelWait := context.WithCancel(ctx)
	defer cancelWait()
	var outcome domain.MeetingMutation

	second := recoveryRun(t, ctx, "recovery-cancelled-completion", func() error {
		var err error
		outcome, err = other.repos.MeetingRecovery.RecordCreateReturned(waiting, identity.ProviderMeetingID, attempt, 100, instant)
		return err
	})
	waitMeetingLock(t, ctx, f)
	cancelWait()
	waitErr := meetingAwait(t, ctx, second)
	for other.pool.Stat().AcquiredConns() != 0 {
		if ctx.Err() != nil {
			t.Fatal("cancelled recovery transition did not release its connection")
		}
		runtime.Gosched()
	}
	release()
	firstErr := meetingAwait(t, ctx, first)
	row := f.recovery(t, identity.ProviderMeetingID)
	var observed int
	queryErr := other.pool.QueryRow(ctx, "SELECT 1").Scan(&observed)

	meetingRequire(t, firstErr, queryErr)
	if !errors.Is(waitErr, context.Canceled) || outcome != domain.MeetingMutationRefused || row.CreateState != domain.MeetingCreateUncertain || row.CreateTime != nil || row.Revision != 3 || observed != 1 {
		t.Fatal("cancelled SQL transition claimed completion or retained a blocked resource")
	}
}

func TestMeetingRecoveryLateCompletion(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"close", "purge"} {
		for _, completionFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/completion-first-%t", operation, completionFirst), func(t *testing.T) {
				testRecoveryOrderedCompletion(t, operation, completionFirst)
			})
		}
	}
}

func testRecoveryOrderedCompletion(t *testing.T, operation string, completionFirst bool) {
	f := newMeetingFixture(t, testdb.New(t))
	other := independentMeetingFixture(t, f)
	identity := f.create(t, 10)
	attempt := domain.MeetingCreateAttemptID{20}
	if f.begin(t, identity, attempt, 1) != domain.MeetingMutationApplied {
		t.Fatal("fixture begin refused")
	}
	before := f.recovery(t, identity.ProviderMeetingID)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	complete := func(repos repositories.Repositories) error {
		outcome, err := repos.MeetingRecovery.RecordCreateReturned(ctx, identity.ProviderMeetingID, attempt, 100, instant)
		if err != nil {
			return err
		}
		if outcome != domain.MeetingMutationApplied {
			return errors.New("late completion refused")
		}
		return nil
	}
	remove := func(repos repositories.Repositories) error {
		if operation == "purge" {
			_, err := repos.Lessons.DeleteBatchOfTutor(ctx, f.tutor, 10)
			return err
		}
		if _, err := repos.Lessons.LockForEdit(ctx, f.tutor, f.lesson, nil, domain.LocalDate{}); err != nil {
			return err
		}
		outcome, err := repos.Meetings.CloseUnderLessonLock(ctx, domain.MeetingExpectation{Identity: identity, State: domain.MeetingCreating}, domain.MeetingCloseProviderError, instant)
		if err != nil {
			return err
		}
		if outcome != domain.MeetingMutationApplied {
			return errors.New("concurrent close refused")
		}
		return nil
	}
	firstWork, secondWork := remove, complete
	if completionFirst {
		firstWork, secondWork = complete, remove
	}
	release, first := holdRecoveryTransaction(t, ctx, f, firstWork)

	second := recoveryRun(t, ctx, "recovery-lifecycle-contender", func() error { return other.store.WithTx(ctx, secondWork) })
	waitMeetingLock(t, ctx, f)
	release()
	firstErr, secondErr := meetingAwait(t, ctx, first), meetingAwait(t, ctx, second)
	row := f.recovery(t, identity.ProviderMeetingID)
	opened, openErr := f.repos.Meetings.OpenUnderLessonLock(ctx, identity, 100, instant)
	retried, retryErr := f.repos.MeetingRecovery.BeginCreateUnderLessonLock(ctx, identity, domain.MeetingCreateAttemptID{21}, before.Revision, instant)
	candidates, candidatesErr := f.repos.MeetingRecovery.CleanupCandidates(ctx, repositories.MeetingRecoveryPage{Limit: 100})

	meetingRequire(t, firstErr, secondErr, openErr, retryErr, candidatesErr)
	if row.CreateState != domain.MeetingCreateReturned || row.CreateTime == nil || *row.CreateTime != 100 || row.AttemptID == nil || *row.AttemptID != attempt || opened != domain.MeetingMutationRefused || retried != domain.MeetingMutationRefused || len(candidates) != 1 || candidates[0].ProviderMeetingID != identity.ProviderMeetingID {
		t.Fatal("late completion lost evidence or published a closed/orphan meeting")
	}
	if operation == "purge" && (row.MeetingID != nil || row.CleanupRequested || row.Revision != before.Revision+1) {
		t.Fatal("purge did not preserve nullable ownership and evidence revision semantics")
	}
	if operation == "close" && (row.MeetingID == nil || !row.CleanupRequested || row.Revision != before.Revision+2) {
		t.Fatal("close lost cleanup obligation")
	}
}

func TestMeetingRecoveryPurge(t *testing.T) {
	t.Parallel()
	t.Run("student preserves tutor history and final account purge preserves orphan", testRecoveryPurge)
	t.Run("unchanged revision cannot dispatch or publish after purge", testRecoveryOldRevision)
	t.Run("late old evidence cannot affect a replacement", testRecoveryReplacement)
}

func testRecoveryPurge(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	otherTutor := f.tutorAccount(t, 6)
	card := f.card(t, invitedCard(7, otherTutor))
	otherLesson := domain.LessonID{8}
	meetingRequire(t, f.repos.Lessons.Create(t.Context(), repositories.NewLessons{TutorID: otherTutor, RelationshipID: &card, Title: "Another lesson", DurationMinutes: 60, Timezone: "UTC", Now: instant, Items: []repositories.NewLessonItem{{ID: otherLesson, StartsAt: instant}}}))
	otherMeeting := meetingInput(11, otherLesson)
	meetingRequire(t, f.repos.Meetings.CreateUnderLessonLock(t.Context(), otherMeeting))
	before := f.recovery(t, identity.ProviderMeetingID)

	forgetErr := f.repos.Relationships.ForgetPurgedStudent(t.Context(), f.student, instant)
	studentDeleted, studentErr := f.repos.Users.Delete(t.Context(), f.student)
	studentRetained := f.recovery(t, identity.ProviderMeetingID)
	deleted, purgeErr := f.repos.Lessons.DeleteBatchOfTutor(t.Context(), f.tutor, 10)
	tutorDeleted, tutorErr := f.repos.Users.Delete(t.Context(), f.tutor)
	retained := f.recovery(t, identity.ProviderMeetingID)
	other := f.recovery(t, otherMeeting.ProviderMeetingID)
	candidates, candidatesErr := f.repos.MeetingRecovery.CleanupCandidates(t.Context(), repositories.MeetingRecoveryPage{Limit: 100})
	var columns []string
	columnsErr := f.pool.QueryRow(t.Context(), "SELECT array_agg(column_name::text ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'meeting_recovery'").Scan(&columns)

	meetingRequire(t, forgetErr, studentErr, purgeErr, tutorErr, candidatesErr, columnsErr)
	if !studentDeleted || !tutorDeleted || deleted != 2 || studentRetained.MeetingID == nil || *studentRetained.MeetingID != identity.ID || retained.MeetingID != nil || retained.Revision != before.Revision || retained.CreateState != before.CreateState || retained.ProviderMeetingID != before.ProviderMeetingID || retained.Provider != before.Provider || retained.Origin != before.Origin || retained.AttemptID != nil || retained.CreateTime != nil || retained.CleanupRequested || !retained.CreatedAt.Equal(before.CreatedAt) || !retained.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("purge erased recovery, retained personal association, or changed evidence")
	}
	if other.MeetingID == nil || *other.MeetingID != otherMeeting.ID || other.CleanupRequested || len(candidates) != 1 || candidates[0].ProviderMeetingID != identity.ProviderMeetingID {
		t.Fatal("purge crossed tutor scope or orphan disappeared from cleanup candidates")
	}
	want := []string{"provider_meeting_id", "provider", "meeting_id", "origin", "create_state", "attempt_id", "create_time", "cleanup_requested", "revision", "created_at", "updated_at"}
	if fmt.Sprint(columns) != fmt.Sprint(want) {
		t.Fatalf("unexpected retained ledger fields: %v", columns)
	}
}

func testRecoveryOldRevision(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	before := f.recovery(t, identity.ProviderMeetingID)

	_, purgeErr := f.repos.Lessons.DeleteBatchOfTutor(t.Context(), f.tutor, 10)
	after := f.recovery(t, identity.ProviderMeetingID)
	began, beginErr := f.repos.MeetingRecovery.BeginCreateUnderLessonLock(t.Context(), identity, domain.MeetingCreateAttemptID{20}, before.Revision, instant)
	opened, openErr := f.repos.Meetings.OpenUnderLessonLock(t.Context(), identity, 100, instant)

	meetingRequire(t, purgeErr, beginErr, openErr)
	if before.Revision != after.Revision || after.MeetingID != nil || began != domain.MeetingMutationRefused || opened != domain.MeetingMutationRefused {
		t.Fatal("old revision conferred authority after purge")
	}
}

func testRecoveryReplacement(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	attempt := domain.MeetingCreateAttemptID{20}
	if f.begin(t, identity, attempt, 1) != domain.MeetingMutationApplied {
		t.Fatal("fixture begin refused")
	}
	expected := domain.MeetingExpectation{Identity: identity, State: domain.MeetingCreating}
	if f.mutate(t, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
		return repo.CloseUnderLessonLock(t.Context(), expected, domain.MeetingCloseProviderError, instant)
	}) != domain.MeetingMutationApplied {
		t.Fatal("fixture close refused")
	}
	replacement := f.create(t, 11)

	returned, returnedErr := f.repos.MeetingRecovery.RecordCreateReturned(t.Context(), identity.ProviderMeetingID, attempt, 100, instant)
	opened := f.mutate(t, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
		return repo.OpenUnderLessonLock(t.Context(), identity, 100, instant)
	})
	closed := f.mutate(t, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
		return repo.CloseUnderLessonLock(t.Context(), expected, domain.MeetingCloseProviderError, instant)
	})
	wrong := expected
	wrong.Identity.ID = replacement.ID
	stale := f.mutate(t, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
		return repo.CloseUnderLessonLock(t.Context(), wrong, domain.MeetingCloseProviderError, instant)
	})
	oldRow, newRow := f.recovery(t, identity.ProviderMeetingID), f.recovery(t, replacement.ProviderMeetingID)
	business := f.read(t, replacement)

	meetingRequire(t, returnedErr)
	if returned != domain.MeetingMutationApplied || opened != domain.MeetingMutationRefused || closed != domain.MeetingAlreadyClosed || stale != domain.MeetingMutationRefused || !oldRow.CleanupRequested || oldRow.CreateTime == nil || *oldRow.CreateTime != 100 || newRow.CleanupRequested || newRow.CreateState != domain.MeetingCreatePrepared || newRow.Revision != 1 || business.State != domain.MeetingCreating {
		t.Fatal("late old evidence or stale close affected replacement")
	}
}

func TestMeetingRecoveryPagination(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	const candidateCount = 105
	card := f.card(t, invitedCard(30, f.tutor))
	items := make([]repositories.NewLessonItem, candidateCount)
	for index := range items {
		items[index] = repositories.NewLessonItem{ID: domain.LessonID{byte(index + 1), 1}, StartsAt: instant.Add(time.Duration(index) * time.Hour)}
	}
	meetingRequire(t, f.repos.Lessons.Create(t.Context(), repositories.NewLessons{TutorID: f.tutor, RelationshipID: &card, Title: "Recovery candidate", DurationMinutes: 60, Timezone: "UTC", Now: instant, Items: items}))
	wanted := make([]domain.ProviderMeetingID, candidateCount)
	for index, lesson := range items {
		id := domain.MeetingID{byte(index + 1), 1}
		input := repositories.NewMeeting{ID: id, LessonID: lesson.ID, Provider: domain.MeetingProviderFake, ProviderMeetingID: domain.ProviderMeetingID(id), RoutingKey: domain.MeetingRoutingKey(id), Now: instant}
		meetingRequire(t, f.repos.Meetings.CreateUnderLessonLock(t.Context(), input))
		wanted[index] = input.ProviderMeetingID
		if index == 0 {
			_, err := f.repos.MeetingRecovery.BeginCreateUnderLessonLock(t.Context(), meetingIdentity(input), domain.MeetingCreateAttemptID{20}, 1, instant)
			meetingRequire(t, err)
			_, err = f.repos.MeetingRecovery.MarkCreateUncertain(t.Context(), input.ProviderMeetingID, domain.MeetingCreateAttemptID{20}, instant)
			meetingRequire(t, err)
		}
		if index%3 != 1 {
			_, err := f.repos.Meetings.CloseUnderLessonLock(t.Context(), domain.MeetingExpectation{Identity: meetingIdentity(input), State: domain.MeetingCreating}, domain.MeetingCloseProviderError, instant)
			meetingRequire(t, err)
		}
		if index%3 != 0 {
			_, err := f.pool.Exec(t.Context(), "DELETE FROM lessons WHERE id = $1", lesson.ID)
			meetingRequire(t, err)
		}
	}
	f.create(t, 200)
	repo := repositories.NewStore(f.pool).Repositories().MeetingRecovery

	_, zeroErr := repo.CleanupCandidates(t.Context(), repositories.MeetingRecoveryPage{Limit: 0})
	_, oversizedErr := repo.CleanupCandidates(t.Context(), repositories.MeetingRecoveryPage{Limit: 101})
	first, firstErr := repo.CleanupCandidates(t.Context(), repositories.MeetingRecoveryPage{Limit: 100})
	second, secondErr := repo.CleanupCandidates(t.Context(), repositories.MeetingRecoveryPage{After: wanted[99], Limit: 100})
	empty, emptyErr := repo.CleanupCandidates(t.Context(), repositories.MeetingRecoveryPage{After: wanted[104], Limit: 100})
	individual := recoveryPages(t, repo, 1)
	uncertain, uncertainErr := repo.ByProviderID(t.Context(), wanted[0])
	var indexDefinition string
	indexErr := f.pool.QueryRow(t.Context(), "SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = 'meeting_recovery_cleanup_idx'").Scan(&indexDefinition)

	meetingRequire(t, firstErr, secondErr, emptyErr, uncertainErr, indexErr)
	if !errors.Is(zeroErr, repositories.ErrMeetingRecoveryBatchSize) || !errors.Is(oversizedErr, repositories.ErrMeetingRecoveryBatchSize) || len(first) != 100 || len(second) != 5 || len(empty) != 0 || len(individual) != candidateCount {
		t.Fatal("cleanup candidate boundaries or invalid limits failed")
	}
	combined := append(first, second...)
	for index, id := range wanted {
		if combined[index].ProviderMeetingID != id || individual[index] != id {
			t.Fatal("keyset pagination duplicated, omitted or reordered a candidate")
		}
	}
	if uncertain.CreateState != domain.MeetingCreateUncertain || uncertain.CreateTime != nil || !uncertain.CleanupRequested || indexDefinition != "CREATE INDEX meeting_recovery_cleanup_idx ON public.meeting_recovery USING btree (provider_meeting_id) WHERE (cleanup_requested OR (meeting_id IS NULL))" {
		t.Fatal("reconstruction lost uncertainty or cleanup predicate index differs")
	}
}

func recoveryPages(t *testing.T, repo repositories.MeetingRecovery, limit int32) []domain.ProviderMeetingID {
	t.Helper()
	var ids []domain.ProviderMeetingID
	page := repositories.MeetingRecoveryPage{Limit: limit}
	for len(ids) <= 105 {
		rows, err := repo.CleanupCandidates(t.Context(), page)
		meetingRequire(t, err)
		if len(rows) == 0 {
			return ids
		}
		for _, row := range rows {
			ids = append(ids, row.ProviderMeetingID)
		}
		page.After = rows[len(rows)-1].ProviderMeetingID
	}
	t.Fatal("candidate cursor did not terminate")
	return nil
}

func TestMeetingRecoveryMigration(t *testing.T) {
	t.Parallel()
	t.Run("legacy evidence is conservative and downgrade refuses obligations", testRecoveryLegacyMigration)
	t.Run("empty ledger supports down and up", testRecoveryEmptyMigration)
}

func testRecoveryLegacyMigration(t *testing.T) {
	f := newMeetingFixture(t, testdb.NewMigratedTo(t, 23))
	inputs := []repositories.NewMeeting{meetingInput(10, f.lesson), meetingInput(11, f.lesson), meetingInput(12, f.otherLesson), meetingInput(13, f.lesson)}
	meetingRequire(t, insertLegacyMeeting(t.Context(), f, inputs[0]))
	_, err := f.pool.Exec(t.Context(), "UPDATE meetings SET state = 'closed', ended_at = updated_at, close_reason = 'provider_error' WHERE id = $1", inputs[0].ID)
	meetingRequire(t, err, insertLegacyMeeting(t.Context(), f, inputs[1]))
	_, err = f.pool.Exec(t.Context(), "UPDATE meetings SET state = 'closed', create_time = 100, ended_at = updated_at, first_join_at = updated_at, close_reason = 'tutor_end' WHERE id = $1", inputs[1].ID)
	meetingRequire(t, err, insertLegacyMeeting(t.Context(), f, inputs[2]))
	_, err = f.pool.Exec(t.Context(), "UPDATE meetings SET state = 'open', create_time = 101 WHERE id = $1", inputs[2].ID)
	meetingRequire(t, err, insertLegacyMeeting(t.Context(), f, inputs[3]))
	before := make([]repositories.StoredMeeting, len(inputs))
	for index, input := range inputs {
		before[index] = f.read(t, meetingIdentity(input))
	}

	upErr := f.database.MigrateUpTo(t.Context(), 24)
	first := recoveryMigrationRows(t, f, inputs)
	downErr := f.database.MigrateDownTo(t.Context(), 23)
	secondUpErr := f.database.MigrateUpTo(t.Context(), 24)
	after := recoveryMigrationRows(t, f, inputs)
	refusals := make([]domain.MeetingMutation, 0, len(inputs)*3)
	for _, input := range inputs {
		id := input.ProviderMeetingID
		begin, beginErr := f.repos.MeetingRecovery.BeginCreateUnderLessonLock(t.Context(), meetingIdentity(input), domain.MeetingCreateAttemptID{20}, 1, instant)
		uncertain, uncertainErr := f.repos.MeetingRecovery.MarkCreateUncertain(t.Context(), id, domain.MeetingCreateAttemptID{20}, instant)
		returned, returnedErr := f.repos.MeetingRecovery.RecordCreateReturned(t.Context(), id, domain.MeetingCreateAttemptID{20}, 100, instant)
		meetingRequire(t, beginErr, uncertainErr, returnedErr)
		refusals = append(refusals, begin, uncertain, returned)
	}
	preserved := make([]repositories.StoredMeeting, len(inputs))
	for index, input := range inputs {
		preserved[index] = f.read(t, meetingIdentity(input))
	}

	meetingRequire(t, upErr, secondUpErr)
	if code, _ := violation(downErr); code != checkViolation {
		t.Fatalf("nonempty downgrade did not refuse recovery erasure: %v", downErr)
	}
	for index, original := range before {
		row := after[index]
		wantState := domain.MeetingCreateUncertain
		if original.CreateTime != nil {
			wantState = domain.MeetingCreateReturned
		}
		if row.ProviderMeetingID != original.ProviderMeetingID || row.Provider != original.Provider || row.MeetingID == nil || *row.MeetingID != original.ID || row.Origin != domain.MeetingRecoveryLegacy || row.AttemptID != nil || row.CreateState != wantState || !sameRecoveryGeneration(row.CreateTime, original.CreateTime) || row.CleanupRequested != (original.State == domain.MeetingClosed) || row.Revision != 1 || !row.CreatedAt.Equal(original.CreatedAt) || !row.UpdatedAt.Equal(original.UpdatedAt) || row.CreateState != first[index].CreateState || !sameRecoveryGeneration(row.CreateTime, first[index].CreateTime) {
			t.Fatal("legacy backfill invented acknowledgement or lost persisted evidence")
		}
		if preserved[index].ID != original.ID || preserved[index].RoutingKey != original.RoutingKey || preserved[index].ProviderMeetingID != original.ProviderMeetingID || preserved[index].State != original.State || !sameRecoveryGeneration(preserved[index].CreateTime, original.CreateTime) || !preserved[index].UpdatedAt.Equal(original.UpdatedAt) {
			t.Fatal("migration altered business meeting metadata")
		}
	}
	for _, outcome := range refusals {
		if outcome != domain.MeetingMutationRefused {
			t.Fatal("legacy provenance accepted new attempt evidence")
		}
	}
}

func recoveryMigrationRows(t *testing.T, f meetingFixture, inputs []repositories.NewMeeting) []repositories.StoredMeetingRecovery {
	t.Helper()
	rows := make([]repositories.StoredMeetingRecovery, len(inputs))
	for index, input := range inputs {
		rows[index] = f.recovery(t, input.ProviderMeetingID)
	}
	return rows
}

func sameRecoveryGeneration(first, second *domain.ProviderCreateTime) bool {
	return first == nil && second == nil || first != nil && second != nil && *first == *second
}

func testRecoveryEmptyMigration(t *testing.T) {
	database := testdb.NewMigratedTo(t, 23)

	upErr := database.MigrateUpTo(t.Context(), 24)
	downErr := database.MigrateDownTo(t.Context(), 23)
	var absent bool
	absentErr := database.Pool.QueryRow(t.Context(), "SELECT to_regclass('public.meeting_recovery') IS NULL").Scan(&absent)
	secondUpErr := database.MigrateUpTo(t.Context(), 24)
	var count int
	countErr := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM meeting_recovery").Scan(&count)

	meetingRequire(t, upErr, downErr, absentErr, secondUpErr, countErr)
	if !absent || count != 0 {
		t.Fatal("empty recovery ledger migration cannot round trip")
	}
}

func recoveryRun(t *testing.T, ctx context.Context, name string, work func() error) <-chan error {
	t.Helper()
	finished := make(chan struct{})
	result := core.Run(ctx, meetingLogger(), name, func() error {
		defer close(finished)
		return work()
	})
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		select {
		case <-finished:
		case <-cleanup.Done():
			t.Error("recovery fixture goroutine did not finish")
		}
	})
	return result
}

func TestMeetingRecoveryAtomicLedgerConflict(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	_, err := f.pool.Exec(t.Context(), "DELETE FROM meetings WHERE id = $1", identity.ID)
	meetingRequire(t, err)
	replacement := meetingInput(11, f.lesson)
	replacement.ProviderMeetingID = identity.ProviderMeetingID

	createErr := f.repos.Meetings.CreateUnderLessonLock(t.Context(), replacement)
	_, businessErr := f.repos.Meetings.ByID(t.Context(), replacement.LessonID, replacement.ID)
	retained := f.recovery(t, identity.ProviderMeetingID)
	var businessCount int
	countErr := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM meetings").Scan(&businessCount)

	meetingRequire(t, countErr)
	if code, _ := violation(createErr); code != uniqueViolation || !errors.Is(businessErr, repositories.ErrNotFound) || businessCount != 0 || retained.MeetingID != nil || retained.Revision != 1 || retained.CreateState != domain.MeetingCreatePrepared {
		t.Fatal("ledger insert failure committed a business row or replaced orphan evidence")
	}
}

func TestMeetingRecoveryLegacyGenerationGuards(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.NewMigratedTo(t, 23))
	input := meetingInput(10, f.lesson)
	meetingRequire(t, insertLegacyMeeting(t.Context(), f, input))
	_, err := f.pool.Exec(t.Context(), "UPDATE meetings SET state = 'open', create_time = 100 WHERE id = $1", input.ID)
	meetingRequire(t, err, f.database.MigrateUpTo(t.Context(), 24))
	identity := meetingIdentity(input)
	created := domain.ProviderCreateTime(100)
	wrongTime := created + 1

	wrongJoin := f.mutate(t, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
		return repo.FirstJoinUnderLessonLock(t.Context(), identity, wrongTime, instant)
	})
	joined := f.mutate(t, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
		return repo.FirstJoinUnderLessonLock(t.Context(), identity, created, instant)
	})
	wrongClose := f.mutate(t, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
		return repo.CloseUnderLessonLock(t.Context(), domain.MeetingExpectation{Identity: identity, State: domain.MeetingOpen, CreateTime: &wrongTime}, domain.MeetingCloseTutorEnd, instant)
	})
	closed := f.mutate(t, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
		return repo.CloseUnderLessonLock(t.Context(), domain.MeetingExpectation{Identity: identity, State: domain.MeetingOpen, CreateTime: &created}, domain.MeetingCloseTutorEnd, instant)
	})
	row := f.recovery(t, identity.ProviderMeetingID)

	if wrongJoin != domain.MeetingMutationRefused || wrongClose != domain.MeetingMutationRefused || joined != domain.MeetingMutationApplied || closed != domain.MeetingMutationApplied || row.Origin != domain.MeetingRecoveryLegacy || row.AttemptID != nil || row.CreateState != domain.MeetingCreateReturned || row.CreateTime == nil || *row.CreateTime != created || !row.CleanupRequested {
		t.Fatal("legacy provenance lost exact-generation business guards")
	}
}
