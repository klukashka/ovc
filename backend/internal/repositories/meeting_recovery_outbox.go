package repositories

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

var errMeetingRecoveryFence = errors.New("meeting recovery completion fence refused")

var ErrMeetingRecoveryCompletionInvalid = errors.New("meeting recovery completion lacks a positive revision or a terminal result")

type MeetingRecoveryEffectCompletion struct {
	OutboxID           domain.OutboxID
	Lease              domain.LeaseToken
	Revision           domain.MeetingRecoveryRevision
	ObservedCreateTime *domain.ProviderCreateTime
	EvaluatedAt        *time.Time
	Result             domain.MeetingRecoveryCycleResult
	Now                time.Time
}

func (s *Store) FinishMeetingRecovery(ctx context.Context, input MeetingRecoveryEffectCompletion) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if input.Revision <= 0 || (input.Result != domain.MeetingRecoveryCycleSettled && input.Result != domain.MeetingRecoveryCycleUnresolved && input.Result != domain.MeetingRecoveryCycleConflict) {
		return false, ErrMeetingRecoveryCompletionInvalid
	}
	return s.meetingRecoveryOutbox(ctx, input.OutboxID, input.Lease, input.Now, false, func(repos Repositories, recovery StoredMeetingRecovery, _ OutboxRow) error {
		if recovery.Revision != input.Revision {
			return errMeetingRecoveryFence
		}
		if input.Result == domain.MeetingRecoveryCycleSettled {
			outcome, err := repos.MeetingRecovery.Settle(ctx, MeetingRecoverySettlement{ProviderMeetingID: recovery.ProviderMeetingID, Revision: input.Revision, ObservedCreateTime: input.ObservedCreateTime, EvaluatedAt: input.EvaluatedAt, Now: input.Now})
			if err != nil {
				return err
			}
			if outcome != domain.MeetingMutationApplied {
				return errMeetingRecoveryFence
			}
		}
		return finishRecoveryOutbox(ctx, repos, recovery, input.OutboxID, input.Lease, input.Result, "", input.Now)
	})
}

func (s *Store) FailMeetingRecovery(ctx context.Context, input OutboxFailure) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.meetingRecoveryOutbox(ctx, input.ID, input.Lease, input.Now, false, func(repos Repositories, recovery StoredMeetingRecovery, row OutboxRow) error {
		if row.Attempts < domain.OutboxMaxAttempts && input.ErrorCode != "permanent" {
			return errMeetingRecoveryFence
		}
		return finishRecoveryOutbox(ctx, repos, recovery, input.ID, input.Lease, domain.MeetingRecoveryCycleExhausted, input.ErrorCode, input.Now)
	})
}

