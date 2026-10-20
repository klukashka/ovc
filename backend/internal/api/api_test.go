package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/testdb"
)

type safeBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type harness struct {
	server  *httptest.Server
	handler http.Handler
	logging *core.Logging
	logs    *safeBuffer
}

func (h *harness) flushLogs() string {
	h.logging.Close()
	return h.logs.String()
}

func newHarness(t *testing.T, readiness api.ReadinessFunc, mount func(*http.ServeMux), docs bool) *harness {
	t.Helper()
	logs := &safeBuffer{}
	logging := core.NewLogging(logs, slog.LevelDebug, core.LogFormatJSON)
	handler, err := api.NewHandler(api.Options{
		Logger:       logging.Logger,
		Now:          clock.System{}.Now,
		NewRequestID: core.NewRandomID,
		Readiness:    readiness,
		DocsEnabled:  docs,
		Mount:        mount,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &harness{server: server, handler: handler, logging: logging, logs: logs}
}

type reply struct {
	StatusCode int
	Header     http.Header
}

func get(t *testing.T, h *harness, method string, path string, headers map[string]string) (reply, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), method, h.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return reply{StatusCode: response.StatusCode, Header: response.Header}, string(body)
}

type errorBody struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

func decodeError(t *testing.T, body string) errorBody {
	t.Helper()
	var decoded errorBody
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("not a structured error: %s", body)
	}
	return decoded
}

func TestHealthRoutesAgainstTheDatabase(t *testing.T) {
	database := testdb.New(t)
	h := newHarness(t, func(ctx context.Context) error { return core.Ping(ctx, database.Pool) }, nil, false)

	live, liveBody := get(t, h, http.MethodGet, "/api/v1/health/live", nil)
	ready, readyBody := get(t, h, http.MethodGet, "/api/v1/health/ready", nil)

	if live.StatusCode != http.StatusOK || strings.TrimSpace(liveBody) != `{"status":"ok"}` {
		t.Fatalf("live: %d %s", live.StatusCode, liveBody)
	}
	if ready.StatusCode != http.StatusOK || strings.TrimSpace(readyBody) != `{"status":"ready","checks":{"database":"ok"}}` {
		t.Fatalf("ready: %d %s", ready.StatusCode, readyBody)
	}
}

func TestReadyReportsUnavailableDatabaseWithoutDriverDetail(t *testing.T) {
	settings := core.Settings{DatabaseURL: core.NewSecret("postgres://ovc:pw@127.0.0.1:1/ovc_dev?connect_timeout=1"), DBPoolSize: 1}
	pool, err := core.OpenPool(context.Background(), settings, slog.New(slog.DiscardHandler), clock.System{}.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	h := newHarness(t, func(ctx context.Context) error { return core.Ping(ctx, pool) }, nil, false)

	response, body := get(t, h, http.MethodGet, "/api/v1/health/ready", nil)

	decoded := decodeError(t, body)
	if response.StatusCode != http.StatusServiceUnavailable || decoded.Error.Code != "service_unavailable" {
		t.Fatalf("unexpected: %d %s", response.StatusCode, body)
	}
	for _, leak := range []string{"dial", "connect", "127.0.0.1", "refused"} {
		if strings.Contains(body, leak) {
			t.Fatalf("body leaks %q: %s", leak, body)
		}
	}
	if decoded.Error.RequestID != response.Header.Get("X-Request-ID") {
		t.Fatalf("request id mismatch: %s vs %s", decoded.Error.RequestID, response.Header.Get("X-Request-ID"))
	}
}

func TestUnmatchedRoutesAndMethodsAreStructured(t *testing.T) {
	h := newHarness(t, nil, nil, false)

	missing, missingBody := get(t, h, http.MethodGet, "/nope?token=sentinel", nil)
	method, methodBody := get(t, h, http.MethodPost, "/api/v1/health/live", nil)
	testRoute, testBody := get(t, h, http.MethodPost, "/__test__/clock", nil)

	if missing.StatusCode != http.StatusNotFound || decodeError(t, missingBody).Error.Code != "not_found" {
		t.Fatalf("missing: %d %s", missing.StatusCode, missingBody)
	}
	if method.StatusCode != http.StatusMethodNotAllowed || decodeError(t, methodBody).Error.Code != "method_not_allowed" {
		t.Fatalf("method: %d %s", method.StatusCode, methodBody)
	}
	if testRoute.StatusCode != http.StatusNotFound || decodeError(t, testBody).Error.Code != "not_found" {
		t.Fatalf("test route without the flag: %d %s", testRoute.StatusCode, testBody)
	}
	if missing.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content type: %s", missing.Header.Get("Content-Type"))
	}
	logs := h.flushLogs()
	if !strings.Contains(logs, `"route":"unmatched"`) || strings.Contains(logs, "sentinel") || strings.Contains(logs, "/nope") {
		t.Fatalf("access records must use the placeholder and no raw path: %s", logs)
	}
}

