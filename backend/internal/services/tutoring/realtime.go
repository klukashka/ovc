package tutoring

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type createdNotification struct {
	id        domain.NotificationID
	event     domain.NotificationEvent
	recipient domain.UserID
}

type notificationRow struct {
	event        domain.NotificationEvent
	recipient    domain.UserID
	tutor        domain.UserID
	student      domain.UserID
	subject      domain.EntityID
	relationship *domain.RelationshipID
	key          string
}

func (s *Service) notify(ctx context.Context, repos repositories.Repositories, row notificationRow, now time.Time) (*createdNotification, error) {
	id, err := core.NewID[domain.NotificationID](s.deps.Random)
	if err != nil {
		return nil, err
	}
	student := row.student
	inserted, err := repos.Notifications.Insert(ctx, repositories.NewNotification{
		ID: id, UserID: row.recipient, TutorID: &row.tutor, StudentID: &student, Event: row.event, SubjectID: row.subject,
		RelationshipID: row.relationship, IdempotencyKey: row.key, CreatedAt: now,
	})
	if err != nil || !inserted {
		return nil, err
	}
	if err := repos.Signals.Notification(ctx, row.recipient, id); err != nil {
		return nil, err
	}
	return &createdNotification{id: id, event: row.event, recipient: row.recipient}, nil
}

func (s *Service) logNotified(ctx context.Context, created ...*createdNotification) {
	for _, notification := range created {
		if notification == nil {
			continue
		}
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "notification.created", slog.String("event", string(notification.event)),
			slog.String("target_user_id", notification.recipient.String()), slog.String("notification_id", notification.id.String()))
	}
}

func (s *Service) invitationAccepted(
	ctx context.Context, repos repositories.Repositories, invitation domain.InvitationID, card repositories.Relationship, student domain.UserID,
	now time.Time,
) (*createdNotification, error) {
	relationship := card.ID
	return s.notify(ctx, repos, notificationRow{
		event: domain.NotificationInvitationAccepted, recipient: card.TutorID, tutor: card.TutorID, student: student,
		subject: domain.EntityID(card.ID), relationship: &relationship, key: domain.InvitationAcceptedKey(invitation, card.TutorID),
	}, now)
}

func (s *Service) requestVisible(
	ctx context.Context, repos repositories.Repositories, id domain.JoinRequestID, tutor domain.UserID, student domain.UserID, now time.Time,
) (*createdNotification, error) {
	if err := requestChanged(ctx, repos, id, tutor); err != nil {
		return nil, err
	}
	return s.notify(ctx, repos, notificationRow{
		event: domain.NotificationJoinRequestCreated, recipient: tutor, tutor: tutor, student: student,
		subject: domain.EntityID(id), key: domain.JoinRequestCreatedKey(id, tutor),
	}, now)
}

func relationshipChanged(ctx context.Context, repos repositories.Repositories, link repositories.Relationship, studentSees bool) error {
	recipients := []domain.UserID{link.TutorID}
	if studentSees && link.StudentID != nil {
		recipients = append(recipients, *link.StudentID)
	}
	return repos.Signals.Entity(ctx, recipients, domain.EntityRelationship, domain.EntityID(link.ID))
}

func cardChanged(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.RelationshipID) error {
	return repos.Signals.Entity(ctx, []domain.UserID{tutor}, domain.EntityRelationship, domain.EntityID(id))
}

func requestChanged(ctx context.Context, repos repositories.Repositories, id domain.JoinRequestID, recipients ...domain.UserID) error {
	return repos.Signals.Entity(ctx, recipients, domain.EntityJoinRequest, domain.EntityID(id))
}
