package meetings

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var ErrRecoveryNotFound = errors.New("meeting recovery was not found")
var ErrRecoveryRetryRefused = errors.New("meeting recovery retry requires current eligible evidence without a pending cycle")
var ErrRecoveryInput = errors.New("meeting recovery input is invalid")

type Operator struct {
	logger *slog.Logger
	store  *repositories.Store
	clock  clock.Source
	random core.RandomSource
}

type RecoveryStatus struct {
	Unsettled int64
	Rows      []RecoveryEntry
	Next      *domain.ProviderMeetingID
}

type RecoveryInspection struct {
	Recovery RecoveryEntry
	Cycles   []repositories.StoredMeetingRecoveryCycle
	Next     *domain.MeetingRecoveryCycleNumber
}

func NewOperator(store *repositories.Store, source clock.Source, random core.RandomSource, logger *slog.Logger) *Operator {
	return &Operator{store: store, clock: source, random: random, logger: logger}
}

func (s *Operator) Status(ctx context.Context, after domain.ProviderMeetingID) (RecoveryStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, recoveryPassBudget)
	defer cancel()
	repo := s.store.Repositories().MeetingRecovery
	count, err := repo.Capacity(ctx)
	if err != nil {
		return RecoveryStatus{}, err
	}
	rows, err := repo.List(ctx, repositories.MeetingRecoveryPage{After: after, Limit: recoveryPassLimit})
	if err != nil {
		return RecoveryStatus{}, err
	}
	result := RecoveryStatus{Unsettled: count}
	for _, row := range rows {
		result.Rows = append(result.Rows, recoveryEntry(row))
	}
	if len(rows) == recoveryPassLimit {
		last := rows[len(rows)-1].ProviderMeetingID
		result.Next = &last
	}
	return result, nil
}

func (s *Operator) Inspect(ctx context.Context, id domain.ProviderMeetingID, after domain.MeetingRecoveryCycleNumber) (RecoveryInspection, error) {
	if id == (domain.ProviderMeetingID{}) || after < 0 {
		return RecoveryInspection{}, ErrRecoveryInput
	}
	ctx, cancel := context.WithTimeout(ctx, recoveryPassBudget)
	defer cancel()
	repo := s.store.Repositories().MeetingRecovery
	row, err := repo.ByProviderID(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return RecoveryInspection{}, ErrRecoveryNotFound
	}
	if err != nil {
		return RecoveryInspection{}, err
	}
	cycles, err := repo.Cycles(ctx, repositories.MeetingRecoveryCyclePage{ProviderMeetingID: id, After: after, Limit: recoveryPassLimit})
	if err != nil {
		return RecoveryInspection{}, err
	}
	result := RecoveryInspection{Recovery: recoveryEntry(row), Cycles: cycles}
	if len(cycles) == recoveryPassLimit {
		last := cycles[len(cycles)-1].CycleNumber
		result.Next = &last
	}
	return result, nil
}

func (s *Operator) Retry(ctx context.Context, id domain.ProviderMeetingID, revision domain.MeetingRecoveryRevision) (repositories.StoredMeetingRecoveryCycle, error) {
	if id == (domain.ProviderMeetingID{}) || revision <= 0 {
		return repositories.StoredMeetingRecoveryCycle{}, ErrRecoveryInput
	}
	ctx, cancel := context.WithTimeout(ctx, recoveryPassBudget)
	defer cancel()
	repo := s.store.Repositories().MeetingRecovery
	if _, err := repo.ByProviderID(ctx, id); errors.Is(err, repositories.ErrNotFound) {
		return repositories.StoredMeetingRecoveryCycle{}, ErrRecoveryNotFound
	} else if err != nil {
		return repositories.StoredMeetingRecoveryCycle{}, err
	}
	now, err := s.clock.Instant(ctx)
	if err != nil {
		return repositories.StoredMeetingRecoveryCycle{}, err
	}
	outbox, err := core.NewID[domain.OutboxID](s.random)
	if err != nil {
		return repositories.StoredMeetingRecoveryCycle{}, err
	}
	result, err := repo.QueueCycle(ctx, repositories.MeetingRecoveryQueue{ProviderMeetingID: id, Revision: revision, OutboxID: outbox, Mode: domain.MeetingRecoveryOperator, Now: now})
	if err != nil {
		return repositories.StoredMeetingRecoveryCycle{}, err
	}
	if result != domain.MeetingMutationApplied {
		return repositories.StoredMeetingRecoveryCycle{}, ErrRecoveryRetryRefused
	}
	s.logger.LogAttrs(ctx, slog.LevelInfo, "meeting.recovery.retry.queued", slog.String("outbox_id", outbox.String()))
	return repo.Cycle(ctx, outbox)
}

type RecoveryEntry struct {
	ProviderMeetingID   domain.ProviderMeetingID
	Provider            domain.MeetingProvider
	Origin              domain.MeetingRecoveryOrigin
	CreateState         domain.MeetingCreateState
	Revision            domain.MeetingRecoveryRevision
	CleanupRequested    bool
	Settled             bool
	Orphaned            bool
	CreateTime          *domain.ProviderCreateTime
	CycleNumber         domain.MeetingRecoveryCycleNumber
	CycleResult         domain.MeetingRecoveryCycleResult
	FinalCycleRequested bool
	FinalCycleUsed      bool
	Settlement          *domain.MeetingRecoverySettlement
	DispatchedAt        *time.Time
	ReconcileCycleUsed  bool
	OperatorAction      bool
}

func recoveryEntry(row repositories.StoredMeetingRecovery) RecoveryEntry {
	return RecoveryEntry{ProviderMeetingID: row.ProviderMeetingID, Provider: row.Provider, Origin: row.Origin, CreateState: row.CreateState, Revision: row.Revision, CleanupRequested: row.CleanupRequested, Settled: row.SettledAt != nil, Orphaned: row.MeetingID == nil, CreateTime: row.CreateTime, CycleNumber: row.CycleNumber, CycleResult: row.CycleResult, FinalCycleRequested: row.FinalCycleRequested, FinalCycleUsed: row.FinalCycleUsed, Settlement: row.Settlement, DispatchedAt: row.DispatchedAt, ReconcileCycleUsed: row.ReconcileCycleUsed, OperatorAction: operatorActionRequired(row)}
}

func operatorActionRequired(row repositories.StoredMeetingRecovery) bool {
	if row.SettledAt != nil || row.CycleNumber == 0 || row.CycleResult == domain.MeetingRecoveryCyclePending || finalCycleDue(row) {
		return false
	}
	unknown := row.CreateState == domain.MeetingCreateStarted || row.CreateState == domain.MeetingCreateUncertain
	return !unknown || row.ReconcileCycleUsed
}
