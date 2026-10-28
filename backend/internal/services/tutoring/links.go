package tutoring

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

var (
	ErrRelationshipNotProvisional = errors.New("only a provisional or reclaim-pending card can be removed")
	ErrRoleMismatch               = errors.New("only a student account can join a tutor")
	ErrEmailNotVerified           = errors.New("an unconfirmed account already holds a provisional link")
)

var errConsumedAnotherCard = errors.New("the consumed invitation belongs to another card")

type removalCause string

const (
	removedByTutor         removalCause = "tutor"
	removedByStudent       removalCause = "student"
	removedByCleanup       removalCause = "cleanup"
	removedByReclaim       removalCause = "reclaim"
	removedByTutorDeletion removalCause = "tutor_deleted"
)

func (s *Service) PrecheckInvitation(ctx context.Context, token string, now time.Time) (accounts.InvitationCheck, error) {
	hash, valid := core.InvitationTokenHash(token)
	if !valid {
		return accounts.InvitationCheck{}, s.refused(ctx, refusalInvalid, nil)
	}
	usable, err := s.deps.Store.Repositories().Invitations.Usable(ctx, hash, now)
	if errors.Is(err, repositories.ErrNotFound) {
		return accounts.InvitationCheck{}, s.refused(ctx, refusalInvalid, nil)
	}
	if err != nil {
		return accounts.InvitationCheck{}, err
	}
	return accounts.InvitationCheck{
		TokenHash: hash, Invitation: usable.ID, Relationship: usable.RelationshipID, Tutor: usable.TutorID,
		CardEmail: usable.Email, BoundStudent: usable.BoundStudentID,
	}, nil
}

func (s *Service) BindNewAccount(ctx context.Context, check accounts.InvitationCheck, email string, now time.Time) error {
	bound := check.BoundStudent == nil && (check.CardEmail == nil || domain.NormalizeEmail(*check.CardEmail) == email)
	if bound {
		return nil
	}
	return s.countMismatch(ctx, check, now)
}

func (s *Service) countMismatch(ctx context.Context, check accounts.InvitationCheck, now time.Time, caller ...slog.Attr) error {
	count, err := s.deps.Store.Repositories().Invitations.CountMismatch(ctx, check.TokenHash, now)
	if errors.Is(err, repositories.ErrNotFound) {
		return s.refused(ctx, refusalInvalid, &check, caller...)
	}
	if err != nil {
		return err
	}
	if count >= domain.InvitationMismatchLimit {
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "invitation.revoked_after_mismatches", cardAttributes(check.Tutor, check.Relationship)...)
	}
	return s.refused(ctx, refusalEmailMismatch, &check, caller...)
}

type refusalReason string

const (
	refusalInvalid       refusalReason = "invalid"
	refusalEmailMismatch refusalReason = "email_mismatch"
	refusalRoleMismatch  refusalReason = "role_mismatch"
	refusalNotVerified   refusalReason = "not_verified"
)

func (reason refusalReason) err() error {
	switch reason {
	case refusalInvalid:
		return accounts.ErrInvitationInvalid
	case refusalEmailMismatch:
		return accounts.ErrInvitationEmailMismatch
	case refusalRoleMismatch:
		return ErrRoleMismatch
	case refusalNotVerified:
		return ErrEmailNotVerified
	}
	panic("an invitation refusal has an unknown reason: " + string(reason))
}

func (s *Service) refused(ctx context.Context, reason refusalReason, check *accounts.InvitationCheck, caller ...slog.Attr) error {
	attributes := []slog.Attr{slog.String("reason", string(reason))}
	if check != nil {
		attributes = append(attributes, cardAttributes(check.Tutor, check.Relationship)...)
	}
	attributes = append(attributes, caller...)
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "invitation.refused", attributes...)
	return reason.err()
}

