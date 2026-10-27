package schedule_test

import (
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/domain/schedule"
)

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func date(year int, month time.Month, day int) domain.LocalDate {
	return domain.LocalDate{Year: year, Month: month, Day: day}
}

func at(hour int, minute int) domain.LocalTime {
	return domain.LocalTime{Hour: hour, Minute: minute}
}

func utc(text string) time.Time {
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		panic(err)
	}
	return parsed
}

type startCase struct {
	zone  string
	date  domain.LocalDate
	time  domain.LocalTime
	want  string
	local string
}

func assertStarts(t *testing.T, cases map[string]startCase) {
	t.Helper()
	for name, testCase := range cases {
		location := zone(t, testCase.zone)

		got := schedule.StartInstant(testCase.date, testCase.time, location)

		if !got.Equal(utc(testCase.want)) || got.In(location).Format("15:04") != testCase.local {
			t.Errorf("%s: got %s (%s local), want %s (%s local)", name, got.Format(time.RFC3339), got.In(location).Format("15:04"), testCase.want, testCase.local)
		}
	}
}

func TestOccurrenceDatesFollowWeekdaysFirstDateAndEnd(t *testing.T) {
	t.Parallel()
	end := date(2030, time.March, 21)
	series := schedule.Series{
		Weekdays: []domain.Weekday{2, 4}, First: date(2030, time.March, 6), EndDate: &end, Start: at(18, 0), Zone: zone(t, "Europe/Moscow"),
	}

	dates := series.Dates()

	want := []domain.LocalDate{date(2030, time.March, 7), date(2030, time.March, 12), date(2030, time.March, 14), date(2030, time.March, 19), date(2030, time.March, 21)}
	if !slices.Equal(dates, want) {
		t.Fatalf("dates %v, want %v", dates, want)
	}
}

func TestCountedSeriesSkipPastOccurrencesWithoutCountingThem(t *testing.T) {
	t.Parallel()
	series := schedule.Series{Weekdays: []domain.Weekday{1, 3}, First: date(2030, time.March, 4), Count: 3, Start: at(10, 0), Zone: zone(t, "Europe/Moscow")}
	now := utc("2030-03-06T08:00:00Z")

	plan := series.Future(now)

	var dates []domain.LocalDate
	for _, occurrence := range plan.Occurrences {
		dates = append(dates, occurrence.Date)
	}
	want := []domain.LocalDate{date(2030, time.March, 11), date(2030, time.March, 13), date(2030, time.March, 18)}
	if plan.Skipped != 2 || !plan.CountFits || !slices.Equal(dates, want) {
		t.Fatalf("skipped %d, fits %t, dates %v, want 2 skipped and %v", plan.Skipped, plan.CountFits, dates, want)
	}
}

func TestAGapMovesTheStartForwardByTheGapLength(t *testing.T) {
	t.Parallel()
	assertStarts(t, map[string]startCase{
		"Berlin spring forward":   {zone: "Europe/Berlin", date: date(2030, time.March, 31), time: at(2, 30), want: "2030-03-31T01:30:00Z", local: "03:30"},
		"New York spring forward": {zone: "America/New_York", date: date(2030, time.March, 10), time: at(2, 30), want: "2030-03-10T07:30:00Z", local: "03:30"},
		"Berlin outside the gap":  {zone: "Europe/Berlin", date: date(2030, time.March, 31), time: at(3, 30), want: "2030-03-31T01:30:00Z", local: "03:30"},
	})
}

func TestARepeatedLocalTimeTakesTheEarlierInstant(t *testing.T) {
	t.Parallel()
	assertStarts(t, map[string]startCase{
		"Berlin fall back":   {zone: "Europe/Berlin", date: date(2030, time.October, 27), time: at(2, 30), want: "2030-10-27T00:30:00Z", local: "02:30"},
		"New York fall back": {zone: "America/New_York", date: date(2030, time.November, 3), time: at(1, 30), want: "2030-11-03T05:30:00Z", local: "01:30"},
	})
}

