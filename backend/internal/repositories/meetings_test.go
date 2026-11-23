package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
)

type meetingFixture struct {
	tutoringFixture
	database    *testdb.Database
	tutor       domain.UserID
	student     domain.UserID
	lesson      domain.LessonID
	otherLesson domain.LessonID
}

func newMeetingFixture(t *testing.T, database *testdb.Database) meetingFixture {
	t.Helper()
	store := repositories.NewStore(database.Pool)
	f := meetingFixture{tutoringFixture: tutoringFixture{pool: database.Pool, store: store, repos: store.Repositories()}, database: database}
	f.tutor, f.student = f.tutorAccount(t, 1), f.studentAccount(t, 2)
	card := domain.RelationshipID{3}
	_, err := database.Pool.Exec(t.Context(), `INSERT INTO relationships (id, tutor_id, student_id, state, display_name, colour, linked_at, created_at, updated_at)
VALUES ($1, $2, $3, 'active', 'A student', 'clay', $4, $4, $4)`, card, f.tutor, f.student, instant)
	meetingRequire(t, err)
	f.lesson, f.otherLesson = domain.LessonID{4}, domain.LessonID{5}
	meetingRequire(t, f.repos.Lessons.Create(t.Context(), repositories.NewLessons{
		TutorID: f.tutor, RelationshipID: &card, Title: "A lesson", DurationMinutes: 60, Timezone: "UTC", Now: instant,
		Items: []repositories.NewLessonItem{{ID: f.lesson, StartsAt: instant.Add(time.Hour)}, {ID: f.otherLesson, StartsAt: instant.Add(2 * time.Hour)}},
	}))
	return f
}

func (f meetingFixture) tutorAccount(t *testing.T, id byte) domain.UserID {
	t.Helper()
	return f.tutoringFixture.tutor(t, id)
}

func (f meetingFixture) studentAccount(t *testing.T, id byte) domain.UserID {
	t.Helper()
	return f.tutoringFixture.student(t, id)
}

func meetingRequire(t *testing.T, failures ...error) {
	t.Helper()
	for _, err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func meetingInput(id byte, lesson domain.LessonID) repositories.NewMeeting {
	return repositories.NewMeeting{ID: domain.MeetingID{id}, LessonID: lesson, Provider: domain.MeetingProviderFake,
		ProviderMeetingID: domain.ProviderMeetingID{id}, RoutingKey: domain.MeetingRoutingKey{id}, Now: instant}
}

func meetingIdentity(input repositories.NewMeeting) domain.MeetingIdentity {
	return domain.MeetingIdentity{ID: input.ID, LessonID: input.LessonID, ProviderMeetingID: input.ProviderMeetingID}
}

func (f meetingFixture) underLock(ctx context.Context, work func(repositories.Meetings) error) error {
	return f.store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := repos.Lessons.LockForEdit(ctx, f.tutor, f.lesson, nil, domain.LocalDate{})
		if err != nil {
			return err
		}
		if len(locked) != 1 {
			return errors.New("the fixture lesson was not locked")
		}
		return work(repos.Meetings)
	})
}

func (f meetingFixture) create(t *testing.T, id byte) domain.MeetingIdentity {
	t.Helper()
	input := meetingInput(id, f.lesson)
	meetingRequire(t, f.underLock(t.Context(), func(repo repositories.Meetings) error { return repo.CreateUnderLessonLock(t.Context(), input) }))
	return meetingIdentity(input)
}

func (f meetingFixture) mutate(t *testing.T, work func(repositories.Meetings) (domain.MeetingMutation, error)) domain.MeetingMutation {
	t.Helper()
	var outcome domain.MeetingMutation
	meetingRequire(t, f.underLock(t.Context(), func(repo repositories.Meetings) error {
		var err error
		outcome, err = work(repo)
		return err
	}))
	return outcome
}

func (f meetingFixture) read(t *testing.T, identity domain.MeetingIdentity) repositories.StoredMeeting {
	t.Helper()
	row, err := f.repos.Meetings.ByID(t.Context(), identity.LessonID, identity.ID)
	meetingRequire(t, err)
	return row
}