func TestRequestIDIsKeptOnlyWhenValid(t *testing.T) {
	h := newHarness(t, nil, nil, false)
	cases := map[string]struct {
		values []string
		kept   bool
	}{
		"valid":            {values: []string{"abc.DEF_123-x"}, kept: true},
		"spaces":           {values: []string{"has space"}},
		"line break":       {values: []string{"abc\r\nInjected: yes"}},
		"too long":         {values: []string{strings.Repeat("a", 65)}},
		"duplicate header": {values: []string{"one", "two"}},
		"non ascii":        {values: []string{"идентификатор"}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/health/live", nil)
			request.Header["X-Request-Id"] = testCase.values
			recorder := httptest.NewRecorder()

			h.handler.ServeHTTP(recorder, request)

			got := recorder.Header().Get("X-Request-ID")
			if testCase.kept && got != testCase.values[0] {
				t.Fatalf("expected %q kept, got %q", testCase.values[0], got)
			}
			if !testCase.kept && (got == "" || got == testCase.values[0] || strings.ContainsAny(got, " \r\n")) {
				t.Fatalf("expected a generated id, got %q", got)
			}
		})
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	h := newHarness(t, nil, nil, false)

	ok, _ := get(t, h, http.MethodGet, "/api/v1/health/live", nil)
	missing, _ := get(t, h, http.MethodGet, "/nope", nil)

	for _, response := range []reply{ok, missing} {
		if response.Header.Get("Referrer-Policy") != "no-referrer" || response.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("missing security headers: %v", response.Header)
		}
	}
}

func TestPanicsAreRecoveredWithoutTheirValue(t *testing.T) {
	mount := func(mux *http.ServeMux) {
		mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) {
			panic(errors.New("sentinel-panic-value"))
		})
	}
	h := newHarness(t, nil, mount, false)

	response, body := get(t, h, http.MethodGet, "/boom", nil)

	decoded := decodeError(t, body)
	if response.StatusCode != http.StatusInternalServerError || decoded.Error.Code != "internal" || decoded.Error.RequestID == "" {
		t.Fatalf("unexpected: %d %s", response.StatusCode, body)
	}
	logs := h.flushLogs()
	if !strings.Contains(logs, "panic.recovered") || !strings.Contains(logs, `"panic_type":"*errors.errorString"`) {
		t.Fatalf("expected a panic record: %s", logs)
	}
	if strings.Contains(logs, "sentinel-panic-value") {
		t.Fatalf("the panic value leaked into the logs: %s", logs)
	}
	if !strings.Contains(logs, decoded.Error.RequestID) {
		t.Fatalf("the panic record lacks the request id")
	}
}

func TestPanicAfterTheResponseStartedAbortsTheConnection(t *testing.T) {
	mount := func(mux *http.ServeMux) {
		mux.HandleFunc("GET /half", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(strings.Repeat("x", 64*1024))); err != nil {
				return
			}
			panic(errors.New("late"))
		})
	}
	h := newHarness(t, nil, mount, false)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, h.server.URL+"/half", nil)
	if err != nil {
		t.Fatal(err)
	}

	response, err := http.DefaultClient.Do(request)
	readErr := error(nil)
	if err == nil {
		_, readErr = io.ReadAll(response.Body)
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}

	if err == nil && readErr == nil {
		t.Fatal("expected the connection to be aborted")
	}
}

func TestAccessRecordsUseRouteTemplates(t *testing.T) {
	h := newHarness(t, nil, nil, false)

	get(t, h, http.MethodGet, "/api/v1/health/live", nil)

	logs := h.flushLogs()
	if !strings.Contains(logs, `"route":"GET /api/v1/health/live"`) || !strings.Contains(logs, `"level":"DEBUG","msg":"http.request.completed"`) {
		t.Fatalf("expected a DEBUG access record with the route template: %s", logs)
	}
}

func TestValidationErrorsNeverEchoSubmittedValues(t *testing.T) {
	newHarness(t, nil, nil, false)

	statusError := huma.NewError(http.StatusUnprocessableEntity, "sentinel-message",
		&huma.ErrorDetail{Message: "sentinel-detail", Location: "body.name", Value: "sentinel-password"},
		&huma.ErrorDetail{Location: "body.<script>alert(1)</script>", Value: 1},
	)
	encoded, err := json.Marshal(statusError)

	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if statusError.GetStatus() != http.StatusUnprocessableEntity || strings.Contains(text, "sentinel") || strings.Contains(text, "script") {
		t.Fatalf("unexpected error body: %s", text)
	}
	if !strings.Contains(text, `"path":"body.name"`) || !strings.Contains(text, `"path":"body"`) || !strings.Contains(text, `"code":"validation_failed"`) {
		t.Fatalf("unexpected fields: %s", text)
	}
}

func TestOpenAPIDocumentAndDocsRoute(t *testing.T) {
	document, err := api.OpenAPIDocument()
	if err != nil {
		t.Fatal(err)
	}
	disabled := newHarness(t, nil, nil, false)
	enabled := newHarness(t, nil, nil, true)

	off, _ := get(t, disabled, http.MethodGet, "/api/docs/openapi.json", nil)
	on, onBody := get(t, enabled, http.MethodGet, "/api/docs/openapi.json", nil)

	text := string(document)
	if !strings.Contains(text, "/api/v1/health/ready") || strings.Contains(text, "__test__") || strings.Contains(text, "$schema") {
		t.Fatalf("unexpected document: %s", text)
	}
	if off.StatusCode != http.StatusNotFound || on.StatusCode != http.StatusOK || !strings.Contains(onBody, "/api/v1/health/live") {
		t.Fatalf("docs route: off %d, on %d", off.StatusCode, on.StatusCode)
	}
}
