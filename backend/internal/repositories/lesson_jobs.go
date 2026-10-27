package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	JobLessonAudience  = dbgen.GetLessonAudienceRow
	JobSeriesAudience  = dbgen.GetSeriesAudienceRow
	LockedJobLesson    = dbgen.LockLessonForJobRow
	ReminderCandidate  = dbgen.ListReminderCandidatesRow
	LessonForEmail     = dbgen.GetLessonForEmailRow
	NotificationLesson = dbgen.ListTutorNotificationLessonsRow
)

type ReminderQuery struct {
	DayBefore   bool
	WindowStart time.Time
	WindowEnd   time.Time
	After       ReminderCandidate
	Limit       int32
}

type SeriesCandidate struct {
	ID                domain.SeriesID
	MaterialisedUntil domain.LocalDate
}

type SeriesCandidateQuery struct {
	Now     time.Time
	Horizon time.Time
	After   SeriesCandidate
	Limit   int32
}

type LockedSeries struct {
	Weekdays          []domain.Weekday
	Start             domain.LocalTime
	Timezone          string
	First             domain.LocalDate
	EndDate           *domain.LocalDate
	MaterialisedUntil domain.LocalDate
	Ended             bool
	TutorState        domain.AccountState
}

type NewOccurrence struct {
	ID       domain.LessonID
	Date     domain.LocalDate
	StartsAt time.Time
}

type DailySlot struct {
	Purpose     domain.RateLimitPurpose
	Subject     []byte
	WindowStart time.Time
	Below       int32
}

func (r Lessons) DueToEnd(ctx context.Context, endedBefore time.Time, limit int32) ([]domain.LessonID, error) {
	ids, err := r.queries.ListLessonsDueToEnd(ctx, dbgen.ListLessonsDueToEndParams{
		LatestStart: endedBefore.Add(-domain.LessonMinDuration * time.Minute), EndedBefore: endedBefore, RowLimit: limit,
	})
	return one(ids, err, "lessons due to end")
}

func (r Lessons) JobAudience(ctx context.Context, id domain.LessonID) (JobLessonAudience, error) {
	row, err := r.queries.GetLessonAudience(ctx, id)
	return one(row, err, "the audience of a lesson")
}

func (r Lessons) LockSeriesShared(ctx context.Context, id domain.SeriesID) error {
	locked, err := r.queries.LockSeriesOfLesson(ctx, id)
	_, err = one(locked, err, "lock the series of a lesson")
	return err
}

func (r Lessons) LockForJob(ctx context.Context, id domain.LessonID) (LockedJobLesson, error) {
	row, err := r.queries.LockLessonForJob(ctx, id)
	return one(row, err, "lock a lesson for a job")
}

func (r Lessons) EndUnjoined(ctx context.Context, id domain.LessonID, endedBefore time.Time, now time.Time) (bool, error) {
	rows, err := r.queries.EndUnjoinedLesson(ctx, dbgen.EndUnjoinedLessonParams{ID: id, EndedBefore: endedBefore, Now: now})
	return applied(rows, err, "end an unjoined lesson")
}

func (r Lessons) FreezeRoster(ctx context.Context, id domain.LessonID, students []domain.UserID, now time.Time) error {
	if len(students) == 0 {
		return nil
	}
	_, err := r.queries.FreezeLessonRoster(ctx, dbgen.FreezeLessonRosterParams{LessonID: id, Students: uuids(students), Now: now})
	return done(err, "freeze a lesson roster")
}

func (r Lessons) ReminderCandidates(ctx context.Context, query ReminderQuery) ([]ReminderCandidate, error) {
	rows, err := r.queries.ListReminderCandidates(ctx, dbgen.ListReminderCandidatesParams{
		DayBefore: query.DayBefore, WindowStart: query.WindowStart, WindowEnd: query.WindowEnd,
		AfterStartsAt: query.After.StartsAt, AfterID: pgtype.UUID{Bytes: query.After.ID, Valid: true}, RowLimit: query.Limit,
	})
	return one(rows, err, "reminder candidates")
}

func (r Lessons) MarkReminded(ctx context.Context, id domain.LessonID, dayBefore bool, revision int32) (bool, error) {
	rows, err := r.queries.MarkLessonReminded(ctx, dbgen.MarkLessonRemindedParams{ID: id, DayBefore: dayBefore, ScheduleRevision: revision})
	return applied(rows, err, "mark a lesson reminded")
}

