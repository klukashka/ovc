package tutoring

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	fieldCardEmail = "card.email"

	purposeInvitationEmail     domain.RateLimitPurpose = "invitation_email_day"
	purposeInvitationRecipient domain.RateLimitPurpose = "invitation_recipient_day"

	newTutorPeriod = 7 * 24 * time.Hour
)

var (
	newTutorInvitationEmailLimit = dailyLimit{purpose: purposeInvitationEmail, limit: 10}
	invitationEmailLimit         = dailyLimit{purpose: purposeInvitationEmail, limit: 50}
	invitationRecipientLimit     = dailyLimit{purpose: purposeInvitationRecipient, limit: 3}
)

type sendOutcome string

const (
	sendQueued              sendOutcome = "queued"
	sendDroppedRecipientCap sendOutcome = "recipient_cap"
	sendDroppedUnavailable  sendOutcome = "recipient_unavailable"
)

func (s *Service) SendInvitation(ctx context.Context, tutor domain.UserID, id domain.RelationshipID) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	var outcome sendOutcome
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		sent, err := s.sendInvitation(ctx, repos, tutor, id, now)
		outcome = sent
		return err
	})
	var throttled *ThrottledError
	if errors.As(err, &throttled) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "invitation.send_limited", cardAttributes(tutor, id)...)
		return err
	}
	if err != nil {
		return err
	}
	switch outcome {
	case sendQueued:
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "invitation.send_queued", cardAttributes(tutor, id)...)
	case sendDroppedRecipientCap, sendDroppedUnavailable:
		attributes := append(cardAttributes(tutor, id), slog.String("reason", string(outcome)))
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "invitation.send_dropped", attributes...)
	}
	return nil
}

func (s *Service) sendInvitation(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.RelationshipID, now time.Time) (sendOutcome, error) {
	card, err := lockCard(ctx, repos, tutor, id)
	if err != nil {
		return "", err
	}
	if card.State == domain.RelationshipDiscarded {
		return "", ErrCardNotFound
	}
	if !domain.CardTakesInvitations(card.State) {
		return "", ErrRelationshipNotActive
	}
	if card.Email == nil {
		return "", &ValidationError{Fields: []FieldProblem{{Path: fieldCardEmail, Code: domain.FieldRequired}}}
	}
	if err := s.reserveTutorBudget(ctx, repos, tutor, now); err != nil {
		return "", err
	}
	invitation, err := s.sendableInvitation(ctx, repos, card, now)
	if err != nil {
		return "", err
	}
	dropped, err := s.recipientUnreachable(ctx, repos, *card.Email, now)
	if err != nil || dropped != "" {
		return dropped, err
	}
	send, err := repos.Invitations.CountSend(ctx, invitation, now)
	if err != nil {
		return "", err
	}
	return sendQueued, s.queueInvitationEmail(ctx, repos, invitation, send, now)
}

func (s *Service) reserveTutorBudget(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, now time.Time) error {
	account, err := repos.Users.ByID(ctx, tutor)
	if err != nil {
		return err
	}
	limit := invitationEmailLimit
	if now.Before(account.CreatedAt.Add(newTutorPeriod)) {
		limit = newTutorInvitationEmailLimit
	}
	return s.reserve(ctx, repos, limit, tutor, now)
}

func (s *Service) sendableInvitation(ctx context.Context, repos repositories.Repositories, card repositories.Relationship, now time.Time) (domain.InvitationID, error) {
	latest, err := repos.Invitations.LatestOf(ctx, card.ID)
	switch {
	case err == nil && latest.UsedAt == nil && latest.RevokedAt == nil && latest.ExpiresAt.After(now):
		return latest.ID, nil
	case err == nil && latest.UsedAt == nil && latest.RevokedAt == nil:
		if _, err := repos.Invitations.RevokeOpen(ctx, card.TutorID, card.ID, domain.RevokeExpiredOnSend, now); err != nil {
			return domain.InvitationID{}, err
		}
	case err != nil && !errors.Is(err, repositories.ErrNotFound):
		return domain.InvitationID{}, err
	default:
		if err := s.reserve(ctx, repos, regenerationLimit, card.TutorID, now); err != nil {
			return domain.InvitationID{}, err
		}
	}
	if err := s.issueInvitation(ctx, repos, card, now); err != nil {
		return domain.InvitationID{}, err
	}
	fresh, err := repos.Invitations.LatestOf(ctx, card.ID)
	return fresh.ID, err
}

func (s *Service) recipientUnreachable(ctx context.Context, repos repositories.Repositories, email string, now time.Time) (sendOutcome, error) {
	account, err := repos.Users.ByEmail(ctx, email)
	switch {
	case err == nil && (account.State == domain.AccountBlocked || account.State == domain.AccountDeleted):
		return sendDroppedUnavailable, nil
	case err != nil && !errors.Is(err, repositories.ErrNotFound):
		return "", err
	}
	err = s.reserveFor(ctx, repos, invitationRecipientLimit, domain.NormalizeEmail(email), now)
	var throttled *ThrottledError
	if errors.As(err, &throttled) {
		return sendDroppedRecipientCap, nil
	}
	return "", err
}

func (s *Service) queueInvitationEmail(ctx context.Context, repos repositories.Repositories, invitation domain.InvitationID, send int16, now time.Time) error {
	identifier := hex.EncodeToString(invitation[:])
	payload, err := json.Marshal(domain.InvitationEmailPayload{Invitation: identifier, Send: send})
	if err != nil {
		return fmt.Errorf("encode the invitation email payload: %w", err)
	}
	id, err := core.NewID[domain.OutboxID](s.deps.Random)
	if err != nil {
		return err
	}
	_, err = repos.Outbox.Insert(ctx, repositories.NewOutboxRow{
		ID: id, Lane: domain.LaneEmail, Priority: domain.PriorityHigh, Kind: domain.EventInvitationCreated, Payload: payload,
		IdempotencyKey: fmt.Sprintf("%s:%s:%d", domain.EventInvitationCreated, identifier, send), NextAttemptAt: now, CreatedAt: now,
	})
	return err
}
