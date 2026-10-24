package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

func (h *authHarness) withAvatar(t *testing.T, token string) string {
	t.Helper()
	avatar := uploadedID(t, h.upload(t, token, "me.png", pngBytes(t, 4, 4)))
	if bound := h.patchMe(t, token, fmt.Sprintf(`{"avatar_id":%q}`, avatar)); bound.status != http.StatusOK {
		t.Fatalf("binding the avatar answered %d %s", bound.status, bound.body)
	}
	return avatar
}

func sameAvatar(got *string, want string) bool {
	return got != nil && *got == want
}

func TestEndedCardHidesADeletedStudentsAccountAndKeepsTheCardUntilThePurge(t *testing.T) {
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.withNotes(t, parties.tutorToken, parties.card.ID, cardNotes)
	avatar := h.withAvatar(t, parties.studentToken)
	before := h.cardOf(t, parties.tutorToken, parties.card.ID)

	deleted := h.deleteMe(t, parties.studentToken, accountPassword)
	shown := h.cardCall(t, parties.tutorToken, http.MethodGet, parties.card.ID, "", "")
	after := decodeCard(t, shown)
	listed := h.send(t, http.MethodGet, studentsPath, "", jsonCall(parties.tutorToken))
	_, restoreErr := h.operators().Restore(context.Background(), studentEmail)
	restored := h.cardOf(t, parties.tutorToken, parties.card.ID)

	if before.Account == nil || before.Account.Name != endingStudentName || before.Account.Email != studentEmail || !sameAvatar(before.Account.AvatarID, avatar) {
		t.Fatalf("before the deletion the card shows the account %+v", before.Account)
	}
	if deleted.status != http.StatusNoContent {
		t.Fatalf("the student's deletion answered %d %s", deleted.status, deleted.body)
	}
	if after.State != "ended" || after.Account != nil {
		t.Fatalf("after the deletion the card is %s with the account %+v", after.State, after.Account)
	}
	if after.DisplayName != cardName || after.Email == nil || *after.Email != studentEmail || after.Notes == nil || *after.Notes != cardNotes {
		t.Fatalf("the card lost its own name, email or notes: %+v", after)
	}
	if listed.status != http.StatusOK || !strings.Contains(listed.body, parties.card.ID) || !strings.Contains(listed.body, `"account":null`) {
		t.Fatalf("the student list answered %d %s", listed.status, listed.body)
	}
	for _, private := range []string{endingStudentName, avatar} {
		if strings.Contains(listed.body, private) || strings.Contains(shown.body, private) {
			t.Fatalf("the tutor still sees %q of the deleted account", private)
		}
	}
	if restoreErr != nil {
		t.Fatalf("the restore failed: %v", restoreErr)
	}
	if restored.Account == nil || restored.Account.Name != endingStudentName || restored.Account.Email != studentEmail || !sameAvatar(restored.Account.AvatarID, avatar) {
		t.Fatalf("after the restore the card shows the account %+v", restored.Account)
	}
}

func TestBlockedListHidesADeletedStudentsAccount(t *testing.T) {
	h := newFlowHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	h.setAccount(t, student, requestingName, domain.LanguageEnglish)
	avatar := h.withAvatar(t, token)
	request := h.enteredRequest(t, token, h.storedCode(t, tutor).Display())
	h.declineRequest(t, tutorToken, requestText(request), true)
	before := h.blockedStudents(t, tutorToken)

	deleted := h.deleteMe(t, token, accountPassword)
	after := h.blockedStudents(t, tutorToken)
	raw := h.send(t, http.MethodGet, blockedPath, "", jsonCall(tutorToken))

	if len(before) != 1 || before[0].Name == nil || *before[0].Name != requestingName || !sameAvatar(before[0].AvatarID, avatar) {
		t.Fatalf("before the deletion the blocked students are %+v", before)
	}
	if deleted.status != http.StatusNoContent {
		t.Fatalf("the student's deletion answered %d %s", deleted.status, deleted.body)
	}
	if len(after) != 1 || after[0].StudentID != student.String() || !after[0].BlockedAt.Equal(before[0].BlockedAt) {
		t.Fatalf("after the deletion the blocked students are %+v", after)
	}
	if after[0].Name != nil || after[0].AvatarID != nil || strings.Contains(raw.body, requestingName) || strings.Contains(raw.body, studentEmail) {
		t.Fatalf("the blocked list still shows the deleted account: %s", raw.body)
	}
}