func TestMeetingPersistenceConstraints(t *testing.T) {
	t.Parallel()
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	f.acknowledge(t, identity, 100)
	cases := []struct{ name, assignment, code string }{
		{"zero id", "id = '00000000-0000-0000-0000-000000000000'", checkViolation},
		{"zero lesson", "lesson_id = '00000000-0000-0000-0000-000000000000'", checkViolation},
		{"null lesson", "lesson_id = NULL", "23502"},
		{"null id", "id = NULL", "23502"},
		{"zero provider id", "provider_meeting_id = '00000000-0000-0000-0000-000000000000'", checkViolation},
		{"zero routing key", "routing_key = '00000000-0000-0000-0000-000000000000'", checkViolation},
		{"unknown provider", "provider = 'other'", checkViolation},
		{"null provider", "provider = NULL", "23502"},
		{"unknown state", "state = 'other'", checkViolation},
		{"null state", "state = NULL", "23502"},
		{"creating with create time", "create_time = 1", checkViolation},
		{"creating with join", "first_join_at = created_at", checkViolation},
		{"creating with end", "ended_at = created_at", checkViolation},
		{"creating with reason", "close_reason = 'provider_error'", checkViolation},
		{"open null time", "state = 'open'", checkViolation},
		{"open zero time", "state = 'open', create_time = 0", checkViolation},
		{"open negative time", "state = 'open', create_time = -1", checkViolation},
		{"open with end", "state = 'open', create_time = 1, ended_at = created_at", checkViolation},
		{"open with reason", "state = 'open', create_time = 1, close_reason = 'tutor_end'", checkViolation},
		{"closed null end", "state = 'closed', close_reason = 'provider_error'", checkViolation},
		{"closed null reason", "state = 'closed', ended_at = created_at", checkViolation},
		{"closed unknown reason", "state = 'closed', ended_at = created_at, close_reason = 'other'", checkViolation},
		{"closed join without generation", "state = 'closed', ended_at = created_at, close_reason = 'provider_error', first_join_at = created_at", checkViolation},
		{"updated before creation", "updated_at = created_at - interval '1 second'", checkViolation},
		{"join before creation", "state = 'open', create_time = 1, first_join_at = created_at - interval '1 second'", checkViolation},
		{"join after update", "state = 'open', create_time = 1, first_join_at = updated_at + interval '1 second'", checkViolation},
		{"end before creation", "state = 'closed', close_reason = 'provider_error', ended_at = created_at - interval '1 second'", checkViolation},
		{"end after update", "state = 'closed', close_reason = 'provider_error', ended_at = updated_at + interval '1 second'", checkViolation},
		{"join after end", "state = 'closed', close_reason = 'tutor_end', create_time = 1, ended_at = created_at, first_join_at = created_at + interval '1 second', updated_at = created_at + interval '2 seconds'", checkViolation},
		{"infinite creation", "created_at = '-infinity'", checkViolation},
		{"infinite update", "updated_at = 'infinity'", checkViolation},
		{"null creation", "created_at = NULL", "23502"},
		{"null update", "updated_at = NULL", "23502"},
		{"null provider identity", "provider_meeting_id = NULL", "23502"},
		{"null routing", "routing_key = NULL", "23502"},
		{"missing lesson", "lesson_id = '00000000-0000-0000-0000-000000000099'", foreignKeyViolation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statement := "UPDATE meetings SET " + tc.assignment + " WHERE id = $1"

			_, err := f.pool.Exec(t.Context(), statement, identity.ID)

			if code, _ := violation(err); code != tc.code {
				t.Fatalf("violation = %s, want %s, error = %v", code, tc.code, err)
			}
		})
	}
	t.Run("unique identities and current lesson", func(t *testing.T) {
		duplicateLesson := meetingInput(11, f.lesson)
		duplicateProvider := meetingInput(12, f.otherLesson)
		duplicateProvider.ProviderMeetingID = identity.ProviderMeetingID
		duplicateRouting := meetingInput(13, f.otherLesson)
		duplicateRouting.RoutingKey = domain.MeetingRoutingKey{10}
		other := meetingInput(14, f.otherLesson)

		lessonErr := f.repos.Meetings.CreateUnderLessonLock(t.Context(), duplicateLesson)
		providerErr := f.repos.Meetings.CreateUnderLessonLock(t.Context(), duplicateProvider)
		routingErr := f.repos.Meetings.CreateUnderLessonLock(t.Context(), duplicateRouting)
		otherErr := f.repos.Meetings.CreateUnderLessonLock(t.Context(), other)

		for _, err := range []error{lessonErr, providerErr, routingErr} {
			if code, _ := violation(err); code != uniqueViolation {
				t.Fatalf("expected uniqueness refusal, got %v", err)
			}
		}
		meetingRequire(t, otherErr)
	})
}

