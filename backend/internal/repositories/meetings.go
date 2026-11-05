package repositories

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type Meetings struct {
	queries *dbgen.Queries
}

type StoredMeeting = dbgen.Meeting

type NewMeeting struct {
	ID                domain.MeetingID
	LessonID          domain.LessonID
	Provider          domain.MeetingProvider
	ProviderMeetingID domain.ProviderMeetingID
	RoutingKey        domain.MeetingRoutingKey
	Now               time.Time
}

func (r Meetings) CreateUnderLessonLock(ctx context.Context, meeting NewMeeting) error {
	return done(r.queries.CreateMeetingRow(ctx, dbgen.CreateMeetingRowParams{
		ID: meeting.ID, LessonID: meeting.LessonID, Provider: meeting.Provider,
		ProviderMeetingID: meeting.ProviderMeetingID, RoutingKey: meeting.RoutingKey, CreatedAt: meeting.Now,
	}), "create meeting")
}

func (r Meetings) ByID(ctx context.Context, lesson domain.LessonID, id domain.MeetingID) (StoredMeeting, error) {
	row, err := r.queries.GetMeetingRow(ctx, dbgen.GetMeetingRowParams{LessonID: lesson, ID: id})
	return one(row, err, "get meeting")
}

func (r Meetings) Current(ctx context.Context, lesson domain.LessonID) (StoredMeeting, error) {
	row, err := r.queries.GetCurrentMeetingRow(ctx, lesson)
	return one(row, err, "get current meeting")
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
