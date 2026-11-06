package repositories

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

var ErrMeetingRecoveryBatchSize = errors.New("meeting recovery batch size must be between 1 and 100")

type MeetingRecovery struct {
	queries *dbgen.Queries
}

type StoredMeetingRecovery = dbgen.MeetingRecovery

type MeetingRecoveryPage struct {
	After domain.ProviderMeetingID
	Limit int32
}

func (r MeetingRecovery) ByProviderID(ctx context.Context, id domain.ProviderMeetingID) (StoredMeetingRecovery, error) {
	row, err := r.queries.GetMeetingRecovery(ctx, id)
	return one(row, err, "get meeting recovery")
}

func (r MeetingRecovery) BeginCreateUnderLessonLock(ctx context.Context, identity domain.MeetingIdentity, attempt domain.MeetingCreateAttemptID, revision domain.MeetingRecoveryRevision, now time.Time) (domain.MeetingMutation, error) {
	if attempt == (domain.MeetingCreateAttemptID{}) || revision <= 0 {
		return domain.MeetingMutationRefused, nil
	}
	rows, err := r.queries.BeginMeetingCreate(ctx, dbgen.BeginMeetingCreateParams{
		LessonID: identity.LessonID, MeetingID: identity.ID, ProviderMeetingID: identity.ProviderMeetingID,
		AttemptID: &attempt, Revision: revision, Now: now,
	})
	return meetingApplied(rows, err, "begin meeting create")
}

func (r MeetingRecovery) RecordCreateReturned(ctx context.Context, id domain.ProviderMeetingID, attempt domain.MeetingCreateAttemptID, created domain.ProviderCreateTime, now time.Time) (domain.MeetingMutation, error) {
	if attempt == (domain.MeetingCreateAttemptID{}) || created <= 0 {
		return domain.MeetingMutationRefused, nil
	}
	rows, err := r.queries.RecordMeetingCreateReturned(ctx, dbgen.RecordMeetingCreateReturnedParams{
		ProviderMeetingID: id, AttemptID: &attempt, CreateTime: &created, Now: now,
	})
	return meetingApplied(rows, err, "record meeting create returned")
}

func (r MeetingRecovery) MarkCreateUncertain(ctx context.Context, id domain.ProviderMeetingID, attempt domain.MeetingCreateAttemptID, now time.Time) (domain.MeetingMutation, error) {
	if attempt == (domain.MeetingCreateAttemptID{}) {
		return domain.MeetingMutationRefused, nil
	}
	rows, err := r.queries.MarkMeetingCreateUncertain(ctx, dbgen.MarkMeetingCreateUncertainParams{
		ProviderMeetingID: id, AttemptID: &attempt, Now: now,
	})
	return meetingApplied(rows, err, "mark meeting create uncertain")
}

func (r MeetingRecovery) CleanupCandidates(ctx context.Context, page MeetingRecoveryPage) ([]StoredMeetingRecovery, error) {
	if page.Limit < 1 || page.Limit > 100 {
		return nil, ErrMeetingRecoveryBatchSize
	}
	rows, err := r.queries.ListMeetingRecoveryCandidates(ctx, dbgen.ListMeetingRecoveryCandidatesParams{
		AfterProviderMeetingID: page.After, BatchSize: page.Limit,
	})
	return rows, done(err, "list meeting recovery candidates")
}

var ErrMeetingRecoveryCapacity = errors.New("meeting recovery admission capacity exhausted")

type StoredMeetingRecoveryCycle = dbgen.MeetingRecoveryCycle
type MeetingRecoveryCursor = dbgen.GetMeetingRecoveryCursorRow

type MeetingRecoverySettlement struct {
	ProviderMeetingID  domain.ProviderMeetingID
	Revision           domain.MeetingRecoveryRevision
	ObservedCreateTime *domain.ProviderCreateTime
	Now                time.Time
}

type MeetingRecoveryQueue struct {
	ProviderMeetingID domain.ProviderMeetingID
	Revision          domain.MeetingRecoveryRevision
	OutboxID          domain.OutboxID
	Mode              domain.MeetingRecoveryCycleMode
	Now               time.Time
}

type MeetingRecoveryCompletion struct {
	ProviderMeetingID domain.ProviderMeetingID
	OutboxID          domain.OutboxID
	Result            domain.MeetingRecoveryCycleResult
	Now               time.Time
}

type MeetingRecoveryCyclePage struct {
	ProviderMeetingID domain.ProviderMeetingID
	After             domain.MeetingRecoveryCycleNumber
	Limit             int32
}

func (r MeetingRecovery) Capacity(ctx context.Context) (int64, error) {
	count, err := r.queries.GetMeetingRecoveryCapacity(ctx)
	return one(count, err, "get meeting recovery capacity")
}

