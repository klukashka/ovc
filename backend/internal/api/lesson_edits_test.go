package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	mondaySeriesStart = "2030-03-11"
	newLessonTitle    = "Grammar workshop"
)

func (h *authHarness) mondays(t *testing.T, token string, card string, count int) createdLessons {
	t.Helper()
	request := oneOffFor(card, mondaySeriesStart, "18:00").weekly([]int{1}, count)
	request.AllowOverlap = true
	return h.scheduled(t, token, request)
}

func (h *authHarness) groupMondays(t *testing.T, token string, group string, count int) createdLessons {
	t.Helper()
	request := groupLessonFor(group, mondaySeriesStart, "20:00").weekly([]int{1}, count)
	request.AllowOverlap = true
	return h.scheduled(t, token, request)
}

func TestEditingThisLessonRecordsOverriddenFields(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	series := h.mondays(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), 3)
	moved := series.Lessons[1]

	edited := decodeChanged(t, h.editLesson(t, token, moved.ID, thisLesson(1).titled(newLessonTitle).startingAt("19:00")))
	h.editLesson(t, token, series.Lessons[0].ID, following(1).titled("Series title"))

	if edited.Series != nil || len(edited.Lessons) != 1 || edited.Lessons[0].Title != newLessonTitle || edited.Lessons[0].Version != 2 ||
		!edited.Lessons[0].StartsAt.Equal(time.Date(2030, time.March, 18, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("the edit answered %+v", edited)
	}
	stored := h.storedLesson(t, moved.ID)
	if !slices.Equal(stored.Overridden, []string{"start", "title"}) || stored.Revision != 2 || stored.Title != newLessonTitle {
		t.Fatalf("the edited lesson is stored as %+v", stored)
	}
	if last := h.storedLesson(t, series.Lessons[2].ID); last.Title != "Series title" || len(last.Overridden) != 0 {
		t.Fatalf("the following occurrence is stored as %+v", last)
	}
}

func TestEditingFollowingSplitsTheSeriesInPlace(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	series := h.mondays(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), 6)
	lessons := series.Lessons
	h.editLesson(t, token, lessons[4].ID, thisLesson(1).titled("A title of its own"))
	h.setStatus(t, lessons[2].ID, "live")
	h.cancelWithRoster(t, lessons[3].ID, domain.UserID{10})

	split := decodeChanged(t, h.editLesson(t, token, lessons[1].ID, following(1).titled(newLessonTitle).startingAt("19:00")))

	if split.Series == nil || split.Series.ID == series.Series.ID || split.Series.FirstDate != "2030-03-18" || split.Series.Count == nil ||
		*split.Series.Count != 5 || split.Series.Start != "19:00" {
		t.Fatalf("the split answered the series %+v", split.Series)
	}
	if want := []string{lessons[1].ID, lessons[2].ID, lessons[3].ID, lessons[4].ID, lessons[5].ID}; !slices.Equal(lessonIDs(split.Lessons), want) {
		t.Fatalf("the split answered the lessons %v, want %v", lessonIDs(split.Lessons), want)
	}
	original := h.storedSeries(t, series.Series.ID)
	if original.EndDate == nil || *original.EndDate != "2030-03-17" || original.Count != nil || original.Ended {
		t.Fatalf("the original series is %+v", original)
	}
	if created := h.storedSeries(t, split.Series.ID); created.SplitFrom == nil || *created.SplitFrom != series.Series.ID ||
		created.Title != newLessonTitle || created.Until != original.Until {
		t.Fatalf("the new series is %+v", created)
	}
	first := h.storedLesson(t, lessons[0].ID)
	selected, live, cancelled := h.storedLesson(t, lessons[1].ID), h.storedLesson(t, lessons[2].ID), h.storedLesson(t, lessons[3].ID)
	overridden, plain := h.storedLesson(t, lessons[4].ID), h.storedLesson(t, lessons[5].ID)
	if *first.SeriesID != series.Series.ID || first.Title != lessonTitle || *selected.SeriesID != split.Series.ID {
		t.Fatalf("the rows are %+v and %+v", first, selected)
	}
	if selected.Title != newLessonTitle || selected.StartsAt.UTC().Hour() != 16 || plain.Title != newLessonTitle || plain.StartsAt.UTC().Hour() != 16 {
		t.Fatalf("the edited rows are %+v and %+v", selected, plain)
	}
	if overridden.Title != "A title of its own" || overridden.StartsAt.UTC().Hour() != 16 {
		t.Fatalf("the overridden row is %+v", overridden)
	}
	if live.Title != lessonTitle || live.StartsAt.UTC().Hour() != 15 || live.Version != 1 || cancelled.Title != lessonTitle || cancelled.Version != 1 ||
		*live.SeriesID != split.Series.ID || *cancelled.SeriesID != split.Series.ID {
		t.Fatalf("the live and cancelled rows are %+v and %+v", live, cancelled)
	}
	for _, viewer := range []string{token, h.studentToken(t, 10)} {
		page := decodeLesson(t, h.lessonPage(t, viewer, lessons[5].ID))
		if page.Series == nil || page.Series.ID != split.Series.ID || page.Series.Start != "19:00" || *page.SeriesID != split.Series.ID {
			t.Fatalf("the lesson page shows the series %+v", page.Series)
		}
	}
	if h.countLogs(`"msg":"series.split"`, series.Series.ID, split.Series.ID) != 1 {
		t.Fatalf("the split was reported as:\n%s", h.logs.String())
	}
}

