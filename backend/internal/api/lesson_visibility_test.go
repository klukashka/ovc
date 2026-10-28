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
			strings.Contains(result.body, `"members":[`) || carriesAudienceIdentifiers(result.body) {
			t.Fatalf("a student response carries a card name, members or an audience identifier: %d %s", result.status, result.body)
		}
	}
	individual, inGroup := decodeLesson(t, pages[0]), decodeLesson(t, pages[1])
	if identifiersOf(individual.Audience) != noIdentifiers || identifiersOf(inGroup.Audience) != noIdentifiers {
		t.Fatalf("the student's lessons carry the audience identifiers %+v and %+v", individual.Audience, inGroup.Audience)
	}
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

func TestStudentLessonResponsesCarryNoAudienceIdentifiers(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.otherStudent)
	secondCard := domain.RelationshipID{0xc0, sharedStudentCard}.String()
	provisional := h.linkedCard(t, parties.tutor, 30, domain.RelationshipProvisional)
	endedRoster := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]
	formerGroup := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0]
	firstTutorLesson := h.scheduled(t, parties.tutorToken, oneOffFor(parties.other, "2030-03-14", "10:00")).Lessons[0]
	provisionalLesson := h.scheduled(t, parties.tutorToken, oneOffFor(provisional, "2030-03-14", "18:00")).Lessons[0]
	secondTutorLesson := h.scheduled(t, secondToken, oneOffFor(secondCard, "2030-03-15", "18:00")).Lessons[0]
	h.cancelWithRoster(t, endedRoster.ID, parties.student)
	h.cancelWithRoster(t, formerGroup.ID, parties.student, parties.otherStudent)
	h.endRelationship(t, parties.card)
	if removed := h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group, "", `{"remove":["`+parties.other+`"]}`); removed.status != http.StatusOK {
		t.Fatalf("removing the member answered %d %s", removed.status, removed.body)
	}
	otherToken, provisionalToken := h.studentToken(t, 11), h.studentToken(t, 30)

	pages := map[string]response{
		"a roster lesson of an ended relationship": h.lessonPage(t, parties.studentToken, endedRoster.ID),
		"a group lesson of a former member":        h.lessonPage(t, otherToken, formerGroup.ID),
		"the unconfirmed provisional student's":    h.lessonPage(t, provisionalToken, provisionalLesson.ID),
		"the first tutor's lesson of two tutors":   h.lessonPage(t, otherToken, firstTutorLesson.ID),
		"the second tutor's lesson of two tutors":  h.lessonPage(t, otherToken, secondTutorLesson.ID),
	}
	calendars := map[string]response{
		"the ended student's calendar":      h.calendarCall(t, parties.studentToken, calendarFrom, calendarTo, ""),
		"the two tutors' student calendar":  h.calendarCall(t, otherToken, calendarFrom, calendarTo, ""),
		"the provisional student's":         h.calendarCall(t, provisionalToken, calendarFrom, calendarTo, ""),
		"the first tutor's calendar items":  h.calendarCall(t, parties.tutorToken, calendarFrom, calendarTo, ""),
		"the second tutor's calendar items": h.calendarCall(t, secondToken, calendarFrom, calendarTo, ""),
	}

	for name, page := range pages {
		if page.status != http.StatusOK || carriesAudienceIdentifiers(page.body) {
			t.Errorf("%s answered %d %s", name, page.status, page.body)
			continue
		}
		if identifiers := identifiersOf(decodeLesson(t, page).Audience); identifiers != noIdentifiers {
			t.Errorf("%s decodes the audience identifiers %+v", name, identifiers)
		}
	}
	for name, calendar := range calendars {
		var decoded calendarPage
		if err := json.Unmarshal([]byte(calendar.body), &decoded); err != nil || calendar.status != http.StatusOK || len(decoded.Items) == 0 ||
			carriesAudienceIdentifiers(calendar.body) {
			t.Errorf("%s answered %d %s", name, calendar.status, calendar.body)
		}
	}
}

func TestTutorLessonDetailCarriesTheAudienceIdentifier(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	oneToOne := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]
	group := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0]

	individual := decodeLesson(t, h.lessonPage(t, parties.tutorToken, oneToOne.ID))
	inGroup := decodeLesson(t, h.lessonPage(t, parties.tutorToken, group.ID))

	if identifiers := identifiersOf(individual.Audience); identifiers != cardIdentifiers(parties.card) {
		t.Fatalf("the tutor's one-to-one lesson carries %+v, want the card %s", identifiers, parties.card)
	}
	if identifiers := identifiersOf(inGroup.Audience); identifiers != groupIdentifiers(parties.group) {
		t.Fatalf("the tutor's group lesson carries %+v, want the group %s", identifiers, parties.group)
	}
}

type storedAudienceCase struct {
	lesson      string
	identifiers audienceIdentifiers
	request     lessonRequest
	status      int
	code        string
}

