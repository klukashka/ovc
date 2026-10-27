package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/schedule"
)

var (
	firstLessonStart = time.Date(2030, time.March, 11, 15, 0, 0, 0, time.UTC)
	firstLessonEnd   = firstLessonStart.Add(time.Hour)
	dayBeforeFirst   = firstLessonStart.Add(-24 * time.Hour).Add(time.Minute)
	hourBeforeFirst  = firstLessonStart.Add(-time.Hour).Add(time.Minute)
)

const (
	firstLessonDate  = "2030-03-11"
	firstLessonLocal = "18:00"
)

func (h *authHarness) tickAt(t *testing.T, instant time.Time) {
	t.Helper()
	h.at(t, instant)
	h.tick(t)
}

func (h *authHarness) queueReminders(t *testing.T) {
	t.Helper()
	h.jobs.mutex.Lock()
	jobs := h.jobs.current
	h.jobs.mutex.Unlock()
	if _, err := jobs.QueueReminders(context.Background(), h.now(t)); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) withJobSettings(settings schedule.JobSettings) {
	h.jobs.use(schedule.NewJobs(h.schedule, settings))
}

func scheduleDefaults() schedule.JobSettings {
	return schedule.DefaultJobSettings()
}

func minuteStepWall() func() time.Time {
	current := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)
	var mutex sync.Mutex
	return func() time.Time {
		mutex.Lock()
		defer mutex.Unlock()
		current = current.Add(time.Minute)
		return current
	}
}

func oneExtensionBatchSettings() schedule.JobSettings {
	settings := schedule.DefaultJobSettings()
	settings.Wall = minuteStepWall()
	settings.ExtensionBudget = 90 * time.Second
	return settings
}

func oneReminderBatchSettings() schedule.JobSettings {
	settings := schedule.DefaultJobSettings()
	settings.Wall = minuteStepWall()
	settings.ReminderBudget = 150 * time.Second
	return settings
}

func (h *authHarness) lessonAt(t *testing.T, token string, card string, date string, start string) string {
	t.Helper()
	request := oneOffFor(card, date, start)
	request.AllowOverlap = true
	created := h.scheduled(t, token, request)
	return created.Lessons[0].ID
}

func (h *authHarness) lessonState(t *testing.T, lesson string) string {
	t.Helper()
	var status string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT status FROM lessons WHERE id = $1::uuid", lesson).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (h *authHarness) rosterOf(t *testing.T, lesson string) []domain.UserID {
	t.Helper()
	rows, err := h.database.Pool.Query(context.Background(), "SELECT student_id FROM lesson_roster WHERE lesson_id = $1::uuid ORDER BY student_id", lesson)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var students []domain.UserID
	for rows.Next() {
		var student domain.UserID
		if err := rows.Scan(&student); err != nil {
			t.Fatal(err)
		}
		students = append(students, student)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return students
}

func (h *authHarness) outboxOf(t *testing.T, kind domain.OutboxKind, status string) int64 {
	t.Helper()
	var count int64
	err := h.database.Pool.QueryRow(context.Background(),
		"SELECT count(id) FROM outbox WHERE kind = $1 AND status || coalesce(':' || done_reason, '') = $2", string(kind), status).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (h *authHarness) remindersFor(t *testing.T, recipient string, kind domain.OutboxKind) []mail.SinkMessage {
	t.Helper()
	prefix := "Lesson in 24 hours"
	if kind == domain.EventLessonReminder1h {
		prefix = "Lesson in 1 hour"
	}
	var found []mail.SinkMessage
	for _, message := range h.mailTo(t, recipient) {
		if strings.HasPrefix(message.Subject, prefix) || strings.HasPrefix(message.Subject, russianPrefix(kind)) {
			found = append(found, message)
		}
	}
	return found
}

func russianPrefix(kind domain.OutboxKind) string {
	if kind == domain.EventLessonReminder1h {
		return "Урок через 1 час"
	}
	return "Урок через 24 часа"
}

func headerToken(t *testing.T, message mail.SinkMessage) string {
	t.Helper()
	header := message.Headers["List-Unsubscribe"]
	address, found := strings.CutPrefix(header, "<")
	address, closed := strings.CutSuffix(address, ">")
	if !found || !closed {
		t.Fatalf("the email has no one-click header: %v", message.Headers)
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Path != "/api/v1/unsubscribe" || parsed.Query().Get("token") == "" {
		t.Fatalf("the one-click header is %q", header)
	}
	return parsed.Query().Get("token")
}

func expectedMessageID(key string) string {
	sum := sha256.Sum256([]byte(key + "\x00"))
	return hex.EncodeToString(sum[:16]) + "@ovc.example"
}

func (h *authHarness) lessonEmailSlots(t *testing.T, student domain.UserID) int64 {
	t.Helper()
	return h.lessonEmailSlotsOn(t, student, h.now(t))
}

func (h *authHarness) lessonEmailSlotsOn(t *testing.T, student domain.UserID, day time.Time) int64 {
	t.Helper()
	window, _ := domain.FixedWindow(day, domain.LessonEmailWindow)
	var count int64
	err := h.database.Pool.QueryRow(context.Background(),
		"SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose = $1 AND subject = $2 AND window_start = $3",
		string(domain.PurposeLessonEmails), h.keys.ThrottleSubject(string(domain.PurposeLessonEmails), student.String()), window).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (h *authHarness) presetLessonEmailSlots(t *testing.T, student domain.UserID, count int) {
	t.Helper()
	h.presetLessonEmailSlotsOn(t, student, h.now(t), count)
}

func (h *authHarness) presetLessonEmailSlotsOn(t *testing.T, student domain.UserID, day time.Time, count int) {
	t.Helper()
	window, _ := domain.FixedWindow(day, domain.LessonEmailWindow)
	_, err := h.database.Pool.Exec(context.Background(),
		"INSERT INTO rate_limit_counters (purpose, subject, window_start, count) VALUES ($1, $2, $3, $4)",
		string(domain.PurposeLessonEmails), h.keys.ThrottleSubject(string(domain.PurposeLessonEmails), student.String()), window, count)
	if err != nil {
		t.Fatal(err)
	}
}
