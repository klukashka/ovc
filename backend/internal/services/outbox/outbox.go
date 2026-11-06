package outbox

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	LeaseDuration    = 5 * time.Minute
	doneRetention    = 7 * 24 * time.Hour
	deadRetention    = 30 * 24 * time.Hour
	cleanupBatchSize = 500
	cleanupMaxRounds = 20
)

type Row = repositories.ClaimedOutboxRow

type Claim struct {
	Row   Row
	Lane  domain.OutboxLane
	Lease domain.LeaseToken
}

type Service struct {
	store  *repositories.Store
	random core.RandomSource
	logger *slog.Logger
}

func New(store *repositories.Store, random core.RandomSource, logger *slog.Logger) *Service {
	return &Service{store: store, random: random, logger: logger}
}

func (s *Service) Claim(ctx context.Context, lane domain.OutboxLane, limit int32, now time.Time) ([]Claim, error) {
	lease, err := core.NewID[domain.LeaseToken](s.random)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.Repositories().Outbox.Claim(ctx, repositories.OutboxClaim{
		Lane: lane, Now: now, Limit: limit, Lease: lease, LeaseEnds: now.Add(LeaseDuration),
	})
	if err != nil {
		return nil, err
	}
	claims := make([]Claim, 0, len(rows))
	for _, row := range rows {
		s.logger.LogAttrs(ctx, slog.LevelDebug, "outbox.row.claimed", rowAttributes(lane, row)...)
		claims = append(claims, Claim{Row: row, Lane: lane, Lease: lease})
	}
	return claims, nil
}

var ErrMeetingRecoveryOutcomeRequired = errors.New("meeting cleanup requires its typed durable outcome")

func (s *Service) Complete(ctx context.Context, claim Claim, reason domain.OutboxDoneReason, now time.Time) error {
	if claim.Row.Kind == domain.KindMeetingCleanup {
		return ErrMeetingRecoveryOutcomeRequired
	}
	completed, err := s.store.Repositories().Outbox.Complete(ctx, repositories.OutboxCompletion{
		ID: claim.Row.ID, Lease: claim.Lease, Reason: reason, Now: now,
	})
	if err != nil {
		return err
	}
	s.logLeaseOutcome(ctx, claim, completed, slog.LevelInfo, "outbox.row.done", slog.String("reason", string(reason)))
	return nil
}

func (s *Service) Fail(ctx context.Context, claim Claim, cause error, now time.Time) error {
	code := errorCode(cause)
	outbox := s.store.Repositories().Outbox
	if errors.Is(cause, domain.ErrPermanentEffect) || claim.Row.Attempts >= domain.OutboxMaxAttempts {
		failure := repositories.OutboxFailure{ID: claim.Row.ID, Lease: claim.Lease, Now: now, ErrorCode: code}
		var failed bool
		var err error
		if claim.Row.Kind == domain.KindMeetingCleanup {
			failed, err = s.store.FailMeetingRecovery(ctx, failure)
		} else {
			failed, err = outbox.Fail(ctx, failure)
		}
		if err != nil {
			return err
		}
		s.logLeaseOutcome(ctx, claim, failed, slog.LevelError, "outbox.row.dead", slog.String("error_code", code))
		return nil
	}
	fraction, err := core.RandomFraction(s.random)
	if err != nil {
		return err
	}
	retried, err := outbox.Retry(ctx, repositories.OutboxRetry{
		ID: claim.Row.ID, Lease: claim.Lease, NextAttemptAt: now.Add(domain.OutboxRetryDelay(claim.Row.Attempts, fraction)), ErrorCode: code,
	})
	if err != nil {
		return err
	}
	s.logLeaseOutcome(ctx, claim, retried, slog.LevelWarn, "outbox.row.retried", slog.String("error_code", code))
	return nil
}

func (s *Service) ReclaimExpired(ctx context.Context, now time.Time) error {
	reclaimed, err := s.store.Repositories().Outbox.ReclaimExpired(ctx, now)
	if err != nil {
		return err
	}
	meetings, meetingErr := s.store.ReclaimMeetingRecovery(ctx, now, 100)
	reclaimed = append(reclaimed, meetings...)
	for _, row := range reclaimed {
		attributes := []slog.Attr{slog.String("outbox_id", hex.EncodeToString(row.ID[:])), slog.String("kind", string(row.Kind))}
		if row.Status == domain.OutboxDead {
			s.logger.LogAttrs(ctx, slog.LevelError, "outbox.row.dead", append(attributes, slog.String("error_code", "lease_expired"))...)
			continue
		}
		s.logger.LogAttrs(ctx, slog.LevelWarn, "outbox.lease.reclaimed", attributes...)
	}
	return meetingErr
}

func (s *Service) Cleanup(ctx context.Context, now time.Time) (int64, error) {
	var deleted int64
	for range cleanupMaxRounds {
		rows, err := s.store.Repositories().Outbox.DeleteFinished(ctx, repositories.FinishedOutboxPurge{
			DoneBefore: now.Add(-doneRetention), DeadBefore: now.Add(-deadRetention), BatchSize: cleanupBatchSize,
		})
		if err != nil {
			return deleted, err
		}
		deleted += rows
		if rows < cleanupBatchSize {
			break
		}
	}
	return deleted, nil
}

func (s *Service) JobDue(ctx context.Context, job domain.JobName, every time.Duration, now time.Time) (bool, error) {
	run, err := s.store.Repositories().JobRuns.ByJob(ctx, job)
	if errors.Is(err, repositories.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return !run.LastRunAt.Add(every).After(now), nil
}

func (s *Service) RecordJob(ctx context.Context, job domain.JobName, result domain.JobResult, now time.Time) error {
	return s.store.Repositories().JobRuns.Record(ctx, repositories.JobRunRecord{Job: job, LastRunAt: now, LastResult: result})
}

func (s *Service) logLeaseOutcome(ctx context.Context, claim Claim, applied bool, level slog.Level, event string, extra slog.Attr) {
	attributes := append(rowAttributes(claim.Lane, claim.Row), extra)
	if !applied {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "outbox.lease.lost", attributes...)
		return
	}
	s.logger.LogAttrs(ctx, level, event, attributes...)
}

func rowAttributes(lane domain.OutboxLane, row repositories.ClaimedOutboxRow) []slog.Attr {
	return []slog.Attr{
		slog.String("outbox_id", hex.EncodeToString(row.ID[:])),
		slog.String("lane", string(lane)),
		slog.String("kind", string(row.Kind)),
		slog.Int("attempts", int(row.Attempts)),
	}
}

func errorCode(cause error) string {
	if errors.Is(cause, domain.ErrPermanentEffect) {
		return "permanent"
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return "timeout"
	}
	return core.ErrorKind(cause)
}
