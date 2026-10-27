package schedule

import (
	"context"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/domain/schedule"
	"ovc/backend/internal/repositories"
)

func minutes(count int) time.Duration {
	return time.Duration(count) * time.Minute
}

func checkOverlap(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, durationMinutes int, occurrences []schedule.Occurrence) error {
	if len(occurrences) == 0 {
		return nil
	}
	candidates := make([]schedule.Interval, 0, len(occurrences))
	for _, occurrence := range occurrences {
		candidates = append(candidates, schedule.Interval{Start: occurrence.StartsAt, End: occurrence.StartsAt.Add(minutes(durationMinutes))})
	}
	from, to := candidates[0].Start, candidates[0].End
	for _, candidate := range candidates {
		from, to = minTime(from, candidate.Start), maxTime(to, candidate.End)
	}
	existing, err := existingIntervals(ctx, repos, tutor, from, to)
	if err != nil {
		return err
	}
	found := schedule.Overlapping(candidates, existing)
	if len(found) == 0 {
		return nil
	}
	refusal := &OverlapError{}
	for index, interval := range found {
		if index == domain.OverlapListLimit {
			refusal.More = len(found) - domain.OverlapListLimit
			break
		}
		refusal.Lessons = append(refusal.Lessons, Conflict{Title: interval.Title, StartsAt: interval.Start.UTC(), EndsAt: interval.End.UTC()})
	}
	return refusal
}

func minTime(a time.Time, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func maxTime(a time.Time, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func existingIntervals(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, from time.Time, to time.Time) ([]schedule.Interval, error) {
	lessons, err := repos.Lessons.ForOverlap(ctx, tutor, from, to)
	if err != nil {
		return nil, err
	}
	intervals := make([]schedule.Interval, 0, len(lessons))
	for _, lesson := range lessons {
		start := lesson.StartsAt.UTC()
		intervals = append(intervals, schedule.Interval{Start: start, End: start.Add(minutes(int(lesson.DurationMinutes))), Title: lesson.Title})
	}
	firstDate := domain.DateOf(from.UTC()).AddDays(-1)
	lastDate := domain.DateOf(to.UTC()).AddDays(1)
	beyond, err := repos.Lessons.SeriesBeyondHorizon(ctx, tutor, firstDate, lastDate)
	if err != nil {
		return nil, err
	}
	for _, series := range beyond {
		zone, err := loadZone(series.Timezone)
		if err != nil {
			return nil, err
		}
		end := series.EndDate
		rule := schedule.Series{Weekdays: series.Weekdays, First: series.First, EndDate: &end, Start: series.Start, Zone: zone}
		for _, occurrence := range schedule.DatesBetween(rule, maxDate(series.MaterialisedUntil, firstDate.AddDays(-1)), minDate(end, lastDate)) {
			intervals = append(intervals, schedule.Interval{
				Start: occurrence.StartsAt, End: occurrence.StartsAt.Add(minutes(int(series.DurationMinutes))), Title: series.Title,
			})
		}
	}
	return intervals, nil
}

func maxDate(a domain.LocalDate, b domain.LocalDate) domain.LocalDate {
	if b.After(a) {
		return b
	}
	return a
}

func minDate(a domain.LocalDate, b domain.LocalDate) domain.LocalDate {
	if b.Before(a) {
		return b
	}
	return a
}
