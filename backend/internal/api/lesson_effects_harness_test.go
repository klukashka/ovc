package api_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
)

const (
	effectSeriesDate   = "2030-04-01"
	archivedGroupName  = "Evening club"
	provisionalStudent = 30
)

func (h *authHarness) instantOf(t *testing.T, query string, key string) time.Time {
	t.Helper()
	var instant *time.Time
	if err := h.database.Pool.QueryRow(context.Background(), query, key).Scan(&instant); err != nil {
		t.Fatal(err)
	}
	if instant == nil {
		t.Fatalf("%s holds no instant for %s", query, key)
	}
	return *instant
}

func (h *authHarness) endedAtOf(t *testing.T, card string) time.Time {
	t.Helper()
	return h.instantOf(t, "SELECT ended_at FROM relationships WHERE id = $1::uuid", card)
}

func (h *authHarness) archivedAtOf(t *testing.T, group string) time.Time {
	t.Helper()
	return h.instantOf(t, "SELECT archived_at FROM groups WHERE id = $1::uuid", group)
}

func (h *authHarness) deletedAtOf(t *testing.T, user domain.UserID) time.Time {
	t.Helper()
	return h.instantOf(t, "SELECT deleted_at FROM users WHERE id = $1::uuid", user.String())
}

func (h *authHarness) rosterRowsOf(t *testing.T, student domain.UserID) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(lesson_id) FROM lesson_roster WHERE student_id = $1::uuid", student.String())
}

func (h *authHarness) effectEmailsQueued(t *testing.T) int64 {
	t.Helper()
	var queued int64
	err := h.database.Pool.QueryRow(context.Background(),
		"SELECT count(id) FROM outbox WHERE kind LIKE 'lesson.%' OR kind IN ('relationship.ended', 'group.archived', 'tutor.deleted')").Scan(&queued)
	if err != nil {
		t.Fatal(err)
	}
	return queued
}

func (h *authHarness) cancelledBy(t *testing.T, lessons []string, cause string, at time.Time, roster ...domain.UserID) {
	t.Helper()
	for _, lesson := range lessons {
		stored := h.storedLesson(t, lesson)
		if stored.Status != "cancelled" || stored.Cause == nil || *stored.Cause != cause || stored.Cancelled == nil || !stored.Cancelled.Equal(at) {
			t.Fatalf("lesson %s is stored as %+v, want cancelled by %s at %s", lesson, stored, cause, at)
		}
		if frozen := h.rosterOf(t, lesson); !slices.Equal(frozen, roster) {
			t.Fatalf("lesson %s has the roster %v, want %v", lesson, frozen, roster)
		}
	}
}

func listedLessons(message mail.SinkMessage) []string {
	var listed []string
	for _, line := range strings.Split(message.Text, "\n") {
		if strings.HasPrefix(line, "- ") {
			listed = append(listed, strings.TrimPrefix(line, "- "))
		}
	}
	return listed
}

func (h *authHarness) emailsWithSubject(t *testing.T, recipient string, subject string) []mail.SinkMessage {
	t.Helper()
	var found []mail.SinkMessage
	for _, message := range h.mailTo(t, recipient) {
		if message.Subject == subject {
			found = append(found, message)
		}
	}
	return found
}

type discardableLink struct {
	tutor      domain.UserID
	tutorToken string
	card       string
	student    domain.UserID
}

type discardPath struct {
	link    func(t *testing.T, h *authHarness) discardableLink
	discard func(t *testing.T, h *authHarness, link discardableLink) string
}

func seededProvisionalLink(t *testing.T, h *authHarness) discardableLink {
	t.Helper()
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, provisionalStudent, domain.RelationshipProvisional)
	return discardableLink{tutor: tutor, tutorToken: token, card: card, student: domain.UserID{provisionalStudent}}
}

func squattedLink(t *testing.T, h *authHarness) discardableLink {
	t.Helper()
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, student := h.squat(t, token, memberEmail(provisionalStudent), memberEmail(provisionalStudent))
	return discardableLink{tutor: tutor, tutorToken: token, card: card.ID, student: student}
}

func discardPaths() map[string]discardPath {
	return map[string]discardPath{
		"a removal": {link: seededProvisionalLink, discard: func(t *testing.T, h *authHarness, link discardableLink) string {
			t.Helper()
			if removed := h.removeStudent(t, link.tutorToken, link.card); removed.status != http.StatusOK {
				t.Fatalf("the removal answered %d %s", removed.status, removed.body)
			}
			return h.studentToken(t, provisionalStudent)
		}},
		"a dropped reclaim link": {link: squattedLink, discard: func(t *testing.T, h *authHarness, link discardableLink) string {
			t.Helper()
			owner := h.reclaimed(t, memberEmail(provisionalStudent))
			if dropped := h.decideReclaim(t, owner, reclaimChoice{DropLinks: []string{link.card}}); dropped.status != http.StatusOK {
				t.Fatalf("the drop answered %d %s", dropped.status, dropped.body)
			}
			return owner
		}},
		"the cleanup": {link: seededProvisionalLink, discard: func(t *testing.T, h *authHarness, _ discardableLink) string {
			t.Helper()
			h.tickAt(t, authStart.Add(8*24*time.Hour))
			if h.userExists(t, memberEmail(provisionalStudent)) {
				t.Fatal("the cleanup kept the unconfirmed account")
			}
			return ""
		}},
		"a tutor deletion": {link: seededProvisionalLink, discard: func(t *testing.T, h *authHarness, link discardableLink) string {
			t.Helper()
			if deleted := h.deleteMe(t, link.tutorToken, accountPassword); deleted.status != http.StatusNoContent {
				t.Fatalf("the deletion answered %d %s", deleted.status, deleted.body)
			}
			return h.studentToken(t, provisionalStudent)
		}},
	}
}

func (h *authHarness) effectSeries(t *testing.T, token string, card string) createdLessons {
	t.Helper()
	request := oneOffFor(card, effectSeriesDate, "18:00").weekly([]int{1}, 3)
	request.AllowOverlap = true
	return h.scheduled(t, token, request)
}

func (h *authHarness) archive(t *testing.T, token string, group string) response {
	t.Helper()
	return h.groupCall(t, token, http.MethodPost, group, "/archive", "")
}