func TestMeetingPersistenceTransitions(t *testing.T) {
	t.Parallel()
	t.Run("complete lifecycle and idempotent observations", func(t *testing.T) {
		f := newMeetingFixture(t, testdb.New(t))
		identity := f.create(t, 10)
		f.acknowledge(t, identity, 100)
		creating := f.read(t, identity)
		created := domain.ProviderCreateTime(100)
		expected := domain.MeetingExpectation{Identity: identity, State: domain.MeetingOpen, CreateTime: &created}

		opened := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
			return r.OpenUnderLessonLock(t.Context(), identity, created, instant.Add(time.Second))
		})
		openRow := f.read(t, identity)
		joined := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
			return r.FirstJoinUnderLessonLock(t.Context(), identity, created, instant.Add(2*time.Second))
		})
		rejoined := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
			return r.FirstJoinUnderLessonLock(t.Context(), identity, created, instant.Add(3*time.Second))
		})
		closed := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
			return r.CloseUnderLessonLock(t.Context(), expected, domain.MeetingCloseTutorEnd, instant.Add(4*time.Second))
		})
		reclosed := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
			return r.CloseUnderLessonLock(t.Context(), expected, domain.MeetingCloseProviderError, instant.Add(5*time.Second))
		})
		row := f.read(t, identity)
		_, currentErr := f.repos.Meetings.Current(t.Context(), f.lesson)

		if opened != domain.MeetingMutationApplied || joined != domain.MeetingMutationApplied || closed != domain.MeetingMutationApplied || rejoined != domain.MeetingAlreadyJoined || reclosed != domain.MeetingAlreadyClosed {
			t.Fatal("unexpected lifecycle mutation outcomes")
		}
		if creating.State != domain.MeetingCreating || creating.CreateTime != nil || openRow.State != domain.MeetingOpen || openRow.CreateTime == nil || *openRow.CreateTime != created {
			t.Fatal("creating/open generation not preserved")
		}
		if row.State != domain.MeetingClosed || row.FirstJoinAt == nil || !row.FirstJoinAt.Equal(instant.Add(2*time.Second)) || row.EndedAt == nil || !row.EndedAt.Equal(instant.Add(4*time.Second)) || !row.UpdatedAt.Equal(*row.EndedAt) || row.CloseReason == nil || *row.CloseReason != domain.MeetingCloseTutorEnd || !errors.Is(currentErr, repositories.ErrNotFound) {
			t.Fatalf("terminal row = %+v, current error = %v", row, currentErr)
		}
	})
	t.Run("failed creation and replacement reject stale operations", testMeetingReplacement)
	t.Run("wrong scope generation and backwards time refuse", testMeetingGenerationRefusal)
	t.Run("backwards time alone refuses opening and first join", testMeetingBackwardsTime)
	t.Run("backwards time cannot strand a close", testMeetingCloseBackwardsTime)
	t.Run("opening isolates every identity predicate", testMeetingOpenIdentity)
}

