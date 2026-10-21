package accounts

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	deletedRetention   = 30 * 24 * time.Hour
	unverifiedLifetime = 7 * 24 * time.Hour
	purgeBatch         = 100
	purgeRounds        = 20
)

type UploadPurger interface {
	PurgeUploadsOf(ctx context.Context, repos repositories.Repositories, user domain.UserID, now time.Time) error
}

func (s *Service) DeleteAccount(ctx context.Context, principal Principal, password string) error {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	user, err := s.verifyCurrentPassword(ctx, principal.UserID, password, fieldPassword, now)
	if err != nil {
		return err
	}
	immediate := false
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := repos.Users.LockCredentials(ctx, principal.UserID)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(locked.PasswordHash), []byte(user.PasswordHash)) != 1 {
			return invalid(fieldPassword, domain.FieldIncorrect)
		}
		switch locked.State {
		case domain.AccountUnverified:
			immediate = true
			return s.purge(ctx, repos, principal.UserID, now)
		case domain.AccountActive:
			if _, err := repos.Users.MarkDeleted(ctx, principal.UserID, now); err != nil {
				return err
			}
			if err := closeEmailRequests(ctx, repos, principal.UserID); err != nil {
				return err
			}
			_, err := repos.Sessions.RevokeAllOf(ctx, principal.UserID, now)
			return err
		case domain.AccountBlocked, domain.AccountDeleted:
		}
		return ErrUnauthenticated
	})
	if err != nil {
		return err
	}
	purge := "scheduled"
	if immediate {
		purge = "immediate"
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "account.deleted", userAttribute(principal.UserID), slog.String("purge", purge))
	return nil
}

func (s *Service) verifyCurrentPassword(ctx context.Context, id domain.UserID, password string, field string, now time.Time) (repositories.User, error) {
	slot := s.limitSlot(currentPasswordLimit, id.String(), now)
	count, err := s.deps.Store.Repositories().RateLimits.Reserve(ctx, slot)
	if err != nil {
		return repositories.User{}, err
	}
	if count > currentPasswordLimit.limit {
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "auth.password.throttled", userAttribute(id))
		_, retryAfter := domain.FixedWindow(now, currentPasswordLimit.window)
		return repositories.User{}, &ThrottledError{RetryAfter: retryAfter}
	}
	user, err := s.deps.Store.Repositories().Users.ByID(ctx, id)
	if err != nil {
		return repositories.User{}, errors.Join(err, s.release(context.WithoutCancel(ctx), slot))
	}
	verification, err := s.deps.Hasher.Verify(ctx, password, user.PasswordHash)
	if err != nil {
		return repositories.User{}, errors.Join(err, s.release(context.WithoutCancel(ctx), slot))
	}
	if !verification.Matches {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "auth.password.check_refused", userAttribute(id))
		return repositories.User{}, invalid(field, domain.FieldIncorrect)
	}
	if err := s.release(ctx, slot); err != nil {
		return repositories.User{}, err
	}
	return user, nil
}

type Purger struct {
	store   *repositories.Store
	uploads UploadPurger
	logger  *slog.Logger
}

func NewPurger(store *repositories.Store, uploads UploadPurger, logger *slog.Logger) *Purger {
	if uploads == nil {
		panic("purging accounts needs an upload purger")
	}
	return &Purger{store: store, uploads: uploads, logger: logger}
}

func (s *Service) purge(ctx context.Context, repos repositories.Repositories, user domain.UserID, now time.Time) error {
	return NewPurger(s.deps.Store, s.deps.Uploads, s.deps.Logger).purge(ctx, repos, user, now)
}

func (p *Purger) purge(ctx context.Context, repos repositories.Repositories, user domain.UserID, now time.Time) error {
	if err := p.uploads.PurgeUploadsOf(ctx, repos, user, now); err != nil {
		return err
	}
	if _, err := repos.Users.Delete(ctx, user); err != nil {
		return err
	}
	p.logger.LogAttrs(ctx, slog.LevelInfo, "account.purged", userAttribute(user))
	return nil
}

func (p *Purger) PurgeDeleted(ctx context.Context, now time.Time) (int64, error) {
	return p.purgeDue(ctx, now, func(ctx context.Context, repos repositories.Repositories) ([]domain.UserID, error) {
		return repos.Users.DeletedBefore(ctx, now.Add(-deletedRetention), purgeBatch)
	})
}

func (p *Purger) CleanupUnverified(ctx context.Context, now time.Time) (int64, error) {
	return p.purgeDue(ctx, now, func(ctx context.Context, repos repositories.Repositories) ([]domain.UserID, error) {
		return repos.Users.UnverifiedSince(ctx, now.Add(-unverifiedLifetime), purgeBatch)
	})
}

func (p *Purger) purgeDue(ctx context.Context, now time.Time, due func(context.Context, repositories.Repositories) ([]domain.UserID, error)) (int64, error) {
	var purged int64
	var failures []error
	failed := map[domain.UserID]bool{}
	for range purgeRounds {
		ids, err := due(ctx, p.store.Repositories())
		if err != nil {
			return purged, errors.Join(append(failures, err)...)
		}
		progressed := false
		for _, id := range ids {
			if failed[id] {
				continue
			}
			done, err := p.purgeIfDue(ctx, id, now)
			if err != nil {
				failed[id] = true
				failures = append(failures, err)
				p.logger.LogAttrs(ctx, slog.LevelError, "account.purge_failed", userAttribute(id), slog.String("kind", core.ErrorKind(err)))
				continue
			}
			progressed = true
			if done {
				purged++
			}
		}
		if len(ids) < purgeBatch || !progressed {
			break
		}
	}
	return purged, errors.Join(failures...)
}

func (p *Purger) purgeIfDue(ctx context.Context, id domain.UserID, now time.Time) (bool, error) {
	purged := false
	err := p.store.WithTx(ctx, func(repos repositories.Repositories) error {
		state, err := repos.Users.LockForPurge(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		deletedLongAgo := state.State == domain.AccountDeleted && state.DeletedAt != nil && !state.DeletedAt.After(now.Add(-deletedRetention))
		unverifiedTooLong := state.State == domain.AccountUnverified && !state.UnverifiedSince.After(now.Add(-unverifiedLifetime))
		if !deletedLongAgo && !unverifiedTooLong {
			return nil
		}
		purged = true
		return p.purge(ctx, repos, id, now)
	})
	return purged, err
}
