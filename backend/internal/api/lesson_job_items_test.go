package api_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const groupRowLock = "SELECT " + lockedRowPosition + " FROM groups WHERE id = $1::uuid FOR NO KEY UPDATE"

type queuedHolder struct {
	row       contestedRow
	granted   chan error
	acquired  bool
	committed bool
	finish    func(statement string, key string) error
}

func (h *authHarness) queueRowHolder(t *testing.T, lockQuery string, key string) *queuedHolder {
	t.Helper()
	transaction, err := h.database.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	queued := &queuedHolder{granted: make(chan error, 1)}
	queued.finish = func(statement string, key string) error {
		queued.committed = true
		if _, err := transaction.Exec(context.Background(), statement, key); err != nil {
			return errors.Join(err, transaction.Rollback(context.Background()))
		}
		return transaction.Commit(context.Background())
	}
	t.Cleanup(func() {
		if queued.acquired && !queued.committed {
			if err := transaction.Rollback(context.Background()); err != nil {
				t.Error(err)
			}
		}
	})
	if err := transaction.QueryRow(context.Background(), "SELECT pg_backend_pid()").Scan(&queued.row.holder); err != nil {
		t.Fatal(err)
	}
	go func() {
		queued.granted <- transaction.QueryRow(context.Background(), lockQuery, key).Scan(&queued.row.relation, &queued.row.page, &queued.row.tuple)
	}()
	return queued
}

func (h *authHarness) awaitHolder(t *testing.T, queued *queuedHolder) {
	t.Helper()
	select {
	case err := <-queued.granted:
		queued.acquired = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(pauseDeadline):
		t.Fatal("the queued holder never got its row")
	}
	h.contested = &queued.row
}

func (h *authHarness) commitHolderAfter(t *testing.T, queued *queuedHolder, statement string, key string) {
	t.Helper()
	if err := queued.finish(statement, key); err != nil {
		t.Fatal(err)
	}
}

type groupLessonParties struct {
	tutor  domain.UserID
	group  string
	cards  []string
	lesson string
}

func (h *authHarness) dueGroupLesson(t *testing.T) groupLessonParties {
	t.Helper()
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 11, 2)
	group := h.newGroup(t, token, "Evening club", cards)
	lesson := h.scheduled(t, token, groupLessonFor(group.ID, firstLessonDate, "20:00")).Lessons[0].ID
	h.at(t, firstLessonEnd.Add(2*time.Hour+30*time.Minute))
	return groupLessonParties{tutor: tutor, group: group.ID, cards: cards, lesson: lesson}
}

func removeMember(card string) string {
	return fmt.Sprintf("DELETE FROM group_members WHERE group_id = $1::uuid AND relationship_id = '%s'::uuid", card)
}

func addMember(card string, tutor domain.UserID) string {
	return fmt.Sprintf("INSERT INTO group_members (group_id, relationship_id, tutor_id, added_at) VALUES ($1::uuid, '%s'::uuid, '%s'::uuid, now())",
		card, tutor.String())
}

func TestTheStatusJobRetriesALessonOnceWhenItsGroupChangesUnderTheLocks(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.dueGroupLesson(t)
	release := h.holdRowThen(t, groupRowLock, parties.group, removeMember(parties.cards[1]))

	ticked := h.tickInBackground(t)
	h.waitForLockWaiters(t, 1)
	release()
	awaitTick(t, ticked)

	if h.lessonState(t, parties.lesson) != "ended" || !slices.Equal(h.rosterOf(t, parties.lesson), []domain.UserID{{11}}) {
		t.Fatalf("the lesson is %s with the roster %v", h.lessonState(t, parties.lesson), h.rosterOf(t, parties.lesson))
	}
	if h.countLogs(`"msg":"lesson.job_deferred"`) != 0 || h.countLogs(`"job":"lessons.status"`, `"result":"ok"`) != 1 {
		t.Fatalf("the status job reported:\n%s", h.logs.String())
	}
}

