package api_test

import (
	"net/http"
	"testing"

	"ovc/backend/internal/domain"
)

type returnRace struct {
	h        *authHarness
	parties  reviewParties
	returned <-chan pendingAnswer
	ended    <-chan pendingAnswer
}

func (h *authHarness) returnRace(t *testing.T, returnFirst bool) returnRace {
	t.Helper()
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Redo", fullyScored("Redo")...))
	race := returnRace{h: h, parties: parties}
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM relationships WHERE id = $1::uuid FOR UPDATE", parties.card.ID)
	sendReturn := func() {
		race.returned = h.sendInBackground(http.MethodPost, submissionsRoute+parties.submission+"/return", `{"revision":1}`,
			jsonCall(parties.tutorToken))
	}
	sendEnding := func() {
		race.ended = h.sendInBackground(http.MethodPost, "/api/v1/students/"+parties.card.ID+"/end", "", jsonCall(parties.tutorToken))
	}
	first, second := sendEnding, sendReturn
	if returnFirst {
		first, second = sendReturn, sendEnding
	}
	first()
	h.waitForLockWaiters(t, 1)
	second()
	h.waitForBlockedStatements(t, 2)
	release()
	return race
}

func (r returnRace) studentHomeworkEvents(t *testing.T) []string {
	t.Helper()
	var events []string
	for _, item := range r.h.notificationsOf(t, r.parties.studentToken) {
		if item.Event == string(domain.NotificationSubmissionReturned) || item.Event == string(domain.NotificationSubmissionChecked) ||
			item.Event == string(domain.NotificationReviewUpdated) {
			events = append(events, item.Event)
		}
	}
	return events
}

func (r returnRace) frozenAnswers(t *testing.T) int64 {
	t.Helper()
	return r.h.countRows(t, "SELECT count(*) FROM answers WHERE submission_id = $1::uuid AND frozen_at IS NOT NULL", r.parties.submission)
}

func TestReturnRacesEnding(t *testing.T) {
	t.Parallel()
	t.Run("return first", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		race := h.returnRace(t, true)

		returned, ended := awaitAnswer(t, race.returned), awaitAnswer(t, race.ended)

		h.mustAnswer(t, returned, http.StatusOK)
		h.mustAnswer(t, ended, http.StatusOK)
		work := h.reviewedWork(t, race.parties.tutorToken, race.parties.submission)
		events := race.studentHomeworkEvents(t)
		if work.Status != string(domain.SubmissionClosed) || len(work.Revisions) != 1 || work.Revisions[0].State != string(domain.ReviewPublished) {
			t.Fatalf("after the return and the ending the work is %s with %+v", work.Status, work.Revisions)
		}
		if len(events) != 1 || events[0] != string(domain.NotificationSubmissionReturned) || race.frozenAnswers(t) != 4 {
			t.Fatalf("the student got %v and %d answers are frozen", events, race.frozenAnswers(t))
		}
	})
	t.Run("ending first", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		race := h.returnRace(t, false)

		ended, returned := awaitAnswer(t, race.ended), awaitAnswer(t, race.returned)

		h.mustAnswer(t, ended, http.StatusOK)
		assertRefused(t, "a return after the ending", returned, http.StatusConflict, "return_not_allowed")
		work := h.reviewedWork(t, race.parties.tutorToken, race.parties.submission)
		if work.Status != string(domain.SubmissionSubmitted) || len(work.Revisions) != 1 || work.Revisions[0].State != string(domain.ReviewDraft) {
			t.Fatalf("after the ending and the refused return the work is %s with %+v", work.Status, work.Revisions)
		}
		if events := race.studentHomeworkEvents(t); len(events) != 0 || race.frozenAnswers(t) != 4 {
			t.Fatalf("the student got %v and %d answers are frozen", events, race.frozenAnswers(t))
		}
	})
}

func TestStudentHomeworkFramesCarryIdentifiersOnly(t *testing.T) {
	t.Parallel()
	live := newLiveHarness(t, liveTuning{})
	h := live.authHarness
	parties := h.activeStudentInAGroup(t)
	_, submissions := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Trip to Paris", string(domain.AnswerPolicyNever), nil,
		workBlocks()...)
	submission := submissions[0].id
	studentSocket := h.connect(t, parties.studentToken)
	draft := h.saved(t, parties.studentToken, submission, answersJSON(t, 1, saveID(1), typedAnswer(workQuestion(5), "We saw the tower")))

	h.submitted(t, parties.studentToken, submission, draft.Version)
	h.reviewSaved(t, parties.tutorToken, submission, reviewJSON(t, 1, 0, saveID(2), "Redo", fullyScored("Redo")...))
	h.published(t, parties.tutorToken, submission, domain.ReviewReturn, 1)
	var frames []serverFrame
	for range 3 {
		frame, closed := studentSocket.next(t)
		if closed != 0 {
			t.Fatalf("the student's socket closed with %d", closed)
		}
		frames = append(frames, frame)
	}

	allowed := map[domain.FrameType][]string{domain.FrameEntityChanged: {"id", "kind"}, domain.FrameNotificationNew: {"id"}}
	check := contractCheck{t: t}
	for _, frame := range frames {
		if _, known := allowed[frame.Type]; !known {
			t.Fatalf("the student got a %s frame", frame.Type)
		}
		check.object(string(frame.Type), frame.Payload, allowed[frame.Type])
		check.noSentinel(string(frame.Type), frame.Payload)
		if frame.Ref != nil {
			t.Errorf("%s carries a ref", frame.Type)
		}
	}
}
