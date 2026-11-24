package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func TestExportRequestNeedsAConfirmedEmail(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, "unconfirmed@example.org")

	requested := h.requestExport(t, token, accountPassword)

	if requested.status != http.StatusForbidden || code(t, requested) != string(domain.ErrorEmailNotVerified) {
		t.Fatalf("an unconfirmed account's request answered %d %s", requested.status, requested.body)
	}
}

func TestExportRequestRefusesAWrongPasswordLikeAPasswordChange(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)

	wrong := h.requestExport(t, token, "not the password")

	_, current := h.currentExport(t, token)
	if wrong.status != http.StatusUnprocessableEntity || !strings.Contains(strings.Join(fields(t, wrong), ","), "body.password:incorrect") {
		t.Fatalf("a wrong password answered %d %s", wrong.status, wrong.body)
	}
	if _, found := h.exportOf(t, user); found || current.Export != nil {
		t.Fatal("a refused request created an export")
	}
}

func TestExportRequestSharesTheCurrentPasswordCounter(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, student := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	for range 5 {
		h.send(t, http.MethodPost, "/api/v1/auth/password/change", `{"current_password":"wrong password","new_password":"another long password"}`, jsonCall(tutor))
		h.requestExport(t, student, "wrong password")
	}

	afterChanges := h.requestExport(t, tutor, accountPassword)
	afterExports := h.send(t, http.MethodPost, "/api/v1/auth/password/change", `{"current_password":"`+accountPassword+`","new_password":"another long password"}`, jsonCall(student))

	if afterChanges.status != http.StatusTooManyRequests || afterChanges.headers.Get("Retry-After") == "" {
		t.Fatalf("an export request after five wrong password changes answered %d %s", afterChanges.status, afterChanges.body)
	}
	if afterExports.status != http.StatusTooManyRequests {
		t.Fatalf("a password change after five wrong export passwords answered %d %s", afterExports.status, afterExports.body)
	}
}

func TestExportRequestIsQueuedAndEmailsTheAccount(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)

	requested := h.requestExport(t, token, accountPassword)
	h.tick(t)

	var created exportBody
	if err := json.Unmarshal([]byte(requested.body), &created); err != nil {
		t.Fatal(err)
	}
	if requested.status != http.StatusAccepted || created.State != string(domain.ExportQueued) || requested.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the request answered %d %v %s", requested.status, requested.headers, requested.body)
	}
	messages := h.mailTo(t, tutorEmail)
	if len(messages) != 1 || len(messages[0].Links) != 1 || messages[0].Links[0] != publicOrigin+"/settings/data" {
		t.Fatalf("the request emails = %+v; want one email linking only to My data", messages)
	}
	if _, found := h.exportOf(t, user); !found {
		t.Fatal("the request created no export")
	}
}

func TestExportRequestRefusesWhileOneIsInProgress(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.requestExport(t, token, accountPassword)

	second := h.requestExport(t, token, accountPassword)

	if second.status != http.StatusConflict || code(t, second) != string(domain.ErrorExportInProgress) {
		t.Fatalf("a request while one is queued answered %d %s", second.status, second.body)
	}
}

func TestExportBuildPublishesAReadyArchive(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.requestExport(t, token, accountPassword)

	h.tick(t)

	read, current := h.currentExport(t, token)
	row, _ := h.exportOf(t, user)
	if read.headers.Get("Cache-Control") != "no-store" || current.Export == nil || current.Export.State != string(domain.ExportReady) ||
		current.Export.SizeBytes == nil || current.Export.ExpiresAt == nil || !current.Export.ExpiresAt.Equal(current.Export.BuiltAt.Add(72*time.Hour)) {
		t.Fatalf("the built export reads %v %s", read.headers, read.body)
	}
	if _, open := h.intentOf(t, *row.BuildID); open || h.exportCapacityUsed(t) != 0 || h.exportNotifications(t, user, domain.NotificationExportReady) != 1 {
		t.Fatal("publication kept its intent or capacity, or sent no ready notification")
	}
	if current.NextAllowedAt == nil || !current.NextAllowedAt.Equal(authStart.Add(24*time.Hour)) {
		t.Fatalf("the next start is allowed at %v; want 24 hours after the request", current.NextAllowedAt)
	}
}

