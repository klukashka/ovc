package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	lessonsRoute  = "/api/v1/lessons"
	calendarRoute = "/api/v1/calendar"
	lessonTitle   = "Conversation practice"
)

type lessonRequest struct {
	RelationshipID *string `json:"relationship_id,omitempty"`
	GroupID        *string `json:"group_id,omitempty"`
	Title          string  `json:"title"`
	Description    *string `json:"description,omitempty"`
	Date           string  `json:"date"`
	Start          string  `json:"start"`
	Duration       int     `json:"duration_minutes"`
	Weekdays       []int   `json:"weekdays,omitempty"`
	EndDate        *string `json:"end_date,omitempty"`
	Count          *int    `json:"count,omitempty"`
	AllowOverlap   bool    `json:"allow_overlap,omitempty"`
}

func oneOffFor(card string, date string, start string) lessonRequest {
	return lessonRequest{RelationshipID: &card, Title: lessonTitle, Date: date, Start: start, Duration: 60}
}

func groupLessonFor(group string, date string, start string) lessonRequest {
	return lessonRequest{GroupID: &group, Title: lessonTitle, Date: date, Start: start, Duration: 60}
}

func (r lessonRequest) weekly(weekdays []int, count int) lessonRequest {
	r.Weekdays, r.Count = weekdays, &count
	return r
}

func (r lessonRequest) until(weekdays []int, end string) lessonRequest {
	r.Weekdays, r.EndDate = weekdays, &end
	return r
}

func (r lessonRequest) body(t *testing.T) string {
	t.Helper()
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

type lessonMember struct {
	Label     string  `json:"label"`
	LabelKind *string `json:"label_kind"`
}

type lessonAudience struct {
	Kind           string          `json:"kind"`
	RelationshipID *string         `json:"relationship_id"`
	GroupID        *string         `json:"group_id"`
	Label          *string         `json:"label"`
	LabelKind      *string         `json:"label_kind"`
	Colour         *string         `json:"colour"`
	Members        *[]lessonMember `json:"members"`
}

const absentIdentifier = "absent"

type audienceIdentifiers struct {
	relationship string
	group        string
}

func presentOrAbsent(value *string) string {
	if value == nil {
		return absentIdentifier
	}
	return *value
}

func identifiersOf(audience lessonAudience) audienceIdentifiers {
	return audienceIdentifiers{relationship: presentOrAbsent(audience.RelationshipID), group: presentOrAbsent(audience.GroupID)}
}

func cardIdentifiers(card string) audienceIdentifiers {
	return audienceIdentifiers{relationship: card, group: absentIdentifier}
}

func groupIdentifiers(group string) audienceIdentifiers {
	return audienceIdentifiers{relationship: absentIdentifier, group: group}
}

var noIdentifiers = audienceIdentifiers{relationship: absentIdentifier, group: absentIdentifier}

func carriesAudienceIdentifiers(body string) bool {
	return strings.Contains(body, `"relationship_id"`) || strings.Contains(body, `"group_id"`)
}

type lessonTutor struct {
	Name   string  `json:"name"`
	Colour *string `json:"colour"`
}

type lessonView struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	StartsAt      time.Time      `json:"starts_at"`
	EndsAt        time.Time      `json:"ends_at"`
	Status        string         `json:"status"`
	Timezone      string         `json:"timezone"`
	SeriesID      *string        `json:"series_id"`
	Occurrence    *string        `json:"occurrence_date"`
	Series        *lessonSeries  `json:"series"`
	Version       int            `json:"version"`
	CancelledAt   *time.Time     `json:"cancelled_at"`
	OtherTimezone *string        `json:"other_timezone"`
	Audience      lessonAudience `json:"audience"`
	Tutor         *lessonTutor   `json:"tutor"`
}

type lessonSeries struct {
	ID        string  `json:"id"`
	Duration  int     `json:"duration_minutes"`
	Weekdays  []int   `json:"weekdays"`
	Start     string  `json:"start"`
	Timezone  string  `json:"timezone"`
	FirstDate string  `json:"first_date"`
	EndDate   *string `json:"end_date"`
	Count     *int    `json:"count"`
}

type createdLessons struct {
	Series  *lessonSeries `json:"series"`
	Lessons []lessonView  `json:"lessons"`
}

type calendarPage struct {
	Items     []lessonView `json:"items"`
	Truncated bool         `json:"truncated"`
}