func TestTheStatusJobLeavesALessonToTheNextRunWhenItsGroupChangesTwice(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.dueGroupLesson(t)
	release := h.holdRowThen(t, groupRowLock, parties.group, removeMember(parties.cards[1]))
	ticked := h.tickInBackground(t)
	h.waitForLockWaiters(t, 1)
	second := h.queueRowHolder(t, groupRowLock, parties.group)
	h.waitForLockWaiters(t, 2)

	release()
	h.awaitHolder(t, second)
	h.waitForLockWaiters(t, 1)
	h.commitHolderAfter(t, second, addMember(parties.cards[1], parties.tutor), parties.group)
	awaitTick(t, ticked)
	afterTheDeferringRun := h.lessonState(t, parties.lesson)
	h.tickAt(t, firstLessonEnd.Add(2*time.Hour+31*time.Minute))

	if afterTheDeferringRun != "scheduled" {
		t.Fatalf("the lesson was %s after the run that saw its group change twice", afterTheDeferringRun)
	}
	if h.countLogs(`"level":"DEBUG"`, `"msg":"lesson.job_deferred"`, `"job":"lessons.status"`, parties.lesson) != 1 {
		t.Fatalf("the deferral was reported as:\n%s", h.logs.String())
	}
	if h.lessonState(t, parties.lesson) != "ended" || !slices.Equal(h.rosterOf(t, parties.lesson), []domain.UserID{{11}, {12}}) {
		t.Fatalf("the next run left the lesson %s with the roster %v", h.lessonState(t, parties.lesson), h.rosterOf(t, parties.lesson))
	}
	if h.countLogs(`"job":"lessons.status"`, `"result":"failed"`) != 0 {
		t.Fatalf("a deferral failed the job:\n%s", h.logs.String())
	}
}

func TestTheStatusJobPagesPastAFailingLessonAndRetriesItEachRun(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	refused := h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), firstLessonDate, firstLessonLocal)
	later := []string{
		h.lessonAt(t, token, h.linkedCard(t, tutor, 11, domain.RelationshipActive), firstLessonDate, "18:05"),
		h.lessonAt(t, token, h.linkedCard(t, tutor, 12, domain.RelationshipActive), firstLessonDate, "18:10"),
	}
	h.refuseEndingOf(t, refused)
	settings := scheduleDefaults()
	settings.StatusBatch = 1
	h.withJobSettings(settings)

	h.tickAt(t, firstLessonEnd.Add(40*time.Minute))
	afterTheFirstRun := []string{h.lessonState(t, refused), h.lessonState(t, later[0]), h.lessonState(t, later[1])}
	h.tickAt(t, firstLessonEnd.Add(41*time.Minute))

	if !slices.Equal(afterTheFirstRun, []string{"scheduled", "ended", "ended"}) || h.lessonState(t, refused) != "scheduled" {
		t.Fatalf("the lessons were %v after the first run and the refused one is %s", afterTheFirstRun, h.lessonState(t, refused))
	}
	if h.countLogs(`"level":"ERROR"`, `"msg":"lesson.end_failed"`, refused) != 2 || h.countLogs(`"job":"lessons.status"`, `"result":"failed"`) != 2 {
		t.Fatalf("the failing lesson was reported as:\n%s", h.logs.String())
	}
	if h.countLogs(`"msg":"lesson.ended_unjoined"`, `"count":2`) != 1 {
		t.Fatalf("the first run reported:\n%s", h.logs.String())
	}
}

func TestAJobSkipsALessonWhoseSeriesIsPurgedUnderItsLocks(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		instant time.Time
		job     domain.JobName
		failure string
	}{
		"the status job":   {instant: firstLessonEnd.Add(30 * time.Minute), job: domain.JobLessonStatus, failure: "lesson.end_failed"},
		"the reminder job": {instant: hourBeforeFirst, job: domain.JobLessonReminders, failure: "lesson.reminder_failed"},
	}
	for name, purged := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			series := h.mondaySeries(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive))
			h.at(t, purged.instant)
			release := h.holdRowThenEach(t, "SELECT "+lockedRowPosition+" FROM lesson_series WHERE id = $1::uuid FOR UPDATE", series,
				[]string{"DELETE FROM lessons WHERE series_id = $1::uuid", "DELETE FROM lesson_series WHERE id = $1::uuid"})

			ticked := h.tickInBackground(t)
			h.waitForLockWaiters(t, 1)
			release()
			awaitTick(t, ticked)

			if h.countLogs(`"msg":"`+purged.failure+`"`) != 0 || h.countLogs(`"level":"ERROR"`) != 0 {
				t.Fatalf("the purge was reported as a failure:\n%s", h.logs.String())
			}
			if h.countLogs(`"job":"`+string(purged.job)+`"`, `"result":"ok"`) != 1 {
				t.Fatalf("the job reported:\n%s", h.logs.String())
			}
		})
	}
}