func TestSplittingAtTheFirstOccurrenceEndsTheOriginalSeries(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	series := h.mondays(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), 3)

	split := decodeChanged(t, h.editLesson(t, token, series.Lessons[0].ID, following(1).lasting(90)))

	original := h.storedSeries(t, series.Series.ID)
	if !original.Ended || original.EndDate == nil || *original.EndDate != "2030-03-10" || original.Count != nil {
		t.Fatalf("the original series is %+v", original)
	}
	if split.Series == nil || split.Series.Duration != 90 || *split.Series.Count != 3 || split.Series.FirstDate != mondaySeriesStart {
		t.Fatalf("the new series is %+v", split.Series)
	}
	for _, lesson := range series.Lessons {
		if stored := h.storedLesson(t, lesson.ID); *stored.SeriesID != split.Series.ID || stored.Duration != 90 || stored.Revision != 2 {
			t.Fatalf("a lesson is stored as %+v", stored)
		}
	}
	if open := h.countRows(t, "SELECT count(id) FROM lesson_series WHERE tutor_id = $1::uuid AND ended_at IS NULL", tutor.String()); open != 1 {
		t.Fatalf("%d series are open after the split", open)
	}
}

func TestConcurrentEditsConflictAndAJoinlessStartMakesItNotEditable(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	lesson := h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), firstLessonDate, firstLessonLocal)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM lessons WHERE id = $1::uuid FOR UPDATE", lesson)
	first := h.sendInBackground(http.MethodPatch, editPath(lesson), thisLesson(1).titled("First title").body(t), jsonCall(token))
	second := h.sendInBackground(http.MethodPatch, editPath(lesson), thisLesson(1).titled("Second title").body(t), jsonCall(token))
	h.waitForLockWaiters(t, 2)

	release()
	answers := []response{awaitAnswer(t, first), awaitAnswer(t, second)}
	h.at(t, firstLessonStart.Add(time.Minute))
	fresh := accessToken(t, h.login(t, tutorEmail, accountPassword))
	started := h.editLesson(t, fresh, lesson, thisLesson(2).titled("Too late"))
	cancelled := h.cancelCall(t, fresh, lesson, "this", 2)

	statuses := []int{answers[0].status, answers[1].status}
	slices.Sort(statuses)
	if !slices.Equal(statuses, []int{http.StatusOK, http.StatusConflict}) {
		t.Fatalf("the concurrent edits answered %d %s and %d %s", answers[0].status, answers[0].body, answers[1].status, answers[1].body)
	}
	for _, answer := range answers {
		if answer.status == http.StatusConflict && code(t, answer) != "conflict" {
			t.Fatalf("the losing edit answered %s", answer.body)
		}
	}
	if stored := h.storedLesson(t, lesson); stored.Version != 2 || (stored.Title != "First title" && stored.Title != "Second title") {
		t.Fatalf("the lesson is stored as %+v", stored)
	}
	for name, refused := range map[string]response{"an edit": started, "a cancellation": cancelled} {
		if refused.status != http.StatusConflict || code(t, refused) != "not_editable" {
			t.Errorf("%s after the start answered %d %s", name, refused.status, refused.body)
		}
	}
}

func TestADateForFollowingIsNotAllowed(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	series := h.mondays(t, token, card, 3)
	oneOff := h.lessonAt(t, token, card, "2030-03-12", "18:00")

	dated := h.editLesson(t, token, series.Lessons[1].ID, following(1).on("2030-03-19"))
	withoutSeries := h.editLesson(t, token, oneOff, following(1).titled(newLessonTitle))
	cancelWithoutSeries := h.cancelCall(t, token, oneOff, "following", 1)
	empty := h.editLesson(t, token, oneOff, thisLesson(1))

	cases := map[string]struct {
		result response
		field  string
	}{
		"a date for following":                {dated, "body.date:not_allowed"},
		"following without a series":          {withoutSeries, "body.scope:not_allowed"},
		"cancelling following without series": {cancelWithoutSeries, "body.scope:not_allowed"},
		"an edit without a field":             {empty, "body:required"},
	}
	for name, testCase := range cases {
		if testCase.result.status != http.StatusUnprocessableEntity || !slices.Equal(fields(t, testCase.result), []string{testCase.field}) {
			t.Errorf("%s answered %d %s", name, testCase.result.status, testCase.result.body)
		}
	}
	for _, lesson := range append(lessonIDs(series.Lessons), oneOff) {
		if stored := h.storedLesson(t, lesson); stored.Version != 1 || stored.Status != "scheduled" {
			t.Fatalf("a refused write changed %+v", stored)
		}
	}
}

