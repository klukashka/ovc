package schedule

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/domain/schedule"
	"ovc/backend/internal/repositories"
)

const (
	fieldBody         = "body"
	fieldRelationship = "body.relationship_id"
	fieldTitle        = "body.title"
	fieldDescription  = "body.description"
	fieldDate         = "body.date"
	fieldStart        = "body.start"
	fieldWeekdays     = "body.weekdays"
	fieldEndDate      = "body.end_date"
	fieldCount        = "body.count"
	audienceAttempts  = 2
)

type NewLesson struct {
	RelationshipID  *domain.RelationshipID
	GroupID         *domain.GroupID
	Title           string
	Description     *string
	Date            string
	Start           string
	DurationMinutes int16
	Weekdays        []int16
	EndDate         *string
	Count           *int16
	AllowOverlap    bool
	CreateKey       *string
}

type SeriesView struct {
	ID              domain.SeriesID
	Weekdays        []domain.Weekday
	Start           domain.LocalTime
	DurationMinutes int
	Timezone        string
	First           domain.LocalDate
	EndDate         *domain.LocalDate
	Count           *int
}

type Created struct {
	Series  *SeriesView
	Lessons []Lesson
}

type audience struct {
	relationship   *domain.RelationshipID
	group          *domain.GroupID
	students       []domain.UserID
	membersChanged bool
}

type plannedLessons struct {
	timezone          string
	series            *schedule.Series
	occurrences       []schedule.Occurrence
	all               []schedule.Occurrence
	skipped           int
	materialisedUntil domain.LocalDate
}

type creationOutcome struct {
	created  Created
	replayed bool
	skipped  int
}

func (s *Service) CreateLessons(ctx context.Context, tutor domain.UserID, request NewLesson) (Created, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Created{}, err
	}
	var outcome creationOutcome
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		var err error
		outcome, err = s.admit(ctx, repos, tutor, request, now)
		return err
	})
	if err != nil || outcome.replayed {
		return outcome.created, err
	}
	for attempt := 1; ; attempt++ {
		err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
			var err error
			outcome, err = s.create(ctx, repos, tutor, request, now)
			return err
		})
		if !errors.Is(err, errMembersChanged) {
			break
		}
		if attempt == audienceAttempts {
			return Created{}, ErrConflict
		}
	}
	var overlap *OverlapError
	if errors.As(err, &overlap) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "lesson.overlap_warned", tutorAttribute(tutor), slog.Int("count", len(overlap.Lessons)+overlap.More))
	}
	if err != nil {
		return Created{}, err
	}
	if !outcome.replayed {
		s.logCreated(ctx, tutor, outcome)
	}
	return outcome.created, nil
}

func (s *Service) logCreated(ctx context.Context, tutor domain.UserID, outcome creationOutcome) {
	if outcome.created.Series != nil {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "series.created", tutorAttribute(tutor), slog.String("series_id", outcome.created.Series.ID.String()),
			slog.Int("materialised", len(outcome.created.Lessons)), slog.Int("skipped", outcome.skipped))
		return
	}
	for _, lesson := range outcome.created.Lessons {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "lesson.created", tutorAttribute(tutor), slog.String("lesson_id", lesson.ID.String()))
	}
}

func (s *Service) replayUnderLock(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, request NewLesson,
) (repositories.User, creationOutcome, error) {
	account, err := s.lockActingTutor(ctx, repos, tutor)
	if err != nil || request.CreateKey == nil {
		return account, creationOutcome{}, err
	}
	replayed, found, err := s.replay(ctx, repos, account, *request.CreateKey)
	return account, creationOutcome{created: replayed, replayed: found}, err
}

func (s *Service) admit(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, request NewLesson, now time.Time) (creationOutcome, error) {
	_, outcome, err := s.replayUnderLock(ctx, repos, tutor, request)
	if err != nil || outcome.replayed {
		return outcome, err
	}
	return creationOutcome{}, s.reserveWrite(ctx, repos, tutor, now)
}

