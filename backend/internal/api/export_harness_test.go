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
