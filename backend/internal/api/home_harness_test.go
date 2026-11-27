package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const homeRoute = "/api/v1/home"

var (
	tutorHomeKeys = []string{
		"today_lessons", "today_lessons_count", "needs_checking", "upcoming_deadlines", "unread_channels", "join_requests", "tutor_code", "checklist", "recent_updates",
	}
	studentHomeKeys = []string{
		"next_lesson", "today_lessons_count", "todo", "recently_checked", "next_seven_days", "next_seven_days_count", "tutors", "tutors_count", "recent_updates",
	}
	calendarItemKeys     = []string{"id", "title", "starts_at", "ends_at", "status", "took_place", "moved", "timezone", "audience", "tutor"}
	calendarAudienceKeys = []string{"kind", "label", "label_kind", "colour"}
	homeTutorItemKeys    = []string{"relationship_id", "state", "name", "avatar_id", "colour", "subjects", "channel_id"}
	homeChannelKeys      = []string{"channel_id", "label", "label_kind", "colour", "unread_count"}
	reviewQueueItemKeys  = []string{
		"submission_id", "issue_id", "title", "student", "submitted_at", "first_submitted_at", "late", "has_draft", "issued_before_reclaim",
	}
	issueItemKeys = []string{
		"id", "title", "audience", "lesson_id", "deadline_at", "answer_policy", "status", "issued_at", "withdrawn_at", "has_manual_questions",
		"max_points", "version", "progress",
	}
	joinRequestItemKeys = []string{"id", "student", "created_at", "expires_at"}
	homeMoscow          = time.FixedZone("Europe/Moscow", 3*60*60)
)

func (h *authHarness) homeAnswer(t *testing.T, token string, query string) response {
	t.Helper()
	return h.send(t, http.MethodGet, homeRoute+query, "", jsonCall(token))
}

func (h *authHarness) home(t *testing.T, token string, query string) map[string]json.RawMessage {
	t.Helper()
	result := h.homeAnswer(t, token, query)
	if result.status != http.StatusOK || result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("home answered %d with Cache-Control %q: %s", result.status, result.headers.Get("Cache-Control"), result.body)
	}
	var parts map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.body), &parts); err != nil {
		t.Fatalf("home is not an object: %s", result.body)
	}
	return parts
}

func (h *authHarness) routeBody(t *testing.T, token string, path string) map[string]json.RawMessage {
	t.Helper()
	result := h.send(t, http.MethodGet, path, "", jsonCall(token))
	if result.status != http.StatusOK {
		t.Fatalf("%s answered %d %s", path, result.status, result.body)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.body), &body); err != nil {
		t.Fatalf("%s is not an object: %s", path, result.body)
	}
	return body
}

func compactItems(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("not an array: %s", raw)
	}
	compacted := make([]string, 0, len(items))
	for _, item := range items {
		var buffer bytes.Buffer
		if err := json.Compact(&buffer, item); err != nil {
			t.Fatal(err)
		}
		compacted = append(compacted, buffer.String())
	}
	return compacted
}

func compactValue(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		t.Fatalf("not JSON: %s", raw)
	}
	return buffer.String()
}

func decodedAs[Value any](t *testing.T, raw json.RawMessage) Value {
	t.Helper()
	var value Value
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("cannot decode %s: %v", raw, err)
	}
	return value
}

type homeCalendarItem struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Status   string    `json:"status"`
}

func (h *authHarness) calendarItems(t *testing.T, token string, from time.Time, to time.Time, tutor string) ([]string, []homeCalendarItem) {
	t.Helper()
	result := h.calendarCall(t, token, from, to, tutor)
	if result.status != http.StatusOK {
		t.Fatalf("the calendar answered %d %s", result.status, result.body)
	}
	body := decodedAs[map[string]json.RawMessage](t, json.RawMessage(result.body))
	return compactItems(t, body["items"]), decodedAs[[]homeCalendarItem](t, body["items"])
}

func homeDayStart(instant time.Time) time.Time {
	local := instant.In(homeMoscow)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, homeMoscow).UTC()
}

func assertKeys(t *testing.T, label string, raw json.RawMessage, expected []string) map[string]json.RawMessage {
	t.Helper()
	return contractCheck{t: t}.object(label, raw, expected)
}

func assertEachKeys(t *testing.T, label string, raw json.RawMessage, expected []string) []json.RawMessage {
	t.Helper()
	items := decodedAs[[]json.RawMessage](t, raw)
	for _, item := range items {
		assertKeys(t, label, item, expected)
	}
	return items
}