func testMeetingReplacement(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	old := f.create(t, 10)
	expected := domain.MeetingExpectation{Identity: old, State: domain.MeetingCreating}

	closed := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
		return r.CloseUnderLessonLock(t.Context(), expected, domain.MeetingCloseProviderError, instant.Add(time.Second))
	})
	failed := f.read(t, old)
	replacement := meetingInput(11, f.lesson)
	createErr := f.underLock(t.Context(), func(r repositories.Meetings) error { return r.CreateUnderLessonLock(t.Context(), replacement) })
	lateOpen := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
		return r.OpenUnderLessonLock(t.Context(), old, 100, instant.Add(2*time.Second))
	})
	lateJoin := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
		return r.FirstJoinUnderLessonLock(t.Context(), old, 100, instant.Add(2*time.Second))
	})
	lateClose := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
		return r.CloseUnderLessonLock(t.Context(), expected, domain.MeetingCloseTutorEnd, instant.Add(2*time.Second))
	})
	current, currentErr := f.repos.Meetings.Current(t.Context(), f.lesson)

	meetingRequire(t, createErr, currentErr)
	if closed != domain.MeetingMutationApplied || failed.CreateTime != nil || failed.FirstJoinAt != nil || failed.State != domain.MeetingClosed || lateOpen != domain.MeetingMutationRefused || lateJoin != domain.MeetingMutationRefused || lateClose != domain.MeetingAlreadyClosed || current.ID != replacement.ID || current.State != domain.MeetingCreating {
		t.Fatal("failed creation or replacement invariant failed")
	}
}

func testMeetingGenerationRefusal(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	f.acknowledge(t, identity, 100)
	created := domain.ProviderCreateTime(100)
	f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
		return r.OpenUnderLessonLock(t.Context(), identity, created, instant.Add(time.Second))
	})
	wrongLesson, wrongProvider, wrongID := identity, identity, identity
	wrongLesson.LessonID, wrongProvider.ProviderMeetingID, wrongID.ID = f.otherLesson, domain.ProviderMeetingID{99}, domain.MeetingID{99}
	wrongTime := created + 1
	cases := []domain.MeetingExpectation{
		{Identity: wrongLesson, State: domain.MeetingOpen, CreateTime: &created},
		{Identity: wrongProvider, State: domain.MeetingOpen, CreateTime: &created},
		{Identity: wrongID, State: domain.MeetingOpen, CreateTime: &created},
		{Identity: identity, State: domain.MeetingOpen, CreateTime: &wrongTime},
	}
	for _, expected := range cases {
		t.Run(fmt.Sprintf("%x-%x-%s-%v", expected.Identity.ID, expected.Identity.ProviderMeetingID, expected.State, expected.Identity.LessonID), func(t *testing.T) {
			joinTime := created
			if expected.CreateTime != nil {
				joinTime = *expected.CreateTime
			}

			closed := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
				return r.CloseUnderLessonLock(t.Context(), expected, domain.MeetingCloseTutorEnd, instant.Add(2*time.Second))
			})
			joined := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
				return r.FirstJoinUnderLessonLock(t.Context(), expected.Identity, joinTime, instant.Add(2*time.Second))
			})
			opened := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
				return r.OpenUnderLessonLock(t.Context(), expected.Identity, created, instant)
			})
			row := f.read(t, identity)

			if closed != domain.MeetingMutationRefused || joined != domain.MeetingMutationRefused || opened != domain.MeetingMutationRefused || row.State != domain.MeetingOpen || row.FirstJoinAt != nil {
				t.Fatal("stale or mismatched operation changed the row")
			}
		})
	}
}

type meetingStep struct {
	outcome domain.MeetingMutation
	err     error
}

func meetingLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func meetingAwait[T any](t *testing.T, ctx context.Context, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-ctx.Done():
		t.Fatal("meeting transaction barrier deadline exceeded")
		var zero T
		return zero
	}
}

