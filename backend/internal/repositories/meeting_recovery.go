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
