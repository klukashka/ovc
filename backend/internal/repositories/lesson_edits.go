package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	EditedLessonTarget = dbgen.GetTutorLessonAudienceRow
	ChangeEmailLesson  = dbgen.ListLessonsForChangeEmailRow
)

type LockedSeriesForEdit struct {
	Series
	Title             string
	Description       *string
	MaterialisedUntil domain.LocalDate
	Ended             bool
}

type LockedLesson struct {
	ID               domain.LessonID
	SeriesID         *domain.SeriesID
	OccurrenceDate   *domain.LocalDate
	Status           domain.LessonStatus
	Title            string
	Description      *string
	StartsAt         time.Time
	DurationMinutes  int16
	Timezone         string
	Version          int32
	OverriddenFields []domain.LessonField
}

type EditedValues struct {
	ID               domain.LessonID
	StartsAt         time.Time
	DurationMinutes  int16
	Title            string
	Description      *string
	OverriddenFields []domain.LessonField
	Moved            bool
	Now              time.Time
}

type SplitSeries struct {
	ID              domain.SeriesID
	OriginalID      domain.SeriesID
	Start           domain.LocalTime
	DurationMinutes int16
	First           domain.LocalDate
	EndDate         *domain.LocalDate
	Count           *int16
	Title           string
	Description     *string
	Now             time.Time
}

func (r Lessons) TutorTarget(ctx context.Context, tutor domain.UserID, id domain.LessonID) (EditedLessonTarget, error) {
	row, err := r.queries.GetTutorLessonAudience(ctx, dbgen.GetTutorLessonAudienceParams{ID: id, TutorID: tutor})
	return one(row, err, "a lesson of a tutor")
}

func (r Lessons) LockSeriesForEdit(ctx context.Context, tutor domain.UserID, id domain.SeriesID) (LockedSeriesForEdit, error) {
	row, err := r.queries.LockTutorSeriesForEdit(ctx, dbgen.LockTutorSeriesForEditParams{ID: id, TutorID: tutor})
	if _, err := one(row, err, "lock a series for an edit"); err != nil {
		return LockedSeriesForEdit{}, err
	}
	return LockedSeriesForEdit{
		Series: Series{
			ID: row.ID, Weekdays: weekdays(row.Weekdays), Start: localTime(row.LocalStart), DurationMinutes: row.DurationMinutes,
			Timezone: row.Timezone, First: localDate(row.FirstDate), EndDate: optionalLocalDate(row.EndDate), Count: row.Count,
		},
		Title: row.Title, Description: row.Description, MaterialisedUntil: localDate(row.MaterialisedUntil), Ended: row.EndedAt != nil,
	}, nil
}

func (r Lessons) LockForEdit(
	ctx context.Context, tutor domain.UserID, id domain.LessonID, series *domain.SeriesID, from domain.LocalDate,
) ([]LockedLesson, error) {
	params := dbgen.LockLessonsForEditParams{TutorID: tutor, ID: pgtype.UUID{Bytes: id, Valid: true}, SeriesID: optionalUUID(series), FromDate: pgDate(from)}
	rows, err := r.queries.LockLessonsForEdit(ctx, params)
	if err != nil {
		return nil, done(err, "lock lessons for an edit")
	}
	locked := make([]LockedLesson, 0, len(rows))
	for _, row := range rows {
		fields := make([]domain.LessonField, 0, len(row.OverriddenFields))
		for _, field := range row.OverriddenFields {
			fields = append(fields, domain.LessonField(field))
		}
		locked = append(locked, LockedLesson{
			ID: row.ID, SeriesID: row.SeriesID, OccurrenceDate: optionalLocalDate(row.OccurrenceDate), Status: row.Status, Title: row.Title,
			Description: row.Description, StartsAt: row.StartsAt, DurationMinutes: row.DurationMinutes, Timezone: row.Timezone,
			Version: row.Version, OverriddenFields: fields,
		})
	}
	return locked, nil
}

