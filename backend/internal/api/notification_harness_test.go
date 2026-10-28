package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const notificationsPath = "/api/v1/notifications"

var sentinelUser = domain.UserID{0xee, 0xee}

type sentFrame struct {
	to   domain.UserID
	kind string
	id   string
}

func entityFrame(to domain.UserID, kind domain.EntityKind, id string) sentFrame {
	return sentFrame{to: to, kind: string(kind), id: id}
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
	case domain.EntitySignal:
		l.record(func() {
			if slices.Equal(typed.Users, []domain.UserID{sentinelUser}) {
				l.sentinels = append(l.sentinels, typed.ID.String())
				return
			}
			for _, user := range typed.Users {
				l.frames = append(l.frames, entityFrame(user, typed.Entity, typed.ID.String()))
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
			ID: id, UserID: recipient, TutorID: tutor, StudentID: &recipient, Event: domain.NotificationJoinRequestDeclined,
			SubjectID: domain.EntityID(id), IdempotencyKey: fmt.Sprintf("seeded:%s", id), CreatedAt: first.Add(time.Duration(index) * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id.String())
	}
	return ids
}
