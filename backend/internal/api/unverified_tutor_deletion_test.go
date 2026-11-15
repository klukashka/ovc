package api_test

import (
	"net/http"
	"testing"

	"ovc/backend/internal/domain"
)

type tutorLibraryRows struct {
	folders   int64
	materials int64
	templates int64
	charges   int64
	usage     int64
}

func (h *authHarness) tutorLibraryRows(t *testing.T, tutor domain.UserID) tutorLibraryRows {
	t.Helper()
	id := tutor.String()
	return tutorLibraryRows{
		folders:   h.countRows(t, "SELECT count(id) FROM folders WHERE tutor_id = $1::uuid", id),
		materials: h.countRows(t, "SELECT count(id) FROM materials WHERE tutor_id = $1::uuid", id),
		templates: h.countRows(t, "SELECT count(id) FROM templates WHERE tutor_id = $1::uuid", id),
		charges:   h.countRows(t, "SELECT count(attachment_id) FROM quota_charges WHERE tutor_id = $1::uuid", id),
		usage:     h.countRows(t, "SELECT COALESCE(sum(file_usage_bytes), 0) FROM tutor_profiles WHERE user_id = $1::uuid", id),
	}
}

func (h *authHarness) seedTutorLibrary(t *testing.T, tutor domain.UserID, token string, name string) string {
	t.Helper()
	folder := h.createFolder(t, token, name, nil)
	file := h.pendingLibraryFile(t, tutor, 61, tutor[0], 64)
	h.createMaterial(t, token, file, "Worksheet", &folder.ID)
	h.createTemplate(t, token, name+" template", &folder.ID, workBlocks()...)
	return file
}

func TestUnverifiedTutorDeletionPurgesLibrary(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)
	other, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, memberEmail(2))
	file := h.seedTutorLibrary(t, tutor, token, "Unit")
	h.seedTutorLibrary(t, other, otherToken, "Other unit")
	otherBefore := h.tutorLibraryRows(t, other)

	deleted := h.deleteMe(t, token, accountPassword)
	h.tick(t)

	h.mustAnswer(t, deleted, http.StatusNoContent)
	if h.userExists(t, tutorEmail) {
		t.Fatal("the unverified tutor was not purged at once")
	}
	if left := h.tutorLibraryRows(t, tutor); left != (tutorLibraryRows{}) {
		t.Fatalf("the unverified tutor's library survived: %+v", left)
	}
	if h.uploadState(t, file) != "gone" {
		t.Fatalf("the unverified tutor's library file is %s", h.uploadState(t, file))
	}
	if after := h.tutorLibraryRows(t, other); after != otherBefore || after.folders != 1 || after.materials != 1 || after.templates != 1 || after.charges == 0 {
		t.Fatalf("another tutor's library changed from %+v to %+v", otherBefore, after)
	}
}

func TestUnverifiedTutorDeletionBatchBudgetIsRetryable(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)
	h.createFolder(t, token, "Unit", nil)
	h.seedInvitedCards(t, tutor, 10*500+1)

	first := h.deleteMe(t, token, accountPassword)
	stillPresent := h.userExists(t, tutorEmail)
	second := h.deleteMe(t, token, accountPassword)

	assertRefused(t, "unverified tutor deletion batch budget", first, http.StatusServiceUnavailable, "service_unavailable")
	if first.headers.Get("Retry-After") != "1" || !stillPresent {
		t.Fatalf("the first attempt lost the account or its retry hint: present=%t retry=%q", stillPresent, first.headers.Get("Retry-After"))
	}
	h.mustAnswer(t, second, http.StatusNoContent)
	if left := h.tutorRows(t, tutor); len(left) != 0 || h.userExists(t, tutorEmail) {
		t.Fatalf("the retry did not finish the purge: %v", left)
	}
}
