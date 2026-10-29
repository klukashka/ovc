package tutoring

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

type acceptanceEffect string

const (
	effectProvisional acceptanceEffect = "provisional"
	effectLinked      acceptanceEffect = "linked"
	effectReactivated acceptanceEffect = "reactivated"
	effectMerged      acceptanceEffect = "merged"
)

type signedInLink struct {
	kept         repositories.Relationship
	effect       acceptanceEffect
	reactivated  bool
	keptReclaim  bool
	unsuppressed []unsuppressedRequest
	notified     *createdNotification
}

func (s *Service) AcceptSignedIn(ctx context.Context, principal accounts.Principal, token string) (MyTutor, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return MyTutor{}, err
	}
	caller := slog.String("user_id", principal.UserID.String())
	if principal.Role != domain.RoleStudent {
		return MyTutor{}, s.refused(ctx, refusalRoleMismatch, nil, caller)
	}
	confirmed := principal.State == domain.AccountActive
	if err := s.refuseSecondProvisional(ctx, s.deps.Store.Repositories(), principal.UserID, confirmed, caller); err != nil {
		return MyTutor{}, err
	}
	check, err := s.PrecheckInvitation(ctx, token, now)
	if err != nil {
		return MyTutor{}, err
	}
	account, err := s.deps.Store.Repositories().Users.ByID(ctx, principal.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return MyTutor{}, accounts.ErrUnauthenticated
	}
	if err != nil {
		return MyTutor{}, err
	}
	if !boundToAccount(check, principal.UserID, account.Email) {
		return MyTutor{}, s.countMismatch(ctx, check, now, caller)
	}
	var link signedInLink
	var tutor MyTutor
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := accounts.LockActingAccount(ctx, repos, principal, now, principal.State); err != nil {
			return err
		}
		if err := s.refuseSecondProvisional(ctx, repos, principal.UserID, confirmed, caller); err != nil {
			return err
		}
		err := repos.TutorProfiles.Lock(ctx, check.Tutor)
		if errors.Is(err, repositories.ErrNotFound) {
			return s.refused(ctx, refusalInvalid, &check, caller)
		}
		if err != nil {
			return err
		}
		link, err = s.linkSignedIn(ctx, repos, principal.UserID, confirmed, check, now, caller)
		if err != nil {
			return err
		}
		if _, err := repos.JoinRequests.AcceptWaiting(ctx, check.Tutor, principal.UserID, now); err != nil {
			return err
		}
		link.unsuppressed, err = s.unsuppress(ctx, repos, check.Tutor, now)
		if err != nil {
			return err
		}
		if err := s.mirror(ctx, repos, domain.PathLinking, directAudience(link.kept.ID), now); err != nil {
			return err
		}
		if link.keptReclaim {
			if err := clearSettledDeadline(ctx, repos, principal.UserID, now); err != nil {
				return err
			}
		}
		tutor, err = s.myTutor(ctx, repos, principal.UserID, link.kept.ID)
		return err
	})
	if err != nil {
		return MyTutor{}, err
	}
	s.announceSignedIn(ctx, principal.UserID, link)
	return tutor, nil
}

func (s *Service) refuseSecondProvisional(
	ctx context.Context, repos repositories.Repositories, student domain.UserID, confirmed bool, caller slog.Attr,
) error {
	if confirmed {
		return nil
	}
	held, err := repos.Relationships.HoldsProvisional(ctx, student)
	if err != nil {
		return err
	}
	if held {
		return s.refused(ctx, refusalNotVerified, nil, caller)
	}
	return nil
}

func boundToAccount(check accounts.InvitationCheck, student domain.UserID, email string) bool {
	switch {
	case check.BoundStudent != nil:
		return *check.BoundStudent == student
	case check.CardEmail != nil:
		return domain.NormalizeEmail(*check.CardEmail) == domain.NormalizeEmail(email)
	}
	return true
}

