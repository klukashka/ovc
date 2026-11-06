package outbox

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type MeetingRecoveryOutcome struct {
	Revision           domain.MeetingRecoveryRevision
	ObservedCreateTime *domain.ProviderCreateTime
	Result             domain.MeetingRecoveryCycleResult
}

func (s *Service) CompleteMeetingRecovery(ctx context.Context, claim Claim, outcome MeetingRecoveryOutcome, now time.Time) error {
	if claim.Row.Kind != domain.KindMeetingCleanup {
		return ErrMeetingRecoveryOutcomeRequired
	}
	completed, err := s.store.FinishMeetingRecovery(ctx, repositories.MeetingRecoveryEffectCompletion{
		OutboxID: claim.Row.ID, Lease: claim.Lease, Revision: outcome.Revision, ObservedCreateTime: outcome.ObservedCreateTime, Result: outcome.Result, Now: now,
	})
	if err != nil {
		return err
	}
	s.logLeaseOutcome(ctx, claim, completed, slog.LevelInfo, "outbox.row.done", slog.String("reason", string(outcome.Result)))
	return nil
}