func TestMovingSendsLessonMovedAndOtherEditsSendNothing(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	group := h.newGroup(t, token, groupName, h.activeCards(t, tutor, 11, 2))
	lesson := h.scheduled(t, token, groupLessonFor(group.ID, "2030-03-12", "18:00")).Lessons[0].ID
	frames := h.recordFrames(t)

	h.editLesson(t, token, lesson, thisLesson(1).titled(newLessonTitle))
	retitled := h.outboxKeysOf(t, domain.EventLessonMoved)
	sent := frames.settle(t, h)
	h.editLesson(t, token, lesson, thisLesson(2).startingAt("19:30"))
	h.tick(t)

	if retitled != 0 {
		t.Fatalf("a title edit queued %d lesson.moved emails", retitled)
	}
	for _, recipient := range []domain.UserID{tutor, {11}, {12}} {
		if !hasFrame(sent, recipient, domain.EntityLesson, lesson) {
			t.Errorf("no lesson frame for %s after the title edit", recipient)
		}
	}
	for _, member := range []byte{11, 12} {
		moved := h.changeEmailsTo(t, memberEmail(member), "Lesson moved: "+newLessonTitle)
		if len(moved) != 1 || !strings.Contains(moved[0].Text, "Tuesday, 12 March 2030, 19:30") {
			t.Fatalf("member %d got %d lesson.moved emails: %+v", member, len(moved), moved)
		}
		rows := h.notificationsOf(t, h.studentToken(t, member))
		if len(rows) != 1 || rows[0].Event != "lesson.moved" || rows[0].Link == nil || rows[0].Link.ID != lesson {
			t.Fatalf("member %d has the rows %+v", member, rows)
		}
	}
	if len(h.changeEmailsTo(t, tutorEmail, "Lesson moved")) != 0 || h.outboxOf(t, domain.EventLessonMoved, "done:sent") != 2 {
		t.Fatal("the lesson.moved emails did not reach exactly the two members")
	}
}

func TestCancellingThisLessonKeepsTheRowWithItsCauseAndDate(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	lesson := h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), "2030-03-12", "18:00")

	cancelled := decodeChanged(t, h.cancelCall(t, token, lesson, "this", 1))
	h.tick(t)

	if cancelled.Series != nil || len(cancelled.Lessons) != 1 || cancelled.Lessons[0].Status != "cancelled" || cancelled.Lessons[0].CancelledAt == nil ||
		!cancelled.Lessons[0].CancelledAt.Equal(authStart) {
		t.Fatalf("the cancellation answered %+v", cancelled)
	}
	stored := h.storedLesson(t, lesson)
	if stored.Status != "cancelled" || stored.Cause == nil || *stored.Cause != "tutor" || !stored.Cancelled.Equal(authStart) || stored.Version != 2 {
		t.Fatalf("the lesson is stored as %+v", stored)
	}
	if !slices.Equal(h.rosterOf(t, lesson), []domain.UserID{{10}}) {
		t.Fatalf("the roster is %v", h.rosterOf(t, lesson))
	}
	if page := decodeLesson(t, h.lessonPage(t, h.studentToken(t, 10), lesson)); page.Status != "cancelled" || page.CancelledAt == nil {
		t.Fatalf("the student sees %+v", page)
	}
	if emails := h.changeEmailsTo(t, memberEmail(10), "Lesson cancelled: "+lessonTitle); len(emails) != 1 {
		t.Fatalf("the student got %d cancellation emails", len(emails))
	}
	if rows := h.notificationsOf(t, h.studentToken(t, 10)); len(rows) != 1 || rows[0].Event != "lesson.cancelled" {
		t.Fatalf("the student has the rows %+v", rows)
	}
	if !slices.Contains(calendarIDs(h.calendarOf(t, token, calendarFrom, calendarTo)), lesson) {
		t.Fatal("the cancelled lesson left the tutor's calendar")
	}
}

func TestCancellingFollowingEndsTheSeriesBeforeIt(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	series := h.mondaySeries(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive))
	lessons := h.seriesLessons(t, series)
	selected := strings.Split(lessons[3], "@")[0]

	cancelled := decodeChanged(t, h.cancelCall(t, token, selected, "following", 1))
	h.tick(t)

	if cancelled.Series == nil || cancelled.Series.ID != series || cancelled.Series.EndDate == nil || *cancelled.Series.EndDate != "2030-03-31" {
		t.Fatalf("the cancellation answered the series %+v", cancelled.Series)
	}
	if len(cancelled.Lessons) != len(lessons)-3 || cancelled.Lessons[0].ID != selected {
		t.Fatalf("the cancellation answered %d lessons starting with %s", len(cancelled.Lessons), cancelled.Lessons[0].ID)
	}
	for index, entry := range lessons {
		want := "scheduled"
		if index >= 3 {
			want = "cancelled"
		}
		if status := h.storedLesson(t, strings.Split(entry, "@")[0]).Status; status != want {
			t.Fatalf("occurrence %s is %s, want %s", entry, status, want)
		}
	}
	if after := h.seriesLessons(t, series); len(after) != len(lessons) {
		t.Fatalf("the ended series gained occurrences: %d instead of %d", len(after), len(lessons))
	}
	if stored := h.storedSeries(t, series); stored.Ended || *stored.EndDate != "2030-03-31" {
		t.Fatalf("the series is %+v", stored)
	}
}