func holdMeetingTransaction(t *testing.T, ctx context.Context, f meetingFixture, lockLesson bool, work func(repositories.Meetings) (domain.MeetingMutation, error)) (func(), <-chan error, domain.MeetingMutation) {
	t.Helper()
	entered := make(chan meetingStep, 1)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	finished := core.Run(ctx, meetingLogger(), "meeting-transaction-holder", func() error {
		transaction := func(repo repositories.Meetings) error {
			outcome, err := work(repo)
			entered <- meetingStep{outcome: outcome, err: err}
			if err != nil {
				return err
			}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if lockLesson {
			return f.underLock(ctx, transaction)
		}
		return f.store.WithTx(ctx, func(repos repositories.Repositories) error { return transaction(repos.Meetings) })
	})
	step := meetingAwait(t, ctx, entered)
	meetingRequire(t, step.err)
	return unblock, finished, step.outcome
}

func waitMeetingLock(t *testing.T, ctx context.Context, f meetingFixture) {
	t.Helper()
	for {
		var waiting bool
		err := f.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND cardinality(pg_blocking_pids(pid)) > 0)").Scan(&waiting)
		meetingRequire(t, err)
		if waiting {
			return
		}
		if ctx.Err() != nil {
			t.Fatal("meeting operation did not reach a database lock wait")
		}
	}
}

func TestMeetingPersistenceConcurrency(t *testing.T) {
	t.Parallel()
	t.Run("unique index serializes concurrent inserts", func(t *testing.T) {
		f := newMeetingFixture(t, testdb.New(t))
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		firstInput, secondInput := meetingInput(10, f.lesson), meetingInput(11, f.lesson)
		release, first, _ := holdMeetingTransaction(t, ctx, f, false, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
			return domain.MeetingMutationApplied, repo.CreateUnderLessonLock(ctx, firstInput)
		})

		second := core.Run(ctx, meetingLogger(), "meeting-insert-contender", func() error {
			return f.store.WithTx(ctx, func(repos repositories.Repositories) error {
				return repos.Meetings.CreateUnderLessonLock(ctx, secondInput)
			})
		})
		waitMeetingLock(t, ctx, f)
		release()
		firstErr, secondErr := meetingAwait(t, ctx, first), meetingAwait(t, ctx, second)
		row, currentErr := f.repos.Meetings.Current(ctx, f.lesson)

		meetingRequire(t, firstErr, currentErr)
		if code, _ := violation(secondErr); code != uniqueViolation || row.ID != firstInput.ID {
			t.Fatalf("concurrent insertion result = %v, row = %+v", secondErr, row)
		}
	})
	for _, scenario := range []string{"close-before-open", "open-before-close", "close-before-join", "join-before-close"} {
		t.Run(scenario, func(t *testing.T) { testMeetingOrderedRace(t, scenario) })
	}
}