func TestTheReminderJobGoesOnPastALessonWhoseTransactionFails(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	refused := h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), firstLessonDate, firstLessonLocal)
	other := h.lessonAt(t, token, h.linkedCard(t, tutor, 11, domain.RelationshipActive), firstLessonDate, "17:55")
	h.raiseOnUpdate(t, "lessons", refused, "NEW.reminded_1h_revision IS DISTINCT FROM OLD.reminded_1h_revision")

	h.tickAt(t, hourBeforeFirst)

	reminded := "SELECT count(id) FROM lessons WHERE id = $1::uuid AND reminded_1h_revision = 1"
	if h.countRows(t, reminded, refused) != 0 || h.countRows(t, reminded, other) != 1 {
		t.Fatalf("the refused lesson was reminded %d times and the other %d", h.countRows(t, reminded, refused), h.countRows(t, reminded, other))
	}
	if queued := h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.EventLessonReminder1h)); queued != 2 {
		t.Fatalf("%d one-hour reminders were queued", queued)
	}
	if h.countLogs(`"level":"ERROR"`, `"msg":"lesson.reminder_failed"`, refused) != 1 || h.countLogs(`"job":"lessons.reminders"`, `"result":"failed"`) != 1 {
		t.Fatalf("the failure was reported as:\n%s", h.logs.String())
	}
}

func TestHorizonExtensionGoesOnPastASeriesWhoseTransactionFails(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	refused := h.mondaySeries(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive))
	other := h.mondaySeries(t, token, h.linkedCard(t, tutor, 11, domain.RelationshipActive))
	for _, series := range []string{refused, other} {
		h.exec(t, "UPDATE lesson_series SET materialised_until = '2030-05-26' WHERE id = $1::uuid", series)
	}
	h.raiseOnUpdate(t, "lesson_series", refused, "NEW.materialised_until IS DISTINCT FROM OLD.materialised_until")

	h.tickAt(t, authStart.Add(7*24*time.Hour))

	if len(h.seriesLessons(t, other)) != 13 || h.materialisedUntil(t, other) != "2030-06-09" {
		t.Fatalf("the other series has %d lessons until %s", len(h.seriesLessons(t, other)), h.materialisedUntil(t, other))
	}
	if len(h.seriesLessons(t, refused)) != 12 || h.materialisedUntil(t, refused) != "2030-05-26" {
		t.Fatalf("the refused series has %d lessons until %s", len(h.seriesLessons(t, refused)), h.materialisedUntil(t, refused))
	}
	if h.countLogs(`"level":"ERROR"`, `"msg":"series.extension_failed"`, refused) != 1 || h.countLogs(`"job":"series.extend"`, `"result":"failed"`) != 1 {
		t.Fatalf("the failure was reported as:\n%s", h.logs.String())
	}
}

func TestHorizonExtensionWaitsUntilASeriesHasAnOccurrenceWithinTheHorizon(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	series := h.mondaySeries(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive))

	h.tickAt(t, authStart.Add(24*time.Hour))
	afterTheMondayEntered := h.materialisedUntil(t, series)
	h.tickAt(t, authStart.Add(2*24*time.Hour))
	afterADayWithoutAnOccurrence := h.materialisedUntil(t, series)
	h.tickAt(t, authStart.Add(8*24*time.Hour))

	if afterTheMondayEntered != "2030-06-03" || afterADayWithoutAnOccurrence != "2030-06-03" {
		t.Fatalf("the series was materialised until %s, then %s", afterTheMondayEntered, afterADayWithoutAnOccurrence)
	}
	if lessons := h.seriesLessons(t, series); len(lessons) != 14 || h.materialisedUntil(t, series) != "2030-06-10" {
		t.Fatalf("after the next Monday entered the series has %d lessons until %s", len(lessons), h.materialisedUntil(t, series))
	}
	if h.countLogs(`"msg":"series.extended"`, `"count":0`) != 1 || h.countLogs(`"msg":"series.extended"`, `"count":1`, `"lessons":1`) != 2 {
		t.Fatalf("the extension reported:\n%s", h.logs.String())
	}
}

