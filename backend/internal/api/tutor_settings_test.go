package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"ovc/backend/internal/domain"
)

const tutorSettingsRoute = "/api/v1/tutor/settings"

type tutorSettings struct {
	FileQuotaBytes *int64 `json:"file_quota_bytes"`
	FileUsageBytes *int64 `json:"file_usage_bytes"`
}

func (h *authHarness) tutorSettings(t *testing.T, bearerToken string) (response, tutorSettings) {
	t.Helper()
	result := h.send(t, http.MethodGet, tutorSettingsRoute, "", jsonCall(bearerToken))
	var settings tutorSettings
	if result.status == http.StatusOK {
		if err := json.Unmarshal([]byte(result.body), &settings); err != nil {
			t.Fatalf("not tutor settings: %s", result.body)
		}
	}
	return result, settings
}

func reportsQuotaAndUsage(result response, settings tutorSettings, quota int64, usage int64) bool {
	return result.status == http.StatusOK && result.headers.Get("Cache-Control") == "no-store" &&
		settings.FileQuotaBytes != nil && *settings.FileQuotaBytes == quota && settings.FileUsageBytes != nil && *settings.FileUsageBytes == usage
}

func (h *authHarness) setFileQuota(t *testing.T, tutor domain.UserID, quota int64) {
	t.Helper()
	changed, err := h.store.Repositories().TutorProfiles.SetQuotas(context.Background(), tutor, quota, 50_000_000_000, authStart)
	if err != nil || !changed {
		t.Fatalf("the quota was not set: %v", err)
	}
}

func TestTutorSettingsReportFileQuotaAndUsage(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	secondTutor, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, secondTutorEmail)
	_, unverifiedToken := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 4, "unconfirmed@example.org")
	shared := h.linkedSignedIn(t, secondToken, parties.studentToken, cardName, studentEmail)
	h.setFileQuota(t, parties.tutor, 1_000_000)
	direct := h.directChannel(t, parties.card.ID)
	secondDirect := h.directChannel(t, shared.ID)
	h.sent(t, parties.studentToken, direct.ID, "First", h.pendingUpload(t, parties.studentToken, direct.ID, "first.txt", 100))
	h.sent(t, parties.studentToken, secondDirect.ID, "Second", h.pendingUpload(t, parties.studentToken, secondDirect.ID, "second.txt", 70))
	h.pendingUpload(t, parties.tutorToken, direct.ID, "unsent.txt", 50)

	firstResult, firstSettings := h.tutorSettings(t, parties.tutorToken)
	secondResult, secondSettings := h.tutorSettings(t, secondToken)
	unverifiedResult, unverifiedSettings := h.tutorSettings(t, unverifiedToken)

	if !reportsQuotaAndUsage(firstResult, firstSettings, 1_000_000, h.usage(t, parties.tutor)) || h.usage(t, parties.tutor) != 100 {
		t.Fatalf("the first tutor's settings answered %s", describe(firstResult))
	}
	if !reportsQuotaAndUsage(secondResult, secondSettings, 5_000_000_000, h.usage(t, secondTutor)) || h.usage(t, secondTutor) != 70 {
		t.Fatalf("the second tutor's settings answered %s", describe(secondResult))
	}
	if !reportsQuotaAndUsage(unverifiedResult, unverifiedSettings, 5_000_000_000, 0) {
		t.Fatalf("the unconfirmed tutor's settings answered %s", describe(unverifiedResult))
	}
}

func TestTutorSettingsAreNotFoundForStudents(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, active := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	_, unverified := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 3, "unconfirmed@example.org")
	unknown := h.unknownRouteRefusal(t)

	activeResult, _ := h.tutorSettings(t, active)
	unverifiedResult, _ := h.tutorSettings(t, unverified)

	for who, result := range map[string]response{"an active student": activeResult, "an unverified student": unverifiedResult} {
		if result.status != http.StatusNotFound || refusal(t, result) != unknown {
			t.Errorf("the tutor settings called by %s answered %s", who, describe(result))
		}
	}
}