func testMeetingOrderedRace(t *testing.T, scenario string) {
	f := newMeetingFixture(t, testdb.New(t))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	identity := f.create(t, 10)
	f.acknowledge(t, identity, 100)
	created := domain.ProviderCreateTime(100)
	creating := domain.MeetingExpectation{Identity: identity, State: domain.MeetingCreating}
	open := domain.MeetingExpectation{Identity: identity, State: domain.MeetingOpen, CreateTime: &created}
	if scenario == "close-before-join" || scenario == "join-before-close" {
		f.mutate(t, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
			return repo.OpenUnderLessonLock(ctx, identity, created, instant.Add(time.Second))
		})
	}
	firstWork := func(repo repositories.Meetings) (domain.MeetingMutation, error) {
		switch scenario {
		case "close-before-open":
			return repo.CloseUnderLessonLock(ctx, creating, domain.MeetingCloseProviderError, instant.Add(2*time.Second))
		case "open-before-close":
			return repo.OpenUnderLessonLock(ctx, identity, created, instant.Add(2*time.Second))
		case "close-before-join":
			return repo.CloseUnderLessonLock(ctx, open, domain.MeetingCloseTutorEnd, instant.Add(2*time.Second))
		case "join-before-close":
			return repo.FirstJoinUnderLessonLock(ctx, identity, created, instant.Add(2*time.Second))
		default:
			return domain.MeetingMutationRefused, errors.New("unknown meeting race scenario")
		}
	}
	release, first, firstOutcome := holdMeetingTransaction(t, ctx, f, true, firstWork)
	var secondOutcome domain.MeetingMutation

	second := core.Run(ctx, meetingLogger(), "meeting-transition-contender", func() error {
		return f.underLock(ctx, func(repo repositories.Meetings) error {
			var err error
			switch scenario {
			case "close-before-open":
				secondOutcome, err = repo.OpenUnderLessonLock(ctx, identity, created, instant.Add(3*time.Second))
			case "close-before-join":
				secondOutcome, err = repo.FirstJoinUnderLessonLock(ctx, identity, created, instant.Add(3*time.Second))
			default:
				secondOutcome, err = repo.CloseUnderLessonLock(ctx, open, domain.MeetingCloseTutorEnd, instant.Add(3*time.Second))
			}
			return err
		})
	})
	waitMeetingLock(t, ctx, f)
	release()
	firstErr, secondErr := meetingAwait(t, ctx, first), meetingAwait(t, ctx, second)
	row := f.read(t, identity)

	meetingRequire(t, firstErr, secondErr)
	expectedSecond := domain.MeetingMutationApplied
	if scenario == "close-before-open" || scenario == "close-before-join" {
		expectedSecond = domain.MeetingMutationRefused
	}
	if firstOutcome != domain.MeetingMutationApplied || secondOutcome != expectedSecond || row.State != domain.MeetingClosed || (row.FirstJoinAt != nil) != (scenario == "join-before-close") {
		t.Fatalf("race %s: first=%s second=%s row=%+v", scenario, firstOutcome, secondOutcome, row)
	}
}

func TestMeetingPersistenceRollbackAndPurge(t *testing.T) {
	t.Parallel()
	t.Run("rollback does not publish a transition", func(t *testing.T) {
		f := newMeetingFixture(t, testdb.New(t))
		identity := f.create(t, 10)
		f.acknowledge(t, identity, 100)
		aborted := errors.New("abort the fixture transaction")
		var outcome domain.MeetingMutation

		err := f.underLock(t.Context(), func(repo repositories.Meetings) error {
			var openErr error
			outcome, openErr = repo.OpenUnderLessonLock(t.Context(), identity, 100, instant.Add(time.Second))
			if openErr != nil {
				return openErr
			}
			return aborted
		})
		row := f.read(t, identity)

		if !errors.Is(err, aborted) || outcome != domain.MeetingMutationApplied || row.State != domain.MeetingCreating || row.CreateTime != nil || !row.UpdatedAt.Equal(instant) {
			t.Fatalf("rollback: outcome=%s error=%v row=%+v", outcome, err, row)
		}
	})
	t.Run("cancelled database lock wait cannot mutate", testMeetingCancellation)
	t.Run("student history survives and tutor purge cascades only owned rows", testMeetingPurge)
}

func testMeetingCancellation(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	identity := f.create(t, 10)
	f.acknowledge(t, identity, 100)
	release, first, _ := holdMeetingTransaction(t, ctx, f, true, func(repo repositories.Meetings) (domain.MeetingMutation, error) {
		return repo.OpenUnderLessonLock(ctx, identity, 100, instant.Add(time.Second))
	})
	waitCtx, cancelWait := context.WithCancel(ctx)
	defer cancelWait()

	second := core.Run(ctx, meetingLogger(), "meeting-cancelled-contender", func() error {
		return f.underLock(waitCtx, func(repo repositories.Meetings) error {
			_, err := repo.FirstJoinUnderLessonLock(waitCtx, identity, 100, instant.Add(2*time.Second))
			return err
		})
	})
	waitMeetingLock(t, ctx, f)
	cancelWait()
	waitErr := meetingAwait(t, ctx, second)
	release()
	firstErr := meetingAwait(t, ctx, first)
	row := f.read(t, identity)

	meetingRequire(t, firstErr)
	if !errors.Is(waitErr, context.Canceled) || row.State != domain.MeetingOpen || row.FirstJoinAt != nil || !row.UpdatedAt.Equal(instant.Add(time.Second)) {
		t.Fatalf("cancelled waiter: error=%v row=%+v", waitErr, row)
	}
}

