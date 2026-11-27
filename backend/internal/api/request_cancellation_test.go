package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/testclock"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

type cancellationRecord struct {
	Message   string `json:"msg"`
	Level     string `json:"level"`
	Kind      string `json:"kind"`
	RequestID string `json:"request_id"`
	Route     string `json:"route"`
	Status    int    `json:"status"`
	Aborted   *bool  `json:"aborted"`
}

func cancellationRecords(t *testing.T, logs string, requestID string) []cancellationRecord {
	t.Helper()
	var records []cancellationRecord
	for line := range strings.Lines(logs) {
		var record cancellationRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record.RequestID == requestID {
			records = append(records, record)
		}
	}
	return records
}

func awaitCancelledAccess(t *testing.T, h *authHarness, queryLogs *safeBuffer, requestID string) []cancellationRecord {
	t.Helper()
	deadline := time.NewTimer(pauseDeadline)
	defer deadline.Stop()
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	for {
		records := cancellationRecords(t, h.logs.String()+queryLogs.String(), requestID)
		for _, record := range records {
			if record.Message == "http.request.completed" {
				return records
			}
		}
		select {
		case <-deadline.C:
			t.Fatalf("no access record for %s: %+v", requestID, records)
		case <-poll.C:
		}
	}
}

func sendAbortable(request *http.Request) <-chan error {
	result := make(chan error, 1)
	go func() {
		answer, err := http.DefaultClient.Do(request)
		if err == nil {
			err = answer.Body.Close()
		}
		result <- err
	}()
	return result
}

func awaitClientCancellation(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(pauseDeadline):
		t.Fatal("the cancelled client did not finish")
		return nil
	}
}

func cancellationRequest(t *testing.T, ctx context.Context, serverURL, method, path, body, token string) *http.Request {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, method, serverURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range jsonCall(token) {
		request.Header.Set(key, value)
	}
	request.Header.Set("X-Request-ID", "client-cancelled-request")
	return request
}

func assertClientCancellation(t *testing.T, clientErr error, records []cancellationRecord, route, expectedEvent string) {
	t.Helper()
	if !errors.Is(clientErr, context.Canceled) {
		t.Errorf("client error = %v", clientErr)
	}
	accessCount, expectedCount := 0, 0
	for _, record := range records {
		if record.Level == "ERROR" || record.Level == "WARN" {
			t.Errorf("a cancelled request logged a server failure: %+v", record)
		}
		if record.Message == expectedEvent && record.Kind == "canceled" {
			expectedCount++
		}
		switch record.Message {
		case "http.request.completed":
			accessCount++
			if record.Aborted == nil || !*record.Aborted || record.Level != "INFO" || record.Route != route || record.Status != http.StatusServiceUnavailable {
				t.Errorf("incorrect access record: %+v", record)
			}
		case "db.query.failed", "auth.authenticate.failed":
			if record.Level != "DEBUG" {
				t.Errorf("a cancelled operation was not DEBUG: %+v", record)
			}
		case "auth.login.failed":
			t.Error("cancellation counted as a failed sign-in")
		}
	}
	if accessCount != 1 || expectedCount == 0 {
		t.Errorf("missing or duplicated cancellation records: %+v", records)
	}
}

func TestAClientCancelledRequestIsLoggedAsAbortedNotAsAServerError(t *testing.T) {
	t.Run("a query waiting on a lock", func(t *testing.T) {
		t.Parallel()
		h, queryLogs := newCancellationHarness(t)
		tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		group := h.newGroup(t, token, groupName, h.activeCards(t, tutor, 10, 1))
		t.Cleanup(h.holdGroup(t, group.ID))
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		request := cancellationRequest(t, ctx, h.server.URL, http.MethodPatch, groupsRoute+"/"+group.ID, `{"name":"Changed group"}`, token)
		result := sendAbortable(request)
		h.waitForLockWaiters(t, 1)

		cancel()
		clientErr := awaitClientCancellation(t, result)
		records := awaitCancelledAccess(t, h, queryLogs, "client-cancelled-request")

		assertClientCancellation(t, clientErr, records, "PATCH /api/v1/groups/{id}", "db.query.failed")
	})
	t.Run("during authentication", func(t *testing.T) {
		t.Parallel()
		h, queryLogs := newCancellationHarness(t)
		_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		pause := h.authenticationClock.PauseNextInstant()
		t.Cleanup(pause.Resume)
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		request := cancellationRequest(t, ctx, h.server.URL, http.MethodGet, "/api/v1/users/me", "", token)
		result := sendAbortable(request)
		awaitAuthenticationPause(t, pause)

		cancel()
		clientErr := awaitClientCancellation(t, result)
		records := awaitCancelledAccess(t, h, queryLogs, "client-cancelled-request")

		assertClientCancellation(t, clientErr, records, "GET /api/v1/users/me", "auth.authenticate.failed")
		failures := 0
		for _, record := range records {
			if record.Message == "auth.authenticate.failed" {
				failures++
			}
		}
		if failures != 1 {
			t.Errorf("authentication failures = %d", failures)
		}
	})
}

func awaitAuthenticationPause(t *testing.T, pause *testclock.Pause) {
	t.Helper()
	select {
	case <-pause.Reached():
	case <-time.After(pauseDeadline):
		t.Fatal("authentication did not reach the clock")
	}
}

func TestAuthenticationReadDeadlineIsNotAClientCancellation(t *testing.T) {
	t.Parallel()
	h, queryLogs := newCancellationHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	options := h.options
	options.ReadDeadline = 100 * time.Millisecond
	handler, err := api.NewHandler(options)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	pause := h.authenticationClock.PauseNextInstant()
	t.Cleanup(pause.Resume)
	ctx, cancel := context.WithTimeout(t.Context(), pauseDeadline)
	t.Cleanup(cancel)
	request := cancellationRequest(t, ctx, server.URL, http.MethodGet, "/api/v1/users/me", "", token)

	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	records := awaitCancelledAccess(t, h, queryLogs, "client-cancelled-request")

	defer func() {
		if err := answer.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	if answer.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d", answer.StatusCode)
	}
	accessCount, failures := 0, 0
	for _, record := range records {
		if record.Aborted != nil {
			t.Errorf("server deadline marked aborted: %+v", record)
		}
		switch record.Message {
		case "auth.authenticate.failed":
			failures++
			if record.Level != "ERROR" || record.Kind != "canceled" {
				t.Errorf("incorrect authentication record: %+v", record)
			}
		case "db.query.failed":
			if record.Level != "WARN" {
				t.Errorf("server deadline lowered query level: %+v", record)
			}
		case "http.request.completed":
			accessCount++
			if record.Level != "INFO" || record.Status != http.StatusInternalServerError {
				t.Errorf("incorrect access record: %+v", record)
			}
		}
	}
	if accessCount != 1 || failures != 1 {
		t.Errorf("missing or duplicated deadline records: %+v", records)
	}
}

func newCancellationHarness(t *testing.T) (*authHarness, *safeBuffer) {
	t.Helper()
	database := testdb.NewForLockContention(t)
	queryLogs := &safeBuffer{}
	logger := slog.New(core.NewContextHandler(slog.NewJSONHandler(queryLogs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	pool, err := core.OpenPool(t.Context(), core.Settings{DatabaseURL: database.URL, DBPoolSize: testdb.DefaultPoolConnections}, logger, clock.System{}.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	traced := &testdb.Database{Name: database.Name, URL: database.URL, Pool: pool}
	h := newAuthHarnessWithDatabase(t, traced, flowProxies(), api.NewHandler, func(local *teststorage.Storage) storage.Store { return local.Store })
	return h, queryLogs
}
