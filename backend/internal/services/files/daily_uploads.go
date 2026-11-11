package files

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	dailyUploadCleanupBudget = 20 * time.Second
	dailyCleanupBatches      = 10
	dailyCleanupBatchRows    = 100
)

type AccountingUnavailableError struct {
	RetryAfter time.Duration
}

func (*AccountingUnavailableError) Error() string {
	return "student chat upload accounting is unavailable"
}

func (s *Service) checkDailyActivation(ctx context.Context, repos repositories.Repositories, policy uploadPolicy, opened openUpload, now time.Time) error {
	if policy.daily != chatDailyAggregate || opened.uploader.Role != domain.RoleStudent {
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

func (p uploadPolicy) dailyCap(opened openUpload) (int64, bool) {
	if opened.uploader.Role != domain.RoleStudent {
		return 0, false
	}
	switch p.daily {
	case chatDailyAggregate:
		return domain.ChatDailyBytesPerTutor, true
	case answerDailyAggregate:
		return domain.AnswerDailyBytesPerTutor, true
	case noDailyAggregate:
		return 0, false
	}
	return 0, false
}

func dailyBytes(ctx context.Context, repos repositories.Repositories, aggregate dailyAggregate, opened openUpload, day time.Time) (int64, error) {
	if aggregate == answerDailyAggregate {
		return repos.DailyUploads.AnswerBytes(ctx, opened.uploader.UserID, opened.tutor, day)
	}
	return repos.DailyUploads.Bytes(ctx, opened.uploader.UserID, opened.tutor, day)
}

func addDailyBytes(ctx context.Context, repos repositories.Repositories, aggregate dailyAggregate, opened openUpload, day time.Time, size int64) error {
	if aggregate == answerDailyAggregate {
		return repos.DailyUploads.AddAnswer(ctx, opened.uploader.UserID, opened.tutor, day, size)
	}
	return repos.DailyUploads.Add(ctx, opened.uploader.UserID, opened.tutor, day, size)
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
		batches := []func(context.Context, []domain.UserID, time.Time) (int64, error){
			repos.DailyUploads.DeletePastBatch, repos.DailyUploads.DeletePastAnswerBatch,
		}
		for range dailyCleanupBatches {
			if len(batches) == 0 {
				break
			}
			count, err := batches[0](bounded, students, utcDay(now))
			if err != nil {
				return err
			}
			deleted += count
			if count < dailyCleanupBatchRows {
				batches = batches[1:]
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}