func (s *Service) create(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, request NewLesson, now time.Time) (creationOutcome, error) {
	account, outcome, err := s.replayUnderLock(ctx, repos, tutor, request)
	if err != nil || outcome.replayed {
		return outcome, err
	}
	target, err := lockAudience(ctx, repos, tutor, request)
	if err != nil {
		return creationOutcome{}, err
	}
	planned, err := plan(request, account.Timezone, now)
	if err != nil {
		return creationOutcome{}, err
	}
	if err := checkCaps(ctx, repos, tutor, planned, now); err != nil {
		return creationOutcome{}, err
	}
	if target.membersChanged {
		return creationOutcome{}, errMembersChanged
	}
	if !request.AllowOverlap {
		if err := checkOverlap(ctx, repos, tutor, int(request.DurationMinutes), planned.all); err != nil {
			return creationOutcome{}, err
		}
	}
	created, err := s.insert(ctx, repos, account, request, target, planned, now)
	return creationOutcome{created: created, skipped: planned.skipped}, err
}

func (s *Service) replay(ctx context.Context, repos repositories.Repositories, tutor repositories.User, key string) (Created, bool, error) {
	lesson, err := repos.Lessons.LessonByCreateKey(ctx, tutor.ID, key)
	if err == nil {
		lessons, err := s.tutorViews(ctx, repos, tutor, []domain.LessonID{lesson})
		return Created{Lessons: lessons}, true, err
	}
	if !errors.Is(err, repositories.ErrNotFound) {
		return Created{}, false, err
	}
	series, err := repos.Lessons.SeriesByCreateKey(ctx, tutor.ID, key)
	if errors.Is(err, repositories.ErrNotFound) {
		return Created{}, false, nil
	}
	if err != nil {
		return Created{}, false, err
	}
	ids, err := repos.Lessons.SeriesLessonIDs(ctx, series.ID)
	if err != nil {
		return Created{}, false, err
	}
	lessons, err := s.tutorViews(ctx, repos, tutor, ids)
	return Created{Series: seriesView(series), Lessons: lessons}, true, err
}

func seriesView(series repositories.Series) *SeriesView {
	view := &SeriesView{
		ID: series.ID, Weekdays: series.Weekdays, Start: series.Start, DurationMinutes: int(series.DurationMinutes), Timezone: series.Timezone,
		First: series.First, EndDate: series.EndDate,
	}
	if series.Count != nil {
		count := int(*series.Count)
		view.Count = &count
	}
	return view
}

func lockAudience(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, request NewLesson) (audience, error) {
	if (request.RelationshipID == nil) == (request.GroupID == nil) {
		return audience{}, invalid(fieldRelationship, domain.FieldRequired)
	}
	if request.RelationshipID != nil {
		locked, err := repos.Lessons.LockRelationship(ctx, tutor, *request.RelationshipID)
		if errors.Is(err, repositories.ErrNotFound) {
			return audience{}, ErrNotFound
		}
		if err != nil {
			return audience{}, err
		}
		if locked.State != domain.RelationshipActive && locked.State != domain.RelationshipProvisional || locked.StudentID == nil {
			return audience{}, ErrAudienceNotActive
		}
		return audience{relationship: request.RelationshipID, students: []domain.UserID{*locked.StudentID}}, nil
	}
	return lockGroupAudience(ctx, repos, tutor, *request.GroupID)
}

func sortedRelationships(ids []domain.RelationshipID) []domain.RelationshipID {
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, func(a, b domain.RelationshipID) int { return bytes.Compare(a[:], b[:]) })
	return sorted
}

func lockGroupAudience(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, group domain.GroupID) (audience, error) {
	if _, err := repos.Groups.ForTutor(ctx, tutor, group); errors.Is(err, repositories.ErrNotFound) {
		return audience{}, ErrNotFound
	} else if err != nil {
		return audience{}, err
	}
	read, err := repos.Groups.MemberIDs(ctx, tutor, group)
	if err != nil {
		return audience{}, err
	}
	locked, err := repos.Lessons.LockRelationships(ctx, tutor, sortedRelationships(read))
	if err != nil {
		return audience{}, err
	}
	archivedAt, err := repos.Lessons.LockGroup(ctx, tutor, group)
	if errors.Is(err, repositories.ErrNotFound) {
		return audience{}, ErrNotFound
	}
	if err != nil {
		return audience{}, err
	}
	if archivedAt != nil {
		return audience{}, ErrAudienceNotActive
	}
	members, err := repos.Groups.MemberIDs(ctx, tutor, group)
	if err != nil {
		return audience{}, err
	}
	lockedIDs := make([]domain.RelationshipID, 0, len(locked))
	students := make([]domain.UserID, 0, len(locked))
	for _, row := range locked {
		lockedIDs = append(lockedIDs, row.ID)
		if row.StudentID != nil {
			students = append(students, *row.StudentID)
		}
	}
	return audience{group: &group, students: students, membersChanged: !slices.Equal(sortedRelationships(members), lockedIDs)}, nil
}

