package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	TutorLessonView    = dbgen.ListTutorLessonViewsRow
	TutorCalendarItem  = dbgen.ListTutorCalendarRow
	StudentCalendarRow = dbgen.ListStudentCalendarRow
	StudentLessonView  = dbgen.GetStudentLessonViewRow
	GroupLessonMember  = dbgen.ListGroupLessonMembersRow
	RosterLessonMember = dbgen.ListRosterLessonMembersRow
	LockedAudience     = dbgen.LockTutorRelationshipsForLessonRow
)

type Lessons struct {
	queries *dbgen.Queries
}

type NewSeries struct {
	ID                domain.SeriesID
	TutorID           domain.UserID
	RelationshipID    *domain.RelationshipID
	GroupID           *domain.GroupID
	Weekdays          []domain.Weekday
	Start             domain.LocalTime
	DurationMinutes   int16
	Timezone          string
	First             domain.LocalDate
	EndDate           *domain.LocalDate
	Count             *int16
	Title             string
	Description       *string
	MaterialisedUntil domain.LocalDate
	CreateKey         *string
	Now               time.Time
}

type Series struct {
	ID              domain.SeriesID
	Weekdays        []domain.Weekday
	Start           domain.LocalTime
	DurationMinutes int16
	Timezone        string
	First           domain.LocalDate
	EndDate         *domain.LocalDate
	Count           *int16
}

type NewLessonItem struct {
	ID       domain.LessonID
	Date     *domain.LocalDate
	StartsAt time.Time
}

type NewLessons struct {
	TutorID         domain.UserID
	SeriesID        *domain.SeriesID
	RelationshipID  *domain.RelationshipID
	GroupID         *domain.GroupID
	Title           string
	Description     *string
	DurationMinutes int16
	Timezone        string
	CreateKey       *string
	Now             time.Time
	Items           []NewLessonItem
}

type TimedLesson struct {
	ID              domain.LessonID
	Title           string
	StartsAt        time.Time
	DurationMinutes int16
}

type SeriesBeyondHorizon struct {
	ID                domain.SeriesID
	Title             string
	Weekdays          []domain.Weekday
	Start             domain.LocalTime
	DurationMinutes   int16
	Timezone          string
	First             domain.LocalDate
	EndDate           domain.LocalDate
	MaterialisedUntil domain.LocalDate
}

func pgDate(date domain.LocalDate) pgtype.Date {
	return pgtype.Date{Time: date.Midnight(), Valid: true}
}

func optionalPgDate(date *domain.LocalDate) pgtype.Date {
	if date == nil {
		return pgtype.Date{}
	}
	return pgDate(*date)
}

func localDate(date pgtype.Date) domain.LocalDate {
	return domain.DateOf(date.Time)
}

func optionalLocalDate(date pgtype.Date) *domain.LocalDate {
	if !date.Valid {
		return nil
	}
	converted := localDate(date)
	return &converted
}

func pgTime(at domain.LocalTime) pgtype.Time {
	return pgtype.Time{Microseconds: at.SinceMidnight().Microseconds(), Valid: true}
}

func localTime(at pgtype.Time) domain.LocalTime {
	return domain.LocalTimeOf(time.Duration(at.Microseconds) * time.Microsecond)
}

func weekdayNumbers(days []domain.Weekday) []int16 {
	numbers := make([]int16, 0, len(days))
	for _, day := range days {
		numbers = append(numbers, int16(day))
	}
	return numbers
}

func weekdays(numbers []int16) []domain.Weekday {
	days := make([]domain.Weekday, 0, len(numbers))
	for _, number := range numbers {
		days = append(days, domain.Weekday(number))
	}
	return days
}