func TestSeriesOperationsSendOneAggregatedEmailPerRecipient(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	group := h.newGroup(t, token, groupName, h.activeCards(t, tutor, 11, 2))
	series := h.groupMondays(t, token, group.ID, 4)
	long := h.mondays(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), 25)

	h.editLesson(t, token, series.Lessons[1].ID, following(1).startingAt("19:00"))
	h.tick(t)
	h.at(t, authStart.Add(time.Minute))
	h.cancelCall(t, token, series.Lessons[2].ID, "following", 2)
	h.cancelCall(t, token, long.Lessons[0].ID, "following", 1)
	h.tick(t)

	for _, member := range []byte{11, 12} {
		changed := h.changeEmailsTo(t, memberEmail(member), "Lessons changed: ")
		if len(changed) != 2 {
			t.Fatalf("member %d got %d aggregated emails", member, len(changed))
		}
		moves, cancellations := strings.Count(changed[0].Text+changed[1].Text, "\n- "), strings.Count(changed[0].Text+changed[1].Text, "(cancelled)")
		if moves != 5 || cancellations != 2 {
			t.Fatalf("member %d's emails list %d lessons, %d cancelled:\n%s\n%s", member, moves, cancellations, changed[0].Text, changed[1].Text)
		}
		rows := h.notificationsOf(t, h.studentToken(t, member))
		if len(rows) != 2 || rows[0].Event != "lesson.series_changed" || rows[1].Link == nil || rows[1].Link.ID != series.Lessons[1].ID {
			t.Fatalf("member %d has the rows %+v", member, rows)
		}
	}
	listed := h.changeEmailsTo(t, memberEmail(10), "Lessons changed: ")
	if len(listed) != 1 || strings.Count(listed[0].Text, "\n- ") != 20 || !strings.Contains(listed[0].Text, "and 5 more") {
		t.Fatalf("the long series' email is %+v", listed)
	}
	if h.outboxKeysOf(t, domain.EventLessonMoved)+h.outboxKeysOf(t, domain.EventLessonCancelled) != 0 ||
		h.outboxKeysOf(t, domain.EventLessonSeriesChanged) != 5 {
		t.Fatal("the series operations queued per-lesson emails or more than one email per recipient and operation")
	}
}

func TestLessonChangeRowsAreScopedToTheCaller(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.student)
	cancelled := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00")).Lessons[0].ID
	moved := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0].ID
	h.cancelCall(t, parties.tutorToken, cancelled, "this", 1)
	h.editLesson(t, parties.tutorToken, moved, thisLesson(1).startingAt("19:00"))
	h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group, "", `{"remove":["`+parties.other+`"]}`)

	member := h.notificationsOf(t, parties.studentToken)
	former := h.notificationsOf(t, h.studentToken(t, 11))
	secondTutor := h.notificationsOf(t, secondToken)
	ownTutor := h.notificationsOf(t, parties.tutorToken)

	if len(member) != 2 {
		t.Fatalf("the member has the rows %+v", member)
	}
	for _, row := range member {
		if row.Label == nil || *row.Label != lessonTitle || row.Colour == nil || row.Link == nil || row.Link.Kind != "lesson" {
			t.Fatalf("the member sees the row %+v", row)
		}
	}
	byEvent := map[string]notificationItem{}
	for _, row := range former {
		byEvent[row.Event] = row
	}
	if row := byEvent["lesson.cancelled"]; len(former) != 2 || row.Label == nil || row.Link == nil || row.Link.ID != cancelled {
		t.Fatalf("the former member sees the cancellation as %+v", row)
	}
	if row := byEvent["lesson.moved"]; row.Label != nil || row.Link != nil {
		t.Fatalf("the former member sees the moved lesson as %+v", row)
	}
	if len(secondTutor) != 0 || len(ownTutor) != 0 {
		t.Fatalf("the tutors have the rows %+v and %+v", secondTutor, ownTutor)
	}
}

func TestEditsLockRelationshipsBeforeSeriesAndLessons(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	series := h.mondays(t, token, card, 4)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM relationships WHERE id = $1::uuid FOR UPDATE", card)
	edit := h.sendInBackground(http.MethodPatch, editPath(series.Lessons[1].ID), following(1).titled(newLessonTitle).body(t), jsonCall(token))
	h.waitForLockWaiters(t, 1)

	seriesFree := !h.lockedElsewhere(t, "lesson_series", "id", series.Series.ID)
	lessonsFree := !h.lockedElsewhere(t, "lessons", "series_id", series.Series.ID)
	release()
	edited := awaitAnswer(t, edit)
	holdSeries := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM lesson_series WHERE id = $1::uuid FOR UPDATE", decodeChanged(t, edited).Series.ID)
	cancellation := h.sendInBackground(http.MethodPost, cancelPath(series.Lessons[2].ID), cancelBody("following", 2), jsonCall(token))
	h.waitForLockWaiters(t, 1)
	relationshipHeld := h.lockedElsewhere(t, "relationships", "id", card)
	holdSeries()
	cancelled := awaitAnswer(t, cancellation)

	if !seriesFree || !lessonsFree {
		t.Fatalf("an edit waiting for the relationship held the series (%t) or its lessons (%t)", !seriesFree, !lessonsFree)
	}
	if !relationshipHeld {
		t.Fatal("a cancellation waiting for the series did not hold the relationship")
	}
	if edited.status != http.StatusOK || cancelled.status != http.StatusOK {
		t.Fatalf("the writes answered %d %s and %d %s", edited.status, edited.body, cancelled.status, cancelled.body)
	}
}

