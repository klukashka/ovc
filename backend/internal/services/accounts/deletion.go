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
	unverifiedLifetime               = 7 * 24 * time.Hour
	purgeBatch                       = 100
	purgeRounds                      = 20
	tutorPurgeBatchesPerRun          = 100
	unverifiedPurgeBatchesPerRequest = 20
)

var ErrPurgeIncomplete = errors.New("account purge exhausted its request batch budget")

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
	announce := Announce(Silent)
	for batches := 0; ; batches++ {
		if batches == unverifiedPurgeBatchesPerRequest {
			return ErrPurgeIncomplete
		}
		var progressed int64
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
				progressed, err = s.unverifiedPurgeBatch(ctx, repos, user, now)
				if err != nil || progressed > 0 {
					return err
				}
				immediate = true
				announce, err = s.purger().purge(ctx, repos, principal.UserID, now, 0)
				return err
			case domain.AccountActive:
				if _, err := repos.Users.MarkDeleted(ctx, principal.UserID, now); err != nil {
					return err
				}
				if err := closeEmailRequests(ctx, repos, principal.UserID); err != nil {
					return err
				}
				if _, err := repos.Sessions.RevokeAllOf(ctx, principal.UserID, now); err != nil {
					return err
				}
				announce, err = s.deps.Tutoring.OnAccountDeleted(ctx, repos, principal.UserID, principal.Role, now)
				if err != nil {
					return err
				}
				return s.deps.Exports.WithdrawOwn(ctx, repos, principal.UserID, domain.ExportCauseAccountDeleted, now)
			case domain.AccountBlocked, domain.AccountDeleted:
			}
			return ErrUnauthenticated
		})
		if err != nil {
			return err
		}
		if progressed == 0 {
			break
		}
	}
	purge := "scheduled"
	if immediate {
		purge = "immediate"
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "account.deleted", userAttribute(principal.UserID), slog.String("purge", purge))
	announce(ctx)
	return nil
}

func (s *Service) unverifiedPurgeBatch(ctx context.Context, repos repositories.Repositories, user repositories.User, now time.Time) (int64, error) {
	if user.Role == domain.RoleStudent {
		return s.deps.Tutoring.PurgeStudentHomeworkBatch(ctx, repos, user.ID, now)
	}
	return s.deps.Tutoring.PurgeBatch(ctx, repos, user.ID, user.Role, now)
}