func (s *Service) LinkAtSignUp(
	ctx context.Context, repos repositories.Repositories, user domain.UserID, check accounts.InvitationCheck, now time.Time,
) (accounts.Announce, error) {
	err := repos.TutorProfiles.Lock(ctx, check.Tutor)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, s.refused(ctx, refusalInvalid, &check)
	}
	if err != nil {
		return nil, err
	}
	active, err := s.tutorActive(ctx, repos, check.Tutor)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, s.refused(ctx, refusalInvalid, &check)
	}
	card, err := repos.Relationships.LockForTutor(ctx, check.Tutor, check.Relationship)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, s.refused(ctx, refusalInvalid, &check)
	}
	if err != nil {
		return nil, err
	}
	transition, allowed := domain.NextRelationshipState(card.State, domain.CauseAcceptedUnconfirmed)
	if !allowed {
		return nil, s.refused(ctx, refusalInvalid, &check)
	}
	consumed, err := repos.Invitations.Consume(ctx, check.TokenHash, user, now)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, s.refused(ctx, refusalInvalid, &check)
	}
	if err != nil {
		return nil, err
	}
	if consumed.RelationshipID != card.ID {
		return nil, errConsumedAnotherCard
	}
	if _, err := repos.Invitations.RevokeOpen(ctx, card.TutorID, card.ID, domain.RevokeLinked, now); err != nil {
		return nil, err
	}
	taken, err := repos.Relationships.ColoursOfStudent(ctx, user)
	if err != nil {
		return nil, err
	}
	linked, err := repos.Relationships.LinkInvited(ctx, repositories.Link{
		ID: card.ID, State: transition.To, Student: user, Colour: domain.ChooseColour(taken), Now: now,
	})
	if err != nil {
		return nil, err
	}
	if !linked {
		return nil, s.refused(ctx, refusalInvalid, &check)
	}
	card.StudentID = &user
	if err := relationshipChanged(ctx, repos, card, true); err != nil {
		return nil, err
	}
	notified, err := s.invitationAccepted(ctx, repos, check.Invitation, card, user, now)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		attributes := append(cardAttributes(card.TutorID, card.ID), slog.String("user_id", user.String()),
			slog.String("path", "new_account"), slog.String("effect", string(transition.To)))
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "invitation.accepted", attributes...)
		s.logNotified(ctx, notified)
	}, nil
}

func (s *Service) OnEmailConfirmed(
	ctx context.Context, repos repositories.Repositories, user domain.UserID, reclaim bool, now time.Time,
) (domain.LinkOutcome, accounts.Announce, error) {
	if reclaim {
		return s.reclaim(ctx, repos, user, now)
	}
	heldFor, err := repos.JoinRequests.HeldTutorsOf(ctx, user)
	if err != nil {
		return "", nil, err
	}
	if len(heldFor) > 0 {
		if err := repos.TutorProfiles.LockAll(ctx, heldFor); err != nil {
			return "", nil, err
		}
	}
	links, err := repos.Relationships.LockOfStudent(ctx, user)
	if err != nil {
		return "", nil, err
	}
	var confirmed []repositories.Relationship
	var outcomes []domain.LinkOutcome
	for _, link := range links {
		transition, allowed := domain.NextRelationshipState(link.State, domain.CauseConfirmedInOwnBrowser)
		if !allowed || transition.To != domain.RelationshipActive {
			continue
		}
		activated, err := repos.Relationships.Activate(ctx, link.ID, now)
		if err != nil {
			return "", nil, err
		}
		if !activated {
			return "", nil, fmt.Errorf("activate the locked relationship %s: %w", link.ID, repositories.ErrNotFound)
		}
		if err := relationshipChanged(ctx, repos, link, true); err != nil {
			return "", nil, err
		}
		confirmed = append(confirmed, link)
		outcomes = append(outcomes, domain.LinkOutcomeLinked)
	}
	released, kept, err := s.releaseAtConfirmation(ctx, repos, user, now)
	if err != nil {
		return "", nil, err
	}
	outcome, set := domain.CombineOutcomes(append(outcomes, released...)...)
	if !set {
		return "", accounts.Silent, nil
	}
	return outcome, func(ctx context.Context) {
		for _, link := range confirmed {
			attributes := append(cardAttributes(link.TutorID, link.ID), slog.String("user_id", user.String()))
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "relationship.confirmed", attributes...)
		}
		for _, request := range kept {
			s.logReleased(ctx, user, request)
		}
	}, nil
}