func TestTutorLessonDetailKeepsTheStoredAudienceAfterItsStateChanges(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	ended := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	pending := h.linkedCard(t, tutor, 11, domain.RelationshipActive)
	removed := h.linkedCard(t, tutor, 12, domain.RelationshipProvisional)
	purged := h.linkedCard(t, tutor, 13, domain.RelationshipActive)
	archived := h.newGroup(t, token, groupName, []string{ended}).ID
	lessonOf := func(request lessonRequest) string { return h.scheduled(t, token, request).Lessons[0].ID }
	cases := map[string]storedAudienceCase{
		"an ended relationship": {
			lesson: lessonOf(oneOffFor(ended, "2030-03-12", "10:00")), identifiers: cardIdentifiers(ended),
			request: oneOffFor(ended, "2030-04-02", "10:00"), status: http.StatusConflict, code: "audience_not_active",
		},
		"a reclaim-pending link": {
			lesson: lessonOf(oneOffFor(pending, "2030-03-12", "12:00")), identifiers: cardIdentifiers(pending),
			request: oneOffFor(pending, "2030-04-02", "12:00"), status: http.StatusConflict, code: "audience_not_active",
		},
		"an archived group": {
			lesson: lessonOf(groupLessonFor(archived, "2030-03-12", "14:00")), identifiers: groupIdentifiers(archived),
			request: groupLessonFor(archived, "2030-04-02", "14:00"), status: http.StatusConflict, code: "audience_not_active",
		},
		"a removed provisional link": {
			lesson: lessonOf(oneOffFor(removed, "2030-03-12", "16:00")), identifiers: noIdentifiers,
			request: oneOffFor(removed, "2030-04-02", "16:00"), status: http.StatusNotFound, code: "not_found",
		},
		"a purged student's link": {
			lesson: lessonOf(oneOffFor(purged, "2030-03-12", "18:00")), identifiers: noIdentifiers,
			request: oneOffFor(purged, "2030-04-02", "18:00"), status: http.StatusNotFound, code: "not_found",
		},
	}
	h.endRelationship(t, ended)
	h.exec(t, "UPDATE relationships SET state = 'reclaim_pending' WHERE id = $1::uuid", pending)
	if result := h.archive(t, token, archived); result.status != http.StatusOK {
		t.Fatalf("archiving answered %d %s", result.status, result.body)
	}
	if result := h.removeStudent(t, token, removed); result.status != http.StatusOK {
		t.Fatalf("the removal answered %d %s", result.status, result.body)
	}
	h.exec2(t, `UPDATE relationships SET state = 'discarded', discard_reason = 'student_purged', discarded_at = $2::timestamptz,
ended_at = $2::timestamptz, ended_by = 'deletion', student_id = NULL, email = NULL WHERE id = $1::uuid`, purged, rfc3339(authStart))

	for name, testCase := range cases {
		view := decodeLesson(t, h.lessonPage(t, token, testCase.lesson))
		creation := h.createLessons(t, token, testCase.request, "")

		if identifiers := identifiersOf(view.Audience); identifiers != testCase.identifiers {
			t.Errorf("the lesson of %s carries %+v, want %+v", name, identifiers, testCase.identifiers)
		}
		if creation.status != testCase.status || code(t, creation) != testCase.code {
			t.Errorf("scheduling for %s answered %d %s", name, creation.status, creation.body)
		}
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

func TestCalendarLeavesOutALessonEndingExactlyAtTheRangeStart(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	from := time.Date(2030, time.March, 12, 15, 0, 0, 0, time.UTC)
	h.lessonAt(t, token, card, "2030-03-12", "17:00")
	endingJustAfter := h.lessonAt(t, token, card, "2030-03-12", "17:05")

	tutorPage := h.calendarOf(t, token, from, from.Add(24*time.Hour))
	studentPage := h.calendarOf(t, h.studentToken(t, 10), from, from.Add(24*time.Hour))

	for name, page := range map[string]calendarPage{"the tutor": tutorPage, "the student": studentPage} {
		if !slices.Equal(calendarIDs(page), []string{endingJustAfter}) {
			t.Errorf("the calendar of %s lists %v", name, calendarIDs(page))
		}
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
	secondCard := domain.RelationshipID{0xc0, sharedStudentCard}.String()
	ownLesson := h.scheduled(t, secondToken, oneOffFor(secondCard, "2030-03-14", "18:00")).Lessons[0]

	page := h.lessonPage(t, secondToken, lesson.ID)
	randomPage := h.lessonPage(t, secondToken, randomCardID)
	own := h.lessonPage(t, secondToken, ownLesson.ID)
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
	if !slices.Equal(calendarIDs(calendar), []string{ownLesson.ID}) {
		t.Fatalf("the second tutor's calendar lists %v", calendarIDs(calendar))
	}
	if identifiers := identifiersOf(decodeLesson(t, own).Audience); identifiers != cardIdentifiers(secondCard) || strings.Contains(own.body, parties.card) {
		t.Fatalf("the second tutor's own lesson carries %+v: %s", identifiers, own.body)
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
	h.endRelationship(t, parties.card)
	h.cardCall(t, parties.tutorToken, http.MethodPost, squatter, "/remove", "")

	denied := map[string]response{
		"an ended relationship's scheduled lesson": h.lessonPage(t, parties.studentToken, individual.ID),
		"a removed member's group lesson":          h.lessonPage(t, h.studentToken(t, 11), groupLesson.ID),
		"a former provisional account's one":       h.lessonPage(t, squatterToken, provisional.ID),
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

func anotherTutorsLessonWrite(method string, suffix string, body string) denialCase {
	return func(t *testing.T, h *authHarness) (response, response) {
		t.Helper()
		parties := h.visibilityParties(t)
		second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
		h.shareStudentWith(t, second, parties.student)
		lesson := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]
		outOfScope := h.send(t, method, lessonsRoute+"/"+lesson.ID+suffix, body, jsonCall(secondToken))
		random := h.send(t, method, lessonsRoute+"/"+randomCardID+suffix, body, jsonCall(secondToken))
		if stored := h.storedLesson(t, lesson.ID); stored.Version != 1 || stored.Status != "scheduled" {
			t.Fatalf("another tutor changed the lesson to %+v", stored)
		}
		return outOfScope, random
	}
}