func (s *Service) verifyCurrentPassword(ctx context.Context, id domain.UserID, password string, field string, now time.Time) (repositories.User, error) {
	slot := s.limitSlot(currentPasswordLimit, id.String(), now)
	count, err := s.deps.Store.Repositories().RateLimits.Reserve(ctx, slot)
	if err != nil {
		return repositories.User{}, err
	}
	if count > currentPasswordLimit.limit {
		if err := s.release(context.WithoutCancel(ctx), slot); err != nil {
			return repositories.User{}, err
		}
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
	store    *repositories.Store
	uploads  UploadPurger
	tutoring TutoringEffects
	exports  ExportEffects
	logger   *slog.Logger
}

func NewPurger(store *repositories.Store, uploads UploadPurger, tutoring TutoringEffects, exports ExportEffects, logger *slog.Logger) *Purger {
	if uploads == nil || tutoring == nil || exports == nil {
		panic("purging accounts needs an upload purger, the tutoring effects and the export effects")
	}
	return &Purger{store: store, uploads: uploads, tutoring: tutoring, exports: exports, logger: logger}
}

func (s *Service) purger() *Purger {
	return NewPurger(s.deps.Store, s.deps.Uploads, s.deps.Tutoring, s.deps.Exports, s.deps.Logger)
}

func (p *Purger) purge(ctx context.Context, repos repositories.Repositories, user domain.UserID, now time.Time, batches int) (Announce, error) {
	account, err := repos.Users.ByID(ctx, user)
	if err != nil {
		return nil, err
	}
	announce, err := p.tutoring.OnAccountPurged(ctx, repos, user, account.Role, now)
	if err != nil {
		return nil, err
	}
	if err := repos.Messages.ForgetDigestOf(ctx, user); err != nil {
		return nil, err
	}
	if err := p.uploads.PurgeUploadsOf(ctx, repos, user, now); err != nil {
		return nil, err
	}
	if _, err := repos.Lessons.DeleteRosterOfStudent(ctx, user); err != nil {
		return nil, err
	}
	if _, err := repos.Meetings.DeleteParticipantsOf(ctx, user); err != nil {
		return nil, err
	}
	if err := p.exports.DeleteForPurge(ctx, repos, user, now); err != nil {
		return nil, err
	}
	if _, err := repos.Users.Delete(ctx, user); err != nil {
		return nil, err
	}
	if _, err := repos.Notifications.DeleteNaming(ctx, user); err != nil {
		return nil, err
	}
	p.logger.LogAttrs(ctx, slog.LevelInfo, "account.purged", userAttribute(user), slog.Int("batches", batches))
	return announce, nil
}

func (p *Purger) PurgeDeleted(ctx context.Context, now time.Time) (int64, error) {
	return p.purgeDue(ctx, now, func(ctx context.Context, repos repositories.Repositories, skip int32) ([]domain.UserID, error) {
		return repos.Users.DeletedBefore(ctx, now.Add(-domain.DeletedAccountRetention), skip, purgeBatch)
	})
}

func (p *Purger) CleanupUnverified(ctx context.Context, now time.Time) (int64, error) {
	return p.purgeDue(ctx, now, func(ctx context.Context, repos repositories.Repositories, skip int32) ([]domain.UserID, error) {
		return repos.Users.UnverifiedSince(ctx, now.Add(-unverifiedLifetime), skip, purgeBatch)
	})
}

type purgeRun struct {
	batchesLeft int
}

func (r *purgeRun) exhausted() bool {
	return r.batchesLeft <= 0
}

type duePurge struct {
	id      domain.UserID
	batches int
}

func (p *Purger) purgeDue(ctx context.Context, now time.Time, due func(context.Context, repositories.Repositories, int32) ([]domain.UserID, error)) (int64, error) {
	var purged int64
	var failures []error
	var skipped int32
	run := &purgeRun{batchesLeft: tutorPurgeBatchesPerRun}
	for range purgeRounds {
		ids, err := due(ctx, p.store.Repositories(), skipped)
		if err != nil {
			return purged, errors.Join(append(failures, err)...)
		}
		pending := make([]duePurge, 0, len(ids))
		for _, id := range ids {
			pending = append(pending, duePurge{id: id})
		}
		for len(pending) > 0 && !run.exhausted() {
			var next []duePurge
			for _, account := range pending {
				if run.exhausted() {
					next = append(next, account)
					continue
				}
				finished, done, err := p.purgeStep(ctx, account, now, run)
				switch {
				case err != nil:
					skipped++
					failures = append(failures, err)
					p.logger.LogAttrs(ctx, slog.LevelError, "account.purge_failed", userAttribute(account.id), slog.String("kind", core.ErrorKind(err)))
				case done:
					purged++
				case !finished:
					account.batches++
					next = append(next, account)
				}
			}
			pending = next
		}
		for _, account := range pending {
			p.logger.LogAttrs(ctx, slog.LevelInfo, "account.purge_paused", userAttribute(account.id), slog.Int("batches", account.batches))
		}
		if run.exhausted() || len(ids) < purgeBatch {
			break
		}
	}
	return purged, errors.Join(failures...)
}

func (p *Purger) purgeStep(ctx context.Context, account duePurge, now time.Time, run *purgeRun) (bool, bool, error) {
	finished, purged := false, false
	announce := Announce(Silent)
	err := p.store.WithTx(ctx, func(repos repositories.Repositories) error {
		state, err := repos.Users.LockForPurge(ctx, account.id)
		if errors.Is(err, repositories.ErrNotFound) {
			finished = true
			return nil
		}
		if err != nil {
			return err
		}
		if !purgeIsDue(state, now) {
			finished = true
			return nil
		}
		deleted, err := p.tutoring.PurgeBatch(ctx, repos, account.id, state.Role, now)
		if err != nil || deleted > 0 {
			return err
		}
		finished, purged = true, true
		announce, err = p.purge(ctx, repos, account.id, now, account.batches)
		return err
	})
	if err != nil {
		return false, false, err
	}
	if finished {
		announce(ctx)
		return true, purged, nil
	}
	run.batchesLeft--
	return false, false, nil
}

func purgeIsDue(state repositories.PurgeState, now time.Time) bool {
	deletedLongAgo := state.State == domain.AccountDeleted && state.DeletedAt != nil && !state.DeletedAt.After(now.Add(-domain.DeletedAccountRetention))
	unverifiedTooLong := state.State == domain.AccountUnverified && !state.UnverifiedSince.After(now.Add(-unverifiedLifetime))
	return deletedLongAgo || unverifiedTooLong
}

func (s *Service) VerifyCurrentPassword(ctx context.Context, id domain.UserID, password string, now time.Time) (repositories.User, error) {
	return s.verifyCurrentPassword(ctx, id, password, fieldPassword, now)
}
