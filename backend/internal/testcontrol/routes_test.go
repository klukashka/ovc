package testcontrol_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/notifications"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/testcontrol"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/worker"
)

const greetingKind domain.OutboxKind = "test.greeting"

type greeting struct{}

func (greeting) Build(_ context.Context, row repositories.ClaimedOutboxRow, _ time.Time) (notifications.Composition, error) {
	var payload struct {
		To string `json:"to"`
	}
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		return notifications.Composition{}, err
	}
	return notifications.Composition{Email: notifications.Email{
		To: payload.To, Subject: "Hello", Text: "Open https://ovc.example/home", HTML: "<p>Hello</p>",
	}, Recipient: notifications.NoAccountRecipient()}, nil
}

func newServer(t *testing.T) (*httptest.Server, *testdb.Database) {
	t.Helper()
	database := testdb.New(t)
	logger := slog.New(slog.DiscardHandler)
	shared := clock.NewShared(database.Pool)
	random := &countingRandom{}
	sink := mail.NewSink(database.Pool, random)
	notifier, err := notifications.New(sink, "https://ovc.example", map[domain.OutboxKind]notifications.Builder{greetingKind: greeting{}},
		notifications.Dispatch{Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	queue := outbox.New(repositories.NewStore(database.Pool), random, logger)
	lanes := []worker.Lane{{Name: domain.LaneEmail, Concurrency: 2, Perform: notifier.Deliver}}
	routes := testcontrol.New(testcontrol.Dependencies{
		Logger: logger, Clock: shared, Worker: worker.New(logger, shared, queue, lanes, nil, worker.DefaultTiming()), Sink: sink,
	})
	handler, err := api.NewHandler(api.Options{
		Logger:       logger,
		Now:          clock.System{}.Now,
		NewRequestID: core.NewRandomID,
		Readiness:    func(context.Context) error { return nil },
		Mount:        routes.Mount,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, database
}

func post(t *testing.T, server *httptest.Server, path string, contentType string, host string, body string) (int, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if host != "" {
		request.Host = host
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
	text, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(text)
}

func errorCode(t *testing.T, body string) string {
	t.Helper()
	var decoded struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("not a structured error: %s", body)
	}
	return decoded.Error.Code
}

func TestControlRoutesRefuseUnsafeRequests(t *testing.T) {
	server, _ := newServer(t)
	cases := map[string]struct {
		contentType string
		host        string
		body        string
		status      int
		code        string
	}{
		"form content type":         {contentType: "application/x-www-form-urlencoded", body: "set=1", status: http.StatusUnsupportedMediaType, code: "unsupported_media_type"},
		"no content type":           {body: "{}", status: http.StatusUnsupportedMediaType, code: "unsupported_media_type"},
		"foreign host":              {contentType: "application/json", host: "attacker.example", body: "{}", status: http.StatusNotFound, code: "not_found"},
		"rebinding host":            {contentType: "application/json", host: "localhost.attacker.example", body: "{}", status: http.StatusNotFound, code: "not_found"},
		"other loopback ip":         {contentType: "application/json", host: "127.0.0.2:8000", body: "{}", status: http.StatusNotFound, code: "not_found"},
		"trailing data":             {contentType: "application/json", body: `{"advance_seconds":1} {}`, status: http.StatusBadRequest, code: "bad_request"},
		"trailing brace":            {contentType: "application/json", body: `{"advance_seconds":1}}`, status: http.StatusBadRequest, code: "bad_request"},
		"trailing bracket":          {contentType: "application/json", body: `{"advance_seconds":1}]`, status: http.StatusBadRequest, code: "bad_request"},
		"oversized trailing object": {contentType: "application/json", body: `{"advance_seconds":1} {"a":"` + strings.Repeat("9", 6000) + `"}`, status: http.StatusRequestEntityTooLarge, code: "payload_too_large"},
		"negative advance":          {contentType: "application/json", body: `{"advance_seconds":-5}`, status: http.StatusUnprocessableEntity, code: "validation_failed"},
		"huge advance":              {contentType: "application/json", body: `{"advance_seconds":9223372036854775807}`, status: http.StatusUnprocessableEntity, code: "validation_failed"},
		"oversized body":            {contentType: "application/json", body: `{"set":"` + strings.Repeat("9", 5000) + `"}`, status: http.StatusRequestEntityTooLarge, code: "payload_too_large"},
		"unknown field":             {contentType: "application/json", body: `{"sentinel":1}`, status: http.StatusBadRequest, code: "bad_request"},
		"neither field":             {contentType: "application/json", body: `{}`, status: http.StatusUnprocessableEntity, code: "validation_failed"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			status, body := post(t, server, "/__test__/clock", testCase.contentType, testCase.host, testCase.body)

			if status != testCase.status || errorCode(t, body) != testCase.code {
				t.Fatalf("got %d %s", status, body)
			}
		})
	}
}

func TestLoopbackHostsAreAccepted(t *testing.T) {
	server, _ := newServer(t)

	for _, host := range []string{"127.0.0.1:8000", "localhost:8000", "localhost", "[::1]:8000", "[::1]"} {
		t.Run(host, func(t *testing.T) {
			status, body := post(t, server, "/__test__/clock", "application/json", host, `{"set":"2030-01-01T00:00:00Z"}`)

			if status != http.StatusOK {
				t.Fatalf("got %d %s", status, body)
			}
		})
	}
}

func TestTickBeforeTheClockIsSetIsReported(t *testing.T) {
	server, _ := newServer(t)

	status, body := post(t, server, "/__test__/tick", "application/json", "", "{}")

	if status != http.StatusUnprocessableEntity || errorCode(t, body) != "validation_failed" || !strings.Contains(body, "not_set") {
		t.Fatalf("got %d %s", status, body)
	}
}

func TestApiAndWorkerShareOneClock(t *testing.T) {
	server, database := newServer(t)
	workerClock := clock.NewShared(testdb.OpenPool(t, database.URL))

	setStatus, _ := post(t, server, "/__test__/clock", "application/json", "", `{"set":"2030-03-10T09:00:00Z"}`)
	advanceStatus, advanceBody := post(t, server, "/__test__/clock", "application/json", "", `{"advance_seconds":90}`)
	tickStatus, tickBody := post(t, server, "/__test__/tick", "application/json", "", "{}")
	seenByWorker, workerErr := workerClock.Instant(context.Background())

	expected := time.Date(2030, 3, 10, 9, 1, 30, 0, time.UTC)
	if setStatus != http.StatusOK || advanceStatus != http.StatusOK || tickStatus != http.StatusOK {
		t.Fatalf("statuses %d %d %d: %s %s", setStatus, advanceStatus, tickStatus, advanceBody, tickBody)
	}
	if !strings.Contains(advanceBody, "2030-03-10T09:01:30Z") || !strings.Contains(tickBody, "2030-03-10T09:01:30Z") {
		t.Fatalf("unexpected bodies: %s %s", advanceBody, tickBody)
	}
	if workerErr != nil || !seenByWorker.Equal(expected) {
		t.Fatalf("the worker sees %v, %v", seenByWorker, workerErr)
	}
}

func TestTickDeliversADueEmailToTheSinkRoute(t *testing.T) {
	server, database := newServer(t)
	instant := time.Date(2030, 3, 10, 9, 0, 0, 0, time.UTC)
	if _, err := repositories.NewStore(database.Pool).Repositories().Outbox.Insert(context.Background(), repositories.NewOutboxRow{
		ID: domain.OutboxID{1}, Lane: domain.LaneEmail, Priority: domain.PriorityHigh, Kind: greetingKind,
		Payload: []byte(`{"to":"tutor@example.org"}`), IdempotencyKey: "greeting:1", NextAttemptAt: instant, CreatedAt: instant,
	}); err != nil {
		t.Fatal(err)
	}
	post(t, server, "/__test__/clock", "application/json", "", `{"set":"2030-03-10T09:00:00Z"}`)

	tickStatus, tickBody := post(t, server, "/__test__/tick", "application/json", "", "{}")
	mailStatus, mailBody := post(t, server, "/__test__/mail", "application/json", "", `{"to":"tutor@example.org"}`)

	var messages []struct {
		Subject string    `json:"subject"`
		Links   []string  `json:"links"`
		SentAt  time.Time `json:"sent_at"`
	}
	if tickStatus != http.StatusOK || mailStatus != http.StatusOK || json.Unmarshal([]byte(mailBody), &messages) != nil {
		t.Fatalf("tick %d %s, mail %d %s", tickStatus, tickBody, mailStatus, mailBody)
	}
	if len(messages) != 1 || messages[0].Subject != "Hello" || len(messages[0].Links) != 1 || !messages[0].SentAt.Equal(instant) {
		t.Fatalf("sink returned %s", mailBody)
	}
}

func TestMailRouteRequiresARecipient(t *testing.T) {
	server, _ := newServer(t)

	status, body := post(t, server, "/__test__/mail", "application/json", "", `{"to":""}`)

	if status != http.StatusUnprocessableEntity || errorCode(t, body) != "validation_failed" {
		t.Fatalf("got %d %s", status, body)
	}
}

type countingRandom struct {
	mutex sync.Mutex
	next  byte
}

func (r *countingRandom) Read(buffer []byte) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for index := range buffer {
		r.next++
		buffer[index] = r.next
	}
	return len(buffer), nil
}
