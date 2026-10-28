package api_test

import (
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

type queuedWrite struct {
	method string
	path   string
	body   string
}

func (h *authHarness) answerQueuedBehindTheTutor(t *testing.T, tutor domain.UserID, passTo time.Time, write queuedWrite) response {
	t.Helper()
	token := accessToken(t, h.login(t, tutorEmail, accountPassword))
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM tutor_profiles WHERE user_id = $1::uuid FOR NO KEY UPDATE", tutor.String())
	answer := h.sendInBackground(write.method, write.path, write.body, jsonCall(token))
	h.waitForLockWaiters(t, 1)
	h.at(t, passTo)
	release()
	return awaitAnswer(t, answer)
}

func TestLessonWritesReadTheClockThroughTheirOwnConnection(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	edited := h.lessonAt(t, token, card, firstLessonDate, firstLessonLocal)
	cancelled := h.lessonAt(t, token, card, firstLessonDate, "19:00")
	release := h.database.HoldAllConnectionsBut(t, 1)

	creation := awaitAnswer(t, h.sendInBackground(http.MethodPost, lessonsRoute, oneOffFor(card, firstLessonDate, "20:00").body(t), jsonCall(token)))
	edit := awaitAnswer(t, h.sendInBackground(http.MethodPatch, editPath(edited), thisLesson(1).titled("A title on one connection").body(t), jsonCall(token)))
	cancellation := awaitAnswer(t, h.sendInBackground(http.MethodPost, cancelPath(cancelled), cancelBody("this", 1), jsonCall(token)))
	release()

	if creation.status != http.StatusCreated || edit.status != http.StatusOK || cancellation.status != http.StatusOK {
		t.Fatalf("with one free connection the writes answered %d %s, %d %s and %d %s",
			creation.status, creation.body, edit.status, edit.body, cancellation.status, cancellation.body)
	}
}

func TestLessonWritesQueuedBehindTheTutorReadTheClockAfterTheWait(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	edited := h.lessonAt(t, token, card, firstLessonDate, firstLessonLocal)
	cancelled := h.lessonAt(t, token, card, firstLessonDate, "19:00")

	edit := h.answerQueuedBehindTheTutor(t, tutor, firstLessonStart.Add(time.Minute),
		queuedWrite{method: http.MethodPatch, path: editPath(edited), body: thisLesson(1).titled("A title too late").body(t)})
	cancellation := h.answerQueuedBehindTheTutor(t, tutor, firstLessonStart.Add(time.Hour+time.Minute),
		queuedWrite{method: http.MethodPost, path: cancelPath(cancelled), body: cancelBody("this", 1)})
	creation := h.answerQueuedBehindTheTutor(t, tutor, firstLessonStart.Add(2*time.Hour+time.Minute),
		queuedWrite{method: http.MethodPost, path: lessonsRoute, body: oneOffFor(card, firstLessonDate, "20:00").body(t)})

	for name, refused := range map[string]response{"the edit": edit, "the cancellation": cancellation} {
		if refused.status != http.StatusConflict || code(t, refused) != "not_editable" {
			t.Errorf("%s of a lesson that started during the wait answered %d %s", name, refused.status, refused.body)
		}
	}
	if creation.status != http.StatusUnprocessableEntity || code(t, creation) != "lesson_in_past" {
		t.Fatalf("the creation of a lesson that started during the wait answered %d %s", creation.status, creation.body)
	}
	if stored := h.storedLesson(t, edited); stored.Title == "A title too late" || h.lessonState(t, cancelled) != "scheduled" {
		t.Fatalf("the lessons are stored as %+v and %s", stored, h.lessonState(t, cancelled))
	}
}
