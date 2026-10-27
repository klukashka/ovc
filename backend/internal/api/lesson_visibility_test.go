package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const privateCardName = "The tutor's private card name"

type visibilityParties struct {
	tutor        domain.UserID
	tutorToken   string
	card         string
	student      domain.UserID
	studentToken string
	other        string
	otherStudent domain.UserID
	group        string
}

func (h *authHarness) visibilityParties(t *testing.T) visibilityParties {
	t.Helper()
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	other := h.linkedCard(t, tutor, 11, domain.RelationshipActive)
	h.exec2(t, "UPDATE relationships SET display_name = $2 WHERE id = $1::uuid", card, privateCardName)
	group := h.newGroup(t, token, groupName, []string{card, other}).ID
	return visibilityParties{
		tutor: tutor, tutorToken: token, card: card, student: domain.UserID{10}, studentToken: h.studentToken(t, 10),
		other: other, otherStudent: domain.UserID{11}, group: group,
	}
}

func TestStudentLessonResponsesCarryNoCardNamesOrMembers(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	oneToOne := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]
	group := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0]

	calendar := h.calendarCall(t, parties.studentToken, calendarFrom, calendarTo, "")
	pages := []response{h.lessonPage(t, parties.studentToken, oneToOne.ID), h.lessonPage(t, parties.studentToken, group.ID)}

	for _, result := range append(pages, calendar) {
		if result.status != http.StatusOK || strings.Contains(result.body, privateCardName) || strings.Contains(result.body, seededCardName) ||
			strings.Contains(result.body, `"members":[`) {
			t.Fatalf("a student response carries a card name or members: %d %s", result.status, result.body)
		}
	}
	individual, inGroup := decodeLesson(t, pages[0]), decodeLesson(t, pages[1])
	if individual.Audience.Label != nil || individual.Audience.LabelKind == nil || *individual.Audience.LabelKind != "individual" ||
		individual.Audience.Members != nil || individual.Tutor == nil || individual.Tutor.Name != "A language tutor" || individual.Tutor.Colour == nil {
		t.Fatalf("the student's one-to-one lesson is %+v", individual)
	}
	if inGroup.Audience.Kind != "group" || inGroup.Audience.Label == nil || *inGroup.Audience.Label != groupName ||
		inGroup.Audience.Colour == nil || individual.Audience.Colour == nil || *inGroup.Audience.Colour != *individual.Audience.Colour {
		t.Fatalf("the student's group lesson is %+v", inGroup.Audience)
	}
	tutorView := decodeLesson(t, h.lessonPage(t, parties.tutorToken, group.ID))
	if tutorView.Audience.Colour == nil || *tutorView.Audience.Colour != "moss" || tutorView.Audience.Members == nil || len(*tutorView.Audience.Members) != 2 {
		t.Fatalf("the tutor's group lesson is %+v", tutorView.Audience)
	}
}

func TestRosterNamesForTheTutorAreCardNames(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	scheduled := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00")).Lessons[0]
	frozen := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0]
	h.cancelWithRoster(t, frozen.ID, parties.student, parties.otherStudent)
	h.exec2(t, "UPDATE users SET state = 'deleted', deleted_at = $2::timestamptz WHERE id = $1::uuid", parties.otherStudent.String(), rfc3339(authStart))
	h.exec(t, "UPDATE users SET name = 'An account name' WHERE id = $1::uuid", parties.student.String())

	views := []lessonView{decodeLesson(t, h.lessonPage(t, parties.tutorToken, scheduled.ID)), decodeLesson(t, h.lessonPage(t, parties.tutorToken, frozen.ID))}

	for _, view := range views {
		members := *view.Audience.Members
		labels := []string{members[0].Label, members[1].Label}
		if !slices.Equal(labels, []string{privateCardName, seededCardName}) || members[0].LabelKind != nil ||
			members[1].LabelKind == nil || *members[1].LabelKind != "deleted_account" {
			t.Fatalf("the %s lesson's members are %+v", view.Status, members)
		}
	}
}

