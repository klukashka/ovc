package tutoring

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	cardEmailRetention   = 90 * 24 * time.Hour
	cardErasureBatch     = 100
	cardErasureMaxRounds = 10
)

func (s *Service) EraseCardEmails(ctx context.Context, now time.Time) (int64, error) {
	dueBefore := now.Add(-cardEmailRetention)
	var erased int64
	var failures []error
	var after *repositories.CardDueForErasure
	for range cardErasureMaxRounds {
		due, err := s.deps.Store.Repositories().Relationships.DueForEmailErasure(ctx, dueBefore, after, cardErasureBatch)
		if err != nil {
			return erased, errors.Join(append(failures, err)...)
		}
		for _, card := range due {
			done, err := s.eraseCardEmail(ctx, card, dueBefore, now)
			if err != nil {
				failures = append(failures, err)
				s.deps.Logger.LogAttrs(ctx, slog.LevelError, "cards.erase_emails_failed",
					slog.String("relationship_id", card.ID.String()), slog.String("kind", core.ErrorKind(err)))
				continue
			}
			if done {
				erased++
				s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "card.email_erased", cardAttributes(card.TutorID, card.ID)...)
			}
		}
		if len(due) < cardErasureBatch {
			break
		}
		after = &due[len(due)-1]
	}
	return erased, errors.Join(failures...)
}

func (s *Service) eraseCardEmail(ctx context.Context, card repositories.CardDueForErasure, dueBefore time.Time, now time.Time) (bool, error) {
	erased := false
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		err := repos.TutorProfiles.Lock(ctx, card.TutorID)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		locked, err := repos.Relationships.LockForTutor(ctx, card.TutorID, card.ID)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !dueForErasure(locked, dueBefore) {
			return nil
		}
		applied, err := repos.Relationships.EraseEmail(ctx, card, dueBefore, now)
		if err != nil || !applied {
			return err
		}
		if _, err := repos.Invitations.RevokeOpen(ctx, card.TutorID, card.ID, domain.RevokeEmailErased, now); err != nil {
			return err
		}
		erased = true
		return cardChanged(ctx, repos, card.TutorID, card.ID)
	})
	return erased, err
}

func dueForErasure(card repositories.Relationship, dueBefore time.Time) bool {
	return card.State == domain.RelationshipInvited && card.Email != nil && card.EmailSetAt != nil && !card.EmailSetAt.After(dueBefore)
}