func (h *authHarness) createdSeries(t *testing.T, token string, request lessonRequest) string {
	t.Helper()
	request.AllowOverlap = true
	return h.scheduled(t, token, request).Series.ID
}

func lessonDates(lessons []string) []string {
	dates := make([]string, 0, len(lessons))
	for _, lesson := range lessons {
		_, date, _ := strings.Cut(lesson, "@")
		dates = append(dates, date)
	}
	return dates
}

func TestHorizonExtensionLeavesASeriesWhoseEndComesBeforeItsNextWeekday(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	series := h.createdSeries(t, token, oneOffFor(card, firstLessonDate, firstLessonLocal).until([]int{1}, "2030-06-08"))

	h.tickAt(t, authStart.Add(24*time.Hour))
	h.tickAt(t, authStart.Add(2*24*time.Hour))
	h.tickAt(t, authStart.Add(7*24*time.Hour))

	dates := lessonDates(h.seriesLessons(t, series))
	if len(dates) != 13 || dates[12] != "2030-06-03" || h.materialisedUntil(t, series) != "2030-06-03" {
		t.Fatalf("the series has %v until %s", dates, h.materialisedUntil(t, series))
	}
	if h.countLogs(`"msg":"series.extended"`, `"count":0`) != 2 {
		t.Fatalf("the extension reported:\n%s", h.logs.String())
	}
}

func TestHorizonExtensionWaitsForAFirstDateBeyondTheHorizon(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	series := h.createdSeries(t, token, oneOffFor(card, "2030-06-17", firstLessonLocal).until([]int{1}, mondaysUntilDecember))
	atCreation := h.materialisedUntil(t, series)

	h.tickAt(t, authStart.Add(7*24*time.Hour))
	beforeTheFirstDate := append([]string{h.materialisedUntil(t, series)}, lessonDates(h.seriesLessons(t, series))...)
	h.tickAt(t, authStart.Add(15*24*time.Hour))

	if !slices.Equal(beforeTheFirstDate, []string{atCreation}) {
		t.Fatalf("a week later the series was %v, materialised at creation until %s", beforeTheFirstDate, atCreation)
	}
	dates := lessonDates(h.seriesLessons(t, series))
	if !slices.Equal(dates, []string{"2030-06-17"}) || h.materialisedUntil(t, series) != "2030-06-17" {
		t.Fatalf("once the first date entered the horizon the series has %v until %s", dates, h.materialisedUntil(t, series))
	}
}

func TestHorizonExtensionTakesASeriesOnEachOfItsWeekdays(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	series := h.createdSeries(t, token, oneOffFor(card, firstLessonDate, firstLessonLocal).until([]int{1, 4}, mondaysUntilDecember))

	var until []string
	for _, days := range []int{1, 2, 4} {
		h.tickAt(t, authStart.Add(time.Duration(days)*24*time.Hour))
		until = append(until, h.materialisedUntil(t, series))
	}

	if !slices.Equal(until, []string{"2030-06-03", "2030-06-03", "2030-06-06"}) {
		t.Fatalf("the series was materialised until %v", until)
	}
	if dates := lessonDates(h.seriesLessons(t, series)); !slices.Equal(dates[len(dates)-3:], []string{"2030-05-30", "2030-06-03", "2030-06-06"}) {
		t.Fatalf("the series ends with %v", dates[len(dates)-3:])
	}
}

func TestHorizonExtensionTakesASplitSeriesFromItsInheritedHorizon(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	original := h.mondaySeries(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive))
	second, _, _ := strings.Cut(h.seriesLessons(t, original)[1], "@")
	split := decodeChanged(t, h.editLesson(t, token, second, following(1).startingAt("19:00"))).Series.ID
	inherited := h.materialisedUntil(t, split)

	h.tickAt(t, authStart.Add(24*time.Hour))

	if inherited != "2030-06-02" || h.materialisedUntil(t, split) != "2030-06-03" || h.materialisedUntil(t, original) != "2030-06-02" {
		t.Fatalf("the split series went from %s to %s and the original is until %s", inherited, h.materialisedUntil(t, split), h.materialisedUntil(t, original))
	}
	if dates := lessonDates(h.seriesLessons(t, split)); dates[len(dates)-1] != "2030-06-03" || len(h.seriesLessons(t, original)) != 1 {
		t.Fatalf("the split series ends with %v and the original keeps %d lessons", dates, len(h.seriesLessons(t, original)))
	}
}
