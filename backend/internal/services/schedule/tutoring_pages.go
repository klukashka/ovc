package schedule

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	fieldCursor     = "query.cursor"
	cursorSeparator = "."
)

var (
	earliestCursorMicros = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMicro()
	latestCursorMicros   = time.Date(9999, time.December, 31, 23, 59, 59, 999999000, time.UTC).UnixMicro()
)

func cardInScope(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, card domain.RelationshipID) error {
	_, err := repos.Relationships.ForTutor(ctx, tutor, card)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func groupInScope(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, group domain.GroupID) error {
	_, err := repos.Groups.ForTutor(ctx, tutor, group)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

type HistoryQuery struct {
	Cursor string
	Limit  int32
}

type LessonPage struct {
	Items      []CalendarItem
	NextCursor *string
}

type GroupLessons struct {
	Next              []CalendarItem
	FutureLessonCount int64
	HasOpenSeries     bool
}

func tutorItems(rows []repositories.TutorCalendarItem, now time.Time) []CalendarItem {
	items := make([]CalendarItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, tutorItem(row, now))
	}
	return items
}

func (s *Service) CardNextLessons(ctx context.Context, tutor domain.UserID, card domain.RelationshipID) ([]CalendarItem, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return nil, err
	}
	var items []CalendarItem
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		if err := cardInScope(ctx, repos, tutor, card); err != nil {
			return err
		}
		rows, err := repos.Lessons.CardNext(ctx, tutor, card, now, domain.CardNextLessonCount)
		items = tutorItems(rows, now)
		return err
	})
	return items, err
}

func (s *Service) CardHistory(ctx context.Context, tutor domain.UserID, card domain.RelationshipID, query HistoryQuery) (LessonPage, error) {
	after, err := parseLessonCursor(query.Cursor)
	if err != nil {
		return LessonPage{}, err
	}
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return LessonPage{}, err
	}
	page := LessonPage{Items: []CalendarItem{}}
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		if err := cardInScope(ctx, repos, tutor, card); err != nil {
			return err
		}
		rows, err := repos.Lessons.CardHistory(ctx, tutor, card, after, query.Limit+1)
		if err != nil {
			return err
		}
		if len(rows) > int(query.Limit) {
			rows = rows[:query.Limit]
			last := rows[len(rows)-1]
			next := encodeLessonCursor(last.StartsAt, last.ID)
			page.NextCursor = &next
		}
		page.Items = tutorItems(rows, now)
		return nil
	})
	return page, err
}

func (s *Service) CardsNextLesson(ctx context.Context, tutor domain.UserID, cards []domain.RelationshipID) (map[domain.RelationshipID]CalendarItem, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return nil, err
	}
	next := map[domain.RelationshipID]CalendarItem{}
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		rows, err := repos.Lessons.CardsNext(ctx, tutor, cards, now)
		for _, row := range rows {
			next[row.Card] = tutorItem(row.Lesson, now)
		}
		return err
	})
	return next, err
}

func (s *Service) GroupLessons(ctx context.Context, tutor domain.UserID, group domain.GroupID) (GroupLessons, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return GroupLessons{}, err
	}
	var lessons GroupLessons
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		if err := groupInScope(ctx, repos, tutor, group); err != nil {
			return err
		}
		rows, err := repos.Lessons.GroupNext(ctx, tutor, group, now, domain.GroupNextLessonCount)
		if err != nil {
			return err
		}
		lessons.Next = tutorItems(rows, now)
		if lessons.FutureLessonCount, err = repos.Lessons.CountFutureOfGroup(ctx, tutor, group, now); err != nil {
			return err
		}
		lessons.HasOpenSeries, err = repos.Lessons.GroupHasOpenSeries(ctx, tutor, group, now)
		return err
	})
	return lessons, err
}

func (s *Service) GroupsNextLesson(ctx context.Context, tutor domain.UserID, groups []domain.GroupID) (map[domain.GroupID]CalendarItem, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return nil, err
	}
	next := map[domain.GroupID]CalendarItem{}
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		rows, err := repos.Lessons.GroupsNext(ctx, tutor, groups, now)
		for _, row := range rows {
			next[row.Group] = tutorItem(row.Lesson, now)
		}
		return err
	})
	return next, err
}

func encodeLessonCursor(startsAt time.Time, id domain.LessonID) string {
	text := strconv.FormatInt(startsAt.UnixMicro(), 10) + cursorSeparator + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

func parseLessonCursor(text string) (*repositories.LessonCursor, error) {
	if text == "" {
		return nil, nil
	}
	problem := invalid(fieldCursor, domain.FieldInvalid)
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil {
		return nil, problem
	}
	micros, idText, found := strings.Cut(string(decoded), cursorSeparator)
	if !found {
		return nil, problem
	}
	instant, err := strconv.ParseInt(micros, 10, 64)
	if err != nil || instant < earliestCursorMicros || instant > latestCursorMicros {
		return nil, problem
	}
	id, valid := domain.ParseUUID[domain.LessonID](idText)
	if !valid {
		return nil, problem
	}
	return &repositories.LessonCursor{StartsAt: time.UnixMicro(instant).UTC(), ID: id}, nil
}
