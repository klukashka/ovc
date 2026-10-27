package api_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func TestUnjoinedLessonsEndThirtyMinutesAfterTheirEndWithTheRosterFrozen(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	group := h.newGroup(t, token, "Evening club", h.activeCards(t, tutor, 11, 2))
	oneToOne := h.lessonAt(t, token, card, firstLessonDate, firstLessonLocal)
	groupLesson := h.scheduled(t, token, groupLessonFor(group.ID, firstLessonDate, "20:00")).Lessons[0].ID

	h.tickAt(t, firstLessonEnd.Add(29*time.Minute))
	beforeTheGrace := h.lessonState(t, oneToOne)
	h.tickAt(t, firstLessonEnd.Add(30*time.Minute))
	afterTheOneToOne := []string{h.lessonState(t, oneToOne), h.lessonState(t, groupLesson)}
	h.tickAt(t, firstLessonEnd.Add(2*time.Hour+30*time.Minute))

	if beforeTheGrace != "scheduled" || !slices.Equal(afterTheOneToOne, []string{"ended", "scheduled"}) || h.lessonState(t, groupLesson) != "ended" {
		t.Fatalf("the lessons were %s, then %v, then %s", beforeTheGrace, afterTheOneToOne, h.lessonState(t, groupLesson))
	}
	if !slices.Equal(h.rosterOf(t, oneToOne), []domain.UserID{{10}}) || !slices.Equal(h.rosterOf(t, groupLesson), []domain.UserID{{11}, {12}}) {
		t.Fatalf("the rosters are %v and %v", h.rosterOf(t, oneToOne), h.rosterOf(t, groupLesson))
	}
	if page := decodeLesson(t, h.lessonPage(t, h.studentToken(t, 10), oneToOne)); page.Status != "ended" {
		t.Fatalf("the student sees the ended lesson as %s", page.Status)
	}
	if h.countLogs(`"msg":"lesson.ended_unjoined"`, `"count":1`) != 2 || h.countLogs(`"job":"lessons.status"`, `"result":"ok"`) < 3 {
		t.Fatalf("the status job reported:\n%s", h.logs.String())
	}
}

func TestTheStatusJobLocksTheAudienceBeforeTheLesson(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	lesson := h.lessonAt(t, token, card, firstLessonDate, firstLessonLocal)
	h.at(t, firstLessonEnd.Add(30*time.Minute))
	fresh := accessToken(t, h.login(t, tutorEmail, accountPassword))
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM lessons WHERE id = $1::uuid FOR UPDATE", lesson)

	ticked := h.tickInBackground(t)
	h.waitForLockWaiters(t, 1)
	ending := h.sendInBackground(http.MethodPost, "/api/v1/students/"+card+"/end", "", jsonCall(fresh))
	h.waitForLockWaiters(t, 2)
	release()
	awaitTick(t, ticked)
	ended := <-ending

	if ended.err != nil || ended.result.status != http.StatusOK {
		t.Fatalf("the ending answered %v %d %s", ended.err, ended.result.status, ended.result.body)
	}
	if h.lessonState(t, lesson) != "ended" || !slices.Equal(h.rosterOf(t, lesson), []domain.UserID{{10}}) {
		t.Fatalf("the lesson is %s with the roster %v", h.lessonState(t, lesson), h.rosterOf(t, lesson))
	}
}

func TestTheStatusJobHandlesEachLessonInItsOwnTransaction(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	refused := h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), firstLessonDate, firstLessonLocal)
	other := h.lessonAt(t, token, h.linkedCard(t, tutor, 11, domain.RelationshipActive), firstLessonDate, "18:05")
	h.refuseEndingOf(t, refused)

	h.tickAt(t, firstLessonEnd.Add(40*time.Minute))

	if h.lessonState(t, refused) != "scheduled" || h.lessonState(t, other) != "ended" || !slices.Equal(h.rosterOf(t, other), []domain.UserID{{11}}) {
		t.Fatalf("the lessons are %s and %s", h.lessonState(t, refused), h.lessonState(t, other))
	}
	if len(h.rosterOf(t, refused)) != 0 {
		t.Fatalf("the refused lesson kept the roster %v", h.rosterOf(t, refused))
	}
	if h.countLogs(`"msg":"lesson.end_failed"`, refused) != 1 || h.countLogs(`"job":"lessons.status"`, `"result":"failed"`) != 1 {
		t.Fatalf("the failure was reported as:\n%s", h.logs.String())
	}
}

