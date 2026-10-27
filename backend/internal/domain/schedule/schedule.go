package schedule

import (
	"slices"
	"sort"
	"time"

	"ovc/backend/internal/domain"
)

const transitionSearch = 48 * time.Hour

func StartInstant(date domain.LocalDate, at domain.LocalTime, zone *time.Location) time.Time {
	wall := date.Midnight().Add(at.SinceMidnight())
	offsetBefore := offsetAt(wall.Add(-transitionSearch), zone)
	offsetAfter := offsetAt(wall.Add(transitionSearch), zone)
	var valid []time.Time
	for _, offset := range []time.Duration{offsetBefore, offsetAfter} {
		candidate := wall.Add(-offset)
		if offsetAt(candidate, zone) == offset && !slices.ContainsFunc(valid, candidate.Equal) {
			valid = append(valid, candidate)
		}
	}
	if len(valid) == 0 {
		return wall.Add(-offsetBefore).UTC()
	}
	slices.SortFunc(valid, time.Time.Compare)
	return valid[0].UTC()
}

func offsetAt(instant time.Time, zone *time.Location) time.Duration {
	_, seconds := instant.In(zone).Zone()
	return time.Duration(seconds) * time.Second
}

type Series struct {
	Weekdays []domain.Weekday
	First    domain.LocalDate
	EndDate  *domain.LocalDate
	Count    int
	Start    domain.LocalTime
	Zone     *time.Location
}

type Occurrence struct {
	Date     domain.LocalDate
	StartsAt time.Time
}

type Problem string

const (
	ProblemWeekdays Problem = "weekdays"
	ProblemEndDate  Problem = "end_date"
	ProblemCount    Problem = "count"
)

func (s Series) Problems() []Problem {
	var problems []Problem
	if len(s.Weekdays) == 0 || slices.ContainsFunc(s.Weekdays, func(day domain.Weekday) bool { return !day.Valid() }) {
		problems = append(problems, ProblemWeekdays)
	}
	limit := s.First.AddMonths(domain.SeriesMaxMonths)
	if s.EndDate != nil && (s.EndDate.Before(s.First) || s.EndDate.After(limit)) {
		problems = append(problems, ProblemEndDate)
	}
	if s.EndDate == nil && (s.Count < 1 || s.Count > domain.SeriesMaxCount) {
		problems = append(problems, ProblemCount)
	}
	return problems
}

func (s Series) lastAllowed() domain.LocalDate {
	if s.EndDate != nil {
		return *s.EndDate
	}
	return s.First.AddMonths(domain.SeriesMaxMonths)
}

func (s Series) Dates() []domain.LocalDate {
	var dates []domain.LocalDate
	last := s.lastAllowed()
	for date := s.First; !date.After(last); date = date.AddDays(1) {
		if slices.Contains(s.Weekdays, date.Weekday()) {
			dates = append(dates, date)
		}
	}
	return dates
}

type Plan struct {
	Occurrences []Occurrence
	Skipped     int
	CountFits   bool
}

func (s Series) Future(now time.Time) Plan {
	plan := Plan{CountFits: true}
	for _, date := range s.Dates() {
		start := StartInstant(date, s.Start, s.Zone)
		if !start.After(now) {
			plan.Skipped++
			continue
		}
		plan.Occurrences = append(plan.Occurrences, Occurrence{Date: date, StartsAt: start})
		if s.EndDate == nil && len(plan.Occurrences) == s.Count {
			return plan
		}
	}
	plan.CountFits = s.EndDate != nil
	return plan
}

func (s Series) Horizon(now time.Time) (domain.LocalDate, bool) {
	horizon := domain.DateOf(now.Add(domain.SeriesHorizon).In(s.Zone))
	if s.EndDate == nil || !s.EndDate.After(horizon) {
		return s.lastAllowed(), true
	}
	return horizon, false
}

func Materialised(plan Plan, through domain.LocalDate) []Occurrence {
	var kept []Occurrence
	for _, occurrence := range plan.Occurrences {
		if !occurrence.Date.After(through) {
			kept = append(kept, occurrence)
		}
	}
	return kept
}

func DatesBetween(s Series, after domain.LocalDate, through domain.LocalDate) []Occurrence {
	var occurrences []Occurrence
	for _, date := range s.Dates() {
		if date.After(after) && !date.After(through) {
			occurrences = append(occurrences, Occurrence{Date: date, StartsAt: StartInstant(date, s.Start, s.Zone)})
		}
	}
	return occurrences
}

type Interval struct {
	Start     time.Time
	End       time.Time
	Title     string
	Cancelled bool
}

func Overlapping(candidates []Interval, existing []Interval) []Interval {
	active := slices.DeleteFunc(slices.Clone(candidates), func(interval Interval) bool { return interval.Cancelled })
	slices.SortFunc(active, func(a, b Interval) int { return a.Start.Compare(b.Start) })
	latestEnd := make([]time.Time, len(active))
	for index, interval := range active {
		latestEnd[index] = interval.End
		if index > 0 && latestEnd[index-1].After(interval.End) {
			latestEnd[index] = latestEnd[index-1]
		}
	}
	var found []Interval
	for _, other := range existing {
		if other.Cancelled {
			continue
		}
		before := sort.Search(len(active), func(index int) bool { return !active[index].Start.Before(other.End) })
		if before > 0 && latestEnd[before-1].After(other.Start) {
			found = append(found, other)
		}
	}
	slices.SortStableFunc(found, func(a, b Interval) int { return a.Start.Compare(b.Start) })
	return found
}