func optionalUUID[ID ~[16]byte](id *ID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func (r Lessons) CreateSeries(ctx context.Context, series NewSeries) error {
	return done(r.queries.CreateLessonSeries(ctx, dbgen.CreateLessonSeriesParams{
		ID: series.ID, TutorID: series.TutorID, RelationshipID: series.RelationshipID, GroupID: series.GroupID,
		Weekdays: weekdayNumbers(series.Weekdays), LocalStart: pgTime(series.Start), DurationMinutes: series.DurationMinutes,
		Timezone: series.Timezone, FirstDate: pgDate(series.First), EndDate: optionalPgDate(series.EndDate), Count: series.Count,
		Title: series.Title, Description: series.Description, MaterialisedUntil: pgDate(series.MaterialisedUntil),
		CreateKey: series.CreateKey, Now: series.Now,
	}), "create lesson series")
}

func (r Lessons) Create(ctx context.Context, lessons NewLessons) error {
	ids := make([]pgtype.UUID, 0, len(lessons.Items))
	dates := make([]pgtype.Date, 0, len(lessons.Items))
	starts := make([]time.Time, 0, len(lessons.Items))
	for _, item := range lessons.Items {
		ids = append(ids, pgtype.UUID{Bytes: item.ID, Valid: true})
		dates = append(dates, optionalPgDate(item.Date))
		starts = append(starts, item.StartsAt)
	}
	_, err := r.queries.CreateLessons(ctx, dbgen.CreateLessonsParams{
		Ids: ids, TutorID: pgtype.UUID{Bytes: lessons.TutorID, Valid: true}, SeriesID: optionalUUID(lessons.SeriesID),
		OccurrenceDates: dates, RelationshipID: optionalUUID(lessons.RelationshipID), GroupID: optionalUUID(lessons.GroupID),
		Title: lessons.Title, Description: lessons.Description, Starts: starts, DurationMinutes: lessons.DurationMinutes,
		Timezone: lessons.Timezone, CreateKey: lessons.CreateKey, Now: lessons.Now,
	})
	return done(err, "create lessons")
}

func (r Lessons) LessonByCreateKey(ctx context.Context, tutor domain.UserID, key string) (domain.LessonID, error) {
	id, err := r.queries.GetTutorLessonByCreateKey(ctx, dbgen.GetTutorLessonByCreateKeyParams{TutorID: tutor, CreateKey: &key})
	return one(id, err, "lesson by create key")
}

func (r Lessons) SeriesByCreateKey(ctx context.Context, tutor domain.UserID, key string) (Series, error) {
	row, err := r.queries.GetTutorSeriesByCreateKey(ctx, dbgen.GetTutorSeriesByCreateKeyParams{TutorID: tutor, CreateKey: &key})
	if _, err := one(row, err, "series by create key"); err != nil {
		return Series{}, err
	}
	return Series{
		ID: row.ID, Weekdays: weekdays(row.Weekdays), Start: localTime(row.LocalStart), DurationMinutes: row.DurationMinutes,
		Timezone: row.Timezone, First: localDate(row.FirstDate), EndDate: optionalLocalDate(row.EndDate), Count: row.Count,
	}, nil
}

func (r Lessons) SeriesLessonIDs(ctx context.Context, series domain.SeriesID) ([]domain.LessonID, error) {
	ids, err := r.queries.ListSeriesLessonIDs(ctx, &series)
	return one(ids, err, "lessons of a series")
}

func (r Lessons) CountOpenSeries(ctx context.Context, tutor domain.UserID, now time.Time) (int64, error) {
	count, err := r.queries.CountOpenSeriesOfTutor(ctx, dbgen.CountOpenSeriesOfTutorParams{TutorID: tutor, Now: now})
	return one(count, err, "count open series of a tutor")
}

func (r Lessons) CountFuture(ctx context.Context, tutor domain.UserID, now time.Time) (int64, error) {
	count, err := r.queries.CountFutureLessonsOfTutor(ctx, dbgen.CountFutureLessonsOfTutorParams{TutorID: tutor, Now: now})
	return one(count, err, "count future lessons of a tutor")
}

func (r Lessons) ForOverlap(ctx context.Context, tutor domain.UserID, from time.Time, to time.Time) ([]TimedLesson, error) {
	rows, err := r.queries.ListTutorLessonsForOverlap(ctx, dbgen.ListTutorLessonsForOverlapParams{TutorID: tutor, RangeStart: from, RangeEnd: to})
	if err != nil {
		return nil, done(err, "lessons for an overlap check")
	}
	lessons := make([]TimedLesson, 0, len(rows))
	for _, row := range rows {
		lessons = append(lessons, TimedLesson{ID: row.ID, Title: row.Title, StartsAt: row.StartsAt, DurationMinutes: row.DurationMinutes})
	}
	return lessons, nil
}

func (r Lessons) SeriesBeyondHorizon(ctx context.Context, tutor domain.UserID, from domain.LocalDate, through domain.LocalDate) ([]SeriesBeyondHorizon, error) {
	rows, err := r.queries.ListTutorSeriesBeyondHorizon(ctx, dbgen.ListTutorSeriesBeyondHorizonParams{
		TutorID: tutor, FromDate: pgDate(from), ThroughDate: pgDate(through),
	})
	if err != nil {
		return nil, done(err, "series beyond their horizon")
	}
	series := make([]SeriesBeyondHorizon, 0, len(rows))
	for _, row := range rows {
		series = append(series, SeriesBeyondHorizon{
			ID: row.ID, Title: row.Title, Weekdays: weekdays(row.Weekdays), Start: localTime(row.LocalStart), DurationMinutes: row.DurationMinutes,
			Timezone: row.Timezone, First: localDate(row.FirstDate), EndDate: localDate(row.EndDate), MaterialisedUntil: localDate(row.MaterialisedUntil),
		})
	}
	return series, nil
}

func (r Lessons) LockRelationship(ctx context.Context, tutor domain.UserID, id domain.RelationshipID) (LockedAudience, error) {
	row, err := r.queries.LockTutorRelationshipForLesson(ctx, dbgen.LockTutorRelationshipForLessonParams{ID: id, TutorID: tutor})
	return one(LockedAudience(row), err, "lock a relationship for a lesson")
}

func (r Lessons) LockRelationships(ctx context.Context, tutor domain.UserID, ids []domain.RelationshipID) ([]LockedAudience, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.LockTutorRelationshipsForLesson(ctx, dbgen.LockTutorRelationshipsForLessonParams{TutorID: tutor, Ids: uuids(ids)})
	return one(rows, err, "lock relationships for a lesson")
}

func (r Lessons) LockGroup(ctx context.Context, tutor domain.UserID, id domain.GroupID) (*time.Time, error) {
	row, err := r.queries.LockTutorGroupForLesson(ctx, dbgen.LockTutorGroupForLessonParams{ID: id, TutorID: tutor})
	if _, err := one(row, err, "lock a group for a lesson"); err != nil {
		return nil, err
	}
	return row.ArchivedAt, nil
}

func (r Lessons) ListedRelationshipTutor(ctx context.Context, student domain.UserID, id domain.RelationshipID) (domain.UserID, error) {
	tutor, err := r.queries.GetStudentListedRelationship(ctx, dbgen.GetStudentListedRelationshipParams{ID: id, StudentID: &student})
	return one(tutor, err, "a relationship the student lists")
}

func (r Lessons) TutorViews(ctx context.Context, tutor domain.UserID, ids []domain.LessonID) ([]TutorLessonView, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListTutorLessonViews(ctx, dbgen.ListTutorLessonViewsParams{TutorID: tutor, Ids: uuids(ids)})
	return one(rows, err, "lesson views of a tutor")
}

func (r Lessons) TutorCalendar(ctx context.Context, tutor domain.UserID, from time.Time, to time.Time, limit int32) ([]TutorCalendarItem, error) {
	rows, err := r.queries.ListTutorCalendar(ctx, dbgen.ListTutorCalendarParams{TutorID: tutor, RangeStart: from, RangeEnd: to, RowLimit: limit})
	return one(rows, err, "calendar of a tutor")
}

func (r Lessons) TutorHasLessonStarting(ctx context.Context, tutor domain.UserID, from time.Time, to time.Time) (bool, error) {
	found, err := r.queries.TutorHasLessonStarting(ctx, dbgen.TutorHasLessonStartingParams{TutorID: tutor, RangeStart: from, RangeEnd: to})
	return one(found, err, "whether a tutor has a lesson in a range")
}

func (r Lessons) GroupMembers(ctx context.Context, tutor domain.UserID, groups []domain.GroupID) ([]GroupLessonMember, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListGroupLessonMembers(ctx, dbgen.ListGroupLessonMembersParams{TutorID: tutor, GroupIds: uuids(groups)})
	return one(rows, err, "members of lesson groups")
}

func (r Lessons) RosterMembers(ctx context.Context, tutor domain.UserID, lessons []domain.LessonID) ([]RosterLessonMember, error) {
	if len(lessons) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListRosterLessonMembers(ctx, dbgen.ListRosterLessonMembersParams{TutorID: tutor, LessonIds: uuids(lessons)})
	return one(rows, err, "roster members of lessons")
}

func (r Lessons) StudentCalendar(
	ctx context.Context, student domain.UserID, tutor *domain.UserID, from time.Time, to time.Time, limit int32,
) ([]StudentCalendarRow, error) {
	rows, err := r.queries.ListStudentCalendar(ctx, dbgen.ListStudentCalendarParams{
		StudentID: pgtype.UUID{Bytes: student, Valid: true}, TutorID: optionalUUID(tutor), RangeStart: from, RangeEnd: to, RowLimit: limit,
	})
	return one(rows, err, "calendar of a student")
}

func (r Lessons) StudentView(ctx context.Context, student domain.UserID, id domain.LessonID) (StudentLessonView, error) {
	row, err := r.queries.GetStudentLessonView(ctx, dbgen.GetStudentLessonViewParams{
		StudentID: pgtype.UUID{Bytes: student, Valid: true}, ID: pgtype.UUID{Bytes: id, Valid: true},
	})
	return one(row, err, "lesson view of a student")
}

func (r Lessons) DeleteRosterBatchOfTutor(ctx context.Context, tutor domain.UserID, batchSize int32) (int64, error) {
	rows, err := r.queries.DeleteTutorRosterBatch(ctx, dbgen.DeleteTutorRosterBatchParams{TutorID: tutor, BatchSize: batchSize})
	return counted(rows, err, "delete a batch of a tutor's lesson rosters")
}

func (r Lessons) DeleteBatchOfTutor(ctx context.Context, tutor domain.UserID, batchSize int32) (int64, error) {
	rows, err := r.queries.DeleteTutorLessonsBatch(ctx, dbgen.DeleteTutorLessonsBatchParams{TutorID: tutor, BatchSize: batchSize})
	return counted(rows, err, "delete a batch of a tutor's lessons")
}

func (r Lessons) DeleteSeriesBatchOfTutor(ctx context.Context, tutor domain.UserID, batchSize int32) (int64, error) {
	rows, err := r.queries.DeleteTutorSeriesBatch(ctx, dbgen.DeleteTutorSeriesBatchParams{TutorID: tutor, BatchSize: batchSize})
	return counted(rows, err, "delete a batch of a tutor's lesson series")
}

func (r Lessons) DeleteRosterOfStudent(ctx context.Context, student domain.UserID) (int64, error) {
	rows, err := r.queries.DeleteStudentRosterRows(ctx, student)
	return counted(rows, err, "delete a student's roster rows")
}

type LessonCursor struct {
	StartsAt time.Time
	ID       domain.LessonID
}

type CardNextLesson struct {
	Card   domain.RelationshipID
	Lesson TutorCalendarItem
}

type GroupNextLesson struct {
	Group  domain.GroupID
	Lesson TutorCalendarItem
}

func (r Lessons) CardNext(ctx context.Context, tutor domain.UserID, card domain.RelationshipID, now time.Time, limit int32) ([]TutorCalendarItem, error) {
	rows, err := r.queries.ListCardNextLessons(ctx, dbgen.ListCardNextLessonsParams{RelationshipID: &card, TutorID: tutor, Now: now, RowLimit: limit})
	if err != nil {
		return nil, done(err, "next lessons of a card")
	}
	items := make([]TutorCalendarItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, TutorCalendarItem(row))
	}
	return items, nil
}