func (h *authHarness) refuseEndingOf(t *testing.T, lesson string) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), fmt.Sprintf(`
CREATE FUNCTION refuse_lesson_ending() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.id = '%s'::uuid AND NEW.status = 'ended' THEN
        RAISE EXCEPTION 'the test refuses to end this lesson';
    END IF;
    RETURN NEW;
END
$$`, lesson))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.database.Pool.Exec(context.Background(),
		"CREATE TRIGGER refuse_lesson_ending BEFORE UPDATE ON lessons FOR EACH ROW EXECUTE FUNCTION refuse_lesson_ending()"); err != nil {
		t.Fatal(err)
	}
}

const mondaysUntilDecember = "2030-12-30"

func (h *authHarness) mondaySeries(t *testing.T, token string, card string) string {
	t.Helper()
	request := oneOffFor(card, firstLessonDate, firstLessonLocal).until([]int{1}, mondaysUntilDecember)
	request.AllowOverlap = true
	created := h.scheduled(t, token, request)
	return created.Series.ID
}

func (h *authHarness) seriesLessons(t *testing.T, series string) []string {
	t.Helper()
	rows, err := h.database.Pool.Query(context.Background(),
		"SELECT id::text || '@' || occurrence_date::text FROM lessons WHERE series_id = $1::uuid ORDER BY occurrence_date", series)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var lessons []string
	for rows.Next() {
		var lesson string
		if err := rows.Scan(&lesson); err != nil {
			t.Fatal(err)
		}
		lessons = append(lessons, lesson)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return lessons
}

func (h *authHarness) materialisedUntil(t *testing.T, series string) string {
	t.Helper()
	var until string
	err := h.database.Pool.QueryRow(context.Background(), "SELECT materialised_until::text FROM lesson_series WHERE id = $1::uuid", series).Scan(&until)
	if err != nil {
		t.Fatal(err)
	}
	return until
}

func TestHorizonExtensionAddsOnlyMissingKeysAndSkipsEndedOrInactiveSeries(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	extended := h.mondaySeries(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive))
	endedCard := h.linkedCard(t, tutor, 11, domain.RelationshipActive)
	ofAnEndedRelationship := h.mondaySeries(t, token, endedCard)
	markedEnded := h.mondaySeries(t, token, h.linkedCard(t, tutor, 12, domain.RelationshipActive))
	h.exec(t, "UPDATE lesson_series SET materialised_until = '2030-05-26' WHERE id = $1::uuid", extended)
	h.exec(t, "UPDATE lesson_series SET ended_at = updated_at WHERE id = $1::uuid", markedEnded)
	if ended := h.endAsTutor(t, token, endedCard); ended.status != http.StatusOK {
		t.Fatalf("ending answered %d %s", ended.status, ended.body)
	}
	before := h.seriesLessons(t, extended)

	h.tickAt(t, authStart.Add(7*24*time.Hour))

	after := h.seriesLessons(t, extended)
	if len(before) != 12 || len(after) != 13 || !slices.Equal(after[:12], before) || !strings.HasSuffix(after[12], "@2030-06-03") {
		t.Fatalf("the series went from %v to %v", before, after)
	}
	if h.materialisedUntil(t, extended) != "2030-06-09" {
		t.Fatalf("the series is materialised until %s", h.materialisedUntil(t, extended))
	}
	for _, skipped := range []string{ofAnEndedRelationship, markedEnded} {
		if len(h.seriesLessons(t, skipped)) != 12 || h.materialisedUntil(t, skipped) != "2030-06-02" {
			t.Fatalf("a skipped series has %d lessons until %s", len(h.seriesLessons(t, skipped)), h.materialisedUntil(t, skipped))
		}
	}
	if h.countLogs(`"msg":"series.extended"`, `"count":1`, `"lessons":1`) != 1 {
		t.Fatalf("the extension reported:\n%s", h.logs.String())
	}
}