func TestARepeatedCancellationReturnsTheCurrentState(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	oneOff := h.lessonAt(t, token, card, "2030-03-12", "18:00")
	series := h.mondays(t, token, card, 4)
	byAnotherCause := h.lessonAt(t, token, card, "2030-03-13", "18:00")
	h.exec2(t, "UPDATE lessons SET status = 'cancelled', cancelled_at = $2::timestamptz, cancel_cause = 'relationship_ended' WHERE id = $1::uuid",
		byAnotherCause, rfc3339(authStart))

	first := decodeChanged(t, h.cancelCall(t, token, oneOff, "this", 1))
	repeated := decodeChanged(t, h.cancelCall(t, token, oneOff, "this", 1))
	firstFollowing := decodeChanged(t, h.cancelCall(t, token, series.Lessons[1].ID, "following", 1))
	repeatedFollowing := decodeChanged(t, h.cancelCall(t, token, series.Lessons[1].ID, "following", 1))
	other := decodeChanged(t, h.cancelCall(t, token, byAnotherCause, "this", 7))

	if repeated.Lessons[0].Version != first.Lessons[0].Version || repeated.Lessons[0].Status != "cancelled" || h.storedLesson(t, oneOff).Version != 2 {
		t.Fatalf("the repeated cancellation answered %+v after %+v", repeated, first)
	}
	if !slices.Equal(lessonIDs(repeatedFollowing.Lessons), lessonIDs(firstFollowing.Lessons)) || repeatedFollowing.Series == nil ||
		*repeatedFollowing.Series.EndDate != "2030-03-17" {
		t.Fatalf("the repeated cancel following answered %+v", repeatedFollowing)
	}
	if stored := h.storedLesson(t, byAnotherCause); other.Lessons[0].Status != "cancelled" || *stored.Cause != "relationship_ended" || stored.Version != 1 {
		t.Fatalf("a lesson cancelled by another cause is now %+v", stored)
	}
	if h.outboxKeysOf(t, domain.EventLessonCancelled) != 1 || h.outboxKeysOf(t, domain.EventLessonSeriesChanged) != 1 {
		t.Fatal("a repeated cancellation queued another email")
	}
}

func TestEditingFollowingAppliesTheStartOnEachOccurrencesCurrentDate(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.setTimezone(t, tutor, "Europe/Berlin")
	series := h.mondays(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), 4)
	h.editLesson(t, token, series.Lessons[1].ID, thisLesson(1).on("2030-03-20"))

	h.editLesson(t, token, series.Lessons[0].ID, following(1).startingAt("19:30"))

	want := []time.Time{
		time.Date(2030, time.March, 11, 18, 30, 0, 0, time.UTC), time.Date(2030, time.March, 20, 18, 30, 0, 0, time.UTC),
		time.Date(2030, time.March, 25, 18, 30, 0, 0, time.UTC), time.Date(2030, time.April, 1, 17, 30, 0, 0, time.UTC),
	}
	for index, lesson := range series.Lessons {
		if stored := h.storedLesson(t, lesson.ID); !stored.StartsAt.Equal(want[index]) {
			t.Errorf("occurrence %d starts at %s, want %s", index, stored.StartsAt.UTC(), want[index])
		}
	}
}

func TestAGroupLessonWriteWhoseMembersChangedAnswersConflict(t *testing.T) {
	t.Parallel()
	writes := map[string]func(t *testing.T, h *authHarness, token string, group string) (string, string, string){
		"a creation": func(t *testing.T, _ *authHarness, _ string, group string) (string, string, string) {
			return http.MethodPost, lessonsRoute, groupLessonFor(group, "2030-03-12", "18:00").body(t)
		},
		"an edit": func(t *testing.T, h *authHarness, token string, group string) (string, string, string) {
			lesson := h.scheduled(t, token, groupLessonFor(group, "2030-03-12", "18:00")).Lessons[0].ID
			return http.MethodPatch, editPath(lesson), thisLesson(1).titled(newLessonTitle).body(t)
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			members := h.activeCards(t, tutor, 11, 2)
			joining, later := h.linkedCard(t, tutor, 13, domain.RelationshipActive), h.linkedCard(t, tutor, 14, domain.RelationshipActive)
			group := h.newGroup(t, token, groupName, members).ID
			method, path, body := write(t, h, token, group)
			before := h.lessonRows(t, tutor)
			insertMember := "INSERT INTO group_members (group_id, relationship_id, tutor_id, added_at) SELECT $1::uuid, '%s'::uuid, tutor_id, now() FROM groups WHERE id = $1::uuid"
			releaseGroup := h.holdRowThen(t, "SELECT "+lockedRowPosition+" FROM groups WHERE id = $1::uuid FOR NO KEY UPDATE", group,
				strings.Replace(insertMember, "%s", joining, 1))
			heldGroup := h.contested
			releaseCard := h.holdRowThen(t, "SELECT "+lockedRowPosition+" FROM relationships WHERE id = $1::uuid FOR NO KEY UPDATE", joining,
				"INSERT INTO group_members (group_id, relationship_id, tutor_id, added_at) SELECT '"+group+"'::uuid, '"+later+"'::uuid, tutor_id, now() FROM relationships WHERE id = $1::uuid")
			heldCard := h.contested
			h.contested = heldGroup
			answer := h.sendInBackground(method, path, body, jsonCall(token))
			h.waitForLockWaiters(t, 1)

			releaseGroup()
			h.contested = heldCard
			h.waitForLockWaiters(t, 1)
			releaseCard()
			refused := awaitAnswer(t, answer)

			if refused.status != http.StatusConflict || code(t, refused) != "conflict" {
				t.Fatalf("the write answered %d %s", refused.status, refused.body)
			}
			if h.lessonRows(t, tutor) != before || h.countRows(t, "SELECT count(id) FROM lessons WHERE tutor_id = $1::uuid AND version > 1", tutor.String()) != 0 {
				t.Fatal("the refused write changed a lesson")
			}
		})
	}
}

