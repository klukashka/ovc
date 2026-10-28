package api_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"ovc/backend/internal/domain"
)

type visibilityCase struct {
	name    string
	student byte
	lesson  string
	visible bool
}

func (h *authHarness) lessonRowFor(t *testing.T, student byte, tutor domain.UserID, lesson string, card string) {
	t.Helper()
	recipient := domain.UserID{student}
	_, err := h.database.Pool.Exec(context.Background(), `
INSERT INTO notifications (id, user_id, tutor_id, student_id, event, subject_id, relationship_id, idempotency_key, created_at)
VALUES (gen_random_uuid(), $1, $2, $1, 'lesson.reminder_1h', $3::uuid, $4::uuid, $5, $6)`,
		recipient, tutor, lesson, card, lesson+":"+recipient.String(), authStart)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTheThreeStudentVisibilityQueriesAgree(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 11, 3)
	group := h.newGroup(t, token, "Evening club", cards[:2])
	scheduledGroup := h.scheduled(t, token, groupLessonFor(group.ID, firstLessonDate, firstLessonLocal)).Lessons[0].ID
	cancelledGroup := h.scheduled(t, token, groupLessonFor(group.ID, firstLessonDate, "20:00")).Lessons[0].ID
	ownLesson := h.lessonAt(t, token, cards[0], firstLessonDate, "21:00")
	endedLesson := h.lessonAt(t, token, cards[2], firstLessonDate, "21:00")
	h.cancelWithRoster(t, cancelledGroup, domain.UserID{11}, domain.UserID{12})
	for student, card := range map[byte]string{11: cards[0], 12: cards[1], 13: cards[2]} {
		for _, lesson := range []string{scheduledGroup, cancelledGroup, ownLesson, endedLesson} {
			h.lessonRowFor(t, student, tutor, lesson, card)
		}
	}
	if removed := h.groupCall(t, token, http.MethodPatch, group.ID, "", `{"remove":`+idList(cards[1:2])+`}`); removed.status != http.StatusOK {
		t.Fatalf("removing the member answered %d %s", removed.status, removed.body)
	}
	h.endRelationship(t, cards[2])
	cases := []visibilityCase{
		{"a current member's scheduled group lesson", 11, scheduledGroup, true},
		{"a current member's cancelled roster lesson", 11, cancelledGroup, true},
		{"an active relationship's scheduled lesson", 11, ownLesson, true},
		{"a former member's scheduled group lesson", 12, scheduledGroup, false},
		{"a former member's cancelled roster lesson", 12, cancelledGroup, true},
		{"another student's lesson", 12, ownLesson, false},
		{"an ended relationship's scheduled lesson", 13, endedLesson, false},
	}

	results := map[string][3]bool{}
	for _, testCase := range cases {
		student := h.studentToken(t, testCase.student)
		inCalendar := slices.Contains(calendarIDs(h.calendarOf(t, student, calendarFrom, calendarTo)), testCase.lesson)
		onItsPage := h.lessonPage(t, student, testCase.lesson).status == http.StatusOK
		labelled := false
		for _, row := range h.notificationsOf(t, student) {
			if row.Link != nil && row.Link.ID == testCase.lesson && row.Label != nil && *row.Label == lessonTitle {
				labelled = true
			}
		}
		results[testCase.name] = [3]bool{inCalendar, onItsPage, labelled}
	}

	for _, testCase := range cases {
		want := [3]bool{testCase.visible, testCase.visible, testCase.visible}
		if results[testCase.name] != want {
			t.Errorf("%s: calendar, page and notification label are %v, want %v", testCase.name, results[testCase.name], want)
		}
	}
}
