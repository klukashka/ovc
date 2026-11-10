package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
)

type lessonPatch struct {
	Scope        string    `json:"scope"`
	Version      int       `json:"version"`
	Date         *string   `json:"date,omitempty"`
	Start        *string   `json:"start,omitempty"`
	Duration     *int      `json:"duration_minutes,omitempty"`
	Title        *string   `json:"title,omitempty"`
	Description  *string   `json:"description,omitempty"`
	AllowOverlap bool      `json:"allow_overlap,omitempty"`
	MaterialIDs  *[]string `json:"material_ids,omitempty"`
}

func (p lessonPatch) body(t *testing.T) string {
	t.Helper()
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func thisLesson(version int) lessonPatch {
	return lessonPatch{Scope: "this", Version: version}
}

func following(version int) lessonPatch {
	return lessonPatch{Scope: "following", Version: version}
}

func (p lessonPatch) startingAt(start string) lessonPatch {
	p.Start = &start
	return p
}

func (p lessonPatch) titled(title string) lessonPatch {
	p.Title = &title
	return p
}

func (p lessonPatch) on(date string) lessonPatch {
	p.Date = &date
	return p
}

func (p lessonPatch) lasting(minutes int) lessonPatch {
	p.Duration = &minutes
	return p
}

func editPath(lesson string) string {
	return lessonsRoute + "/" + lesson
}

func cancelPath(lesson string) string {
	return lessonsRoute + "/" + lesson + "/cancel"
}

func cancelBody(scope string, version int) string {
	return `{"scope":"` + scope + `","version":` + strconv.Itoa(version) + `}`
}

func (h *authHarness) editLesson(t *testing.T, token string, lesson string, patch lessonPatch) response {
	t.Helper()
	return h.send(t, http.MethodPatch, editPath(lesson), patch.body(t), jsonCall(token))
}

func (h *authHarness) cancelCall(t *testing.T, token string, lesson string, scope string, version int) response {
	t.Helper()
	return h.send(t, http.MethodPost, cancelPath(lesson), cancelBody(scope, version), jsonCall(token))
}

func decodeChanged(t *testing.T, result response) createdLessons {
	t.Helper()
	var changed createdLessons
	if err := json.Unmarshal([]byte(result.body), &changed); err != nil || result.status != http.StatusOK || changed.Lessons == nil {
		t.Fatalf("not a changed lesson answer: %d %s", result.status, result.body)
	}
	return changed
}

type storedLesson struct {
	SeriesID   *string
	Status     string
	Title      string
	StartsAt   time.Time
	Duration   int
	Version    int
	Revision   int
	Overridden []string
	Cause      *string
	Cancelled  *time.Time
}

func (h *authHarness) storedLesson(t *testing.T, lesson string) storedLesson {
	t.Helper()
	var stored storedLesson
	err := h.database.Pool.QueryRow(context.Background(), `
SELECT series_id::text, status, title, starts_at, duration_minutes, version, schedule_revision, overridden_fields, cancel_cause, cancelled_at
FROM lessons WHERE id = $1::uuid`, lesson).Scan(&stored.SeriesID, &stored.Status, &stored.Title, &stored.StartsAt, &stored.Duration,
		&stored.Version, &stored.Revision, &stored.Overridden, &stored.Cause, &stored.Cancelled)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

type storedSeries struct {
	First     string
	EndDate   *string
	Count     *int
	Start     string
	Duration  int
	Title     string
	SplitFrom *string
	Until     string
	Ended     bool
}

func (h *authHarness) storedSeries(t *testing.T, series string) storedSeries {
	t.Helper()
	var stored storedSeries
	err := h.database.Pool.QueryRow(context.Background(), `
SELECT first_date::text, end_date::text, count, to_char(local_start, 'HH24:MI'), duration_minutes, title, split_from_series_id::text,
    materialised_until::text, ended_at IS NOT NULL
FROM lesson_series WHERE id = $1::uuid`, series).Scan(&stored.First, &stored.EndDate, &stored.Count, &stored.Start, &stored.Duration,
		&stored.Title, &stored.SplitFrom, &stored.Until, &stored.Ended)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func (h *authHarness) changeEmailsTo(t *testing.T, recipient string, prefix string) []mail.SinkMessage {
	t.Helper()
	var found []mail.SinkMessage
	for _, message := range h.mailTo(t, recipient) {
		if strings.HasPrefix(message.Subject, prefix) {
			found = append(found, message)
		}
	}
	return found
}

func memberEmail(id byte) string {
	return fmt.Sprintf("member-%d@example.org", id)
}

func (h *authHarness) lockedElsewhere(t *testing.T, table string, column string, key string) bool {
	t.Helper()
	total := h.countRows(t, "SELECT count(id) FROM "+table+" WHERE "+column+" = $1::uuid", key)
	free := h.countRows(t, "SELECT count(id) FROM (SELECT id FROM "+table+" WHERE "+column+" = $1::uuid FOR UPDATE SKIP LOCKED) AS free", key)
	if total == 0 {
		t.Fatalf("no %s row has %s %s", table, column, key)
	}
	return free < total
}

func (h *authHarness) setStatus(t *testing.T, lesson string, status string) {
	t.Helper()
	h.exec2(t, "UPDATE lessons SET status = $2 WHERE id = $1::uuid", lesson, status)
}

func (h *authHarness) outboxKeysOf(t *testing.T, kind domain.OutboxKind) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(kind))
}

func (p lessonPatch) allowingOverlap() lessonPatch {
	p.AllowOverlap = true
	return p
}
