package api_test

import (
	"net/http"
	"testing"

	"ovc/backend/internal/domain"
)

func TestDailyUploadCapSurvivesRealMessageAndObjectCleanup(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.dailyUploadParties(t)
	channel := h.directChannel(t, parties.card.ID).ID
	content := plainText(int(domain.ChatMaxBytes))

	var cycles []dailyCleanupObservation
	for consumed := int64(0); consumed < domain.ChatDailyBytesPerTutor; consumed += int64(len(content)) {
		cycles = append(cycles, h.consumeAndDeleteUpload(t, parties, channel, content))
	}
	refused := h.chatUpload(t, parties.studentToken, channel, "over.txt", plainText(1))
	remaining := h.uploadsBy(t, parties.student)
	usage := h.usage(t, parties.tutor)

	for index, cycle := range cycles {
		if cycle.rows != 0 || cycle.objectPresent {
			t.Errorf("cleanup cycle %d left %d rows, object present %t", index, cycle.rows, cycle.objectPresent)
		}
	}
	if refused.status != http.StatusTooManyRequests || refused.headers.Get("Retry-After") == "" || usage != 0 || len(remaining) != 0 {
		t.Fatalf("physical deletion refunded the daily allowance: %s quota %d remaining %d", describe(refused), usage, len(remaining))
	}
}

type dailyCleanupObservation struct {
	rows          int
	objectPresent bool
}

func (h *authHarness) consumeAndDeleteUpload(t *testing.T, parties dailyUploadParties, channel string, content []byte) dailyCleanupObservation {
	t.Helper()
	result := h.chatUpload(t, parties.studentToken, channel, "consumed.txt", content)
	h.mustAnswer(t, result, http.StatusCreated)
	row := h.onlyUploadBy(t, parties.student)
	message := h.sent(t, parties.studentToken, channel, "", row.ID)
	h.mustAnswer(t, h.deleteMessage(t, parties.studentToken, message.ID), http.StatusNoContent)
	h.tick(t)
	rows := len(h.uploadsBy(t, parties.student))
	_, present := h.storedObject(t, row.Key)
	return dailyCleanupObservation{rows: rows, objectPresent: present}
}

type dailyUploadParties struct {
	tutor        domain.UserID
	tutorToken   string
	student      domain.UserID
	studentToken string
	card         studentCard
}

func (h *authHarness) dailyUploadParties(t *testing.T) dailyUploadParties {
	t.Helper()
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	h.setAccount(t, tutor, endingTutorAccountName, domain.LanguageEnglish)
	h.setAccount(t, student, endingStudentName, domain.LanguageEnglish)
	card := h.linkedSignedIn(t, tutorToken, studentToken, cardName, studentEmail)
	return dailyUploadParties{tutor: tutor, tutorToken: tutorToken, student: student, studentToken: studentToken, card: card}
}
