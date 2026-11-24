package export

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func (s *Service) Recover(ctx context.Context, now time.Time) (int64, error) {
	if !s.recoveryMutex.TryLock() {
		return 0, nil
	}
	defer s.recoveryMutex.Unlock()
	bounded, cancel := context.WithTimeout(ctx, domain.ExportRecoveryBudget)
	defer cancel()
	cutoff := now.Add(-domain.ExportRecoveryAge)
	ids, err := s.deps.Store.Repositories().Exports.StaleBuilding(bounded, cutoff, domain.ExportRecoveryBatch)
	if err != nil {
		return 0, err
	}
	var recovered int64
	var failures error
	for _, id := range ids {
		failed, err := s.recoverOne(bounded, id, cutoff, now)
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		if failed != nil {
			recovered++
			s.deps.Logger.LogAttrs(bounded, slog.LevelWarn, "export.failed", exportAttribute(failed.ID), buildAttribute(failed.BuildID),
				userAttribute(failed.UserID), slog.String("failure", string(domain.ExportFailureTooLong)),
				slog.Int64("duration_ms", now.Sub(*failed.BuildStartedAt).Milliseconds()))
		}
	}
	return recovered, failures
}

func (s *Service) recoverOne(ctx context.Context, id domain.ExportID, cutoff time.Time, now time.Time) (*repositories.Export, error) {
	var failed *repositories.Export
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		row, err := repos.Exports.Lock(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if row.State != domain.ExportBuilding || row.BuildID == nil || row.BuildStartedAt == nil || row.BuildStartedAt.After(cutoff) {
			return nil
		}
		if _, err := repos.Exports.Fail(ctx, row.ID, domain.ExportFailureTooLong, now); err != nil {
			return err
		}
		if err := s.requestCleanupIfOpen(ctx, repos, *row.BuildID, now); err != nil {
			return err
		}
		failed = &row
		return s.notifyOwner(ctx, repos, row, *row.BuildID, domain.NotificationExportFailed, now)
	})
	return failed, err
}

func (s *Service) Expire(ctx context.Context, now time.Time) (int64, error) {
	var handled int64
	sweeps := []struct {
		due    func(context.Context, repositories.Repositories, time.Time) ([]domain.ExportID, error)
		settle func(context.Context, repositories.Repositories, repositories.Export, time.Time) (bool, error)
	}{
		{dueReady, s.expireReady},
		{dueFinished, s.deleteFinished},
	}
	for _, sweep := range sweeps {
		for range domain.ExportExpireRounds {
			ids, err := sweep.due(ctx, s.deps.Store.Repositories(), now)
			if err != nil {
				return handled, err
			}
			for _, id := range ids {
				settled, err := s.settleExpiry(ctx, id, now, sweep.settle)
				if err != nil {
					return handled, err
				}
				if settled {
					handled++
				}
			}
			if len(ids) < domain.ExportExpireBatch {
				break
			}
		}
	}
	return handled, nil
}

func dueReady(ctx context.Context, repos repositories.Repositories, now time.Time) ([]domain.ExportID, error) {
	return repos.Exports.DueReady(ctx, now, domain.ExportExpireBatch)
}

func dueFinished(ctx context.Context, repos repositories.Repositories, now time.Time) ([]domain.ExportID, error) {
	return repos.Exports.DueFinished(ctx, now.Add(-domain.ExportRowRetention), domain.ExportExpireBatch)
}

func (s *Service) settleExpiry(ctx context.Context, id domain.ExportID, now time.Time, settle func(context.Context, repositories.Repositories, repositories.Export, time.Time) (bool, error)) (bool, error) {
	settled := false
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		row, err := repos.Exports.Lock(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		settled, err = settle(ctx, repos, row, now)
		return err
	})
	return settled, err
}

func (s *Service) expireReady(ctx context.Context, repos repositories.Repositories, row repositories.Export, now time.Time) (bool, error) {
	if row.State != domain.ExportReady || row.ExpiresAt == nil || row.ExpiresAt.After(now) {
		return false, nil
	}
	expired, err := repos.Exports.Expire(ctx, row.ID, now)
	if err != nil || !expired {
		return false, err
	}
	return true, s.queueObjectDeletion(ctx, repos, row, now)
}

func (s *Service) deleteFinished(ctx context.Context, repos repositories.Repositories, row repositories.Export, now time.Time) (bool, error) {
	finished := row.BuiltAt
	if row.State == domain.ExportExpired && row.ExpiresAt != nil {
		finished = row.ExpiresAt
	}
	if finished == nil {
		finished = &row.RequestedAt
	}
	if row.State != domain.ExportFailed && row.State != domain.ExportExpired || finished.After(now.Add(-domain.ExportRowRetention)) {
		return false, nil
	}
	return true, s.deleteLocked(ctx, repos, row, now)
}

func (s *Service) WithdrawOwn(ctx context.Context, repos repositories.Repositories, user domain.UserID, cause domain.ExportInvalidationCause, now time.Time) error {
	row, found, err := lockOwnExport(ctx, repos, user)
	if err != nil || !found {
		return err
	}
	return s.withdraw(ctx, repos, row, cause, false, now)
}

func (s *Service) withdraw(ctx context.Context, repos repositories.Repositories, row repositories.Export, cause domain.ExportInvalidationCause, notify bool, now time.Time) error {
	switch row.State {
	case domain.ExportBuilding:
		if _, err := repos.Exports.Fail(ctx, row.ID, domain.ExportFailureWithdrawn, now); err != nil {
			return err
		}
		if err := s.requestCleanupIfOpen(ctx, repos, *row.BuildID, now); err != nil {
			return err
		}
	case domain.ExportReady:
		if _, err := repos.Exports.Fail(ctx, row.ID, domain.ExportFailureWithdrawn, now); err != nil {
			return err
		}
		if _, err := repos.Exports.ClearStorage(ctx, row.ID, now); err != nil {
			return err
		}
		if err := s.queueObjectDeletion(ctx, repos, row, now); err != nil {
			return err
		}
		if _, err := repos.Exports.DeleteLimitOf(ctx, row.UserID, row.ID); err != nil {
			return err
		}
	case domain.ExportQueued, domain.ExportFailed, domain.ExportExpired:
		return nil
	}
	if notify {
		if err := s.notifyOwner(ctx, repos, row, *row.BuildID, domain.NotificationExportFailed, now); err != nil {
			return err
		}
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "export.invalidated", exportAttribute(row.ID), buildAttribute(row.BuildID),
			userAttribute(row.UserID), slog.String("state", string(row.State)), slog.String("cause", string(cause)))
	})
	return nil
}

func (s *Service) DeleteForPurge(ctx context.Context, repos repositories.Repositories, user domain.UserID, now time.Time) error {
	row, found, err := lockOwnExport(ctx, repos, user)
	if err != nil {
		return err
	}
	if found {
		if err := s.deleteLocked(ctx, repos, row, now); err != nil {
			return err
		}
	}
	_, err = repos.Exports.DeleteLimit(ctx, user)
	return err
}
