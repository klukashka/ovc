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
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/chat"
)

var errEndedWithoutTime = errors.New("an ended relationship has no ending time")

type StudentEnding struct {
	Tutor   MyTutor
	Removed bool
}

type ending struct {
	link        repositories.Relationship
	by          domain.EndedBy
	memberships int64
	lessons     lessonEffects
	notified    *createdNotification
}

func (s *Service) EndByTutor(ctx context.Context, tutor domain.UserID, id domain.RelationshipID) (Card, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Card{}, err
	}
	var card Card
	var ended *ending
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockActingTutor(ctx, repos, tutor); err != nil {
			return err
		}
		row, err := lockCard(ctx, repos, tutor, id)
		if err != nil {
			return err
		}
		switch row.State {
		case domain.RelationshipDiscarded:
			return ErrCardNotFound
		case domain.RelationshipEnded:
		case domain.RelationshipActive:
			done, err := s.endLink(ctx, repos, row, domain.CauseTutorEnded, now)
			if err != nil {
				return err
			}
			ended, row = &done, done.link
		case domain.RelationshipInvited, domain.RelationshipProvisional, domain.RelationshipReclaimPending:
			return ErrRelationshipNotActive
		}
		card, err = s.fullCard(ctx, repos, tutor, row, now)
		return err
	})
	if err != nil {
		return Card{}, err
	}
	if ended != nil {
		s.logEnded(ctx, *ended)
	}
	return card, nil
}

func (s *Service) StopStudying(ctx context.Context, principal accounts.Principal, id domain.RelationshipID) (StudentEnding, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return StudentEnding{}, err
	}
	var result StudentEnding
	var ended *ending
	var removed *repositories.Relationship
	var lessons lessonEffects
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := accounts.LockActingAccount(ctx, repos, principal, now, principal.State); err != nil {
			return err
		}
		link, err := repos.Relationships.LockOfStudentByID(ctx, principal.UserID, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrCardNotFound
		}
		if err != nil {
			return err
		}
		switch link.State {
		case domain.RelationshipProvisional:
			_, effects, err := s.removeLink(ctx, repos, link, settleEffectsNow, now)
			if err != nil {
				return err
			}
			removed, result.Removed, lessons = &link, true, effects
			return nil
		case domain.RelationshipActive:
			done, err := s.endLink(ctx, repos, link, domain.CauseStudentStopped, now)
			if err != nil {
				return err
			}
			ended = &done
		case domain.RelationshipEnded:
		case domain.RelationshipInvited, domain.RelationshipReclaimPending, domain.RelationshipDiscarded:
			return ErrCardNotFound
		}
		result.Tutor, err = s.myTutor(ctx, repos, principal.UserID, id)
		return err
	})
	if err != nil {
		return StudentEnding{}, err
	}
	if removed != nil {
		s.logRemoved(ctx, *removed, removedByStudent)
		s.logLessonEffects(ctx, lessons)
	}
	if ended != nil {
		s.logEnded(ctx, *ended)
	}
	return result, nil
}

func (s *Service) endLink(
	ctx context.Context, repos repositories.Repositories, link repositories.Relationship, cause domain.RelationshipCause, now time.Time,
) (ending, error) {
	transition, allowed := domain.NextRelationshipState(link.State, cause)
	if !allowed || transition.To != domain.RelationshipEnded || link.StudentID == nil {
		return ending{}, ErrRelationshipNotActive
	}
	student := *link.StudentID
	ended, err := repos.Relationships.End(ctx, link, transition.EndedBy, now)
	if errors.Is(err, repositories.ErrNotFound) {
		return ending{}, fmt.Errorf("end the locked relationship %s: %w", link.ID, err)
	}
	if err != nil {
		return ending{}, err
	}
	groups, err := repos.Relationships.DeleteMemberships(ctx, ended)
	if err != nil {
		return ending{}, err
	}
	audiences := chat.Audiences{Relationships: []domain.RelationshipID{ended.ID}, Groups: groups}
	if err := s.mirror(ctx, repos, domain.PathEnding, audiences, now); err != nil {
		return ending{}, err
	}
	lessons, err := s.endLinkLessons(ctx, repos, ended, student, groups)
	if err != nil {
		return ending{}, err
	}
	if err := relationshipChanged(ctx, repos, ended, true); err != nil {
		return ending{}, err
	}
	done := ending{link: ended, by: transition.EndedBy, memberships: int64(len(groups)), lessons: lessons}
	if transition.EndedBy.EmailsTheOtherParty() {
		if err := s.queueEndedEmail(ctx, repos, ended, now); err != nil {
			return ending{}, err
		}
		done.notified, err = s.notifyEnded(ctx, repos, ended, transition.EndedBy, now)
		if err != nil {
			return ending{}, err
		}
	}
	return done, nil
}

func (s *Service) notifyEnded(
	ctx context.Context, repos repositories.Repositories, ended repositories.Relationship, by domain.EndedBy, now time.Time,
) (*createdNotification, error) {
	if ended.StudentID == nil || ended.EndedAt == nil {
		return nil, fmt.Errorf("relationship %s: %w", ended.ID, errEndedWithoutTime)
	}
	recipient := *ended.StudentID
	if by == domain.EndedByStudent {
		recipient = ended.TutorID
	}
	relationship := ended.ID
	return s.notify(ctx, repos, notificationRow{
		event: domain.NotificationRelationshipEnded, recipient: recipient, tutor: ended.TutorID, student: *ended.StudentID,
		subject: domain.EntityID(ended.ID), relationship: &relationship,
		key: domain.RelationshipEndedKey(ended.ID, ended.EndedAt.UnixMicro()),
	}, now)
}

func (s *Service) queueEndedEmail(ctx context.Context, repos repositories.Repositories, ended repositories.Relationship, now time.Time) error {
	if ended.EndedAt == nil {
		return fmt.Errorf("relationship %s: %w", ended.ID, errEndedWithoutTime)
	}
	identifier := hex.EncodeToString(ended.ID[:])
	endedAt := ended.EndedAt.UnixMicro()
	payload, err := json.Marshal(domain.RelationshipEndedPayload{Relationship: identifier, EndedAtMicros: endedAt})
	if err != nil {
		return fmt.Errorf("encode the relationship ended payload: %w", err)
	}
	id, err := core.NewID[domain.OutboxID](s.deps.Random)
	if err != nil {
		return err
	}
	_, err = repos.Outbox.Insert(ctx, repositories.NewOutboxRow{
		ID: id, Lane: domain.LaneEmail, Priority: domain.PriorityNormal, Kind: domain.EventRelationshipEnded, Payload: payload,
		IdempotencyKey: domain.RelationshipEndedKey(ended.ID, endedAt), NextAttemptAt: now, CreatedAt: now,
	})
	return err
}

func (s *Service) logEnded(ctx context.Context, ended ending) {
	attributes := append(cardAttributes(ended.link.TutorID, ended.link.ID), slog.String("by", string(ended.by)),
		slog.Int64("memberships_removed", ended.memberships))
	if ended.link.StudentID != nil {
		attributes = append(attributes, slog.String("user_id", ended.link.StudentID.String()))
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "relationship.ended", attributes...)
	s.logLessonEffects(ctx, ended.lessons)
	s.logNotified(ctx, ended.notified)
}