func (r Lessons) CardHistory(
	ctx context.Context, tutor domain.UserID, card domain.RelationshipID, after *LessonCursor, limit int32,
) ([]TutorCalendarItem, error) {
	params := dbgen.ListCardLessonHistoryParams{RelationshipID: &card, TutorID: tutor, RowLimit: limit}
	if after != nil {
		params.AfterStartsAt = &after.StartsAt
		params.AfterID = pgtype.UUID{Bytes: after.ID, Valid: true}
	}
	rows, err := r.queries.ListCardLessonHistory(ctx, params)
	if err != nil {
		return nil, done(err, "lesson history of a card")
	}
	items := make([]TutorCalendarItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, TutorCalendarItem(row))
	}
	return items, nil
}

func (r Lessons) CardsNext(ctx context.Context, tutor domain.UserID, cards []domain.RelationshipID, now time.Time) ([]CardNextLesson, error) {
	if len(cards) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListCardsNextLesson(ctx, dbgen.ListCardsNextLessonParams{TutorID: tutor, Ids: uuids(cards), Now: now})
	if err != nil {
		return nil, done(err, "next lessons of cards")
	}
	next := make([]CardNextLesson, 0, len(rows))
	for _, row := range rows {
		next = append(next, CardNextLesson{Card: row.CardID, Lesson: TutorCalendarItem{
			ID: row.ID, Title: row.Title, StartsAt: row.StartsAt, DurationMinutes: row.DurationMinutes, Status: row.Status,
			Timezone: row.Timezone, RelationshipID: row.RelationshipID, GroupID: row.GroupID, CardName: row.CardName,
			CardState: row.CardState, CardDiscardReason: row.CardDiscardReason, CardColour: row.CardColour, GroupName: row.GroupName,
			GroupColour: row.GroupColour,
		}})
	}
	return next, nil
}

