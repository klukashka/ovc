package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

type removalSubmitRace struct {
	h          *authHarness
	parties    endingParties
	submission string
	lesson     string
	frames     *frameLog
	submitted  <-chan pendingAnswer
	removed    <-chan pendingAnswer
}

func (h *authHarness) removalSubmitRace(t *testing.T, submitFirst bool) removalSubmitRace {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	lesson := h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group.ID, "2030-03-12", "10:00")).Lessons[0].ID
	issue, submissions := h.issueWork(t, parties.tutorToken, toGroup(parties.group.ID), "Trip to Paris", string(domain.AnswerPolicyNever), nil,
		workBlocks()...)
	h.mustAnswer(t, h.patchIssue(t, parties.tutorToken, issue, `{"lesson_id":"`+lesson+`","version":1}`), http.StatusOK)
	submission := submissions[0].id
	draft := h.saved(t, parties.studentToken, submission, answersJSON(t, 1, saveID(1), typedAnswer(workQuestion(5), "We saw the tower")))
	h.nextSecond(t)
	race := removalSubmitRace{h: h, parties: parties, submission: submission, lesson: lesson, frames: h.recordFrames(t)}
	held := "SELECT " + lockedRowPosition + " FROM groups WHERE id = $1::uuid FOR UPDATE"
	key := parties.group.ID
	if submitFirst {
		held, key = "SELECT "+lockedRowPosition+" FROM issues WHERE id = $1::uuid FOR UPDATE", issue
	}
	release := h.holdRow(t, held, key)
	sendSubmit := func() {
		race.submitted = h.sendInBackground(http.MethodPost, submissionsRoute+submission+"/submit", fmt.Sprintf(`{"version":%d}`, draft.Version),
			jsonCall(parties.studentToken))
	}
	sendRemoval := func() {
		race.removed = h.sendInBackground(http.MethodPatch, groupsRoute+"/"+parties.group.ID, fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID),
			jsonCall(parties.tutorToken))
	}
	first, second := sendRemoval, sendSubmit
	if submitFirst {
		first, second = sendSubmit, sendRemoval
	}
	first()
	h.waitForLockWaiters(t, 1)
	second()
	h.waitForBlockedStatements(t, 2)
	release()
	return race
}

func (r removalSubmitRace) checkStudentFrame(t *testing.T, lessons string) {
	t.Helper()
	sent := r.frames.settle(t, r.h)
	frame := onlyFrame(t, homeworkFramesTo(sent, r.parties.student), "the student's homework frame")
	if frame.kind != string(domain.EntitySubmission) || frame.id != r.submission {
		t.Errorf("the student's homework frame is %+v", frame)
	}
	expectStudentHomeworkFrames(t, sent, r.parties.student, lessons)
	if lessons == "" && strings.Contains(frame.payload, `"lesson_ids"`) {
		t.Errorf("the student's homework frame carries a lesson_ids key: %s", frame.payload)
	}
}

func TestGroupRemovalSerialisesWithASubmit(t *testing.T) {
	t.Parallel()
	t.Run("submit first", func(t *testing.T) {
		t.Parallel()
		h := newAuthHarness(t)
		race := h.removalSubmitRace(t, true)

		submitted, removed := awaitAnswer(t, race.submitted), awaitAnswer(t, race.removed)

		h.mustAnswer(t, submitted, http.StatusOK)
		h.mustAnswer(t, removed, http.StatusOK)
		race.checkStudentFrame(t, race.lesson)
		if memberships := h.membershipsOf(t, race.parties.card.ID); memberships != 0 {
			t.Errorf("the removed student still has %d memberships", memberships)
		}
	})
	t.Run("removal first", func(t *testing.T) {
		t.Parallel()
		h := newAuthHarness(t)
		race := h.removalSubmitRace(t, false)

		removed, submitted := awaitAnswer(t, race.removed), awaitAnswer(t, race.submitted)

		h.mustAnswer(t, removed, http.StatusOK)
		h.mustAnswer(t, submitted, http.StatusOK)
		race.checkStudentFrame(t, "")
		if memberships := h.membershipsOf(t, race.parties.card.ID); memberships != 0 {
			t.Errorf("the removed student still has %d memberships", memberships)
		}
	})
}