func TestLessonWritesAreLimitedPerTutor(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	lesson := h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), "2030-03-12", "18:00")

	for index := range domain.LessonWritesPerHour - 1 {
		refused := h.refusedWrite(t, token, lesson, index%2 == 1)
		if refused.status != http.StatusConflict {
			t.Fatalf("attempt %d below the limit answered %d %s", index, refused.status, refused.body)
		}
	}
	outOfScope := h.editLesson(t, token, randomCardID, thisLesson(1).titled(newLessonTitle))
	limitedEdit := h.editLesson(t, token, lesson, thisLesson(1).titled(newLessonTitle))
	limitedCancel := h.cancelCall(t, token, lesson, "this", 1)
	used := h.countRows(t, "SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose = $1", "lesson_writes_hour")
	h.at(t, authStart.Add(time.Hour))
	nextHour := h.editLesson(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), lesson, thisLesson(1).titled(newLessonTitle))

	if outOfScope.status != http.StatusNotFound {
		t.Fatalf("an edit outside the tutor's scope answered %d %s", outOfScope.status, outOfScope.body)
	}
	for name, limited := range map[string]response{"an edit": limitedEdit, "a cancellation": limitedCancel} {
		if limited.status != http.StatusTooManyRequests || code(t, limited) != "rate_limited" || limited.headers.Get("Retry-After") == "" {
			t.Errorf("%s over the limit answered %d %s", name, limited.status, limited.body)
		}
	}
	if used != domain.LessonWritesPerHour {
		t.Fatalf("the hourly counter holds %d slots", used)
	}
	if nextHour.status != http.StatusOK || h.storedLesson(t, lesson).Title != newLessonTitle {
		t.Fatalf("an edit in the next hour answered %d %s", nextHour.status, nextHour.body)
	}
}

func TestLessonChangeEmailsShareTheDailyCapWithReminders(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	capped := h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), "2030-03-12", "18:00")
	below := h.lessonAt(t, token, h.linkedCard(t, tutor, 11, domain.RelationshipActive), "2030-03-13", "18:00")
	h.presetLessonEmailSlots(t, domain.UserID{10}, domain.LessonEmailsPerDay)
	h.presetLessonEmailSlots(t, domain.UserID{11}, domain.LessonEmailsPerDay-1)

	h.editLesson(t, token, capped, thisLesson(1).startingAt("19:00"))
	h.cancelCall(t, token, below, "this", 1)
	h.tick(t)

	if len(h.changeEmailsTo(t, memberEmail(10), "Lesson moved")) != 0 || len(h.notificationsOf(t, h.studentToken(t, 10))) != 1 {
		t.Fatal("the capped student got the email or lost the in-app row")
	}
	if len(h.changeEmailsTo(t, memberEmail(11), "Lesson cancelled")) != 1 || h.lessonEmailSlots(t, domain.UserID{11}) != domain.LessonEmailsPerDay {
		t.Fatalf("the student below the cap has %d slots", h.lessonEmailSlots(t, domain.UserID{11}))
	}
	if h.countLogs(`"msg":"email.dropped"`, `"event":"lesson.moved"`, `"reason":"daily_cap"`) != 1 {
		t.Fatalf("the drop was reported as:\n%s", h.logs.String())
	}
}

func TestLessonEditRecordsCarryNoTitlesOrPersonalData(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	series := h.groupMondays(t, parties.tutorToken, parties.group, 3)
	secret := "A distinctive edited title"

	h.editLesson(t, parties.tutorToken, series.Lessons[0].ID, thisLesson(1).titled(secret).startingAt("21:00"))
	h.editLesson(t, parties.tutorToken, series.Lessons[1].ID, following(1).titled(secret))
	h.cancelCall(t, parties.tutorToken, series.Lessons[2].ID, "this", 2)
	h.tick(t)

	logs := h.logs.String()
	for _, hidden := range []string{secret, lessonTitle, privateCardName, groupName, "member-10@example.org", "Member account 10"} {
		if strings.Contains(logs, hidden) {
			t.Errorf("the records contain %q", hidden)
		}
	}
	if h.countLogs(`"msg":"lesson.edited"`, `"scope":"this"`, `"fields":"start,title"`) != 1 ||
		h.countLogs(`"msg":"lesson.edited"`, `"scope":"following"`) != 1 || h.countLogs(`"msg":"series.split"`) != 1 ||
		h.countLogs(`"msg":"lesson.cancelled"`, `"scope":"this"`, `"cause":"tutor"`, `"count":1`) != 1 ||
		h.countLogs(`"msg":"notification.created"`, `"event":"lesson.series_changed"`) != 2 {
		t.Fatalf("the records are:\n%s", logs)
	}
}

