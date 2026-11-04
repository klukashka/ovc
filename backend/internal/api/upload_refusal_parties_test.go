package api_test

import (
	"fmt"
	"testing"

	"ovc/backend/internal/domain"
)

func (h *authHarness) uploadRefusalParties(t *testing.T, caseID byte) endingParties {
	t.Helper()
	tutorEmail := fmt.Sprintf("upload-%d-tutor@example.org", caseID)
	studentEmail := fmt.Sprintf("upload-%d-student@example.org", caseID)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2*caseID-1, tutorEmail)
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2*caseID, studentEmail)
	h.setAccount(t, tutor, endingTutorAccountName, domain.LanguageEnglish)
	h.setAccount(t, student, endingStudentName, domain.LanguageEnglish)
	card := h.linkedSignedIn(t, tutorToken, studentToken, cardName, studentEmail)
	group := h.newGroup(t, tutorToken, fmt.Sprintf("Upload refusal group %d", caseID), []string{card.ID})
	return endingParties{tutor: tutor, tutorToken: tutorToken, student: student, studentToken: studentToken, card: card, group: group}
}
