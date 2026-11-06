package meetings

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/outbox"
)

type Service struct {
	logger       *slog.Logger
	store        *repositories.Store
	provider     meeting.Provider
	providerKind domain.MeetingProvider
}

func New(store *repositories.Store, provider meeting.Provider, kind domain.MeetingProvider, logger *slog.Logger) *Service {
	return &Service{store: store, provider: provider, providerKind: kind, logger: logger}
}

var errRecoveryEvidence = errors.New("meeting cleanup evidence does not match its pending cycle")
var errProviderSnapshot = errors.New("meeting provider returned an invalid inspection")

func (s *Service) Cleanup(ctx context.Context, row outbox.Row, _ time.Time) (outcome outbox.MeetingRecoveryOutcome, err error) {
	defer func() { s.reportCleanup(ctx, row.ID, outcome, err) }()
	recovery, err := s.cleanupEvidence(ctx, row)
	if err != nil {
		return outbox.MeetingRecoveryOutcome{}, err
	}
	outcome = outbox.MeetingRecoveryOutcome{Revision: recovery.Revision, Result: domain.MeetingRecoveryCycleUnresolved}
	if recovery.CreateState == domain.MeetingCreatePrepared {
		outcome.Result = domain.MeetingRecoveryCycleSettled
		return outcome, ctx.Err()
	}
	info, err := s.inspect(ctx, recovery.ProviderMeetingID)
	if err != nil {
		return outcome, err
	}
	if recovery.CreateTime != nil && info.Status != domain.MeetingNotFound && info.CreateTime != *recovery.CreateTime {
		outcome.Result = domain.MeetingRecoveryCycleConflict
		return outcome, ctx.Err()
	}
	if info.Status != domain.MeetingNotFound {
		conflict, err := s.end(ctx, recovery.ProviderMeetingID, info.CreateTime)
		if err != nil {
			return outcome, err
		}
		if conflict {
			outcome.Result = domain.MeetingRecoveryCycleConflict
			return outcome, ctx.Err()
		}
	}
	if recovery.CreateState == domain.MeetingCreateReturned {
		outcome.Result = domain.MeetingRecoveryCycleSettled
		outcome.ObservedCreateTime = recovery.CreateTime
	}
	return outcome, ctx.Err()
}

func (s *Service) cleanupEvidence(ctx context.Context, row outbox.Row) (repositories.StoredMeetingRecovery, error) {
	if row.Kind != domain.KindMeetingCleanup {
		return repositories.StoredMeetingRecovery{}, errRecoveryEvidence
	}
	repo := s.store.Repositories().MeetingRecovery
	cycle, err := repo.Cycle(ctx, row.ID)
	if err != nil {
		return repositories.StoredMeetingRecovery{}, err
	}
	recovery, err := repo.ByProviderID(ctx, cycle.ProviderMeetingID)
	if err != nil {
		return recovery, err
	}
	if recovery.Provider != s.providerKind || recovery.SettledAt != nil || (!recovery.CleanupRequested && recovery.MeetingID != nil) || recovery.CycleOutboxID == nil || *recovery.CycleOutboxID != row.ID || recovery.CycleNumber != cycle.CycleNumber || recovery.CycleResult != domain.MeetingRecoveryCyclePending || cycle.Result != domain.MeetingRecoveryCyclePending {
		return recovery, errRecoveryEvidence
	}
	return recovery, nil
}

func (s *Service) inspect(ctx context.Context, id domain.ProviderMeetingID) (domain.MeetingInfo, error) {
	info, err := s.provider.InspectMeeting(ctx, id)
	if err != nil {
		return info, fmt.Errorf("inspect cleanup meeting: %w", err)
	}
	switch info.Status {
	case domain.MeetingNotFound:
		if info.CreateTime != 0 || info.Participants != 0 {
			return info, errProviderSnapshot
		}
	case domain.MeetingCreated, domain.MeetingRunning:
		if info.CreateTime <= 0 || info.Participants < 0 {
			return info, errProviderSnapshot
		}
	default:
		return info, errProviderSnapshot
	}
	return info, nil
}

func (s *Service) end(ctx context.Context, id domain.ProviderMeetingID, generation domain.ProviderCreateTime) (bool, error) {
	err := s.provider.EndMeeting(ctx, domain.Meeting{ID: id, CreateTime: generation})
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, meeting.FailureNotFound) && !errors.Is(err, meeting.FailureConflict) {
		return false, fmt.Errorf("end cleanup meeting: %w", err)
	}
	current, inspectErr := s.inspect(ctx, id)
	if inspectErr != nil {
		return false, inspectErr
	}
	if current.Status == domain.MeetingNotFound {
		return false, nil
	}
	if current.CreateTime != generation {
		return true, nil
	}
	return false, fmt.Errorf("end cleanup meeting refused current generation: %w", err)
}

func (s *Service) reportCleanup(ctx context.Context, id domain.OutboxID, outcome outbox.MeetingRecoveryOutcome, err error) {
	attributes := []slog.Attr{slog.String("outbox_id", hex.EncodeToString(id[:]))}
	if err != nil {
		level := slog.LevelWarn
		if errors.Is(err, context.Canceled) {
			level = slog.LevelDebug
		}
		s.logger.LogAttrs(ctx, level, "meeting.cleanup.failed", append(attributes, slog.String("kind", core.ErrorKind(err)))...)
		return
	}
	s.logger.LogAttrs(ctx, slog.LevelDebug, "meeting.cleanup.evaluated", append(attributes, slog.String("result", string(outcome.Result)))...)
}
