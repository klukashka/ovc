package core

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"ovc/backend/internal/domain"
)

type LessonEmailText struct {
	SubjectPrefix   string
	Intro           string
	When            string
	Lesson          string
	Group           string
	Student         string
	Tutor           string
	Individual      string
	GroupLesson     string
	RemovedStudent  string
	FormerStudent   string
	Action          string
	Note            string
	Unsubscribe     string
	UnsubscribeNote string
}

type lessonEmailWords struct {
	hours           PluralForms
	subjectPrefix   string
	intro           string
	when            string
	lesson          string
	group           string
	student         string
	tutor           string
	individual      string
	groupLesson     string
	removedStudent  string
	formerStudent   string
	action          string
	note            string
	unsubscribe     string
	unsubscribeNote string
}

var lessonEmailCatalogue = map[domain.Language]lessonEmailWords{
	domain.LanguageEnglish: {
		hours:           PluralForms{PluralOne: "hour", PluralOther: "hours"},
		subjectPrefix:   "Lesson in %d %s",
		intro:           "Your lesson starts in %d %s.",
		when:            "When",
		lesson:          "Lesson",
		group:           "Group",
		student:         "Student",
		tutor:           "Tutor",
		individual:      "Individual lesson",
		groupLesson:     "Group lesson",
		removedStudent:  "Removed student",
		formerStudent:   "Former student",
		action:          "Open the lesson",
		note:            "The lesson's description is on its page in OVC.",
		unsubscribe:     "Turn off these emails",
		unsubscribeNote: "You receive this email because lesson reminders are turned on in your OVC email settings.",
	},
	domain.LanguageRussian: {
		hours:           PluralForms{PluralOne: "час", PluralFew: "часа", PluralMany: "часов", PluralOther: "часа"},
		subjectPrefix:   "Урок через %d %s",
		intro:           "Урок начнётся через %d %s.",
		when:            "Когда",
		lesson:          "Урок",
		group:           "Группа",
		student:         "Ученик",
		tutor:           "Преподаватель",
		individual:      "Индивидуальный урок",
		groupLesson:     "Групповой урок",
		removedStudent:  "Удалённый ученик",
		formerStudent:   "Бывший ученик",
		action:          "Открыть урок",
		note:            "Описание урока — на его странице в OVC.",
		unsubscribe:     "Отключить эти письма",
		unsubscribeNote: "Вы получили это письмо, потому что в настройках писем OVC включены напоминания об уроках.",
	},
}

func LessonReminderText(language domain.Language, before time.Duration) (LessonEmailText, bool) {
	words, found := lessonEmailCatalogue[language]
	if !found {
		return LessonEmailText{}, false
	}
	hours := int(before / time.Hour)
	unit := words.hours.For(language, hours)
	return LessonEmailText{
		SubjectPrefix: fmt.Sprintf(words.subjectPrefix, hours, unit), Intro: fmt.Sprintf(words.intro, hours, unit),
		When: words.when, Lesson: words.lesson, Group: words.group, Student: words.student, Tutor: words.tutor,
		Individual: words.individual, GroupLesson: words.groupLesson, RemovedStudent: words.removedStudent,
		FormerStudent: words.formerStudent, Action: words.action, Note: words.note, Unsubscribe: words.unsubscribe, UnsubscribeNote: words.unsubscribeNote,
	}, true
}

type ZoneLabel struct {
	City   string
	Offset string
}

var (
	englishWeekdays = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	russianWeekdays = [...]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}
	englishMonths   = [...]string{
		"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December",
	}
	russianGenitiveMonths = [...]string{
		"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря",
	}
)

func LessonStartText(language domain.Language, at time.Time, zone *time.Location, label *ZoneLabel) string {
	local := at.In(zone)
	clock := local.Format("15:04")
	if language == domain.LanguageRussian {
		text := fmt.Sprintf("%s, %d %s %d, %s", russianWeekdays[local.Weekday()], local.Day(), russianGenitiveMonths[local.Month()-1], local.Year(), clock)
		return text + russianZoneSuffix(label)
	}
	text := fmt.Sprintf("%s, %d %s %d, %s", englishWeekdays[local.Weekday()], local.Day(), englishMonths[local.Month()-1], local.Year(), clock)
	return text + englishZoneSuffix(label)
}

func englishZoneSuffix(label *ZoneLabel) string {
	switch {
	case label == nil:
		return ""
	case label.City != "":
		return " " + label.City + " time"
	}
	return " " + label.Offset
}

func russianZoneSuffix(label *ZoneLabel) string {
	switch {
	case label == nil:
		return ""
	case label.City != "":
		return " по времени " + label.City
	}
	return " по " + label.Offset
}

var invisibleCharacters = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x034f, Hi: 0x034f, Stride: 1},
		{Lo: 0x115f, Hi: 0x1160, Stride: 1},
		{Lo: 0x17b4, Hi: 0x17b5, Stride: 1},
		{Lo: 0x180b, Hi: 0x180f, Stride: 1},
		{Lo: 0x2060, Hi: 0x206f, Stride: 1},
		{Lo: 0x2800, Hi: 0x2800, Stride: 1},
		{Lo: 0x3164, Hi: 0x3164, Stride: 1},
		{Lo: 0xfe00, Hi: 0xfe0e, Stride: 1},
		{Lo: 0xffa0, Hi: 0xffa0, Stride: 1},
		{Lo: 0xfff0, Hi: 0xfff8, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0xe0000, Hi: 0xe0fff, Stride: 1},
	},
}

func EmailSafeText(text string) string {
	kept := strings.Map(func(character rune) rune {
		if invisibleFormatting(character) {
			return -1
		}
		return character
	}, text)
	return strings.Join(strings.Fields(kept), " ")
}

func invisibleFormatting(character rune) bool {
	return unicode.Is(unicode.Cf, character) || unicode.IsControl(character) || unicode.Is(invisibleCharacters, character)
}