func plan(request NewLesson, profileTimezone string, now time.Time) (plannedLessons, error) {
	date, validDate := domain.ParseLocalDate(request.Date)
	start, validStart := domain.ParseLocalTime(request.Start)
	problems := textProblems(request)
	if !validDate {
		problems = append(problems, FieldProblem{Path: fieldDate, Code: domain.FieldInvalid})
	}
	if !validStart {
		problems = append(problems, FieldProblem{Path: fieldStart, Code: domain.FieldInvalid})
	}
	if len(problems) > 0 {
		return plannedLessons{}, &ValidationError{Fields: problems}
	}
	zone, err := loadZone(profileTimezone)
	if err != nil {
		return plannedLessons{}, err
	}
	if request.Weekdays == nil {
		return planOneOff(request, date, start, zone, now)
	}
	return planSeries(request, date, start, zone, now)
}

func planOneOff(request NewLesson, date domain.LocalDate, start domain.LocalTime, zone *time.Location, now time.Time) (plannedLessons, error) {
	if request.EndDate != nil || request.Count != nil {
		return plannedLessons{}, invalid(fieldWeekdays, domain.FieldRequired)
	}
	occurrence := schedule.Occurrence{Date: date, StartsAt: schedule.StartInstant(date, start, zone)}
	if !occurrence.StartsAt.After(now) {
		return plannedLessons{}, ErrLessonInPast
	}
	occurrences := []schedule.Occurrence{occurrence}
	return plannedLessons{timezone: zone.String(), occurrences: occurrences, all: occurrences}, nil
}

func planSeries(request NewLesson, first domain.LocalDate, start domain.LocalTime, zone *time.Location, now time.Time) (plannedLessons, error) {
	series := schedule.Series{First: first, Start: start, Zone: zone}
	for _, day := range request.Weekdays {
		series.Weekdays = append(series.Weekdays, domain.Weekday(day))
	}
	if (request.EndDate == nil) == (request.Count == nil) {
		return plannedLessons{}, invalid(fieldEndDate, domain.FieldRequired)
	}
	if request.EndDate != nil {
		end, valid := domain.ParseLocalDate(*request.EndDate)
		if !valid {
			return plannedLessons{}, invalid(fieldEndDate, domain.FieldInvalid)
		}
		series.EndDate = &end
	} else {
		series.Count = int(*request.Count)
	}
	if problems := seriesProblems(series); problems != nil {
		return plannedLessons{}, problems
	}
	future := series.Future(now)
	if !future.CountFits {
		return plannedLessons{}, invalid(fieldCount, domain.FieldInvalid)
	}
	if len(future.Occurrences) == 0 {
		return plannedLessons{}, ErrLessonInPast
	}
	through, complete := series.Horizon(now)
	materialised := schedule.Materialised(future, through)
	if complete {
		materialised = future.Occurrences
		through = future.Occurrences[len(future.Occurrences)-1].Date
		if series.EndDate != nil {
			through = *series.EndDate
		}
	}
	return plannedLessons{
		timezone: zone.String(), series: &series, occurrences: materialised, all: future.Occurrences, skipped: future.Skipped, materialisedUntil: through,
	}, nil
}

func seriesProblems(series schedule.Series) *ValidationError {
	paths := map[schedule.Problem]string{schedule.ProblemWeekdays: fieldWeekdays, schedule.ProblemEndDate: fieldEndDate, schedule.ProblemCount: fieldCount}
	var fields []FieldProblem
	for _, problem := range series.Problems() {
		fields = append(fields, FieldProblem{Path: paths[problem], Code: domain.FieldInvalid})
	}
	if len(fields) == 0 {
		return nil
	}
	return &ValidationError{Fields: fields}
}