func (h *authHarness) createLessons(t *testing.T, token string, request lessonRequest, key string) response {
	t.Helper()
	headers := jsonCall(token)
	if key != "" {
		headers["Idempotency-Key"] = key
	}
	return h.send(t, http.MethodPost, lessonsRoute, request.body(t), headers)
}

func decodeCreated(t *testing.T, result response) createdLessons {
	t.Helper()
	var created createdLessons
	if err := json.Unmarshal([]byte(result.body), &created); err != nil || result.status != http.StatusCreated || created.Lessons == nil {
		t.Fatalf("not created lessons: %d %s", result.status, result.body)
	}
	return created
}

func (h *authHarness) scheduled(t *testing.T, token string, request lessonRequest) createdLessons {
	t.Helper()
	return decodeCreated(t, h.createLessons(t, token, request, ""))
}

func (h *authHarness) lessonPage(t *testing.T, token string, id string) response {
	t.Helper()
	return h.send(t, http.MethodGet, lessonsRoute+"/"+id, "", jsonCall(token))
}

func decodeLesson(t *testing.T, result response) lessonView {
	t.Helper()
	var lesson lessonView
	if err := json.Unmarshal([]byte(result.body), &lesson); err != nil || result.status != http.StatusOK || lesson.ID == "" {
		t.Fatalf("not a lesson: %d %s", result.status, result.body)
	}
	return lesson
}

func (h *authHarness) calendarCall(t *testing.T, token string, from time.Time, to time.Time, tutor string) response {
	t.Helper()
	query := url.Values{"from": {from.Format(time.RFC3339)}, "to": {to.Format(time.RFC3339)}}
	if tutor != "" {
		query.Set("tutor", tutor)
	}
	return h.send(t, http.MethodGet, calendarRoute+"?"+query.Encode(), "", jsonCall(token))
}

func (h *authHarness) calendarOf(t *testing.T, token string, from time.Time, to time.Time) calendarPage {
	t.Helper()
	result := h.calendarCall(t, token, from, to, "")
	var page calendarPage
	if err := json.Unmarshal([]byte(result.body), &page); err != nil || result.status != http.StatusOK || page.Items == nil {
		t.Fatalf("not a calendar: %d %s", result.status, result.body)
	}
	if result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the calendar has Cache-Control %q", result.headers.Get("Cache-Control"))
	}
	return page
}

func calendarIDs(page calendarPage) []string {
	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

func (h *authHarness) exec(t *testing.T, statement string, first string) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), statement, first); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) exec2(t *testing.T, statement string, first string, second string) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), statement, first, second); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) exec3(t *testing.T, statement string, first string, second string, third string) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), statement, first, second, third); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) exec4(t *testing.T, statement string, first string, second string, third string, fourth string) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), statement, first, second, third, fourth); err != nil {
		t.Fatal(err)
	}
}

func rfc3339(instant time.Time) string {
	return instant.UTC().Format(time.RFC3339Nano)
}

func (h *authHarness) studentToken(t *testing.T, id byte) string {
	t.Helper()
	return accessToken(t, h.login(t, fmt.Sprintf("member-%d@example.org", id), accountPassword))
}

func (h *authHarness) setTimezone(t *testing.T, user domain.UserID, zone string) {
	t.Helper()
	h.exec2(t, "UPDATE users SET timezone = $2 WHERE id = $1::uuid", user.String(), zone)
}

func (h *authHarness) cancelWithRoster(t *testing.T, lesson string, students ...domain.UserID) {
	t.Helper()
	h.exec2(t, "UPDATE lessons SET status = 'cancelled', cancelled_at = $2::timestamptz, cancel_cause = 'tutor' WHERE id = $1::uuid", lesson, rfc3339(authStart))
	for _, student := range students {
		h.exec3(t, `INSERT INTO lesson_roster (lesson_id, student_id, lesson_starts_at, added_at)
SELECT id, $2::uuid, starts_at, $3::timestamptz FROM lessons WHERE id = $1::uuid`, lesson, student.String(), rfc3339(authStart))
	}
}

func (h *authHarness) lessonRows(t *testing.T, tutor domain.UserID) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM lessons WHERE tutor_id = $1::uuid", tutor.String())
}

func pointer[Value any](value Value) *Value {
	return &value
}

var (
	calendarFrom = time.Date(2030, time.March, 10, 0, 0, 0, 0, time.UTC)
	calendarTo   = time.Date(2030, time.April, 30, 0, 0, 0, 0, time.UTC)
)
