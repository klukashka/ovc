package schedule

import (
	"context"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	homeLookaheadDays = 63
	homeWeek          = 7 * 24 * time.Hour
)

type StudentLessons struct {
	Next          *CalendarItem
	TodayCount    int
	NextSevenDays []CalendarItem
}

type localDay struct {
	local time.Time
	start time.Time
	end   time.Time
	now   time.Time
}

func (s *Service) localDayOf(ctx context.Context, repos repositories.Repositories, viewer domain.UserID) (localDay, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return localDay{}, err
	}
	account, err := repos.Users.ByID(ctx, viewer)
	if err != nil {
		return localDay{}, err
	}
	zone, err := loadZone(account.Timezone)
	if err != nil {
		return localDay{}, err
	}
	local := now.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	return localDay{local: start, start: start.UTC(), end: start.AddDate(0, 0, 1).UTC(), now: now.UTC()}, nil
}

func lookaheadEnd(day localDay) time.Time {
	return day.local.AddDate(0, 0, homeLookaheadDays).UTC()
}

func (s *Service) TutorDay(ctx context.Context, tutor domain.UserID) ([]CalendarItem, error) {
	var items []CalendarItem
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		day, err := s.localDayOf(ctx, repos, tutor)
		if err != nil {
			return err
		}
		calendar, err := tutorCalendar(ctx, repos, tutor, CalendarQuery{From: day.start, To: day.end})
		if err != nil {
			return err
		}
		items = startingWithin(calendar.Items, day.start, day.end)
		return nil
	})
	return items, err
}

func (s *Service) TutorHasLessonAhead(ctx context.Context, tutor domain.UserID) (bool, error) {
	var found bool
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		day, err := s.localDayOf(ctx, repos, tutor)
		if err != nil {
			return err
		}
		found, err = repos.Lessons.TutorHasLessonStarting(ctx, tutor, day.start, lookaheadEnd(day))
		return err
	})
	return found, err
}

func (s *Service) StudentLessons(ctx context.Context, student domain.UserID, tutor *domain.RelationshipID) (StudentLessons, error) {
	lessons := StudentLessons{NextSevenDays: []CalendarItem{}}
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		day, err := s.localDayOf(ctx, repos, student)
		if err != nil {
			return err
		}
		calendar, err := studentCalendar(ctx, repos, student, CalendarQuery{From: day.start, To: lookaheadEnd(day), Tutor: tutor})
		if err != nil {
			return err
		}
		weekEnd := day.now.Add(homeWeek)
		for _, item := range calendar.Items {
			if item.Status == domain.LessonCancelled {
				continue
			}
			if !item.StartsAt.Before(day.start) && item.StartsAt.Before(day.end) {
				lessons.TodayCount++
			}
			if !item.EndsAt().After(day.now) {
				continue
			}
			if lessons.Next == nil {
				next := item
				lessons.Next = &next
			}
			if item.StartsAt.Before(weekEnd) {
				lessons.NextSevenDays = append(lessons.NextSevenDays, item)
			}
		}
		return nil
	})
	return lessons, err
}

func startingWithin(items []CalendarItem, from time.Time, to time.Time) []CalendarItem {
	within := make([]CalendarItem, 0, len(items))
	for _, item := range items {
		if !item.StartsAt.Before(from) && item.StartsAt.Before(to) {
			within = append(within, item)
		}
	}
	return within
}