func (h *authHarness) refusedWrite(t *testing.T, token string, lesson string, cancellation bool) response {
	t.Helper()
	if cancellation {
		return h.cancelCall(t, token, lesson, "this", 99)
	}
	return h.editLesson(t, token, lesson, thisLesson(99).titled(newLessonTitle))
}

func TestMovesThatOverlapAreRefusedUntilAllowed(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	h.lessonAt(t, token, card, "2030-03-12", "18:00")
	moved := h.lessonAt(t, token, card, "2030-03-12", "20:00")
	series := h.mondaySeries(t, token, h.linkedCard(t, tutor, 11, domain.RelationshipActive))
	beyond := h.lessonAt(t, token, card, "2030-07-01", "19:00")
	selected := strings.Split(h.seriesLessons(t, series)[1], "@")[0]

	refusedMove := h.editLesson(t, token, moved, thisLesson(1).startingAt("18:30"))
	refusedSplit := h.editLesson(t, token, selected, following(1).startingAt("19:00"))
	allowedMove := h.editLesson(t, token, moved, thisLesson(1).startingAt("18:30").allowingOverlap())

	if details := decodeOverlap(t, refusedMove); len(details.Lessons) != 1 || details.Lessons[0].Title != lessonTitle || details.More != 0 {
		t.Fatalf("the move was refused with %+v", details)
	}
	details := decodeOverlap(t, refusedSplit)
	if len(details.Lessons) != 1 || !details.Lessons[0].StartsAt.Equal(time.Date(2030, time.July, 1, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("the split was refused with %+v", details)
	}
	if stored := h.storedSeries(t, series); *stored.EndDate != mondaysUntilDecember || h.storedLesson(t, selected).Version != 1 {
		t.Fatalf("the refused split changed the series to %+v", stored)
	}
	if allowedMove.status != http.StatusOK || h.storedLesson(t, beyond).Version != 1 {
		t.Fatalf("the allowed move answered %d %s", allowedMove.status, allowedMove.body)
	}
}

func TestASplitBeyondTheOpenSeriesCapIsRefused(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	series := h.mondaySeries(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive))
	h.copySeries(t, series, domain.OpenSeriesCapPerTutor-1, h.materialisedUntil(t, series))
	lessons := h.seriesLessons(t, series)

	refused := h.editLesson(t, token, strings.Split(lessons[1], "@")[0], following(1).titled(newLessonTitle))
	atTheFirst := h.editLesson(t, token, strings.Split(lessons[0], "@")[0], following(1).titled(newLessonTitle))

	if refused.status != http.StatusUnprocessableEntity || !slices.Equal(fields(t, refused), []string{"body:too_many"}) {
		t.Fatalf("a split at the cap answered %d %s", refused.status, refused.body)
	}
	if atTheFirst.status != http.StatusOK || !h.storedSeries(t, series).Ended {
		t.Fatalf("a split at the first occurrence, which ends the original, answered %d %s", atTheFirst.status, atTheFirst.body)
	}
}

func TestAnEditIntoThePastIsRefused(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	lesson := h.lessonAt(t, token, h.linkedCard(t, tutor, 10, domain.RelationshipActive), "2030-03-12", "18:00")

	refused := h.editLesson(t, token, lesson, thisLesson(1).on("2030-03-09"))

	if refused.status != http.StatusUnprocessableEntity || code(t, refused) != "lesson_in_past" {
		t.Fatalf("an edit into the past answered %d %s", refused.status, refused.body)
	}
	if stored := h.storedLesson(t, lesson); stored.Version != 1 || stored.Revision != 1 {
		t.Fatalf("the refused edit changed the lesson to %+v", stored)
	}
}

func TestAnUnchangedEditAnswersTheCurrentStateAndChangesNothing(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	lesson := h.lessonAt(t, token, card, "2030-03-12", "18:00")
	series := h.mondays(t, token, card, 3)
	frames := h.recordFrames(t)

	unchanged := decodeChanged(t, h.editLesson(t, token, lesson, thisLesson(1).titled(lessonTitle).startingAt("18:00").lasting(60)))
	unchangedFollowing := decodeChanged(t, h.editLesson(t, token, series.Lessons[1].ID, following(1).titled(lessonTitle)))
	sent := frames.settle(t, h)

	if unchanged.Lessons[0].Version != 1 || unchangedFollowing.Series == nil || unchangedFollowing.Series.ID != series.Series.ID {
		t.Fatalf("the unchanged edits answered %+v and %+v", unchanged, unchangedFollowing)
	}
	if stored := h.storedLesson(t, lesson); stored.Version != 1 || stored.Revision != 1 || len(stored.Overridden) != 0 {
		t.Fatalf("the unchanged edit stored %+v", stored)
	}
	if h.countRows(t, "SELECT count(id) FROM lesson_series WHERE tutor_id = $1::uuid", tutor.String()) != 1 || h.storedSeries(t, series.Series.ID).EndDate != nil {
		t.Fatal("the unchanged edit of following lessons split the series")
	}
	if len(framesTo(sent, tutor)) != 0 || h.countLogs(`"msg":"lesson.edited"`) != 0 {
		t.Fatalf("the unchanged edits signalled %v or were recorded", framesTo(sent, tutor))
	}
}