func testMeetingPurge(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	f.acknowledge(t, identity, 100)
	otherTutor := f.tutorAccount(t, 6)
	card := f.card(t, invitedCard(7, otherTutor))
	otherLesson := domain.LessonID{8}
	meetingRequire(t, f.repos.Lessons.Create(t.Context(), repositories.NewLessons{TutorID: otherTutor, RelationshipID: &card, Title: "Another lesson", DurationMinutes: 60, Timezone: "UTC", Now: instant, Items: []repositories.NewLessonItem{{ID: otherLesson, StartsAt: instant}}}))
	otherMeeting := meetingInput(11, otherLesson)
	meetingRequire(t, f.repos.Meetings.CreateUnderLessonLock(t.Context(), otherMeeting))

	forgetErr := f.repos.Relationships.ForgetPurgedStudent(t.Context(), f.student, instant.Add(time.Second))
	studentDeleted, studentErr := f.repos.Users.Delete(t.Context(), f.student)
	retained, retainedErr := f.repos.Meetings.ByID(t.Context(), f.lesson, identity.ID)
	deleted, purgeErr := f.repos.Lessons.DeleteBatchOfTutor(t.Context(), f.tutor, 10)
	_, removedErr := f.repos.Meetings.ByID(t.Context(), f.lesson, identity.ID)
	other, otherErr := f.repos.Meetings.ByID(t.Context(), otherLesson, otherMeeting.ID)

	meetingRequire(t, forgetErr, studentErr, retainedErr, purgeErr, otherErr)
	if !studentDeleted || retained.ID != identity.ID || deleted != 2 || !errors.Is(removedErr, repositories.ErrNotFound) || other.ID != otherMeeting.ID {
		t.Fatalf("purge ownership failed: deleted=%d removed=%v", deleted, removedErr)
	}
}

func TestMeetingMigrationRoundTrip(t *testing.T) {
	t.Parallel()
	database := testdb.NewMigratedTo(t, 21)
	f := newMeetingFixture(t, database)

	upErr := database.MigrateUpTo(t.Context(), 22)
	if upErr == nil {
		upErr = insertLegacyMeeting(t.Context(), f, meetingInput(10, f.lesson))
	}
	downErr := database.MigrateDownTo(t.Context(), 21)
	var absent bool
	absentErr := database.Pool.QueryRow(t.Context(), "SELECT to_regclass('public.meetings') IS NULL").Scan(&absent)
	secondUpErr := database.MigrateUpTo(t.Context(), 22)
	var count int
	countErr := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM lessons WHERE title = 'A lesson' AND duration_minutes = 60 AND ((id = $1 AND starts_at = $3) OR (id = $2 AND starts_at = $4))", f.lesson, f.otherLesson, instant.Add(time.Hour), instant.Add(2*time.Hour)).Scan(&count)
	createErr := insertLegacyMeeting(t.Context(), f, meetingInput(11, f.lesson))
	row, readErr := f.repos.Meetings.Current(t.Context(), f.lesson)

	meetingRequire(t, upErr, downErr, absentErr, secondUpErr, countErr, createErr, readErr)
	if !absent || count != 2 || row.ID != (domain.MeetingID{11}) || row.State != domain.MeetingCreating {
		t.Fatalf("migration round trip: absent=%t lessons=%d row=%+v", absent, count, row)
	}
}

