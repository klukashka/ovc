package api_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const exportsRoute = "/api/v1/exports"

type exportBody struct {
	ID            string     `json:"id"`
	State         string     `json:"state"`
	Failure       *string    `json:"failure"`
	BuiltAt       *time.Time `json:"built_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	SizeBytes     *int64     `json:"size_bytes"`
	NextAllowedAt *time.Time `json:"next_allowed_at"`
}

type currentExportBody struct {
	Export        *exportBody `json:"export"`
	NextAllowedAt *time.Time  `json:"next_allowed_at"`
}

type downloadBody struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (h *authHarness) requestExport(t *testing.T, token string, password string) response {
	t.Helper()
	return h.send(t, http.MethodPost, exportsRoute, fmt.Sprintf(`{"password":%q}`, password), jsonCall(token))
}

func (h *authHarness) currentExport(t *testing.T, token string) (response, currentExportBody) {
	t.Helper()
	result := h.send(t, http.MethodGet, exportsRoute+"/current", "", jsonCall(token))
	var body currentExportBody
	if result.status == http.StatusOK {
		if err := json.Unmarshal([]byte(result.body), &body); err != nil {
			t.Fatal(err)
		}
	}
	return result, body
}

func (h *authHarness) deleteExport(t *testing.T, token string) response {
	t.Helper()
	return h.send(t, http.MethodDelete, exportsRoute+"/current", "", jsonCall(token))
}

func (h *authHarness) mintExport(t *testing.T, token string) (response, downloadBody) {
	t.Helper()
	result := h.send(t, http.MethodPost, exportsRoute+"/current/download", "", jsonCall(token))
	var body downloadBody
	if result.status == http.StatusOK {
		if err := json.Unmarshal([]byte(result.body), &body); err != nil {
			t.Fatal(err)
		}
	}
	return result, body
}

func (h *authHarness) exportOf(t *testing.T, user domain.UserID) (repositories.Export, bool) {
	t.Helper()
	row, err := h.store.Repositories().Exports.OfUser(context.Background(), user)
	if errors.Is(err, repositories.ErrNotFound) {
		return row, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return row, true
}

func (h *authHarness) intentOf(t *testing.T, build domain.ExportBuildID) (repositories.UploadIntent, bool) {
	t.Helper()
	intent, err := h.store.Repositories().UploadIntents.ByID(context.Background(), build.Identity())
	if errors.Is(err, repositories.ErrNotFound) {
		return intent, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return intent, true
}

func (h *authHarness) exportCapacityUsed(t *testing.T) int32 {
	t.Helper()
	used, err := h.store.Repositories().UploadIntents.CapacityUsed(context.Background(), domain.UploadCapacityExport)
	if err != nil {
		t.Fatal(err)
	}
	return used
}

func (h *authHarness) fillExportCapacity(t *testing.T) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE upload_intent_capacity SET used = $1 WHERE capacity_class = 'export'", domain.ExportUploadIntentCapacity); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) openUploads(t *testing.T, storageKey string) int {
	t.Helper()
	uploads, err := h.objects.IncompleteUploads(context.Background(), storageKey)
	if err != nil {
		t.Fatal(err)
	}
	return len(uploads)
}

func (h *authHarness) outboxKinds(t *testing.T, kind domain.OutboxKind) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(kind))
}

func (h *authHarness) exportNotifications(t *testing.T, user domain.UserID, event domain.NotificationEvent) int64 {
	t.Helper()
	var count int64
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT count(id) FROM notifications WHERE user_id = $1 AND event = $2", user, string(event)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (h *authHarness) readyExport(t *testing.T, token string) {
	t.Helper()
	if requested := h.requestExport(t, token, accountPassword); requested.status != http.StatusAccepted {
		t.Fatalf("the export request answered %d %s", requested.status, requested.body)
	}
	h.tick(t)
	if _, current := h.currentExport(t, token); current.Export == nil || current.Export.State != string(domain.ExportReady) {
		t.Fatalf("the export did not become ready: %+v", current.Export)
	}
}

type downloadedArchive struct {
	status      int
	contentType string
	disposition string
	entries     map[string][]byte
}

func downloadArchive(t *testing.T, address string) downloadedArchive {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(answer.Body)
	if err := errors.Join(readErr, answer.Body.Close()); err != nil {
		t.Fatal(err)
	}
	result := downloadedArchive{status: answer.StatusCode, contentType: answer.Header.Get("Content-Type"), disposition: answer.Header.Get("Content-Disposition"), entries: map[string][]byte{}}
	if answer.StatusCode != http.StatusOK {
		return result
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(opened)
		if err := errors.Join(readErr, opened.Close()); err != nil {
			t.Fatal(err)
		}
		result.entries[file.Name] = content
	}
	return result
}

func (h *authHarness) unloadableTimezone(t *testing.T, user domain.UserID) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE users SET timezone = 'Nowhere/Unknown' WHERE id = $1", user); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) signInAgain(t *testing.T, email string) string {
	t.Helper()
	return accessToken(t, h.login(t, email, accountPassword))
}

const (
	firstExportEmail   = "first-export@example.org"
	secondExportEmail  = "second-export@example.org"
	outsiderTutorEmail = "outsider-tutor@example.org"
	exportGroupName    = "Export group"
	invalidatedRecord  = `"msg":"export.invalidated"`
)

type exportAccount struct {
	id       domain.UserID
	token    string
	email    string
	password string
}

type exportWorld struct {
	tutor      exportAccount
	first      exportAccount
	second     exportAccount
	outsider   exportAccount
	firstCard  studentCard
	secondCard studentCard
	group      studentGroup
}

func (h *authHarness) exportAccountAs(t *testing.T, role domain.Role, id byte, email string) exportAccount {
	t.Helper()
	user, token := h.signedInAs(t, role, domain.AccountActive, id, email)
	return exportAccount{id: user, token: token, email: email, password: accountPassword}
}

func (h *authHarness) exportWorld(t *testing.T) exportWorld {
	t.Helper()
	world := exportWorld{
		tutor:    h.exportAccountAs(t, domain.RoleTutor, 1, tutorEmail),
		first:    h.exportAccountAs(t, domain.RoleStudent, 2, firstExportEmail),
		second:   h.exportAccountAs(t, domain.RoleStudent, 3, secondExportEmail),
		outsider: h.exportAccountAs(t, domain.RoleTutor, 9, outsiderTutorEmail),
	}
	world.firstCard = h.linkedSignedIn(t, world.tutor.token, world.first.token, "First student", firstExportEmail)
	world.secondCard = h.linkedSignedIn(t, world.tutor.token, world.second.token, "Second student", secondExportEmail)
	world.group = h.newGroup(t, world.tutor.token, exportGroupName, []string{world.firstCard.ID, world.secondCard.ID})
	return world
}

func (h *authHarness) signedInAgain(t *testing.T, account exportAccount) exportAccount {
	t.Helper()
	account.token = accessToken(t, h.login(t, account.email, account.password))
	return account
}

func (h *authHarness) readyExportOf(t *testing.T, owner exportAccount) repositories.Export {
	t.Helper()
	if requested := h.requestExport(t, owner.token, owner.password); requested.status != http.StatusAccepted {
		t.Fatalf("the export request answered %d %s", requested.status, requested.body)
	}
	h.tick(t)
	row, found := h.exportOf(t, owner.id)
	if !found || row.State != domain.ExportReady || row.StorageKey == nil {
		t.Fatalf("the export of %s did not become ready: %s", owner.id, row.State)
	}
	return row
}

type heldBuild struct {
	row      repositories.Export
	held     *heldStorage
	building <-chan error
}

func (h *authHarness) holdBuildOf(t *testing.T, held *heldStorage, owner exportAccount) heldBuild {
	t.Helper()
	if requested := h.requestExport(t, owner.token, owner.password); requested.status != http.StatusAccepted {
		t.Fatalf("the export request answered %d %s", requested.status, requested.body)
	}
	held.holdNext(multipartPart)
	building := h.tickInBackground(t)
	held.awaitReached(t)
	row, found := h.exportOf(t, owner.id)
	if !found || row.State != domain.ExportBuilding || row.BuildID == nil || row.StorageKey == nil {
		t.Fatalf("the held export of %s is %s", owner.id, row.State)
	}
	return heldBuild{row: row, held: held, building: building}
}

func (b heldBuild) finish(t *testing.T, h *authHarness) {
	t.Helper()
	b.held.resume()
	awaitTick(t, b.building)
	h.tick(t)
}

type invalidationRecord struct {
	Message  string `json:"msg"`
	ExportID string `json:"export_id"`
	State    string `json:"state"`
	Cause    string `json:"cause"`
}

func (h *authHarness) invalidationsOf(t *testing.T, exportID domain.ExportID) []invalidationRecord {
	t.Helper()
	var found []invalidationRecord
	for _, line := range strings.Split(h.logs.String(), "\n") {
		if !strings.Contains(line, invalidatedRecord) {
			continue
		}
		var record invalidationRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record.ExportID == exportID.String() {
			found = append(found, record)
		}
	}
	return found
}

func (h *authHarness) exportLimitRows(t *testing.T, user domain.UserID) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(user_id) FROM export_limits WHERE user_id = $1::uuid", user.String())
}

func (h *authHarness) assertWithdrawn(t *testing.T, before repositories.Export, cause domain.ExportInvalidationCause, notified bool) {
	t.Helper()
	row, found := h.exportOf(t, before.UserID)
	if !found || row.ID != before.ID || row.State != domain.ExportFailed || row.Failure == nil || *row.Failure != domain.ExportFailureWithdrawn {
		t.Fatalf("the %s export of %s is %s, want it failed as withdrawn", before.State, before.UserID, row.State)
	}
	if before.StorageKey != nil && h.objectExists(t, *before.StorageKey) {
		t.Fatalf("the withdrawn export of %s kept its object", before.UserID)
	}
	if before.State == domain.ExportReady && (row.StorageKey != nil || h.exportLimitRows(t, before.UserID) != 0) {
		t.Fatalf("the withdrawn ready export of %s kept its key or its limit row", before.UserID)
	}
	if before.State == domain.ExportBuilding {
		if _, open := h.intentOf(t, *before.BuildID); open || h.openUploads(t, *before.StorageKey) != 0 {
			t.Fatalf("the withdrawn build of %s kept its intent or an open upload", before.UserID)
		}
	}
	want := int64(0)
	if notified {
		want = 1
	}
	if got := h.exportNotifications(t, before.UserID, domain.NotificationExportFailed); got != want {
		t.Fatalf("the owner %s got %d export.failed notifications, want %d", before.UserID, got, want)
	}
	if records := h.invalidationsOf(t, before.ID); len(records) != 1 || records[0].Cause != string(cause) || records[0].State != string(before.State) {
		t.Fatalf("the withdrawal of %s was recorded as %+v, want one %s record", before.UserID, records, cause)
	}
}

func (h *authHarness) assertStillReady(t *testing.T, before repositories.Export) {
	t.Helper()
	row, found := h.exportOf(t, before.UserID)
	if !found || row.ID != before.ID || row.State != domain.ExportReady || !h.objectExists(t, *before.StorageKey) {
		t.Fatalf("the export of %s, which nobody named, is %s", before.UserID, row.State)
	}
	if records := h.invalidationsOf(t, before.ID); len(records) != 0 || h.exportNotifications(t, before.UserID, domain.NotificationExportFailed) != 0 {
		t.Fatalf("the export of %s, which nobody named, was recorded as withdrawn %+v", before.UserID, records)
	}
}
