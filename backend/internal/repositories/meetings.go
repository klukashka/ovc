package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type Meetings struct {
	queries *dbgen.Queries
}

var ErrMeetingCloseRefused = errors.New("a meeting read under the lesson lock refused its close")

type StoredMeeting = dbgen.GetMeetingRowRow

type CurrentTutorMeeting = dbgen.ListCurrentMeetingsOfTutorRow

type NewMeeting struct {
	ID                domain.MeetingID
	LessonID          domain.LessonID
	Provider          domain.MeetingProvider
	ProviderMeetingID domain.ProviderMeetingID
	RoutingKey        domain.MeetingRoutingKey
	Now               time.Time
}

func (r Meetings) CreateUnderLessonLock(ctx context.Context, meeting NewMeeting) error {
	rows, err := r.queries.CreateMeetingRow(ctx, dbgen.CreateMeetingRowParams{
		ID: meeting.ID, LessonID: meeting.LessonID, Provider: meeting.Provider,
		ProviderMeetingID: meeting.ProviderMeetingID, RoutingKey: meeting.RoutingKey, CreatedAt: meeting.Now,
	})
	if err != nil {
		return done(err, "create meeting")
	}
	if rows != 1 {
		return ErrMeetingRecoveryCapacity
	}
	return nil
}

func (r Meetings) ByID(ctx context.Context, lesson domain.LessonID, id domain.MeetingID) (StoredMeeting, error) {
	row, err := r.queries.GetMeetingRow(ctx, dbgen.GetMeetingRowParams{LessonID: lesson, ID: id})
	return one(row, err, "get meeting")
}

func (r Meetings) Current(ctx context.Context, lesson domain.LessonID) (StoredMeeting, error) {
	row, err := r.queries.GetCurrentMeetingRow(ctx, lesson)
	return one(StoredMeeting(row), err, "get current meeting")
}

func (r Meetings) OpenUnderLessonLock(ctx context.Context, identity domain.MeetingIdentity, created domain.ProviderCreateTime, now time.Time) (domain.MeetingMutation, error) {
	if created <= 0 {
		return domain.MeetingMutationRefused, nil
	}
	rows, err := r.queries.OpenMeetingRow(ctx, dbgen.OpenMeetingRowParams{
		LessonID: identity.LessonID, ID: identity.ID, ProviderMeetingID: identity.ProviderMeetingID, CreateTime: &created, Now: now,
	})
	return meetingApplied(rows, err, "open meeting")
}

func (r Meetings) FirstJoinUnderLessonLock(ctx context.Context, identity domain.MeetingIdentity, created domain.ProviderCreateTime, now time.Time) (domain.MeetingMutation, error) {
	rows, err := r.queries.RecordMeetingFirstJoin(ctx, dbgen.RecordMeetingFirstJoinParams{
		LessonID: identity.LessonID, ID: identity.ID, ProviderMeetingID: identity.ProviderMeetingID, CreateTime: &created, Now: &now,
	})
	outcome, err := meetingApplied(rows, err, "record first meeting join")
	if err != nil || outcome == domain.MeetingMutationApplied {
		return outcome, err
	}
	row, err := r.ByID(ctx, identity.LessonID, identity.ID)
	if errors.Is(err, ErrNotFound) {
		return domain.MeetingMutationRefused, nil
	}
	if err != nil {
		return domain.MeetingMutationRefused, err
	}
	if row.ProviderMeetingID == identity.ProviderMeetingID && row.State == domain.MeetingOpen && sameCreateTime(row.CreateTime, &created) && row.FirstJoinAt != nil {
		return domain.MeetingAlreadyJoined, nil
	}
	return domain.MeetingMutationRefused, nil
}

func (r Meetings) CloseUnderLessonLock(ctx context.Context, expected domain.MeetingExpectation, reason domain.MeetingCloseReason, now time.Time) (domain.MeetingMutation, error) {
	if (expected.State != domain.MeetingCreating && expected.State != domain.MeetingOpen) || (expected.State == domain.MeetingCreating) != (expected.CreateTime == nil) {
		return domain.MeetingMutationRefused, nil
	}
	var created *int64
	if expected.CreateTime != nil {
		value := int64(*expected.CreateTime)
		created = &value
	}
	identity := expected.Identity
	rows, err := r.queries.CloseMeetingRow(ctx, dbgen.CloseMeetingRowParams{
		LessonID: identity.LessonID, ID: identity.ID, ProviderMeetingID: identity.ProviderMeetingID,
		ExpectedState: expected.State, ExpectedCreateTime: created, CloseReason: &reason, Now: now,
	})
	outcome, err := meetingApplied(rows, err, "close meeting")
	if err != nil || outcome == domain.MeetingMutationApplied {
		return outcome, err
	}
	row, err := r.ByID(ctx, identity.LessonID, identity.ID)
	if errors.Is(err, ErrNotFound) {
		return domain.MeetingMutationRefused, nil
	}
	if err != nil {
		return domain.MeetingMutationRefused, err
	}
	if row.ProviderMeetingID == identity.ProviderMeetingID && row.State == domain.MeetingClosed && sameCreateTime(row.CreateTime, expected.CreateTime) {
		return domain.MeetingAlreadyClosed, nil
	}
	return domain.MeetingMutationRefused, nil
}

