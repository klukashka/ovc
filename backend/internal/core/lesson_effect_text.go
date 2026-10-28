package core

import (
	"fmt"

	"ovc/backend/internal/domain"
)

type CancelledLessonsText struct {
	Heading    string
	moreFormat string
}

func (t CancelledLessonsText) More(count int) string {
	return fmt.Sprintf(t.moreFormat, count)
}

var cancelledLessonsTexts = map[domain.Language]CancelledLessonsText{
	domain.LanguageEnglish: {Heading: "These lessons were cancelled:", moreFormat: "and %d more"},
	domain.LanguageRussian: {Heading: "Эти уроки отменены:", moreFormat: "и ещё %d"},
}

func CancelledLessonsEmailText(language domain.Language) (CancelledLessonsText, bool) {
	text, found := cancelledLessonsTexts[language]
	return text, found
}

type GroupArchivedText struct {
	SubjectPrefix   string
	Intro           string
	Tutor           string
	GroupLesson     string
	Action          string
	Note            string
	Unsubscribe     string
	UnsubscribeNote string
}

var groupArchivedTexts = map[domain.Language]GroupArchivedText{
	domain.LanguageEnglish: {
		SubjectPrefix:   "Group archived",
		Intro:           "Your tutor has archived a group you belong to. Its future lessons are cancelled.",
		Tutor:           "Tutor",
		GroupLesson:     "Group lesson",
		Action:          "Open your schedule",
		Note:            "Your other lessons are not affected.",
		Unsubscribe:     "Turn off these emails",
		UnsubscribeNote: "You receive this email because lesson change emails are turned on in your OVC email settings.",
	},
	domain.LanguageRussian: {
		SubjectPrefix:   "Группа в архиве",
		Intro:           "Преподаватель перенёс в архив группу, в которой вы занимаетесь. Её будущие уроки отменены.",
		Tutor:           "Преподаватель",
		GroupLesson:     "Групповой урок",
		Action:          "Открыть расписание",
		Note:            "Ваши другие уроки не затронуты.",
		Unsubscribe:     "Отключить эти письма",
		UnsubscribeNote: "Вы получили это письмо, потому что в настройках писем OVC включены письма об изменениях уроков.",
	},
}

func GroupArchivedEmailText(language domain.Language) (GroupArchivedText, bool) {
	text, found := groupArchivedTexts[language]
	return text, found
}