func (s *Service) linkSignedIn(
	ctx context.Context, repos repositories.Repositories, student domain.UserID, confirmed bool, check accounts.InvitationCheck,
	now time.Time, caller slog.Attr,
) (signedInLink, error) {
	invalid := func() (signedInLink, error) {
		return signedInLink{}, s.refused(ctx, refusalInvalid, &check, caller)
	}
	rows, err := repos.Relationships.LockForLink(ctx, check.Tutor, check.Relationship, student)
	if err != nil {
		return signedInLink{}, err
	}
	card, existing, found := splitLinkRows(rows, check.Relationship)
	if !found || !domain.CardTakesInvitations(card.State) {
		return invalid()
	}
	tutor, err := repos.Users.ByID(ctx, check.Tutor)
	if err != nil {
		return signedInLink{}, err
	}
	if tutor.State != domain.AccountActive {
		return invalid()
	}
	consumed, err := repos.Invitations.Consume(ctx, check.TokenHash, student, now)
	if errors.Is(err, repositories.ErrNotFound) {
		return invalid()
	}
	if err != nil {
		return signedInLink{}, err
	}
	if consumed.RelationshipID != card.ID {
		return signedInLink{}, errConsumedAnotherCard
	}
	cause := domain.CauseAcceptedUnconfirmed
	if confirmed {
		cause = domain.CauseAcceptedConfirmed
	}
	var link signedInLink
	var linked bool
	if existing != nil {
		link, linked, err = s.mergeInto(ctx, repos, *existing, card, cause, now)
	} else {
		link, linked, err = s.linkCard(ctx, repos, card, student, cause, now)
	}
	if err != nil {
		return signedInLink{}, err
	}
	if !linked {
		return invalid()
	}
	if _, err := repos.Invitations.RevokeOpen(ctx, link.kept.TutorID, link.kept.ID, domain.RevokeLinked, now); err != nil {
		return signedInLink{}, err
	}
	link.kept.StudentID = &student
	if err := relationshipChanged(ctx, repos, link.kept, true); err != nil {
		return signedInLink{}, err
	}
	if link.kept.ID != card.ID {
		if err := cardChanged(ctx, repos, card.TutorID, card.ID); err != nil {
			return signedInLink{}, err
		}
	}
	link.notified, err = s.invitationAccepted(ctx, repos, check.Invitation, link.kept, student, now)
	return link, err
}

func splitLinkRows(rows []repositories.Relationship, card domain.RelationshipID) (repositories.Relationship, *repositories.Relationship, bool) {
	var found *repositories.Relationship
	var existing *repositories.Relationship
	for index := range rows {
		if rows[index].ID == card {
			found = &rows[index]
		} else {
			existing = &rows[index]
		}
	}
	if found == nil {
		return repositories.Relationship{}, nil, false
	}
	return *found, existing, true
}

func (s *Service) linkCard(
	ctx context.Context, repos repositories.Repositories, card repositories.Relationship, student domain.UserID,
	cause domain.RelationshipCause, now time.Time,
) (signedInLink, bool, error) {
	transition, allowed := domain.NextRelationshipState(card.State, cause)
	if !allowed {
		return signedInLink{}, false, nil
	}
	if card.State == domain.RelationshipEnded {
		if card.StudentID == nil || *card.StudentID != student || transition.To != domain.RelationshipActive {
			return signedInLink{}, false, nil
		}
		activated, err := repos.Relationships.ActivateLinked(ctx, card, now)
		return signedInLink{kept: card, effect: effectReactivated, reactivated: true}, activated, err
	}
	taken, err := repos.Relationships.ColoursOfStudent(ctx, student)
	if err != nil {
		return signedInLink{}, false, err
	}
	linked, err := repos.Relationships.LinkInvited(ctx, repositories.Link{
		ID: card.ID, State: transition.To, Student: student, Colour: domain.ChooseColour(taken), Now: now,
	})
	effect := effectLinked
	if transition.To == domain.RelationshipProvisional {
		effect = effectProvisional
	}
	return signedInLink{kept: card, effect: effect}, linked, err
}

func (s *Service) mergeInto(
	ctx context.Context, repos repositories.Repositories, existing repositories.Relationship, card repositories.Relationship,
	cause domain.RelationshipCause, now time.Time,
) (signedInLink, bool, error) {
	if _, allowed := domain.NextRelationshipState(card.State, domain.CauseMerged); !allowed {
		return signedInLink{}, false, nil
	}
	merged, err := repos.Relationships.MergeCard(ctx, card.TutorID, card.ID, now)
	if err != nil || !merged {
		return signedInLink{}, false, err
	}
	link := signedInLink{kept: existing, effect: effectMerged, reactivated: existing.State == domain.RelationshipEnded}
	if existing.State == domain.RelationshipActive {
		return link, true, nil
	}
	transition, allowed := domain.NextRelationshipState(existing.State, cause)
	if !allowed || transition.To != domain.RelationshipActive {
		return signedInLink{}, false, nil
	}
	activated, err := repos.Relationships.ActivateLinked(ctx, existing, now)
	link.keptReclaim = existing.State == domain.RelationshipReclaimPending
	return link, activated, err
}

func (s *Service) announceSignedIn(ctx context.Context, student domain.UserID, link signedInLink) {
	attributes := append(cardAttributes(link.kept.TutorID, link.kept.ID), slog.String("user_id", student.String()))
	accepted := append(append([]slog.Attr{}, attributes...), slog.String("path", "signed_in"), slog.String("effect", string(link.effect)))
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "invitation.accepted", accepted...)
	if link.reactivated {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "relationship.reactivated", attributes...)
	}
	if link.keptReclaim {
		s.logReclaimDecided(ctx, student, reclaimDecision{kept: 1}, decidedByOwner)
	}
	s.logUnsuppressed(ctx, link.unsuppressed)
	s.logNotified(ctx, link.notified)
}