func TestDescriptionsCanBeRemoved(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	described := oneOffFor(card, "2030-03-12", "18:00")
	described.Description = pointer("Bring the workbook")
	lesson := h.scheduled(t, token, described).Lessons[0].ID
	empty := oneOffFor(card, "2030-03-13", "18:00")
	empty.Description = pointer("")
	emptyLesson := h.scheduled(t, token, empty).Lessons[0].ID

	removed := h.send(t, http.MethodPatch, editPath(lesson), `{"scope":"this","version":1,"description":null}`, jsonCall(token))
	emptied := h.send(t, http.MethodPatch, editPath(emptyLesson), `{"scope":"this","version":1,"description":""}`, jsonCall(token))

	if removed.status != http.StatusOK || emptied.status != http.StatusOK {
		t.Fatalf("removing descriptions answered %d %s and %d %s", removed.status, removed.body, emptied.status, emptied.body)
	}
	if left := h.countRows(t, "SELECT count(id) FROM lessons WHERE tutor_id = $1::uuid AND description IS NOT NULL", tutor.String()); left != 0 {
		t.Fatalf("%d lessons kept a description", left)
	}
	if stored := h.storedLesson(t, lesson); stored.Version != 2 || !slices.Equal(stored.Overridden, []string{"description"}) {
		t.Fatalf("the removal is stored as %+v", stored)
	}
	if stored := h.storedLesson(t, emptyLesson); stored.Version != 1 {
		t.Fatalf("an empty description over no description changed the lesson to %+v", stored)
	}
}

func TestASplitSignalsLessonsThatLeftScheduledToTheTutorOnly(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	group := h.newGroup(t, token, groupName, []string{h.linkedCard(t, tutor, 11, domain.RelationshipActive)}).ID
	joining := h.linkedCard(t, tutor, 12, domain.RelationshipActive)
	series := h.groupMondays(t, token, group, 3)
	cancelled := series.Lessons[2].ID
	h.cancelCall(t, token, cancelled, "this", 1)
	h.groupCall(t, token, http.MethodPatch, group, "", `{"add":["`+joining+`"]}`)
	frames := h.recordFrames(t)

	h.editLesson(t, token, series.Lessons[1].ID, following(1).titled(newLessonTitle))
	sent := frames.settle(t, h)

	for _, recipient := range []domain.UserID{tutor, {11}, {12}} {
		if !hasFrame(sent, recipient, domain.EntityLesson, series.Lessons[1].ID) {
			t.Errorf("no frame for %s about the scheduled lesson", recipient)
		}
	}
	if !hasFrame(sent, tutor, domain.EntityLesson, cancelled) {
		t.Error("no frame for the tutor about the cancelled lesson")
	}
	for _, student := range []domain.UserID{{11}, {12}} {
		if hasFrame(sent, student, domain.EntityLesson, cancelled) {
			t.Errorf("%s got a frame about the cancelled lesson", student)
		}
	}
}

type audienceAnswer struct {
	lessons []lessonView
	want    audienceIdentifiers
}

func TestLessonWritesAnswerTheAudienceIdentifier(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	group := h.newGroup(t, token, groupName, []string{card}).ID
	request := oneOffFor(card, mondaySeriesStart, "18:00").weekly([]int{1}, 3)
	request.AllowOverlap = true

	created := decodeCreated(t, h.createLessons(t, token, request, "audience-series-key"))
	replayed := decodeCreated(t, h.createLessons(t, token, request, "audience-series-key"))
	groupSeries := h.groupMondays(t, token, group, 3)
	edited := decodeChanged(t, h.editLesson(t, token, created.Lessons[1].ID, following(1).titled(newLessonTitle)))
	cancelled := decodeChanged(t, h.cancelCall(t, token, groupSeries.Lessons[1].ID, "following", 1))

	answers := map[string]audienceAnswer{
		"the created series":            {lessons: created.Lessons, want: cardIdentifiers(card)},
		"the replayed series":           {lessons: replayed.Lessons, want: cardIdentifiers(card)},
		"the created group series":      {lessons: groupSeries.Lessons, want: groupIdentifiers(group)},
		"the edit of following":         {lessons: edited.Lessons, want: cardIdentifiers(card)},
		"the cancellation of following": {lessons: cancelled.Lessons, want: groupIdentifiers(group)},
	}
	for name, answer := range answers {
		if len(answer.lessons) == 0 {
			t.Errorf("%s answered no lessons", name)
		}
		for _, lesson := range answer.lessons {
			if identifiers := identifiersOf(lesson.Audience); identifiers != answer.want {
				t.Errorf("%s answered the lesson %s with %+v, want %+v", name, lesson.ID, identifiers, answer.want)
			}
		}
	}
}