func TestCalendarReturnsLessonsIntersectingTheRange(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	from := time.Date(2030, time.March, 12, 15, 0, 0, 0, time.UTC)
	to := time.Date(2030, time.March, 13, 15, 0, 0, 0, time.UTC)
	reaching := h.scheduled(t, token, oneOffFor(card, "2030-03-12", "17:30")).Lessons[0]
	h.scheduled(t, token, lessonRequest{RelationshipID: &card, Title: lessonTitle, Date: "2030-03-12", Start: "16:00", Duration: 60})
	inside := h.scheduled(t, token, oneOffFor(card, "2030-03-13", "10:00")).Lessons[0]
	h.scheduled(t, token, oneOffFor(card, "2030-03-13", "18:00"))

	page := h.calendarOf(t, token, from, to)
	reversed := h.calendarCall(t, token, to, from, "")
	tooLong := h.calendarCall(t, token, from, from.Add(domain.CalendarMaxRange+2*domain.CalendarRangeMargin+time.Second), "")
	widest := h.calendarCall(t, token, from, from.Add(domain.CalendarMaxRange+2*domain.CalendarRangeMargin), "")
	withFilter := h.calendarCall(t, token, from, to, card)

	if !slices.Equal(calendarIDs(page), []string{reaching.ID, inside.ID}) || page.Truncated {
		t.Fatalf("the calendar lists %v", calendarIDs(page))
	}
	for name, refused := range map[string]response{"a reversed range": reversed, "a range too long": tooLong} {
		if refused.status != http.StatusUnprocessableEntity || code(t, refused) != "validation_failed" {
			t.Errorf("%s answered %d %s", name, refused.status, refused.body)
		}
	}
	if widest.status != http.StatusOK {
		t.Fatalf("the widest range answered %d %s", widest.status, widest.body)
	}
	if withFilter.status != http.StatusUnprocessableEntity || !slices.Equal(fieldsOf(t, withFilter), []string{"query.tutor:not_allowed"}) {
		t.Fatalf("a tutor's filter answered %d %s", withFilter.status, withFilter.body)
	}
}

func TestStudentCalendarShowsCurrentAudienceAndRosterOnly(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	individual := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "10:00")).Lessons[0]
	groupLesson := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00")).Lessons[0]
	frozen := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0]
	h.cancelWithRoster(t, frozen.ID, parties.student)
	before := h.calendarOf(t, parties.studentToken, calendarFrom, calendarTo)
	if removed := h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group, "", `{"remove":["`+parties.card+`"]}`); removed.status != http.StatusOK {
		t.Fatalf("removing the member answered %d %s", removed.status, removed.body)
	}

	after := h.calendarOf(t, parties.studentToken, calendarFrom, calendarTo)
	formerPage := h.lessonPage(t, parties.studentToken, groupLesson.ID)

	if !slices.Equal(calendarIDs(before), []string{individual.ID, groupLesson.ID, frozen.ID}) {
		t.Fatalf("the member's calendar lists %v", calendarIDs(before))
	}
	if !slices.Equal(calendarIDs(after), []string{individual.ID, frozen.ID}) {
		t.Fatalf("the former member's calendar lists %v", calendarIDs(after))
	}
	cancelled := after.Items[1]
	if cancelled.Status != "cancelled" || cancelled.Audience.Label != nil || cancelled.Audience.LabelKind == nil || *cancelled.Audience.LabelKind != "group" {
		t.Fatalf("the former member's roster lesson is %+v", cancelled)
	}
	if formerPage.status != http.StatusNotFound {
		t.Fatalf("a scheduled lesson of a group the student left answered %d %s", formerPage.status, formerPage.body)
	}
}

func TestTheTutorFilterRefusesAnotherStudentsTutor(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.student)
	secondCard := domain.RelationshipID{0xc0, sharedStudentCard}.String()
	mine := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "10:00")).Lessons[0]
	theirs := h.scheduled(t, secondToken, oneOffFor(secondCard, "2030-03-12", "12:00")).Lessons[0]
	pendingStudent := h.seedStudentAccount(t, 20, domain.AccountActive)
	pending := h.seedCard(t, second, 20, domain.RelationshipReclaimPending, &pendingStudent)
	discarded := h.seedCard(t, second, 21, domain.RelationshipDiscarded, &parties.student)

	filtered := h.calendarCall(t, parties.studentToken, calendarFrom, calendarTo, parties.card)
	unfiltered := h.calendarOf(t, parties.studentToken, calendarFrom, calendarTo)
	refusals := map[string]response{
		"another student's relationship": h.calendarCall(t, parties.studentToken, calendarFrom, calendarTo, parties.other),
		"a discarded relationship":       h.calendarCall(t, parties.studentToken, calendarFrom, calendarTo, discarded),
		"a reclaim-pending relationship": h.calendarCall(t, h.studentToken(t, 20), calendarFrom, calendarTo, pending),
	}
	random := h.calendarCall(t, parties.studentToken, calendarFrom, calendarTo, randomCardID)

	var page calendarPage
	if err := json.Unmarshal([]byte(filtered.body), &page); err != nil || filtered.status != http.StatusOK || !slices.Equal(calendarIDs(page), []string{mine.ID}) {
		t.Fatalf("the filtered calendar answered %d %s", filtered.status, filtered.body)
	}
	if !slices.Equal(calendarIDs(unfiltered), []string{mine.ID, theirs.ID}) {
		t.Fatalf("the unfiltered calendar lists %v", calendarIDs(unfiltered))
	}
	for name, refused := range refusals {
		if refusal(t, refused) != refusal(t, random) || refused.status != http.StatusNotFound {
			t.Errorf("%s answered %d %s", name, refused.status, refused.body)
		}
	}
}

