package outbox

import (
	"context"
	"encoding/hex"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type MeetingRecoveryOutcome struct {
	Revision           domain.MeetingRecoveryRevision
	ObservedCreateTime *domain.ProviderCreateTime
	EvaluatedAt        *time.Time
	Settlement         domain.MeetingRecoverySettlement
	Result             domain.MeetingRecoveryCycleResult
}

func (s *Service) CompleteMeetingRecovery(ctx context.Context, claim Claim, outcome MeetingRecoveryOutcome, now time.Time) error {
	if claim.Row.Kind != domain.KindMeetingCleanup {
		return ErrMeetingRecoveryOutcomeRequired
	}
	completed, err := s.store.FinishMeetingRecovery(ctx, repositories.MeetingRecoveryEffectCompletion{
		OutboxID: claim.Row.ID, Lease: claim.Lease, Revision: outcome.Revision, ObservedCreateTime: outcome.ObservedCreateTime, EvaluatedAt: outcome.EvaluatedAt, Result: outcome.Result, Now: now,
	})
	if err != nil {
		return err
	}
	s.logLeaseOutcome(ctx, claim, completed, slog.LevelInfo, "outbox.row.done", slog.String("reason", string(outcome.Result)))
	if completed && outcome.Result == domain.MeetingRecoveryCycleSettled {
		s.logger.LogAttrs(ctx, slog.LevelInfo, "meeting.recovery.settled", slog.String("outbox_id", hex.EncodeToString(claim.Row.ID[:])), slog.String("settlement", string(outcome.Settlement)))
	}
	return nil
}