type StaleCreatingMeeting = dbgen.ListStaleCreatingMeetingsRow

func (r Meetings) UnsettledOfTutor(ctx context.Context, tutor domain.UserID) (int64, error) {
	count, err := r.queries.CountUnsettledMeetingsOfTutor(ctx, tutor)
	return one(count, err, "count the unsettled meetings of a tutor")
}

func (r Meetings) HasFreshUncertain(ctx context.Context, lesson domain.LessonID, since time.Time, now time.Time) (bool, error) {
	fresh, err := r.queries.LessonHasFreshUncertainMeeting(ctx, dbgen.LessonHasFreshUncertainMeetingParams{LessonID: lesson, Since: since, Now: now})
	return one(fresh, err, "check a lesson for a fresh uncertain meeting")
}

func (r Meetings) StaleCreating(ctx context.Context, before time.Time, limit int32) ([]StaleCreatingMeeting, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrMeetingRecoveryBatchSize
	}
	rows, err := r.queries.ListStaleCreatingMeetings(ctx, dbgen.ListStaleCreatingMeetingsParams{Before: before, BatchSize: limit})
	return rows, done(err, "list stale creating meetings")
}

func (r Repositories) CloseMeetingForCleanup(ctx context.Context, expected domain.MeetingExpectation, reason domain.MeetingCloseReason, cycle domain.OutboxID, now time.Time) (domain.MeetingMutation, error) {
	outcome, err := r.Meetings.CloseUnderLessonLock(ctx, expected, reason, now)
	if err != nil {
		return outcome, err
	}
	if outcome == domain.MeetingMutationRefused {
		return outcome, ErrMeetingCloseRefused
	}
	if outcome != domain.MeetingMutationApplied {
		return outcome, nil
	}
	recovery, err := r.MeetingRecovery.ByProviderID(ctx, expected.Identity.ProviderMeetingID)
	if err != nil {
		return domain.MeetingMutationRefused, err
	}
	if _, err := r.MeetingRecovery.QueueCycle(ctx, MeetingRecoveryQueue{
		ProviderMeetingID: recovery.ProviderMeetingID, Revision: recovery.Revision, OutboxID: cycle, Mode: domain.MeetingRecoveryAutomatic, Now: now,
	}); err != nil {
		return domain.MeetingMutationRefused, err
	}
	return outcome, nil
}

func (r Repositories) CloseCurrentMeetingForCleanup(ctx context.Context, lesson domain.LessonID, reason domain.MeetingCloseReason, cycle domain.OutboxID, now time.Time) (bool, error) {
	current, err := r.Meetings.Current(ctx, lesson)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return r.closeStored(ctx, current, reason, cycle, now)
}

func (r Repositories) CloseOpenMeetingForCleanup(ctx context.Context, lesson domain.LessonID, reason domain.MeetingCloseReason, cycle domain.OutboxID, now time.Time) (bool, error) {
	current, err := r.Meetings.Current(ctx, lesson)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if current.State != domain.MeetingOpen {
		return false, nil
	}
	return r.closeStored(ctx, current, reason, cycle, now)
}

func (r Repositories) CloseTutorMeetingForCleanup(ctx context.Context, current CurrentTutorMeeting, reason domain.MeetingCloseReason, cycle domain.OutboxID, now time.Time) (bool, error) {
	identity := domain.MeetingIdentity{LessonID: current.LessonID, ID: current.ID, ProviderMeetingID: current.ProviderMeetingID}
	outcome, err := r.CloseMeetingForCleanup(ctx, domain.MeetingExpectation{Identity: identity, State: current.State, CreateTime: current.CreateTime}, reason, cycle, now)
	return outcome == domain.MeetingMutationApplied, err
}

func (r Repositories) closeStored(ctx context.Context, current StoredMeeting, reason domain.MeetingCloseReason, cycle domain.OutboxID, now time.Time) (bool, error) {
	identity := domain.MeetingIdentity{LessonID: current.LessonID, ID: current.ID, ProviderMeetingID: current.ProviderMeetingID}
	outcome, err := r.CloseMeetingForCleanup(ctx, domain.MeetingExpectation{Identity: identity, State: current.State, CreateTime: current.CreateTime}, reason, cycle, now)
	return outcome == domain.MeetingMutationApplied, err
}