func assertTutorHomeShape(t *testing.T, parts map[string]json.RawMessage) {
	t.Helper()
	raw, err := json.Marshal(parts)
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, "tutor home", raw, tutorHomeKeys)
	assertEachKeys(t, "recent update", parts["recent_updates"], recentUpdateKeys)
	for _, lesson := range assertEachKeys(t, "today lesson", parts["today_lessons"], calendarItemKeys) {
		assertKeys(t, "today lesson audience", decodedAs[map[string]json.RawMessage](t, lesson)["audience"], calendarAudienceKeys)
	}
	if string(parts["needs_checking"]) != "null" {
		checking := assertKeys(t, "needs checking", parts["needs_checking"], []string{"count", "items"})
		assertEachKeys(t, "needs checking item", checking["items"], reviewQueueItemKeys)
	}
	if string(parts["upcoming_deadlines"]) != "null" {
		deadlines := assertKeys(t, "upcoming deadlines", parts["upcoming_deadlines"], []string{"count", "items"})
		assertEachKeys(t, "upcoming deadline", deadlines["items"], issueItemKeys)
	}
	unread := assertKeys(t, "unread channels", parts["unread_channels"], []string{"items", "unread_total"})
	assertEachKeys(t, "unread channel", unread["items"], homeChannelKeys)
	if string(parts["join_requests"]) != "null" {
		requests := assertKeys(t, "join requests", parts["join_requests"], []string{"count", "items", "next_cursor"})
		assertEachKeys(t, "join request", requests["items"], joinRequestItemKeys)
	}
	assertKeys(t, "tutor code", parts["tutor_code"], []string{"code", "active"})
	checklist := assertKeys(t, "checklist", parts["checklist"], []string{"onboarding_dismissed", "steps"})
	assertKeys(t, "checklist steps", checklist["steps"], []string{"profile", "student", "lesson", "assignment"})
}

func (c contractCheck) studentHome(label string, raw []byte) {
	c.t.Helper()
	decoded := c.object(label, raw, studentHomeKeys)
	if string(decoded["next_lesson"]) != "null" {
		lesson := c.object(label+".next_lesson", decoded["next_lesson"], calendarItemKeys)
		c.object(label+".next_lesson.audience", lesson["audience"], calendarAudienceKeys)
	}
	todo := c.object(label+".todo", decoded["todo"], []string{"count", "items"})
	for _, field := range []json.RawMessage{todo["items"], decoded["recently_checked"]} {
		for _, item := range c.array(label+".work", field) {
			fields := c.object(label+".work.item", item, assignmentItemKeys)
			c.object(label+".work.item.tutor", fields["tutor"], tutorRefKeys)
		}
	}
	for _, lesson := range c.array(label+".next_seven_days", decoded["next_seven_days"]) {
		fields := c.object(label+".next_seven_days.item", lesson, calendarItemKeys)
		c.object(label+".next_seven_days.item.audience", fields["audience"], calendarAudienceKeys)
	}
	for _, tutor := range c.array(label+".tutors", decoded["tutors"]) {
		c.object(label+".tutors.item", tutor, homeTutorItemKeys)
	}
	for _, row := range c.array(label+".recent_updates", decoded["recent_updates"]) {
		fields := c.object(label+".recent_updates.item", row, recentUpdateKeys)
		if string(fields["link"]) != "null" {
			c.object(label+".recent_updates.link", fields["link"], []string{"kind", "id"})
		}
	}
	c.noSentinel(label, raw)
}

func homeQuery(tutor string) string {
	if tutor == "" {
		return ""
	}
	return "?" + url.Values{"tutor": {tutor}}.Encode()
}

func (h *authHarness) seedTutorAccount(t *testing.T, id byte) domain.UserID {
	t.Helper()
	user := domain.UserID{id}
	encoded, err := h.hasher.Hash(context.Background(), accountPassword)
	if err != nil {
		t.Fatal(err)
	}
	err = h.store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: user, Role: domain.RoleTutor, State: domain.AccountActive, Email: fmt.Sprintf("tutor-%d@example.org", id), EmailVerifiedAt: &authStart,
		PasswordHash: encoded, Name: fmt.Sprintf("Seeded tutor %d", id), Language: domain.LanguageEnglish, Timezone: "Europe/Moscow",
		Theme: domain.ThemeSystem, ConsentVersion: "2026-10", ConsentAt: authStart, CreatedAt: authStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = h.store.Repositories().TutorProfiles.Create(context.Background(), repositories.NewTutorProfile{
		UserID: user, FileQuotaBytes: 5_000_000_000, RecordingQuotaBytes: 50_000_000_000, CreatedAt: authStart,
	}, tutorCodeSource(user))
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func (h *authHarness) seededStudentToken(t *testing.T, id byte) string {
	t.Helper()
	return accessToken(t, h.login(t, fmt.Sprintf("member-%d@example.org", id), accountPassword))
}

func (h *authHarness) firstChannelOf(t *testing.T, token string) string {
	t.Helper()
	body := h.routeBody(t, token, channelsRoute)
	items := decodedAs[[]struct {
		ID string `json:"id"`
	}](t, body["items"])
	if len(items) == 0 {
		t.Fatal("the caller has no channel")
	}
	return items[0].ID
}

type homeRefusal struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func refusalWithoutRequestID(t *testing.T, result response) homeRefusal {
	t.Helper()
	return decodedAs[homeRefusal](t, json.RawMessage(result.body))
}

func namedIn(body string, identifiers ...string) []string {
	var found []string
	for _, identifier := range identifiers {
		if bytes.Contains([]byte(body), []byte(identifier)) {
			found = append(found, identifier)
		}
	}
	slices.Sort(found)
	return found
}

func (h *authHarness) homeNow(t *testing.T) time.Time {
	t.Helper()
	now, err := h.clock.Instant(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return now
}

func rawOf[Value any](t *testing.T, value Value) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func anotherStudentsHomeFilter(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	owners := h.issueOwners(t)
	h.linkedCard(t, owners.owner, 11, domain.RelationshipActive)
	token := h.studentToken(t, 11)
	outOfScope := h.homeAnswer(t, token, homeQuery(owners.card))
	random := h.homeAnswer(t, token, homeQuery(randomSubmissionID))
	return outOfScope, random
}
