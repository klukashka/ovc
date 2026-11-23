package core

import (
	"fmt"

	"ovc/backend/internal/domain"
)

type lessonChangeWords struct {
	subjectPrefix string
	intro         string
}

type lessonChangeCatalogue struct {
	changes         map[domain.OutboxKind]lessonChangeWords
	unsubscribeNote string
	cancelled       string
	more            string
}

var lessonChangeTexts = map[domain.Language]lessonChangeCatalogue{
	domain.LanguageEnglish: {
		changes: map[domain.OutboxKind]lessonChangeWords{
			domain.EventLessonMoved:         {subjectPrefix: "Lesson moved", intro: "Your lesson has been moved to a new time."},
			domain.EventLessonCancelled:     {subjectPrefix: "Lesson cancelled", intro: "Your lesson has been cancelled."},
			domain.EventLessonSeriesChanged: {subjectPrefix: "Lessons changed", intro: "Your tutor changed these lessons of a series:"},
		},
		unsubscribeNote: "You receive this email because lesson change emails are turned on in your OVC email settings.",
		cancelled:       "cancelled",
		more:            "and %d more",
	},
	domain.LanguageRussian: {
		changes: map[domain.OutboxKind]lessonChangeWords{
			domain.EventLessonMoved:         {subjectPrefix: "Урок перенесён", intro: "Ваш урок перенесён на новое время."},
			domain.EventLessonCancelled:     {subjectPrefix: "Урок отменён", intro: "Ваш урок отменён."},
			domain.EventLessonSeriesChanged: {subjectPrefix: "Уроки изменены", intro: "Преподаватель изменил эти уроки серии:"},
		},
		unsubscribeNote: "Вы получили это письмо, потому что в настройках писем OVC включены письма об изменениях уроков.",
		cancelled:       "отменён",
		more:            "и ещё %d",
	},
}

type LessonChangeText struct {
	LessonEmailText
	Cancelled  string
	moreFormat string
}

func (t LessonChangeText) More(count int) string {
	return fmt.Sprintf(t.moreFormat, count)
}

func LessonChangeEmailText(language domain.Language, kind domain.OutboxKind) (LessonChangeText, bool) {
	words, found := lessonEmailCatalogue[language]
	changes, known := lessonChangeTexts[language]
	if !found || !known {
		return LessonChangeText{}, false
	}
	change, defined := changes.changes[kind]
	if !defined {
		return LessonChangeText{}, false
	}
	return LessonChangeText{
		LessonEmailText: LessonEmailText{
			SubjectPrefix: change.subjectPrefix, Intro: change.intro, When: words.when, Lesson: words.lesson, Group: words.group,
			Student: words.student, Tutor: words.tutor, Individual: words.individual, GroupLesson: words.groupLesson,
			RemovedStudent: words.removedStudent, FormerStudent: words.formerStudent, Action: words.action, Note: words.note, Unsubscribe: words.unsubscribe,
			UnsubscribeNote: changes.unsubscribeNote,
		},
		Cancelled: changes.cancelled, moreFormat: changes.more,
	}, true
}
