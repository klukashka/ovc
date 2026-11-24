package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
	"ovc/backend/internal/repositories"
)

const notificationsPath = "/api/v1/notifications"

var sentinelUser = domain.UserID{0xee, 0xee}

type sentFrame struct {
	to      domain.UserID
	kind    string
	id      string
	issue   string
	lessons string
	payload string
}

func renderedEntityFrame(to domain.UserID, signal domain.EntitySignal) sentFrame {
	payload := realtime.EntityPayload(signal)
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic("an entity payload could not be encoded")
	}
	frame := sentFrame{to: to, kind: string(signal.Entity), id: payload.ID, lessons: strings.Join(payload.LessonIDs, ","), payload: string(encoded)}
	if payload.IssueID != nil {
		frame.issue = *payload.IssueID
	}
	return frame
}

type frameLog struct {
	mu        sync.Mutex
	frames    []sentFrame
	sentinels []string
	connected bool
	changed   chan struct{}
}

func (l *frameLog) record(update func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	update()
	close(l.changed)
	l.changed = make(chan struct{})
}

func (l *frameLog) ListenerConnected(listener.Generation) {
	l.record(func() { l.connected = true })
}

func (l *frameLog) ListenerLost(listener.Generation, listener.LossKind) {
	l.record(func() { l.connected = false })
}

func (l *frameLog) Signal(signal domain.Signal) {
	switch typed := signal.(type) {
	case domain.NotificationSignal:
		l.record(func() {
			l.frames = append(l.frames, sentFrame{to: typed.User, kind: string(domain.FrameNotificationNew), id: typed.Notification.String()})
		})
	case domain.ChatSignal:
		l.record(func() {
			id := typed.Channel.String()
			if typed.Message != nil {
				id = typed.Message.String()
			}
			for _, user := range typed.Users {
				l.frames = append(l.frames, sentFrame{to: user, kind: string(typed.Event), id: id})
			}
		})
	case domain.TypingSignal:
		l.record(func() {
			for _, user := range typed.Shown {
				l.frames = append(l.frames, sentFrame{to: user, kind: string(domain.FrameTyping), id: typed.Account.String()})
			}
			for _, user := range typed.Anonymous {
				l.frames = append(l.frames, sentFrame{to: user, kind: string(domain.FrameTyping)})
			}
		})
	case domain.PresenceSignal:
		l.record(func() {
			for _, user := range typed.Users {
				l.frames = append(l.frames, sentFrame{to: user, kind: presenceKind(typed.Online), id: typed.Account.String()})
			}
		})
	case domain.EntitySignal:
		l.record(func() {
			if slices.Equal(typed.Users, []domain.UserID{sentinelUser}) {
				l.sentinels = append(l.sentinels, typed.ID.String())
				return
			}
			for _, user := range typed.Users {
				l.frames = append(l.frames, renderedEntityFrame(user, typed))
			}
		})
	}
}

func (l *frameLog) waitUntil(t *testing.T, what string, found func() bool) {
	t.Helper()
	deadline := time.NewTimer(socketDeadline)
	defer deadline.Stop()
	for {
		l.mu.Lock()
		done, changed := found(), l.changed
		l.mu.Unlock()
		if done {
			return
		}
		select {
		case <-changed:
		case <-deadline.C:
			t.Fatalf("no %s within %s", what, socketDeadline)
		}
	}
}

func (h *authHarness) recordFrames(t *testing.T) *frameLog {
	t.Helper()
	log := &frameLog{changed: make(chan struct{})}
	instance, err := core.NewID[domain.InstanceID](core.CryptoRandom())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := listener.New(listener.Dependencies{
		DatabaseURL: h.database.URL, Instance: instance, Heartbeats: listener.ConnectionHeartbeats{}, Consumer: log,
		Logger: slog.New(slog.NewJSONHandler(h.logs, nil)), Timing: listener.DefaultTiming(),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		adapter.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	log.waitUntil(t, "listening connection", func() bool { return log.connected })
	return log
}

func (l *frameLog) settle(t *testing.T, h *authHarness) []sentFrame {
	t.Helper()
	id, err := core.NewID[domain.EntityID](core.CryptoRandom())
	if err != nil {
		t.Fatal(err)
	}
	h.signal(t, func(ctx context.Context, signals repositories.Signals) error {
		return signals.Entity(ctx, []domain.UserID{sentinelUser}, domain.EntityRelationship, id)
	})
	l.waitUntil(t, "sentinel signal", func() bool { return slices.Contains(l.sentinels, id.String()) })
	l.mu.Lock()
	defer l.mu.Unlock()
	settled := l.frames
	l.frames = nil
	return settled
}

func presenceKind(online bool) string {
	if online {
		return "presence.online"
	}
	return "presence.offline"
}

func framesTo(frames []sentFrame, user domain.UserID) []sentFrame {
	var addressed []sentFrame
	for _, frame := range frames {
		if frame.to == user {
			addressed = append(addressed, sentFrame{kind: frame.kind, id: frame.id})
		}
	}
	return addressed
}

func entityKinds(frames []sentFrame, user domain.UserID) []string {
	var kinds []string
	for _, frame := range framesTo(frames, user) {
		if frame.kind != string(domain.FrameNotificationNew) {
			kinds = append(kinds, frame.kind)
		}
	}
	return kinds
}

func notificationFrames(frames []sentFrame, user domain.UserID) int {
	count := 0
	for _, frame := range framesTo(frames, user) {
		if frame.kind == string(domain.FrameNotificationNew) {
			count++
		}
	}
	return count
}

func hasFrame(frames []sentFrame, user domain.UserID, kind domain.EntityKind, id string) bool {
	return slices.Contains(framesTo(frames, user), sentFrame{kind: string(kind), id: id})
}

type notificationLink struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type notificationItem struct {
	ID           string            `json:"id"`
	Event        string            `json:"event"`
	CreatedAt    time.Time         `json:"created_at"`
	ReadAt       *time.Time        `json:"read_at"`
	Label        *string           `json:"label"`
	LabelDeleted bool              `json:"label_deleted"`
	LabelKind    *string           `json:"label_kind"`
	Colour       *string           `json:"colour"`
	Link         *notificationLink `json:"link"`
}

type notificationPage struct {
	Items       []notificationItem `json:"items"`
	NextCursor  *string            `json:"next_cursor"`
	UnreadCount int64              `json:"unread_count"`
}

func (h *authHarness) notificationPage(t *testing.T, token string, query string) (response, notificationPage) {
	t.Helper()
	result := h.send(t, http.MethodGet, notificationsPath+query, "", jsonCall(token))
	var page notificationPage
	if result.status == http.StatusOK {
		if err := json.Unmarshal([]byte(result.body), &page); err != nil {
			t.Fatalf("not a notification page: %s", result.body)
		}
	}
	return result, page
}

func (h *authHarness) notificationsOf(t *testing.T, token string) []notificationItem {
	t.Helper()
	result, page := h.notificationPage(t, token, "?limit=100")
	if result.status != http.StatusOK {
		t.Fatalf("the notification list answered %d %s", result.status, result.body)
	}
	return page.Items
}

func (h *authHarness) markRead(t *testing.T, token string, body string) response {
	t.Helper()
	return h.send(t, http.MethodPost, notificationsPath+"/read", body, jsonCall(token))
}

func eventsOf(items []notificationItem) []string {
	events := make([]string, 0, len(items))
	for _, item := range items {
		events = append(events, item.Event)
	}
	return events
}

func (h *authHarness) notificationRows(t *testing.T, user domain.UserID) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM notifications WHERE user_id = $1::uuid", user.String())
}

func (h *authHarness) rowsNaming(t *testing.T, user domain.UserID) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM notifications WHERE user_id = $1::uuid OR tutor_id = $1::uuid OR student_id = $1::uuid",
		user.String())
}