func TestExportNotificationLinksToMyData(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.readyExport(t, token)

	listed := h.send(t, http.MethodGet, "/api/v1/notifications", "", jsonCall(token))

	if listed.status != http.StatusOK || !strings.Contains(listed.body, `"event":"export.ready"`) || !strings.Contains(listed.body, `"kind":"my_data"`) {
		t.Fatalf("the notifications read %d %s", listed.status, listed.body)
	}
}

func TestExportArchiveHoldsTheOwnAccountData(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.readyExport(t, token)
	_, minted := h.mintExport(t, token)

	archive := downloadArchive(t, minted.URL)

	var data struct {
		FormatVersion int    `json:"format_version"`
		Role          string `json:"role"`
		Account       struct {
			Email string `json:"email"`
		} `json:"account"`
	}
	if err := json.Unmarshal(archive.entries["data.json"], &data); err != nil {
		t.Fatal(err)
	}
	if len(archive.entries) != 3 || archive.entries["tables.xlsx"] == nil || !strings.HasPrefix(string(archive.entries["calendar.ics"]), "BEGIN:VCALENDAR\r\n") {
		t.Fatalf("the archive holds %d entries", len(archive.entries))
	}
	if data.FormatVersion != 1 || data.Role != string(domain.RoleTutor) || data.Account.Email != tutorEmail {
		t.Fatalf("the data copy holds %+v", data)
	}
}

