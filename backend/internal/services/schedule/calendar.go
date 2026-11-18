package schedule

import (
	"context"
	"errors"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	fieldFrom  = "query.from"
	fieldTo    = "query.to"
	fieldTutor = "query.tutor"
)

type CalendarQuery struct {
	From  time.Time
	To    time.Time
	Tutor *domain.RelationshipID
}

type CalendarItem struct {
	ID              domain.LessonID
	Title           string
	StartsAt        time.Time
	DurationMinutes int
	Status          domain.LessonStatus
	TookPlace       bool
	Timezone        string
	Audience        Audience
	Tutor           *TutorLabel
}

func (i CalendarItem) EndsAt() time.Time {
	return i.StartsAt.Add(time.Duration(i.DurationMinutes) * time.Minute)
}

type Calendar struct {
	Items     []CalendarItem
	Truncated bool
}

func calendarProblems(query CalendarQuery) *ValidationError {
	if !query.From.Before(query.To) {
		return invalid(fieldTo, domain.FieldInvalid)
	}
	if query.To.Sub(query.From) > domain.CalendarMaxRange+2*domain.CalendarRangeMargin {
		return invalid(fieldTo, domain.FieldTooLong)
	}
	return nil
}

func (s *Service) Calendar(ctx context.Context, viewer domain.UserID, role domain.Role, query CalendarQuery) (Calendar, error) {
	if role == domain.RoleTutor && query.Tutor != nil {
		return Calendar{}, invalid(fieldTutor, domain.FieldNotAllowed)
	}
	if problem := calendarProblems(query); problem != nil {
		return Calendar{}, problem
	}
	var calendar Calendar
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		var err error
		if role == domain.RoleTutor {
			calendar, err = tutorCalendar(ctx, repos, viewer, query)
			return err
		}
		calendar, err = s.studentCalendar(ctx, repos, viewer, query)
		return err
	})
	return calendar, err
}

func truncate[Row any](rows []Row) ([]Row, bool) {
	if len(rows) > domain.CalendarItemLimit {
		return rows[:domain.CalendarItemLimit], true
	}
	return rows, false
}

func tutorCalendar(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, query CalendarQuery) (Calendar, error) {
	rows, err := repos.Lessons.TutorCalendar(ctx, tutor, query.From, query.To, domain.CalendarItemLimit+1)
	if err != nil {
		return Calendar{}, err
	}
	rows, truncated := truncate(rows)
	calendar := Calendar{Items: make([]CalendarItem, 0, len(rows)), Truncated: truncated}
	for _, row := range rows {
		calendar.Items = append(calendar.Items, tutorItem(row))
	}
	return calendar, nil
}

func tutorItem(row repositories.TutorCalendarItem) CalendarItem {
	source := tutorLabelSource{
		relationship: row.RelationshipID, group: row.GroupID, cardName: row.CardName, cardState: row.CardState, cardDiscardReason: row.CardDiscardReason,
		cardColour: row.CardColour, groupName: row.GroupName, groupColour: row.GroupColour,
	}
	audience := source.audience()
	audience.Members = nil
	return CalendarItem{
		ID: row.ID, Title: row.Title, StartsAt: row.StartsAt.UTC(), DurationMinutes: int(row.DurationMinutes), Status: row.Status, TookPlace: row.TookPlace,
		Timezone: row.Timezone, Audience: audience,
	}
}

func (s *Service) studentCalendar(ctx context.Context, repos repositories.Repositories, student domain.UserID, query CalendarQuery) (Calendar, error) {
	var tutor *domain.UserID
	if query.Tutor != nil {
		now, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
		if err != nil {
			return Calendar{}, err
		}
		listed, err := repos.Lessons.ListedRelationshipTutor(ctx, student, *query.Tutor, now)
		if errors.Is(err, repositories.ErrNotFound) {
			return Calendar{}, ErrNotFound
		}
		if err != nil {
			return Calendar{}, err
		}
		tutor = &listed
	}
	rows, err := repos.Lessons.StudentCalendar(ctx, student, tutor, query.From, query.To, domain.CalendarItemLimit+1)
	if err != nil {
		return Calendar{}, err
	}
	rows, truncated := truncate(rows)
	calendar := Calendar{Items: make([]CalendarItem, 0, len(rows)), Truncated: truncated}
	for _, row := range rows {
		source := studentLabelSource{
			group: row.GroupID, groupName: row.GroupName, currentMember: row.CurrentMember, tutorName: row.TutorName,
			myState: row.MyState, myColour: row.MyColour,
		}
		audience, label := source.labels()
		calendar.Items = append(calendar.Items, CalendarItem{
			ID: row.ID, Title: row.Title, StartsAt: row.StartsAt.UTC(), DurationMinutes: int(row.DurationMinutes), Status: row.Status, TookPlace: row.TookPlace,
			Timezone: row.Timezone, Audience: audience, Tutor: label,
		})
	}
	return calendar, nil
}
