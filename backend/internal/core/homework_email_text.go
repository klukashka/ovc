package core

import "ovc/backend/internal/domain"

type HomeworkEmailText struct {
	Subject         string
	Intro           string
	Tutor           string
	Deadline        string
	Action          string
	Note            string
	Unsubscribe     string
	UnsubscribeNote string
}

var issueCreatedCatalogue = map[domain.Language]HomeworkEmailText{
	domain.LanguageEnglish: {
		Subject:         "New assignment",
		Intro:           "Your tutor gave you a new assignment.",
		Tutor:           "Tutor",
		Deadline:        "Due",
		Action:          "Open the assignment",
		Note:            "The questions are on the assignment's page in OVC.",
		Unsubscribe:     "Turn off these emails",
		UnsubscribeNote: "You receive this email because homework emails are turned on in your OVC email settings.",
	},
	domain.LanguageRussian: {
		Subject:         "Новое задание",
		Intro:           "Преподаватель выдал вам новое задание.",
		Tutor:           "Преподаватель",
		Deadline:        "Срок",
		Action:          "Открыть задание",
		Note:            "Вопросы — на странице задания в OVC.",
		Unsubscribe:     "Отключить эти письма",
		UnsubscribeNote: "Вы получили это письмо, потому что в настройках писем OVC включены письма о домашних заданиях.",
	},
}

func IssueCreatedText(language domain.Language) (HomeworkEmailText, bool) {
	text, found := issueCreatedCatalogue[language]
	return text, found
}