func TestLessonPageShowsTheOtherPartysTimezone(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	h.setTimezone(t, parties.student, "Europe/Berlin")
	oneToOne := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]
	group := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0]

	views := map[string]lessonView{
		"the tutor of a one-to-one lesson": decodeLesson(t, h.lessonPage(t, parties.tutorToken, oneToOne.ID)),
		"the tutor of a group lesson":      decodeLesson(t, h.lessonPage(t, parties.tutorToken, group.ID)),
		"the student of a one-to-one":      decodeLesson(t, h.lessonPage(t, parties.studentToken, oneToOne.ID)),
		"the student of a group lesson":    decodeLesson(t, h.lessonPage(t, parties.studentToken, group.ID)),
		"a student in the same zone":       decodeLesson(t, h.lessonPage(t, h.studentToken(t, 11), group.ID)),
	}

	want := map[string]string{
		"the tutor of a one-to-one lesson": "Europe/Berlin", "the tutor of a group lesson": "",
		"the student of a one-to-one": "Europe/Moscow", "the student of a group lesson": "Europe/Moscow", "a student in the same zone": "",
	}
	for name, view := range views {
		got := ""
		if view.OtherTimezone != nil {
			got = *view.OtherTimezone
		}
		if got != want[name] {
			t.Errorf("%s sees the second timezone %q, want %q", name, got, want[name])
		}
	}
}

func TestUnconfirmedProvisionalStudentSeesOnlyTheProvisionalLessons(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	provisional := h.linkedCard(t, tutor, 10, domain.RelationshipProvisional)
	active := h.linkedCard(t, tutor, 11, domain.RelationshipActive)
	own := h.scheduled(t, token, oneOffFor(provisional, "2030-03-12", "18:00")).Lessons[0]
	others := h.scheduled(t, token, oneOffFor(active, "2030-03-12", "10:00")).Lessons[0]
	student := h.studentToken(t, 10)

	page := h.calendarOf(t, student, calendarFrom, calendarTo)
	ownPage := h.lessonPage(t, student, own.ID)
	othersPage := h.lessonPage(t, student, others.ID)

	if !slices.Equal(calendarIDs(page), []string{own.ID}) || decodeLesson(t, ownPage).Audience.LabelKind == nil {
		t.Fatalf("the unconfirmed student sees %v and %s", calendarIDs(page), ownPage.body)
	}
	if othersPage.status != http.StatusNotFound {
		t.Fatalf("another student's lesson answered %d %s", othersPage.status, othersPage.body)
	}
}

func TestLessonRoutesDenyASecondTutorSharingTheStudent(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.student)
	lesson := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]

	page := h.lessonPage(t, secondToken, lesson.ID)
	randomPage := h.lessonPage(t, secondToken, randomCardID)
	creation := h.createLessons(t, secondToken, oneOffFor(parties.card, "2030-03-13", "18:00"), "")
	groupCreation := h.createLessons(t, secondToken, groupLessonFor(parties.group, "2030-03-13", "18:00"), "")
	randomCreation := h.createLessons(t, secondToken, oneOffFor(randomCardID, "2030-03-13", "18:00"), "")
	calendar := h.calendarOf(t, secondToken, calendarFrom, calendarTo)

	if page.status != http.StatusNotFound || refusal(t, page) != refusal(t, randomPage) {
		t.Fatalf("the second tutor's lesson page answered %d %s", page.status, page.body)
	}
	for name, refused := range map[string]response{"a one-to-one lesson": creation, "a group lesson": groupCreation} {
		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, randomCreation) {
			t.Errorf("the second tutor scheduling %s for the first tutor's audience answered %d %s", name, refused.status, refused.body)
		}
	}
	if len(calendar.Items) != 0 {
		t.Fatalf("the second tutor's calendar lists %v", calendarIDs(calendar))
	}
}

