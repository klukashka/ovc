package domain

import (
	"encoding/hex"
	"fmt"
	"time"
)

const (
	EventLessonReminder24h OutboxKind = "lesson.reminder_24h"
	EventLessonReminder1h  OutboxKind = "lesson.reminder_1h"
)

const (
	JobLessonStatus    JobName = "lessons.status"
	JobLessonReminders JobName = "lessons.reminders"
	JobSeriesExtension JobName = "series.extend"
)

const (
	LessonEndGrace         = 30 * time.Minute
	ReminderLateness       = 15 * time.Minute
	LessonEmailsPerDay     = 20
	LessonEmailWindow      = 24 * time.Hour
	PurposeLessonEmails    = RateLimitPurpose("lesson_emails")
	NotificationLessonLink = NotificationLinkKind("lesson")
)

type ReminderKind struct {
	Event     OutboxKind
	Offset    time.Duration
	DayBefore bool
	InApp     bool
}

func ReminderKinds() []ReminderKind {
	return []ReminderKind{
		{Event: EventLessonReminder1h, Offset: time.Hour, InApp: true},
		{Event: EventLessonReminder24h, Offset: 24 * time.Hour, DayBefore: true},
	}
}

func ReminderKindOf(event OutboxKind) (ReminderKind, bool) {
	for _, kind := range ReminderKinds() {
		if kind.Event == event {
			return kind, true
		}
	}
	return ReminderKind{}, false
}

func (k ReminderKind) Window(now time.Time) (time.Time, time.Time) {
	end := now.Add(k.Offset)
	return end.Add(-ReminderLateness), end
}

func (k ReminderKind) Late(startsAt time.Time, now time.Time) bool {
	return now.After(startsAt.Add(-k.Offset).Add(ReminderLateness))
}

func LessonReminderKey(event OutboxKind, lesson LessonID, recipient UserID, revision int32) string {
	return fmt.Sprintf("%s:%s:%s:%d", event, hex.EncodeToString(lesson[:]), hex.EncodeToString(recipient[:]), revision)
}

type LessonReminderPayload struct {
	Lesson    string `json:"lesson"`
	Recipient string `json:"recipient"`
	Revision  int32  `json:"revision"`
}

type EmailDropReason string

const (
	DropPreference    EmailDropReason = "preference"
	DropDailyCap      EmailDropReason = "daily_cap"
	DropTutorInactive EmailDropReason = "tutor_inactive"
)