func (r Lessons) ApplyEdit(ctx context.Context, values EditedValues) (int32, error) {
	fields := make([]string, 0, len(values.OverriddenFields))
	for _, field := range values.OverriddenFields {
		fields = append(fields, string(field))
	}
	revision, err := r.queries.UpdateEditedLesson(ctx, dbgen.UpdateEditedLessonParams{
		StartsAt: values.StartsAt, DurationMinutes: values.DurationMinutes, Title: values.Title, Description: values.Description,
		OverriddenFields: fields, Moved: values.Moved, Now: values.Now, ID: values.ID,
	})
	return one(revision, err, "apply a lesson edit")
}

func (r Lessons) EndSeriesBefore(ctx context.Context, id domain.SeriesID, from domain.LocalDate, now time.Time) error {
	_, err := r.queries.EndSeriesBefore(ctx, dbgen.EndSeriesBeforeParams{
		EndDate: pgDate(from.AddDays(-1)), FromDate: pgDate(from), Now: now, ID: pgtype.UUID{Bytes: id, Valid: true},
	})
	_, err = one(id, err, "end a series before an occurrence")
	return err
}

func (r Lessons) CreateSplitSeries(ctx context.Context, series SplitSeries) error {
	return done(r.queries.CreateSplitLessonSeries(ctx, dbgen.CreateSplitLessonSeriesParams{
		ID: pgtype.UUID{Bytes: series.ID, Valid: true}, LocalStart: pgTime(series.Start), DurationMinutes: series.DurationMinutes,
		FirstDate: pgDate(series.First), EndDate: optionalPgDate(series.EndDate), Count: series.Count, Title: series.Title,
		Description: series.Description, Now: series.Now, OriginalID: pgtype.UUID{Bytes: series.OriginalID, Valid: true},
	}), "create a split series")
}

func (r Lessons) MoveToSeries(ctx context.Context, original domain.SeriesID, split domain.SeriesID, from domain.LocalDate, now time.Time) (int64, error) {
	rows, err := r.queries.MoveLessonsToSeries(ctx, dbgen.MoveLessonsToSeriesParams{
		NewSeriesID: pgtype.UUID{Bytes: split, Valid: true}, Now: now, OriginalID: pgtype.UUID{Bytes: original, Valid: true}, FromDate: pgDate(from),
	})
	return counted(rows, err, "move lessons to a split series")
}

func (r Lessons) CancelByTutor(ctx context.Context, ids []domain.LessonID, now time.Time) ([]domain.LessonID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cancelled, err := r.queries.CancelLessonsByTutor(ctx, dbgen.CancelLessonsByTutorParams{Now: now, Ids: uuids(ids)})
	return one(cancelled, err, "cancel lessons")
}

func (r Lessons) SeriesViews(ctx context.Context, ids []domain.SeriesID) ([]Series, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListLessonSeriesViews(ctx, uuids(ids))
	if err != nil {
		return nil, done(err, "series of lessons")
	}
	series := make([]Series, 0, len(rows))
	for _, row := range rows {
		series = append(series, Series{
			ID: row.ID, Weekdays: weekdays(row.Weekdays), Start: localTime(row.LocalStart), DurationMinutes: row.DurationMinutes,
			Timezone: row.Timezone, First: localDate(row.FirstDate), EndDate: optionalLocalDate(row.EndDate), Count: row.Count,
		})
	}
	return series, nil
}

func (r Lessons) SeriesLessonsFrom(ctx context.Context, series domain.SeriesID, from domain.LocalDate) ([]domain.LessonID, error) {
	ids, err := r.queries.ListLessonsOfSeriesFrom(ctx, dbgen.ListLessonsOfSeriesFromParams{
		SeriesID: pgtype.UUID{Bytes: series, Valid: true}, FromDate: pgDate(from),
	})
	return one(ids, err, "lessons of a series from an occurrence")
}

func (r Lessons) OnRoster(ctx context.Context, lesson domain.LessonID, student domain.UserID) (bool, error) {
	listed, err := r.queries.IsOnLessonRoster(ctx, dbgen.IsOnLessonRosterParams{LessonID: lesson, StudentID: student})
	return one(listed, err, "a lesson roster entry")
}

func (r Lessons) ForChangeEmail(ctx context.Context, ids []domain.LessonID) ([]ChangeEmailLesson, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListLessonsForChangeEmail(ctx, uuids(ids))
	return one(rows, err, "lessons of a change email")
}