func TestExportDownloadIsASavedZipFromTheExportEndpoint(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.readyExport(t, token)

	minted, download := h.mintExport(t, token)
	archive := downloadArchive(t, download.URL)

	address, err := url.Parse(download.URL)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := url.Parse(h.storage.Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if minted.status != http.StatusOK || minted.headers.Get("Cache-Control") != "no-store" || address.Host != endpoint.Host || address.Scheme != endpoint.Scheme {
		t.Fatalf("minting answered %d %v with an address on %s", minted.status, minted.headers, address.Host)
	}
	if archive.status != http.StatusOK || archive.contentType != "application/zip" ||
		archive.disposition != `attachment; filename="ovc-export-`+authStart.Format(time.DateOnly)+`.zip"` {
		t.Fatalf("the download answered %d with %q and %q", archive.status, archive.contentType, archive.disposition)
	}
	if !download.ExpiresAt.Equal(authStart.Add(5 * time.Minute)) {
		t.Fatalf("the address expires at %v; want five minutes after minting", download.ExpiresAt)
	}
}

func TestExportDownloadIsRefusedUntilReadyAndAfterExpiry(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	missing, _ := h.mintExport(t, token)
	h.requestExport(t, token, accountPassword)
	queued, _ := h.mintExport(t, token)
	h.tick(t)
	h.at(t, authStart.Add(72*time.Hour))
	later := h.signInAgain(t, studentEmail)

	expired, _ := h.mintExport(t, later)

	for name, result := range map[string]response{"no export": missing, "queued": queued, "past expiry": expired} {
		if result.status != http.StatusNotFound {
			t.Errorf("minting with %s answered %d %s", name, result.status, result.body)
		}
	}
}

func TestExportDownloadMintsAreLimitedPerHour(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.readyExport(t, token)
	for range domain.ExportDownloadLimit {
		if minted, _ := h.mintExport(t, token); minted.status != http.StatusOK {
			t.Fatalf("a mint within the limit answered %d", minted.status)
		}
	}

	refused, _ := h.mintExport(t, token)

	if refused.status != http.StatusTooManyRequests || refused.headers.Get("Retry-After") == "" {
		t.Fatalf("the thirty-first mint answered %d %v", refused.status, refused.headers)
	}
}

func TestExportRequestWaits24HoursAfterAReadyExport(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.readyExport(t, token)
	h.at(t, authStart.Add(23*time.Hour))
	early := h.requestExport(t, h.signInAgain(t, tutorEmail), accountPassword)
	h.at(t, authStart.Add(24*time.Hour))

	allowed := h.requestExport(t, h.signInAgain(t, tutorEmail), accountPassword)

	if early.status != http.StatusTooManyRequests || early.headers.Get("Retry-After") != "3600" {
		t.Fatalf("a request 23 hours later answered %d %v", early.status, early.headers)
	}
	if allowed.status != http.StatusAccepted {
		t.Fatalf("a request 24 hours later answered %d %s", allowed.status, allowed.body)
	}
}

func TestExportRequestsAreLimitedToFivePerDay(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	for range domain.ExportRequestLimit {
		if requested := h.requestExport(t, token, accountPassword); requested.status != http.StatusAccepted {
			t.Fatalf("a request within the limit answered %d %s", requested.status, requested.body)
		}
		h.deleteExport(t, token)
	}

	sixth := h.requestExport(t, token, accountPassword)

	if sixth.status != http.StatusTooManyRequests || sixth.headers.Get("Retry-After") == "" {
		t.Fatalf("the sixth request in a day answered %d %s", sixth.status, sixth.body)
	}
}

func TestExportRequestAfterAFailedExportIsAccepted(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.unloadableTimezone(t, user)
	h.requestExport(t, token, accountPassword)
	h.tick(t)

	again := h.requestExport(t, token, accountPassword)

	if again.status != http.StatusAccepted {
		t.Fatalf("a request after a failed export answered %d %s", again.status, again.body)
	}
}

func TestExportCapacityFullRefusesRequestsAndKeepsWaitingExportsQueued(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, other := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	h.requestExport(t, token, accountPassword)
	h.fillExportCapacity(t)

	refused := h.requestExport(t, other, "not even the password")
	h.tick(t)

	if refused.status != http.StatusServiceUnavailable || refused.headers.Get("Retry-After") != "60" {
		t.Fatalf("a request with the class full answered %d %v %s", refused.status, refused.headers, refused.body)
	}
	if row, _ := h.exportOf(t, user); row.State != domain.ExportQueued || row.BuildID != nil {
		t.Fatalf("a waiting export with the class full is %s", row.State)
	}
}

func TestExportBuildsUseDistinctKeysAndANewRequestDeletesThePreviousObject(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.readyExport(t, token)
	first, _ := h.exportOf(t, user)
	h.at(t, authStart.Add(24*time.Hour))
	later := h.signInAgain(t, tutorEmail)

	h.readyExport(t, later)
	h.tick(t)

	second, _ := h.exportOf(t, user)
	if *first.StorageKey == *second.StorageKey || *first.BuildID == *second.BuildID {
		t.Fatal("two builds of one user shared a key or a build identifier")
	}
	if h.objectExists(t, *first.StorageKey) || !h.objectExists(t, *second.StorageKey) {
		t.Fatal("the new request did not delete the previous object, or lost the new one")
	}
}

func TestExportExpiresAfterThreeDaysAndItsRowGoesThreeDaysLater(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	h.readyExport(t, token)
	ready, _ := h.exportOf(t, user)
	h.at(t, authStart.Add(72*time.Hour))

	h.tick(t)
	h.tick(t)
	expired, _ := h.exportOf(t, user)
	h.at(t, authStart.Add(144*time.Hour))
	h.tick(t)

	if expired.State != domain.ExportExpired || expired.StorageKey != nil || h.objectExists(t, *ready.StorageKey) {
		t.Fatalf("after three days the export is %s with its object kept %v", expired.State, h.objectExists(t, *ready.StorageKey))
	}
	if _, found := h.exportOf(t, user); found {
		t.Fatal("the expired export's row outlived three more days")
	}
}