func (h *authHarness) seedNotifications(t *testing.T, recipient domain.UserID, tutor domain.UserID, count int, first time.Time) []string {
	t.Helper()
	ids := make([]string, 0, count)
	for index := range count {
		id := domain.NotificationID{0xab, recipient[0], byte(index >> 8), byte(index)}
		_, err := h.store.Repositories().Notifications.Insert(context.Background(), repositories.NewNotification{
			ID: id, UserID: recipient, TutorID: &tutor, StudentID: &recipient, Event: domain.NotificationJoinRequestDeclined,
			SubjectID: domain.EntityID(id), IdempotencyKey: fmt.Sprintf("seeded:%s", id), CreatedAt: first.Add(time.Duration(index) * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id.String())
	}
	return ids
}

func homeworkFramesTo(frames []sentFrame, user domain.UserID) []sentFrame {
	var addressed []sentFrame
	for _, frame := range frames {
		if frame.to == user && (frame.kind == string(domain.EntityIssue) || frame.kind == string(domain.EntitySubmission)) {
			addressed = append(addressed, frame)
		}
	}
	return addressed
}

func lessonList(lessons ...string) string {
	return strings.Join(lessons, ",")
}

func checkStudentHomeworkFrames(t *testing.T, frames []sentFrame, student domain.UserID, visible ...string) {
	t.Helper()
	for _, frame := range homeworkFramesTo(frames, student) {
		if frame.kind != string(domain.EntitySubmission) || frame.issue != "" {
			t.Errorf("a frame to the student names an issue: %+v", frame)
		}
		for _, lesson := range strings.Split(frame.lessons, ",") {
			if lesson != "" && !slices.Contains(visible, lesson) {
				t.Errorf("a frame to the student names the lesson %s it cannot see: %+v", lesson, frame)
			}
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(frame.payload), &fields); err != nil {
			t.Fatal(err)
		}
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		if !slices.Equal(keys, []string{"id", "kind"}) && !slices.Equal(keys, []string{"id", "kind", "lesson_ids"}) {
			t.Errorf("a frame to the student carries the keys %v", keys)
		}
	}
}

func expectStudentHomeworkFrames(t *testing.T, frames []sentFrame, student domain.UserID, lessons string) {
	t.Helper()
	addressed := homeworkFramesTo(frames, student)
	if len(addressed) == 0 {
		t.Fatalf("the student %s got no homework frame", student)
	}
	for _, frame := range addressed {
		if frame.lessons != lessons {
			t.Errorf("a frame to the student names the lessons %q, want %q: %+v", frame.lessons, lessons, frame)
		}
	}
	checkStudentHomeworkFrames(t, frames, student, strings.Split(lessons, ",")...)
}

func checkStudentFrameKeys(t *testing.T, frames []sentFrame, student domain.UserID) {
	t.Helper()
	for _, frame := range frames {
		if frame.to != student {
			continue
		}
		if frame.payload == "" {
			if strings.ContainsAny(frame.id, " \"{") {
				t.Errorf("a frame to the student carries more than an identifier: %+v", frame)
			}
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(frame.payload), &fields); err != nil {
			t.Fatal(err)
		}
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		withLessons := frame.kind == string(domain.EntitySubmission) && slices.Equal(keys, []string{"id", "kind", "lesson_ids"})
		if !slices.Equal(keys, []string{"id", "kind"}) && !withLessons {
			t.Errorf("an entity frame to the student carries the keys %v: %+v", keys, frame)
		}
	}
}