func (r MeetingRecovery) Settle(ctx context.Context, input MeetingRecoverySettlement) (domain.MeetingMutation, error) {
	var observed *int64
	if input.ObservedCreateTime != nil {
		value := int64(*input.ObservedCreateTime)
		observed = &value
	}
	rows, err := r.queries.SettleMeetingRecovery(ctx, dbgen.SettleMeetingRecoveryParams{ProviderMeetingID: input.ProviderMeetingID, Revision: input.Revision, ObservedCreateTime: observed, Now: &input.Now})
	return meetingApplied(rows, err, "settle meeting recovery")
}

func (r MeetingRecovery) QueueCycle(ctx context.Context, input MeetingRecoveryQueue) (domain.MeetingMutation, error) {
	if input.OutboxID == (domain.OutboxID{}) || (input.Mode != domain.MeetingRecoveryAutomatic && input.Mode != domain.MeetingRecoveryOperator) {
		return domain.MeetingMutationRefused, nil
	}
	rows, err := r.queries.QueueMeetingRecoveryCycle(ctx, dbgen.QueueMeetingRecoveryCycleParams{ProviderMeetingID: input.ProviderMeetingID, Revision: input.Revision, OutboxID: &input.OutboxID, Mode: string(input.Mode), Now: input.Now})
	if err != nil || rows == 1 {
		return meetingApplied(rows, err, "queue meeting recovery cycle")
	}
	prior, err := r.Cycle(ctx, input.OutboxID)
	if errors.Is(err, ErrNotFound) {
		return domain.MeetingMutationRefused, nil
	}
	if err != nil {
		return domain.MeetingMutationRefused, err
	}
	if prior.ProviderMeetingID == input.ProviderMeetingID && prior.Mode == input.Mode {
		return domain.MeetingMutationApplied, nil
	}
	return domain.MeetingMutationRefused, nil
}

func (r MeetingRecovery) CompleteCycle(ctx context.Context, input MeetingRecoveryCompletion) (domain.MeetingMutation, error) {
	switch input.Result {
	case domain.MeetingRecoveryCycleSettled, domain.MeetingRecoveryCycleUnresolved, domain.MeetingRecoveryCycleConflict, domain.MeetingRecoveryCycleExhausted:
	case domain.MeetingRecoveryCycleNone, domain.MeetingRecoveryCyclePending:
		return domain.MeetingMutationRefused, nil
	default:
		return domain.MeetingMutationRefused, nil
	}
	rows, err := r.queries.CompleteMeetingRecoveryCycle(ctx, dbgen.CompleteMeetingRecoveryCycleParams{ProviderMeetingID: input.ProviderMeetingID, OutboxID: &input.OutboxID, Result: input.Result, Now: &input.Now})
	return meetingApplied(rows, err, "complete meeting recovery cycle")
}

func (r MeetingRecovery) List(ctx context.Context, page MeetingRecoveryPage) ([]StoredMeetingRecovery, error) {
	if page.Limit < 1 || page.Limit > 100 {
		return nil, ErrMeetingRecoveryBatchSize
	}
	rows, err := r.queries.ListMeetingRecovery(ctx, dbgen.ListMeetingRecoveryParams{AfterProviderMeetingID: page.After, BatchSize: page.Limit})
	return rows, done(err, "list meeting recovery")
}

func (r MeetingRecovery) Cycle(ctx context.Context, id domain.OutboxID) (StoredMeetingRecoveryCycle, error) {
	row, err := r.queries.GetMeetingRecoveryCycle(ctx, id)
	return one(row, err, "get meeting recovery cycle")
}

func (r MeetingRecovery) Cycles(ctx context.Context, page MeetingRecoveryCyclePage) ([]StoredMeetingRecoveryCycle, error) {
	if page.Limit < 1 || page.Limit > 100 {
		return nil, ErrMeetingRecoveryBatchSize
	}
	rows, err := r.queries.ListMeetingRecoveryCycles(ctx, dbgen.ListMeetingRecoveryCyclesParams{ProviderMeetingID: page.ProviderMeetingID, AfterCycleNumber: page.After, BatchSize: page.Limit})
	return rows, done(err, "list meeting recovery cycles")
}

func (r MeetingRecovery) Cursor(ctx context.Context) (MeetingRecoveryCursor, error) {
	row, err := r.queries.GetMeetingRecoveryCursor(ctx)
	return one(row, err, "get meeting recovery cursor")
}

func (r MeetingRecovery) AdvanceCursor(ctx context.Context, expected MeetingRecoveryCursor, after domain.ProviderMeetingID) (domain.MeetingMutation, error) {
	rows, err := r.queries.AdvanceMeetingRecoveryCursor(ctx, dbgen.AdvanceMeetingRecoveryCursorParams{AfterProviderMeetingID: after, Revision: expected.Revision})
	return meetingApplied(rows, err, "advance meeting recovery cursor")
}

func (r MeetingRecovery) CleanupReceipts(ctx context.Context, now time.Time, limit int32) (int64, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrMeetingRecoveryBatchSize
	}
	rows, err := r.queries.DeleteMeetingRecoveryReceipts(ctx, dbgen.DeleteMeetingRecoveryReceiptsParams{Now: now, BatchSize: limit})
	return counted(rows, err, "delete meeting recovery receipts")
}