func TestLessonPagesDenyFormerAudiences(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	squatter := h.linkedCard(t, parties.tutor, 30, domain.RelationshipProvisional)
	individual := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "10:00")).Lessons[0]
	groupLesson := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00")).Lessons[0]
	provisional := h.scheduled(t, parties.tutorToken, oneOffFor(squatter, "2030-03-14", "18:00")).Lessons[0]
	squatterToken := h.studentToken(t, 30)
	h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group, "", `{"remove":["`+parties.other+`"]}`)
	h.endAsTutor(t, parties.tutorToken, parties.card)
	h.cardCall(t, parties.tutorToken, http.MethodPost, squatter, "/remove", "")

	denied := map[string]response{
		"an ended relationship's lesson":     h.lessonPage(t, parties.studentToken, individual.ID),
		"a removed member's group lesson":    h.lessonPage(t, h.studentToken(t, 11), groupLesson.ID),
		"a former provisional account's one": h.lessonPage(t, squatterToken, provisional.ID),
	}

	random := h.lessonPage(t, parties.studentToken, randomCardID)
	for name, result := range denied {
		if result.status != http.StatusNotFound || refusal(t, result) != refusal(t, random) {
			t.Errorf("%s answered %d %s", name, result.status, result.body)
		}
	}
}

func TestLessonCreationSignalsEntityChangedToTheAudienceUnderTheRelationshipLock(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	second, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.student)
	frames := h.recordFrames(t)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM relationships WHERE id = $1::uuid FOR UPDATE", parties.other)
	answer := h.sendInBackground(http.MethodPost, lessonsRoute, groupLessonFor(parties.group, "2030-03-12", "18:00").weekly([]int{2}, 2).body(t), jsonCall(parties.tutorToken))
	h.waitForLockWaiters(t, 1)

	release()
	created := decodeCreated(t, awaitAnswer(t, answer))
	sent := frames.settle(t, h)

	for _, lesson := range created.Lessons {
		for _, recipient := range []domain.UserID{parties.tutor, parties.student, parties.otherStudent} {
			if !hasFrame(sent, recipient, domain.EntityLesson, lesson.ID) {
				t.Errorf("no lesson frame for %s about %s: %v", recipient, lesson.ID, sent)
			}
		}
	}
	if len(framesTo(sent, second)) != 0 {
		t.Fatalf("the second tutor got frames: %v", framesTo(sent, second))
	}
}

func TestScheduleRecordsCarryNoTitlesOrPersonalData(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	request := groupLessonFor(parties.group, "2030-03-12", "18:00")
	request.Title, request.Description = "A distinctive lesson title", pointer("A distinctive lesson description")
	series := request.weekly([]int{2}, 2)
	series.Title, series.Date = "A distinctive series title", "2030-03-13"

	one := h.scheduled(t, parties.tutorToken, request)
	many := decodeCreated(t, h.createLessons(t, parties.tutorToken, series, "a-distinctive-key"))
	h.calendarOf(t, parties.studentToken, calendarFrom, calendarTo)
	h.lessonPage(t, parties.studentToken, one.Lessons[0].ID)

	logs := h.logs.String()
	for _, secret := range []string{request.Title, *request.Description, series.Title, privateCardName, groupName, "a-distinctive-key", "member-10@example.org", "Member account 10"} {
		if strings.Contains(logs, secret) {
			t.Errorf("the records contain %q", secret)
		}
	}
	if !strings.Contains(logs, `"msg":"lesson.created"`) || !strings.Contains(logs, `"lesson_id":"`+one.Lessons[0].ID+`"`) ||
		!strings.Contains(logs, `"msg":"series.created"`) || !strings.Contains(logs, `"series_id":"`+many.Series.ID+`"`) ||
		!strings.Contains(logs, `"materialised":2`) {
		t.Fatal("the lesson.created and series.created records are missing")
	}
}

func (h *authHarness) markDeleted(t *testing.T, user domain.UserID, at time.Time) {
	t.Helper()
	if _, err := h.store.Repositories().Users.MarkDeleted(context.Background(), user, at); err != nil {
		t.Fatal(err)
	}
}

func anotherTutorsLesson(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	parties := h.visibilityParties(t)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.student)
	lesson := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]
	return h.lessonPage(t, secondToken, lesson.ID), h.lessonPage(t, secondToken, randomCardID)
}

func anotherTutorsLessonAudience(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	parties := h.visibilityParties(t)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.student)
	outOfScope := h.createLessons(t, secondToken, oneOffFor(parties.card, "2030-03-12", "18:00"), "")
	random := h.createLessons(t, secondToken, oneOffFor(randomCardID, "2030-03-12", "18:00"), "")
	if h.lessonRows(t, parties.tutor) != 0 || h.lessonRows(t, second) != 0 {
		t.Fatal("a lesson was scheduled for another tutor's audience")
	}
	return outOfScope, random
}