func (s *Service) RemoveStudent(ctx context.Context, tutor domain.UserID, id domain.RelationshipID) (Card, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Card{}, err
	}
	var card Card
	var removed repositories.Relationship
	var lessons lessonEffects
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		seen, err := repos.Relationships.ForTutor(ctx, tutor, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrCardNotFound
		}
		if err != nil {
			return err
		}
		if !removable(seen) {
			return ErrRelationshipNotProvisional
		}
		_, err = repos.Users.LockCredentials(ctx, *seen.StudentID)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrCardNotFound
		}
		if err != nil {
			return err
		}
		if err := s.lockActingTutor(ctx, repos, tutor); err != nil {
			return err
		}
		locked, err := lockCard(ctx, repos, tutor, id)
		if err != nil {
			return err
		}
		if locked.State == domain.RelationshipDiscarded {
			return ErrCardNotFound
		}
		if !removable(locked) || *locked.StudentID != *seen.StudentID {
			return ErrRelationshipNotProvisional
		}
		fresh, effects, err := s.removeLink(ctx, repos, locked, now)
		if err != nil {
			return err
		}
		removed, lessons = locked, effects
		card, err = s.fullCard(ctx, repos, tutor, fresh, now)
		return err
	})
	if err != nil {
		return Card{}, err
	}
	s.logRemoved(ctx, removed, removedByTutor)
	s.logLessonEffects(ctx, lessons)
	return card, nil
}

func removable(card repositories.Relationship) bool {
	transition, allowed := domain.NextRelationshipState(card.State, domain.CauseTutorRemoved)
	return allowed && transition.NewCard && card.StudentID != nil
}

func (s *Service) removeLink(
	ctx context.Context, repos repositories.Repositories, link repositories.Relationship, now time.Time,
) (repositories.Relationship, lessonEffects, error) {
	discarded, err := repos.Relationships.Remove(ctx, link.TutorID, link.ID, now)
	if errors.Is(err, repositories.ErrNotFound) {
		return repositories.Relationship{}, lessonEffects{}, ErrRelationshipNotProvisional
	}
	if err != nil {
		return repositories.Relationship{}, lessonEffects{}, err
	}
	if _, err := repos.Invitations.RevokeOpen(ctx, link.TutorID, link.ID, domain.RevokeRemoved, now); err != nil {
		return repositories.Relationship{}, lessonEffects{}, err
	}
	var lessons lessonEffects
	if discarded.StudentID != nil {
		discardedLink := []repositories.LinkStudent{{Relationship: link.ID, Student: *discarded.StudentID}}
		if lessons, err = s.discardLinkLessons(ctx, repos, discardedLink, link.TutorID, now); err != nil {
			return repositories.Relationship{}, lessonEffects{}, err
		}
	}
	id, err := core.NewID[domain.RelationshipID](s.deps.Random)
	if err != nil {
		return repositories.Relationship{}, lessonEffects{}, err
	}
	err = repos.Relationships.Create(ctx, repositories.NewRelationship{
		ID: id, TutorID: link.TutorID, State: domain.RelationshipInvited, DisplayName: discarded.DisplayName, Email: discarded.Email,
		Notes: discarded.Notes, CreatedAt: now,
	})
	if err != nil {
		return repositories.Relationship{}, lessonEffects{}, err
	}
	fresh, err := repos.Relationships.ForTutor(ctx, link.TutorID, id)
	if err != nil {
		return repositories.Relationship{}, lessonEffects{}, err
	}
	if err := relationshipChanged(ctx, repos, link, true); err != nil {
		return repositories.Relationship{}, lessonEffects{}, err
	}
	if err := cardChanged(ctx, repos, link.TutorID, fresh.ID); err != nil {
		return repositories.Relationship{}, lessonEffects{}, err
	}
	return fresh, lessons, s.issueInvitation(ctx, repos, fresh, now)
}

func (s *Service) logRemoved(ctx context.Context, link repositories.Relationship, cause removalCause) {
	attributes := append(cardAttributes(link.TutorID, link.ID), slog.String("cause", string(cause)))
	if link.StudentID != nil {
		attributes = append(attributes, slog.String("user_id", link.StudentID.String()))
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "relationship.removed", attributes...)
}
