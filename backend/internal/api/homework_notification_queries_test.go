package api_test

import (
	"errors"
	"fmt"
	"testing"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func TestHomeworkNotificationVisibilityQueriesAgree(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	studentNumber := byte(10)
	for _, state := range []domain.RelationshipState{domain.RelationshipActive, domain.RelationshipProvisional, domain.RelationshipEnded, domain.RelationshipReclaimPending, domain.RelationshipDiscarded} {
		for _, through := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/through_%t", state, through), func(t *testing.T) {
				card := h.linkedCard(t, parties.tutor, studentNumber, domain.RelationshipActive)
				student := domain.UserID{studentNumber}
				studentNumber++
				events := []domain.NotificationEvent{domain.NotificationIssueCreated, domain.NotificationIssueWithdrawn, domain.NotificationSubmitted, domain.NotificationSubmissionChecked, domain.NotificationSubmissionReturned, domain.NotificationReviewUpdated}
				ids := []domain.NotificationID{}
				visible := map[domain.NotificationID]bool{}
				for _, marked := range []bool{false, true} {
					issue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card)))
					submission := h.submissionsOf(t, issue.ID)[0].id
					if marked {
						h.markIssueForErasure(t, issue.ID)
					}
					for _, event := range events {
						id, err := core.NewID[domain.NotificationID](h.random)
						if err != nil {
							t.Fatal(err)
						}
						if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO notifications (id, user_id, tutor_id, student_id, event, subject_id, relationship_id, idempotency_key, created_at)
 VALUES ($1, $2, $3, $2, $4, $5::uuid, $6::uuid, $7, $8)`, id, student, parties.tutor, event, submission, card, id.String(), h.now(t)); err != nil {
							t.Fatal(err)
						}
						ids = append(ids, id)
						visible[id] = !marked && state != domain.RelationshipReclaimPending && state != domain.RelationshipDiscarded
					}
				}
				if _, err := h.database.Pool.Exec(t.Context(), "DELETE FROM notifications WHERE idempotency_key <> id::text"); err != nil {
					t.Fatal(err)
				}
				h.setRelationshipState(t, card, state)
				repo := h.store.Repositories().Notifications
				want := int64(0)
				for _, expected := range visible {
					if expected {
						want++
					}
				}

				listed, listErr := repo.Of(t.Context(), student, nil, 100)
				unread, unreadErr := repo.UnreadCount(t.Context(), student)
				own, ownErr := repo.CountOwn(t.Context(), student, ids)
				positions := make(map[domain.NotificationID]error, len(ids))
				for _, id := range ids {
					_, positions[id] = repo.Own(t.Context(), student, id)
				}
				var changed int64
				var writeErr error
				if through {
					changed, writeErr = repo.MarkReadThrough(t.Context(), student, repositories.NotificationPosition{ID: domain.NotificationID{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}, CreatedAt: h.now(t)}, h.now(t))
				} else {
					changed, writeErr = repo.MarkRead(t.Context(), student, ids, h.now(t))
				}

				if listErr != nil || unreadErr != nil || ownErr != nil || writeErr != nil || int64(len(listed)) != want || unread != want || own != want || changed != want {
					t.Fatalf("notification queries disagree: list=%d unread=%d own=%d changed=%d want=%d errors=%v/%v/%v/%v", len(listed), unread, own, changed, want, listErr, unreadErr, ownErr, writeErr)
				}
				for _, row := range listed {
					if !visible[row.ID] {
						t.Fatal("list includes hidden notification")
					}
				}
				for _, id := range ids {
					err := positions[id]
					if visible[id] && err != nil || !visible[id] && !errors.Is(err, repositories.ErrNotFound) {
						t.Fatalf("own notification visibility disagrees: %v", err)
					}
					read := h.countRows(t, "SELECT count(*) FROM notifications WHERE id = $1::uuid AND read_at IS NOT NULL", id.String()) == 1
					if read != visible[id] {
						t.Fatal("read mark disagrees with visibility")
					}
				}
			})
		}
	}
}