func (s *Store) meetingRecoveryOutbox(ctx context.Context, id domain.OutboxID, lease domain.LeaseToken, now time.Time, expired bool, work func(Repositories, StoredMeetingRecovery, OutboxRow) error) (bool, error) {
	if lease == (domain.LeaseToken{}) {
		return false, nil
	}
	err := s.WithTx(ctx, func(repos Repositories) error {
		queries := repos.MeetingRecovery.queries
		lockCapacity := queries.LockMeetingRecoveryCapacity
		lockRecovery := queries.LockMeetingRecoveryForOutbox
		lockOutbox := queries.LockMeetingRecoveryOutbox
		if expired {
			lockCapacity, lockRecovery, lockOutbox = queries.TryLockMeetingRecoveryCapacity, queries.TryLockMeetingRecoveryForOutbox, queries.TryLockMeetingRecoveryOutbox
		}
		count, err := lockCapacity(ctx)
		if err != nil {
			_, err = one(count, err, "lock meeting recovery capacity")
			return err
		}
		recovery, err := lockRecovery(ctx, id)
		if err != nil {
			_, err = one(recovery, err, "lock meeting recovery for outbox")
			return err
		}
		row, err := lockOutbox(ctx, id)
		if err != nil {
			_, err = one(row, err, "lock meeting recovery outbox")
			return err
		}
		if row.Status != domain.OutboxInProgress || row.LeaseToken == nil || *row.LeaseToken != lease || row.LeaseExpiresAt == nil || row.LeaseExpiresAt.After(now) == expired || recovery.CycleOutboxID == nil || *recovery.CycleOutboxID != id || recovery.CycleResult != domain.MeetingRecoveryCyclePending {
			return errMeetingRecoveryFence
		}
		return work(repos, recovery, row)
	})
	if errors.Is(err, errMeetingRecoveryFence) || errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func finishRecoveryOutbox(ctx context.Context, repos Repositories, recovery StoredMeetingRecovery, id domain.OutboxID, lease domain.LeaseToken, result domain.MeetingRecoveryCycleResult, code string, now time.Time) error {
	completed, err := repos.MeetingRecovery.CompleteCycle(ctx, MeetingRecoveryCompletion{ProviderMeetingID: recovery.ProviderMeetingID, OutboxID: id, Result: result, Now: now})
	if err != nil {
		return err
	}
	if completed != domain.MeetingMutationApplied {
		return errMeetingRecoveryFence
	}
	reason := domain.DoneSent
	input := dbgen.FinishMeetingRecoveryOutboxParams{ID: id, LeaseToken: &lease, Status: domain.OutboxDone, DoneReason: &reason, Now: &now}
	if result == domain.MeetingRecoveryCycleExhausted {
		input.Status, input.DoneReason, input.ErrorCode = domain.OutboxDead, nil, &code
	}
	rows, err := repos.MeetingRecovery.queries.FinishMeetingRecoveryOutbox(ctx, input)
	if err != nil {
		return done(err, "finish meeting recovery outbox")
	}
	if rows != 1 {
		return errMeetingRecoveryFence
	}
	return nil
}

var ErrMeetingRecoveryLeaseMissing = errors.New("expired meeting recovery outbox has no lease")

type MeetingRecoveryReclaimFailure struct {
	OutboxID domain.OutboxID
	Err      error
}

type MeetingRecoveryReclaim struct {
	Reclaimed []ReclaimedOutboxRow
	Failures  []MeetingRecoveryReclaimFailure
}

func (s *Store) ReclaimMeetingRecovery(parent context.Context, now time.Time, limit int32) (MeetingRecoveryReclaim, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	var result MeetingRecoveryReclaim
	if limit < 1 || limit > 100 {
		return result, ErrMeetingRecoveryBatchSize
	}
	candidates, err := s.Repositories().MeetingRecovery.queries.ListExpiredMeetingRecoveryOutbox(ctx, dbgen.ListExpiredMeetingRecoveryOutboxParams{Now: &now, BatchSize: limit})
	if err != nil {
		return result, done(err, "list expired meeting recovery outbox")
	}
	for _, candidate := range candidates {
		if candidate.LeaseToken == nil {
			result.Failures = append(result.Failures, MeetingRecoveryReclaimFailure{OutboxID: candidate.ID, Err: ErrMeetingRecoveryLeaseMissing})
			continue
		}
		status := domain.OutboxPending
		applied, err := s.meetingRecoveryOutbox(ctx, candidate.ID, *candidate.LeaseToken, now, true, func(repos Repositories, recovery StoredMeetingRecovery, row OutboxRow) error {
			if row.Attempts >= domain.OutboxMaxAttempts {
				status = domain.OutboxDead
				return finishRecoveryOutbox(ctx, repos, recovery, candidate.ID, *candidate.LeaseToken, domain.MeetingRecoveryCycleExhausted, "lease_expired", now)
			}
			rows, err := repos.MeetingRecovery.queries.RequeueMeetingRecoveryOutbox(ctx, dbgen.RequeueMeetingRecoveryOutboxParams(candidate))
			if err != nil {
				return done(err, "requeue meeting recovery outbox")
			}
			if rows != 1 {
				return errMeetingRecoveryFence
			}
			return nil
		})
		if err != nil {
			if parentErr := parent.Err(); parentErr != nil {
				return result, errors.Join(parentErr, err)
			}
			result.Failures = append(result.Failures, MeetingRecoveryReclaimFailure{OutboxID: candidate.ID, Err: err})
			if ctx.Err() != nil {
				return result, nil
			}
			continue
		}
		if applied {
			result.Reclaimed = append(result.Reclaimed, ReclaimedOutboxRow{ID: candidate.ID, Kind: domain.KindMeetingCleanup, Status: status})
		}
	}
	return result, nil
}