func (r Meetings) CurrentOfTutor(ctx context.Context, tutor domain.UserID) ([]CurrentTutorMeeting, error) {
	rows, err := r.queries.ListCurrentMeetingsOfTutor(ctx, tutor)
	return rows, done(err, "list current meetings of a tutor")
}

func (r Meetings) Participant(ctx context.Context, lesson domain.LessonID, account domain.UserID, candidate domain.ParticipantID, now time.Time) (domain.ParticipantID, error) {
	if err := r.queries.InsertLessonParticipant(ctx, dbgen.InsertLessonParticipantParams{LessonID: lesson, AccountID: account, ParticipantID: candidate, Now: now}); err != nil {
		return domain.ParticipantID{}, done(err, "insert a lesson participant")
	}
	row, err := r.queries.GetLessonParticipant(ctx, dbgen.GetLessonParticipantParams{LessonID: lesson, AccountID: account})
	return one(row, err, "get a lesson participant")
}

func (r Meetings) DeleteParticipantsOf(ctx context.Context, account domain.UserID) (int64, error) {
	rows, err := r.queries.DeleteAccountParticipants(ctx, account)
	return counted(rows, err, "delete lesson participants of an account")
}

type MeetingRevocation = dbgen.ListUnconfirmedMeetingRevocationsRow

func (r Meetings) UnconfirmedRevocations(ctx context.Context, lesson domain.LessonID) ([]MeetingRevocation, error) {
	rows, err := r.queries.ListUnconfirmedMeetingRevocations(ctx, lesson)
	return rows, done(err, "list unconfirmed meeting revocations")
}

func (r Meetings) ConfirmRevocation(ctx context.Context, lesson domain.LessonID, id domain.MeetingID, now time.Time) error {
	_, err := r.queries.ConfirmMeetingRevocation(ctx, dbgen.ConfirmMeetingRevocationParams{LessonID: lesson, ID: id, Now: now})
	return done(err, "confirm a meeting revocation")
}

func (r Meetings) AcceptEndedHint(ctx context.Context, routing domain.MeetingRoutingKey, outbox domain.OutboxID, now time.Time) (bool, error) {
	rows, err := r.queries.AcceptMeetingEndedHint(ctx, dbgen.AcceptMeetingEndedHintParams{
		RoutingKey: routing, OutboxID: pgtype.UUID{Bytes: outbox, Valid: true}, Now: now, ChainUntil: now.Add(domain.MeetingHintChainSpan),
		CreatedBefore: now.Add(-domain.MeetingHintMinimumAge), DebouncedBefore: now.Add(-domain.MeetingHintDebounce),
	})
	return applied(rows, err, "accept a meeting ended hint")
}

func (r Meetings) EndedHintChecks(ctx context.Context, lesson domain.LessonID, id domain.MeetingID) (int16, error) {
	row, err := r.queries.GetMeetingEndedHintChecks(ctx, dbgen.GetMeetingEndedHintChecksParams{LessonID: lesson, ID: id})
	return one(row, err, "get the ended hint checks of a meeting")
}

func (r Meetings) AdvanceEndedHint(ctx context.Context, identity domain.MeetingIdentity, check int16, outbox domain.OutboxID, now time.Time) error {
	if check >= domain.MeetingHintRechecks {
		_, err := r.queries.FinishMeetingEndedHint(ctx, dbgen.FinishMeetingEndedHintParams{LessonID: identity.LessonID, ID: identity.ID, ExpectedCheck: check})
		return done(err, "finish a meeting ended hint")
	}
	_, err := r.queries.AdvanceMeetingEndedHint(ctx, dbgen.AdvanceMeetingEndedHintParams{
		LessonID: identity.LessonID, ID: identity.ID, ExpectedCheck: check, OutboxID: pgtype.UUID{Bytes: outbox, Valid: true}, Now: now, NextAttemptAt: now.Add(domain.MeetingHintRecheckAfter),
	})
	return done(err, "advance a meeting ended hint")
}

func sameCreateTime(first *domain.ProviderCreateTime, second *domain.ProviderCreateTime) bool {
	return (first == nil && second == nil) || (first != nil && second != nil && *first == *second)
}

func meetingApplied(rows int64, err error, action string) (domain.MeetingMutation, error) {
	if err != nil {
		return domain.MeetingMutationRefused, done(err, action)
	}
	if rows == 1 {
		return domain.MeetingMutationApplied, nil
	}
	return domain.MeetingMutationRefused, nil
}