func checkCaps(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, planned plannedLessons, now time.Time) error {
	if planned.series != nil {
		open, err := repos.Lessons.CountOpenSeries(ctx, tutor, now)
		if err != nil {
			return err
		}
		if open >= domain.OpenSeriesCapPerTutor {
			return invalid(fieldBody, domain.FieldTooMany)
		}
	}
	future, err := repos.Lessons.CountFuture(ctx, tutor, now)
	if err != nil {
		return err
	}
	if future+int64(len(planned.occurrences)) > domain.FutureLessonCapPerTutor {
		return invalid(fieldBody, domain.FieldTooMany)
	}
	return nil
}

func (s *Service) insert(
	ctx context.Context, repos repositories.Repositories, tutor repositories.User, request NewLesson, target audience, planned plannedLessons, now time.Time,
) (Created, error) {
	rows := repositories.NewLessons{
		TutorID: tutor.ID, RelationshipID: target.relationship, GroupID: target.group, Title: request.Title, Description: request.Description,
		DurationMinutes: request.DurationMinutes, Timezone: planned.timezone, Now: now,
	}
	var created Created
	if planned.series != nil {
		seriesID, err := core.NewID[domain.SeriesID](s.deps.Random)
		if err != nil {
			return Created{}, err
		}
		series := repositories.NewSeries{
			ID: seriesID, TutorID: tutor.ID, RelationshipID: target.relationship, GroupID: target.group, Weekdays: planned.series.Weekdays,
			Start: planned.series.Start, DurationMinutes: request.DurationMinutes, Timezone: planned.timezone, First: planned.series.First,
			EndDate: planned.series.EndDate, Title: request.Title, Description: request.Description, MaterialisedUntil: planned.materialisedUntil,
			CreateKey: request.CreateKey, Now: now,
		}
		if planned.series.EndDate == nil {
			series.Count = request.Count
		}
		if err := repos.Lessons.CreateSeries(ctx, series); err != nil {
			return Created{}, err
		}
		rows.SeriesID = &seriesID
		created.Series = seriesView(repositories.Series{
			ID: seriesID, Weekdays: series.Weekdays, Start: series.Start, DurationMinutes: series.DurationMinutes, Timezone: series.Timezone,
			First: series.First, EndDate: series.EndDate, Count: series.Count,
		})
	} else {
		rows.CreateKey = request.CreateKey
	}
	ids := make([]domain.LessonID, 0, len(planned.occurrences))
	for _, occurrence := range planned.occurrences {
		id, err := core.NewID[domain.LessonID](s.deps.Random)
		if err != nil {
			return Created{}, err
		}
		item := repositories.NewLessonItem{ID: id, StartsAt: occurrence.StartsAt}
		if planned.series != nil {
			date := occurrence.Date
			item.Date = &date
		}
		rows.Items = append(rows.Items, item)
		ids = append(ids, id)
	}
	if len(rows.Items) > 0 {
		if err := repos.Lessons.Create(ctx, rows); err != nil {
			return Created{}, err
		}
	}
	if err := signalLessons(ctx, repos, tutor.ID, target.students, ids); err != nil {
		return Created{}, err
	}
	lessons, err := s.tutorViews(ctx, repos, tutor, ids)
	created.Lessons = lessons
	return created, err
}

func signalLessons(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, students []domain.UserID, lessons []domain.LessonID) error {
	recipients := append([]domain.UserID{tutor}, students...)
	signals := make([]domain.EntitySignal, 0, len(lessons))
	for _, lesson := range lessons {
		signals = append(signals, domain.EntitySignal{Users: recipients, Entity: domain.EntityLesson, ID: domain.EntityID(lesson)})
	}
	return repos.Signals.Entities(ctx, signals)
}

func textProblems(request NewLesson) []FieldProblem {
	var problems []FieldProblem
	if strings.ContainsFunc(request.Title, unicode.IsControl) {
		problems = append(problems, FieldProblem{Path: fieldTitle, Code: domain.FieldInvalid})
	}
	if request.Description != nil && strings.ContainsFunc(*request.Description, controlOtherThanLineBreak) {
		problems = append(problems, FieldProblem{Path: fieldDescription, Code: domain.FieldInvalid})
	}
	return problems
}

func controlOtherThanLineBreak(character rune) bool {
	return unicode.IsControl(character) && character != '\n'
}
