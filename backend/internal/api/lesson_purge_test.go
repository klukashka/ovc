package api_test

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/tutoring"
)

func (h *authHarness) scheduleRows(t *testing.T, tutor domain.UserID) [3]int64 {
	t.Helper()
	return [3]int64{
		h.lessonRows(t, tutor),
		h.countRows(t, "SELECT count(id) FROM lesson_series WHERE tutor_id = $1::uuid", tutor.String()),
		h.countRows(t, "SELECT count(lesson_roster.lesson_id) FROM lesson_roster JOIN lessons ON lessons.id = lesson_roster.lesson_id WHERE lessons.tutor_id = $1::uuid", tutor.String()),
	}
}

func TestTutorPurgeDeletesLessonsSeriesAndRostersInBatches(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.visibilityParties(t)
	original := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-11", "18:00").weekly([]int{2, 4}, 4))
	h.exec(t, `INSERT INTO lesson_series (id, tutor_id, group_id, weekdays, local_start, duration_minutes, timezone, first_date, count, title,
    materialised_until, split_from_series_id, created_at, updated_at)
SELECT gen_random_uuid(), tutor_id, group_id, weekdays, local_start, duration_minutes, timezone, first_date, 2, title, materialised_until, id,
    created_at + interval '1 minute', updated_at
FROM lesson_series WHERE id = $1::uuid`, original.Series.ID)
	h.exec2(t, "UPDATE lessons SET series_id = (SELECT id FROM lesson_series WHERE split_from_series_id = $1::uuid) WHERE id = $2::uuid",
		original.Series.ID, original.Lessons[3].ID)
	h.cancelWithRoster(t, original.Lessons[0].ID, parties.student, parties.otherStudent)
	h.seedFutureLessons(t, parties.tutor, parties.card, 600)
	before := h.scheduleRows(t, parties.tutor)
	h.deleteMe(t, parties.tutorToken, accountPassword)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	if before != [3]int64{604, 2, 2} {
		t.Fatalf("the seed holds lessons, series and roster rows %v", before)
	}
	if after := h.scheduleRows(t, parties.tutor); after != [3]int64{} || h.userExists(t, tutorEmail) {
		t.Fatalf("the purged tutor keeps lessons, series and roster rows %v", after)
	}
	if !h.userExists(t, "member-10@example.org") {
		t.Fatal("the tutor's purge removed a student")
	}
	if !strings.Contains(h.logs.String(), `"msg":"account.purged"`) || strings.Contains(h.logs.String(), `"msg":"account.purge_failed"`) {
		t.Fatal("the tutor's purge did not finish")
	}
}

func TestStudentPurgeDeletesItsRosterRows(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.visibilityParties(t)
	lesson := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00")).Lessons[0]
	h.cancelWithRoster(t, lesson.ID, parties.student, parties.otherStudent)
	h.deleteMe(t, parties.studentToken, accountPassword)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	if h.userExists(t, "member-10@example.org") {
		t.Fatal("the student was not purged")
	}
	if left := h.countRows(t, "SELECT count(lesson_id) FROM lesson_roster WHERE student_id = $1::uuid", parties.student.String()); left != 0 {
		t.Fatalf("the purged student keeps %d roster rows", left)
	}
	if kept := h.countRows(t, "SELECT count(lesson_id) FROM lesson_roster WHERE lesson_id = $1::uuid", lesson.ID); kept != 1 {
		t.Fatalf("the other member's roster row is gone: %d", kept)
	}
}

func TestEachDueAccountGetsItsShareOfAPurgeRun(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	const largeTutors, smallAccounts, lessonsEach = 11, 9, 300
	var large, small []domain.UserID
	for index := range largeTutors {
		id := byte(1 + index)
		tutor := h.seedUserAs(t, domain.RoleTutor, id, fmt.Sprintf("large-%d@example.org", id), accountPassword, currentHash)
		card := h.seedCard(t, tutor, id, domain.RelationshipInvited, nil)
		h.seedFutureLessons(t, tutor, card, lessonsEach)
		h.markDeleted(t, tutor, authStart.Add(-40*24*time.Hour+time.Duration(index)*time.Minute))
		large = append(large, tutor)
	}
	for index := range smallAccounts {
		student := h.seedStudentAccount(t, byte(100+index), domain.AccountActive)
		h.markDeleted(t, student, authStart.Add(-35*24*time.Hour+time.Duration(index)*time.Minute))
		small = append(small, student)
	}
	random := randomPerTest(t)
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(core.NewContextHandler(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	uploads := files.New(files.Dependencies{Store: h.store, Objects: h.objects, Clock: h.clock, Random: random, Keys: keys, Logger: logger})
	effects := tutoring.New(tutoring.Dependencies{Store: h.store, Clock: h.clock, Random: random, Logger: logger, PurgeBatchRows: 1})
	purger := accounts.NewPurger(h.store, uploads, effects, logger)

	purged, err := purger.PurgeDeleted(context.Background(), authStart)

	if err != nil || purged != smallAccounts {
		t.Fatalf("the run purged %d accounts: %v", purged, err)
	}
	for _, student := range small {
		if h.countRows(t, "SELECT count(id) FROM users WHERE id = $1::uuid", student.String()) != 0 {
			t.Fatalf("the small account %s waits behind the large purges", student)
		}
	}
	spent := int64(0)
	for _, tutor := range large {
		left := h.lessonRows(t, tutor)
		if left == lessonsEach || h.countRows(t, "SELECT count(id) FROM users WHERE id = $1::uuid", tutor.String()) != 1 {
			t.Fatalf("the large tutor %s got no share of the run, or finished: %d lessons left", tutor, left)
		}
		spent += lessonsEach - left
	}
	if spent != 100 || strings.Count(h.logs.String(), `"msg":"account.purge_paused"`) != largeTutors {
		t.Fatalf("the run spent %d batches on the large tutors", spent)
	}
}
