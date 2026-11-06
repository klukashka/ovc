package meetings

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const recoveryPassBudget = 20 * time.Second
const recoveryPassLimit = 100

type RecoveryScanner struct {
	store     *repositories.Store
	random    core.RandomSource
	logger    *slog.Logger
	monotonic func() time.Time
	permit    chan struct{}
}

func NewRecoveryScanner(store *repositories.Store, random core.RandomSource, logger *slog.Logger, monotonic func() time.Time) *RecoveryScanner {
	return &RecoveryScanner{store: store, random: random, logger: logger, monotonic: monotonic, permit: make(chan struct{}, 1)}
}

func (s *RecoveryScanner) Run(ctx context.Context, now time.Time) (queued int64, err error) {
	ctx, cancel := context.WithTimeout(ctx, recoveryPassBudget)
	defer cancel()
	deadline := s.monotonic().Add(recoveryPassBudget)
	var removed int64
	defer func() {
		attributes := []slog.Attr{slog.Int64("queued", queued), slog.Int64("removed", removed)}
		if err != nil {
			level := slog.LevelWarn
			if errors.Is(err, context.Canceled) {
				level = slog.LevelDebug
			}
			s.logger.LogAttrs(ctx, level, "meeting.recovery.failed", append(attributes, slog.String("kind", core.ErrorKind(err)))...)
			return
		}
		s.logger.LogAttrs(ctx, slog.LevelDebug, "meeting.recovery.scanned", attributes...)
	}()
	select {
	case s.permit <- struct{}{}:
		defer func() { <-s.permit }()
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	queued, err = s.scan(ctx, now, deadline)
	if budgetErr := s.checkBudget(ctx, deadline); budgetErr != nil {
		return queued, errors.Join(err, budgetErr)
	}
	var cleanupErr error
	removed, cleanupErr = s.store.Repositories().MeetingRecovery.CleanupReceipts(ctx, now, recoveryPassLimit)
	return queued, errors.Join(err, cleanupErr)
}

func (s *RecoveryScanner) scan(ctx context.Context, now, deadline time.Time) (int64, error) {
	repo := s.store.Repositories().MeetingRecovery
	cursor, err := repo.Cursor(ctx)
	if err != nil {
		return 0, err
	}
	candidates, err := repo.CleanupCandidates(ctx, repositories.MeetingRecoveryPage{After: cursor.AfterProviderMeetingID, Limit: recoveryPassLimit})
	if err != nil {
		return 0, err
	}
	var queued int64
	var failures []error
	for _, candidate := range candidates {
		if err := s.checkBudget(ctx, deadline); err != nil {
			return queued, errors.Join(append(failures, err)...)
		}
		advanced, err := repo.AdvanceCursor(ctx, cursor, candidate.ProviderMeetingID)
		if err != nil {
			return queued, errors.Join(append(failures, err)...)
		}
		if advanced != domain.MeetingMutationApplied {
			return queued, errors.Join(failures...)
		}
		cursor.AfterProviderMeetingID = candidate.ProviderMeetingID
		cursor.Revision++
		applied, err := s.queue(ctx, candidate, now)
		if err != nil {
			failures = append(failures, err)
		} else if applied {
			queued++
		}
	}
	if len(candidates) < recoveryPassLimit {
		if err := s.checkBudget(ctx, deadline); err != nil {
			return queued, errors.Join(append(failures, err)...)
		}
		if _, err := repo.AdvanceCursor(ctx, cursor, domain.ProviderMeetingID{}); err != nil {
			failures = append(failures, err)
		}
	}
	return queued, errors.Join(failures...)
}

func (s *RecoveryScanner) queue(ctx context.Context, row repositories.StoredMeetingRecovery, now time.Time) (bool, error) {
	if row.CycleResult == domain.MeetingRecoveryCyclePending || (row.CycleNumber > 0 && (!row.FinalCycleRequested || row.FinalCycleUsed)) {
		return false, nil
	}
	id, err := core.NewID[domain.OutboxID](s.random)
	if err != nil {
		return false, err
	}
	result, err := s.store.Repositories().MeetingRecovery.QueueCycle(ctx, repositories.MeetingRecoveryQueue{
		ProviderMeetingID: row.ProviderMeetingID, Revision: row.Revision, OutboxID: id, Mode: domain.MeetingRecoveryAutomatic, Now: now,
	})
	return result == domain.MeetingMutationApplied, err
}

func (s *RecoveryScanner) checkBudget(ctx context.Context, deadline time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.monotonic().Before(deadline) {
		return context.DeadlineExceeded
	}
	return nil
}
