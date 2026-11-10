package files

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const dailyUploadCleanupBudget = 20 * time.Second

type AccountingUnavailableError struct {
	RetryAfter time.Duration
}

func (*AccountingUnavailableError) Error() string {
	return "student chat upload accounting is unavailable"
}

func (s *Service) checkDailyActivation(ctx context.Context, repos repositories.Repositories, policy uploadPolicy, opened openUpload, now time.Time) error {
	if !policy.dailyBytes || opened.uploader.Role != domain.RoleStudent {
		return nil
	}
	activation, err := repos.DailyUploads.Activation(ctx)
	if err != nil {
		s.deps.Logger.LogAttrs(ctx, slog.LevelError, "upload.accounting.unavailable", slog.String("kind", core.ErrorKind(err)))
		return &AccountingUnavailableError{RetryAfter: time.Minute}
	}
	if now.Before(activation) || utcDay(opened.created).Before(activation) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "upload.accounting.cutover")
		return &AccountingUnavailableError{RetryAfter: min(max(activation.Sub(now), time.Second), 24*time.Hour)}
	}
	return nil
}

func (s *Service) CleanupDailyUploads(ctx context.Context, now time.Time) (int64, error) {
	bounded, cancel := context.WithTimeout(ctx, dailyUploadCleanupBudget)
	defer cancel()
	var deleted int64
	err := s.deps.Store.WithTx(bounded, func(repos repositories.Repositories) error {
		students, err := repos.DailyUploads.LockCleanupStudents(bounded, utcDay(now))
		if err != nil || len(students) == 0 {
			return err
		}
		for range 10 {
			count, err := repos.DailyUploads.DeletePastBatch(bounded, students, utcDay(now))
			if err != nil {
				return err
			}
			deleted += count
			if count < 100 {
				break
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}