func TestAThirtyMinuteShiftAndAStandardOffsetChange(t *testing.T) {
	t.Parallel()
	assertStarts(t, map[string]startCase{
		"Lord Howe gap of 30 minutes":       {zone: "Australia/Lord_Howe", date: date(2030, time.October, 6), time: at(2, 15), want: "2030-10-05T15:45:00Z", local: "02:45"},
		"Lord Howe repeated half hour":      {zone: "Australia/Lord_Howe", date: date(2030, time.April, 7), time: at(1, 45), want: "2030-04-06T14:45:00Z", local: "01:45"},
		"Moscow standard offset change":     {zone: "Europe/Moscow", date: date(2014, time.October, 26), time: at(1, 30), want: "2014-10-25T21:30:00Z", local: "01:30"},
		"Moscow after the standard change":  {zone: "Europe/Moscow", date: date(2014, time.October, 27), time: at(18, 0), want: "2014-10-27T15:00:00Z", local: "18:00"},
		"Moscow before the standard change": {zone: "Europe/Moscow", date: date(2014, time.October, 24), time: at(18, 0), want: "2014-10-24T14:00:00Z", local: "18:00"},
	})
}

func TestSeriesLongerThanTwelveWeeksMaterialiseTheHorizonOnly(t *testing.T) {
	t.Parallel()
	location := zone(t, "Europe/Moscow")
	now := utc("2030-03-10T09:00:00Z")
	long := date(2031, time.March, 1)
	short := date(2030, time.May, 1)
	longSeries := schedule.Series{Weekdays: []domain.Weekday{1}, First: date(2030, time.March, 11), EndDate: &long, Start: at(18, 0), Zone: location}
	shortSeries := schedule.Series{Weekdays: []domain.Weekday{1}, First: date(2030, time.March, 11), EndDate: &short, Start: at(18, 0), Zone: location}

	longThrough, longComplete := longSeries.Horizon(now)
	shortThrough, shortComplete := shortSeries.Horizon(now)
	materialised := schedule.Materialised(longSeries.Future(now), longThrough)

	if longComplete || longThrough != date(2030, time.June, 2) {
		t.Fatalf("the long series is materialised through %s (complete %t)", longThrough, longComplete)
	}
	if !shortComplete || shortThrough != short {
		t.Fatalf("the short series is materialised through %s (complete %t)", shortThrough, shortComplete)
	}
	if len(materialised) != 12 || materialised[len(materialised)-1].Date != date(2030, time.May, 27) {
		t.Fatalf("the long series materialises %d occurrences: %v", len(materialised), materialised)
	}
}

func TestOverlapSweepIgnoresTouchingAndCancelledLessons(t *testing.T) {
	t.Parallel()
	candidates := []schedule.Interval{
		{Start: utc("2030-03-11T15:00:00Z"), End: utc("2030-03-11T16:00:00Z")},
		{Start: utc("2030-03-12T15:00:00Z"), End: utc("2030-03-12T16:00:00Z")},
		{Start: utc("2030-03-13T15:00:00Z"), End: utc("2030-03-13T16:00:00Z"), Cancelled: true},
	}
	existing := []schedule.Interval{
		{Start: utc("2030-03-12T15:30:00Z"), End: utc("2030-03-12T16:30:00Z"), Title: "inside"},
		{Start: utc("2030-03-11T16:00:00Z"), End: utc("2030-03-11T17:00:00Z"), Title: "touching after"},
		{Start: utc("2030-03-11T14:00:00Z"), End: utc("2030-03-11T15:00:00Z"), Title: "touching before"},
		{Start: utc("2030-03-11T14:30:00Z"), End: utc("2030-03-11T17:30:00Z"), Title: "covering"},
		{Start: utc("2030-03-12T15:10:00Z"), End: utc("2030-03-12T15:20:00Z"), Title: "cancelled", Cancelled: true},
		{Start: utc("2030-03-13T15:10:00Z"), End: utc("2030-03-13T15:20:00Z"), Title: "against a cancelled candidate"},
	}

	found := schedule.Overlapping(candidates, existing)

	var titles []string
	for _, interval := range found {
		titles = append(titles, interval.Title)
	}
	if !slices.Equal(titles, []string{"covering", "inside"}) {
		t.Fatalf("overlaps %v", titles)
	}
}
