package domain

import (
	"fmt"
	"time"
)

type LessonID [16]byte

type SeriesID [16]byte

func (id LessonID) String() string {
	return formatUUID(id)
}

func (id SeriesID) String() string {
	return formatUUID(id)
}

type LessonStatus string

const (
	LessonScheduled LessonStatus = "scheduled"
	LessonLive      LessonStatus = "live"
	LessonEnded     LessonStatus = "ended"
	LessonCancelled LessonStatus = "cancelled"
)

type CancelCause string

const (
	CancelByTutor            CancelCause = "tutor"
	CancelRelationshipEnded  CancelCause = "relationship_ended"
	CancelProvisionalRemoved CancelCause = "provisional_removed"
	CancelGroupArchived      CancelCause = "group_archived"
	CancelTutorDeleted       CancelCause = "tutor_deleted"
)

type LessonField string

const (
	LessonFieldStart       LessonField = "start"
	LessonFieldDate        LessonField = "date"
	LessonFieldDuration    LessonField = "duration"
	LessonFieldTitle       LessonField = "title"
	LessonFieldDescription LessonField = "description"
	LessonFieldMaterials   LessonField = "materials"
)

type AudienceKind string

const (
	AudienceOneToOne AudienceKind = "one_to_one"
	AudienceGroup    AudienceKind = "group"
)

type LabelKind string

const (
	LabelIndividual     LabelKind = "individual"
	LabelGroup          LabelKind = "group"
	LabelRemovedStudent LabelKind = "removed_student"
	LabelDeletedAccount LabelKind = "deleted_account"
)

const (
	LessonTitleMaxRunes       = 120
	LessonDescriptionMaxRunes = 2000
	LessonMinDuration         = 15
	LessonMaxDuration         = 240
	LessonDurationStep        = 5
	SeriesMaxCount            = 100
	SeriesMaxMonths           = 18
	SeriesHorizon             = 12 * 7 * 24 * time.Hour
	OpenSeriesCapPerTutor     = 100
	FutureLessonCapPerTutor   = 3000
	LessonWritesPerHour       = 120
	OverlapListLimit          = 20
	CalendarItemLimit         = 1000
	CardNextLessonCount       = 3
	GroupNextLessonCount      = 5
	CalendarMaxRange          = 62 * 24 * time.Hour
	CalendarRangeMargin       = 24 * time.Hour
	LessonMaxDurationSpan     = LessonMaxDuration * time.Minute
	PurposeLessonWrites       = RateLimitPurpose("lesson_writes_hour")
	LessonWriteWindow         = time.Hour
	MaxISOWeekday             = 7
)

type Weekday int16

func (w Weekday) Valid() bool {
	return w >= 1 && w <= MaxISOWeekday
}

var isoWeekdays = [...]Weekday{7, 1, 2, 3, 4, 5, 6}

func ISOWeekday(day time.Weekday) Weekday {
	return isoWeekdays[day]
}

type LocalDate struct {
	Year  int
	Month time.Month
	Day   int
}

func DateOf(instant time.Time) LocalDate {
	year, month, day := instant.Date()
	return LocalDate{Year: year, Month: month, Day: day}
}

func ParseLocalDate(text string) (LocalDate, bool) {
	parsed, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return LocalDate{}, false
	}
	return DateOf(parsed), true
}

func (d LocalDate) civil() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

func (d LocalDate) String() string {
	return d.civil().Format(time.DateOnly)
}

func (d LocalDate) AddDays(days int) LocalDate {
	return DateOf(d.civil().AddDate(0, 0, days))
}

func (d LocalDate) AddMonths(months int) LocalDate {
	return DateOf(d.civil().AddDate(0, months, 0))
}

func (d LocalDate) Weekday() Weekday {
	return ISOWeekday(d.civil().Weekday())
}

func (d LocalDate) Compare(other LocalDate) int {
	return d.civil().Compare(other.civil())
}

func (d LocalDate) After(other LocalDate) bool {
	return d.Compare(other) > 0
}

func (d LocalDate) Before(other LocalDate) bool {
	return d.Compare(other) < 0
}

func (d LocalDate) Midnight() time.Time {
	return d.civil()
}

type LocalTime struct {
	Hour   int
	Minute int
}

func ParseLocalTime(text string) (LocalTime, bool) {
	parsed, err := time.Parse("15:04", text)
	if err != nil {
		return LocalTime{}, false
	}
	return LocalTime{Hour: parsed.Hour(), Minute: parsed.Minute()}, true
}

func (t LocalTime) String() string {
	return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
}

func (t LocalTime) SinceMidnight() time.Duration {
	return time.Duration(t.Hour)*time.Hour + time.Duration(t.Minute)*time.Minute
}

func LocalTimeOf(sinceMidnight time.Duration) LocalTime {
	minutes := int(sinceMidnight / time.Minute)
	return LocalTime{Hour: minutes / 60, Minute: minutes % 60}
}