func (r Lessons) SeriesToExtend(ctx context.Context, query SeriesCandidateQuery) ([]SeriesCandidate, error) {
	rows, err := r.queries.ListSeriesToExtend(ctx, dbgen.ListSeriesToExtendParams{
		Now: query.Now, Horizon: query.Horizon, AfterUntil: pgDate(query.After.MaterialisedUntil),
		AfterID: pgtype.UUID{Bytes: query.After.ID, Valid: true}, RowLimit: query.Limit,
	})
	if err != nil {
		return nil, done(err, "series to extend")
	}
	candidates := make([]SeriesCandidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, SeriesCandidate{ID: row.ID, MaterialisedUntil: localDate(row.MaterialisedUntil)})
	}
	return candidates, nil
}

func (r Lessons) SeriesJobAudience(ctx context.Context, id domain.SeriesID) (JobSeriesAudience, error) {
	row, err := r.queries.GetSeriesAudience(ctx, id)
	return one(row, err, "the audience of a series")
}

func (r Lessons) LockSeriesForExtension(ctx context.Context, id domain.SeriesID) (LockedSeries, error) {
	row, err := r.queries.LockSeriesForExtension(ctx, id)
	if _, err := one(row, err, "lock a series for extension"); err != nil {
		return LockedSeries{}, err
	}
	return LockedSeries{
		Weekdays: weekdays(row.Weekdays), Start: localTime(row.LocalStart), Timezone: row.Timezone, First: localDate(row.FirstDate),
		EndDate: optionalLocalDate(row.EndDate), MaterialisedUntil: localDate(row.MaterialisedUntil), Ended: row.EndedAt != nil,
		TutorState: row.TutorState,
	}, nil
}

func (r Lessons) InsertOccurrences(ctx context.Context, series domain.SeriesID, occurrences []NewOccurrence, now time.Time) ([]domain.LessonID, error) {
	if len(occurrences) == 0 {
		return nil, nil
	}
	params := dbgen.InsertSeriesOccurrencesParams{SeriesID: series, Now: now}
	for _, occurrence := range occurrences {
		params.Ids = append(params.Ids, pgtype.UUID{Bytes: occurrence.ID, Valid: true})
		params.OccurrenceDates = append(params.OccurrenceDates, pgDate(occurrence.Date))
		params.Starts = append(params.Starts, occurrence.StartsAt)
	}
	ids, err := r.queries.InsertSeriesOccurrences(ctx, params)
	return one(ids, err, "insert series occurrences")
}

func (r Lessons) SetMaterialisedUntil(ctx context.Context, series domain.SeriesID, through domain.LocalDate, now time.Time) error {
	return done(r.queries.SetSeriesMaterialisedUntil(ctx, dbgen.SetSeriesMaterialisedUntilParams{
		ID: series, MaterialisedUntil: pgDate(through), Now: now,
	}), "set a series' materialised horizon")
}

func (r Lessons) ForEmail(ctx context.Context, id domain.LessonID) (LessonForEmail, error) {
	row, err := r.queries.GetLessonForEmail(ctx, id)
	return one(row, err, "a lesson for an email")
}

func (r Lessons) IsCurrentMember(ctx context.Context, group domain.GroupID, student domain.UserID) (bool, error) {
	member, err := r.queries.IsCurrentGroupMember(ctx, dbgen.IsCurrentGroupMemberParams{
		GroupID: group, StudentID: pgtype.UUID{Bytes: student, Valid: true},
	})
	return one(member, err, "current group membership")
}

func (r Lessons) TutorNotificationLessons(ctx context.Context, tutor domain.UserID, ids []domain.LessonID) ([]NotificationLesson, error) {
	rows, err := r.queries.ListTutorNotificationLessons(ctx, dbgen.ListTutorNotificationLessonsParams{TutorID: tutor, Ids: uuids(ids)})
	return one(rows, err, "lessons of a tutor's notifications")
}

func (r Lessons) StudentNotificationLessons(ctx context.Context, student domain.UserID, ids []domain.LessonID) ([]NotificationLesson, error) {
	rows, err := r.queries.ListStudentNotificationLessons(ctx, dbgen.ListStudentNotificationLessonsParams{
		StudentID: pgtype.UUID{Bytes: student, Valid: true}, Ids: uuids(ids),
	})
	if err != nil {
		return nil, done(err, "lessons of a student's notifications")
	}
	lessons := make([]NotificationLesson, 0, len(rows))
	for _, row := range rows {
		lessons = append(lessons, NotificationLesson(row))
	}
	return lessons, nil
}

func (r RateLimits) ReserveBelow(ctx context.Context, slot DailySlot) (bool, error) {
	_, err := r.queries.ReserveRateLimitSlotBelow(ctx, dbgen.ReserveRateLimitSlotBelowParams(slot))
	if _, err := one(0, err, "reserve a capped rate limit slot"); errors.Is(err, ErrNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