func (r Lessons) GroupNext(ctx context.Context, tutor domain.UserID, group domain.GroupID, now time.Time, limit int32) ([]TutorCalendarItem, error) {
	rows, err := r.queries.ListGroupNextLessons(ctx, dbgen.ListGroupNextLessonsParams{GroupID: &group, TutorID: tutor, Now: now, RowLimit: limit})
	if err != nil {
		return nil, done(err, "next lessons of a group")
	}
	items := make([]TutorCalendarItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, TutorCalendarItem(row))
	}
	return items, nil
}

func (r Lessons) GroupsNext(ctx context.Context, tutor domain.UserID, groups []domain.GroupID, now time.Time) ([]GroupNextLesson, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListGroupsNextLesson(ctx, dbgen.ListGroupsNextLessonParams{TutorID: tutor, Ids: uuids(groups), Now: now})
	if err != nil {
		return nil, done(err, "next lessons of groups")
	}
	next := make([]GroupNextLesson, 0, len(rows))
	for _, row := range rows {
		next = append(next, GroupNextLesson{Group: row.RequestedGroupID, Lesson: TutorCalendarItem{
			ID: row.ID, Title: row.Title, StartsAt: row.StartsAt, DurationMinutes: row.DurationMinutes, Status: row.Status,
			Timezone: row.Timezone, RelationshipID: row.RelationshipID, GroupID: row.GroupID, CardName: row.CardName,
			CardState: row.CardState, CardDiscardReason: row.CardDiscardReason, CardColour: row.CardColour, GroupName: row.GroupName,
			GroupColour: row.GroupColour,
		}})
	}
	return next, nil
}

func (r Lessons) CountFutureOfGroup(ctx context.Context, tutor domain.UserID, group domain.GroupID, now time.Time) (int64, error) {
	count, err := r.queries.CountFutureLessonsOfGroup(ctx, dbgen.CountFutureLessonsOfGroupParams{GroupID: &group, TutorID: tutor, Now: now})
	return one(count, err, "count future lessons of a group")
}

func (r Lessons) GroupHasOpenSeries(ctx context.Context, tutor domain.UserID, group domain.GroupID, now time.Time) (bool, error) {
	open, err := r.queries.GroupHasOpenSeries(ctx, dbgen.GroupHasOpenSeriesParams{GroupID: &group, TutorID: tutor, Now: now})
	return one(open, err, "whether a group has an open series")
}