func testMeetingBackwardsTime(t *testing.T) {
	for _, operation := range []string{"open", "first_join"} {
		t.Run(operation, func(t *testing.T) {
			f := newMeetingFixture(t, testdb.New(t))
			identity := f.create(t, 10)
			f.acknowledge(t, identity, 100)
			created := domain.ProviderCreateTime(100)
			if operation != "open" {
				opened := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
					return r.OpenUnderLessonLock(t.Context(), identity, created, instant.Add(2*time.Second))
				})
				if opened != domain.MeetingMutationApplied {
					t.Fatal("fixture meeting did not open")
				}
			}
			before := f.read(t, identity)
			earlier := before.UpdatedAt.Add(-time.Second)

			outcome := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
				switch operation {
				case "open":
					return r.OpenUnderLessonLock(t.Context(), identity, created, earlier)
				case "first_join":
					return r.FirstJoinUnderLessonLock(t.Context(), identity, created, earlier)
				default:
					panic("unknown fixture transition")
				}
			})
			row := f.read(t, identity)

			if outcome != domain.MeetingMutationRefused || row.State != before.State || !row.UpdatedAt.Equal(before.UpdatedAt) || row.FirstJoinAt != nil || row.EndedAt != nil || row.CloseReason != nil {
				t.Fatalf("backwards time changed meeting: outcome=%s row=%+v", outcome, row)
			}
			if (row.CreateTime == nil) != (before.CreateTime == nil) || (row.CreateTime != nil && *row.CreateTime != *before.CreateTime) {
				t.Fatal("backwards time changed generation")
			}
		})
	}
}

func testMeetingCloseBackwardsTime(t *testing.T) {
	f := newMeetingFixture(t, testdb.New(t))
	identity := f.create(t, 10)
	f.acknowledge(t, identity, 100)
	created := domain.ProviderCreateTime(100)
	opened := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
		return r.OpenUnderLessonLock(t.Context(), identity, created, instant.Add(2*time.Second))
	})
	before := f.read(t, identity)

	outcome := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
		return r.CloseUnderLessonLock(t.Context(), domain.MeetingExpectation{Identity: identity, State: domain.MeetingOpen, CreateTime: &created}, domain.MeetingCloseTutorEnd, before.UpdatedAt.Add(-time.Second))
	})

	row, recovery := f.read(t, identity), f.recovery(t, identity.ProviderMeetingID)
	if opened != domain.MeetingMutationApplied || outcome != domain.MeetingMutationApplied || row.State != domain.MeetingClosed || !recovery.CleanupRequested {
		t.Fatalf("a close before the row's last change: outcome=%s row=%+v", outcome, row)
	}
	if !row.UpdatedAt.Equal(before.UpdatedAt) || row.EndedAt == nil || !row.EndedAt.Equal(before.UpdatedAt) {
		t.Fatalf("the close moved the row backwards: %+v", row)
	}
}

func testMeetingOpenIdentity(t *testing.T) {
	for _, field := range []string{"lesson", "meeting", "provider"} {
		t.Run(field, func(t *testing.T) {
			f := newMeetingFixture(t, testdb.New(t))
			identity := f.create(t, 10)
			f.acknowledge(t, identity, 100)
			wrong := identity
			switch field {
			case "lesson":
				wrong.LessonID = f.otherLesson
			case "meeting":
				wrong.ID = domain.MeetingID{99}
			case "provider":
				wrong.ProviderMeetingID = domain.ProviderMeetingID{99}
			}
			created := domain.ProviderCreateTime(100)

			refused := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
				return r.OpenUnderLessonLock(t.Context(), wrong, created, instant.Add(time.Second))
			})
			unchanged := f.read(t, identity)
			applied := f.mutate(t, func(r repositories.Meetings) (domain.MeetingMutation, error) {
				return r.OpenUnderLessonLock(t.Context(), identity, created, instant.Add(time.Second))
			})
			opened := f.read(t, identity)

			if refused != domain.MeetingMutationRefused || unchanged.State != domain.MeetingCreating || unchanged.CreateTime != nil || !unchanged.UpdatedAt.Equal(instant) {
				t.Fatal("wrong identity opened the creating meeting")
			}
			if applied != domain.MeetingMutationApplied || opened.State != domain.MeetingOpen || opened.CreateTime == nil || *opened.CreateTime != created {
				t.Fatal("valid identity did not open the creating meeting")
			}
		})
	}
}