func TestHorizonExtensionAndASplitNeverRace(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	series := h.mondaySeries(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive))
	h.at(t, authStart.Add(24*time.Hour))
	release := h.holdRowThen(t, "SELECT "+lockedRowPosition+" FROM lesson_series WHERE id = $1::uuid FOR UPDATE", series,
		"UPDATE lesson_series SET end_date = '2030-05-27' WHERE id = $1::uuid")

	ticked := h.tickInBackground(t)
	h.waitForLockWaiters(t, 1)
	release()
	awaitTick(t, ticked)

	lessons := h.seriesLessons(t, series)
	if len(lessons) != 12 || !strings.HasSuffix(lessons[11], "@2030-05-27") || h.materialisedUntil(t, series) != "2030-06-02" {
		t.Fatalf("after the concurrent end the series has %v until %s", lessons, h.materialisedUntil(t, series))
	}
}

func (h *authHarness) copySeries(t *testing.T, series string, copies int, until string) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), `
INSERT INTO lesson_series (id, tutor_id, relationship_id, group_id, weekdays, local_start, duration_minutes, timezone, first_date, end_date,
    count, title, description, materialised_until, created_at, updated_at)
SELECT gen_random_uuid(), tutor_id, relationship_id, group_id, weekdays, local_start, duration_minutes, timezone, first_date, end_date,
    count, title, description, $3::date, created_at, updated_at
FROM lesson_series CROSS JOIN generate_series(1, $2::int)
WHERE id = $1::uuid`, series, copies, until)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHorizonExtensionSkipsSeriesOfAnInactiveTutor(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	blocked, blockedToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	other, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "second-tutor@example.org")
	blockedSeries := h.mondaySeries(t, blockedToken, h.linkedCard(t, blocked, 10, domain.RelationshipActive))
	h.exec(t, "UPDATE lesson_series SET materialised_until = '2030-05-01' WHERE id = $1::uuid", blockedSeries)
	h.copySeries(t, blockedSeries, 201, "2030-05-01")
	otherSeries := h.mondaySeries(t, otherToken, h.linkedCard(t, other, 20, domain.RelationshipActive))
	if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}
	h.withJobSettings(oneExtensionBatchSettings())

	h.tickAt(t, authStart.Add(25*time.Hour))
	otherUntil, otherLessons := h.materialisedUntil(t, otherSeries), len(h.seriesLessons(t, otherSeries))
	duringTheBlock := h.countRows(t, "SELECT count(id) FROM lessons WHERE tutor_id = $1::uuid", blocked.String())
	if _, err := h.operators().Unblock(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}
	h.withJobSettings(scheduleDefaults())
	unblockedAt := authStart.Add(120 * 24 * time.Hour)
	h.tickAt(t, unblockedAt)

	if otherUntil != "2030-06-03" || otherLessons != 13 {
		t.Fatalf("the other tutor's series has %d lessons until %s", otherLessons, otherUntil)
	}
	if duringTheBlock != 12 {
		t.Fatalf("the blocked tutor has %d lessons during the block", duringTheBlock)
	}
	stale := h.countRows(t, "SELECT count(id) FROM lesson_series WHERE tutor_id = $1::uuid AND materialised_until <> '2030-09-30'", blocked.String())
	past := h.countRows(t, "SELECT count(id) FROM lessons WHERE tutor_id = $1::uuid AND created_at > '2030-03-10T09:00:00Z' AND starts_at <= '"+
		rfc3339(unblockedAt)+"'", blocked.String())
	extended := h.countRows(t, "SELECT count(DISTINCT series_id) FROM lessons WHERE tutor_id = $1::uuid AND starts_at > '"+rfc3339(unblockedAt)+"'",
		blocked.String())
	if stale != 0 || past != 0 || extended != 202 {
		t.Fatalf("after the unblock %d series are behind, %d past lessons were created and %d series were extended", stale, past, extended)
	}
}
