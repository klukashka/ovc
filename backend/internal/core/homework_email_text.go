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

var submittedCatalogue = map[domain.Language]HomeworkEmailText{
	domain.LanguageEnglish: {
		Subject:         "Work submitted",
		Intro:           "A student submitted work that needs checking.",
		Tutor:           "Student",
		Deadline:        "Due",
		Action:          "Check the work",
		Note:            "The answers are on the review page in OVC.",
		Unsubscribe:     "Turn off these emails",
		UnsubscribeNote: "You receive this email because emails about submitted work are turned on in your OVC email settings.",
	},
	domain.LanguageRussian: {
		Subject:         "Работа сдана",
		Intro:           "Ученик сдал работу, которую нужно проверить.",
		Tutor:           "Ученик",
		Deadline:        "Срок",
		Action:          "Проверить работу",
		Note:            "Ответы — на странице проверки в OVC.",
		Unsubscribe:     "Отключить эти письма",
		UnsubscribeNote: "Вы получили это письмо, потому что в настройках писем OVC включены письма о сданных работах.",
	},
}

var checkedCatalogue = map[domain.Language]HomeworkEmailText{
	domain.LanguageEnglish: {
		Subject:         "Assignment checked",
		Intro:           "Your assignment has been checked.",
		Tutor:           "Tutor",
		Deadline:        "Due",
		Action:          "Open the assignment",
		Note:            "The result is on the assignment's page in OVC.",
		Unsubscribe:     "Turn off these emails",
		UnsubscribeNote: "You receive this email because emails about results are turned on in your OVC email settings.",
	},
	domain.LanguageRussian: {
		Subject:         "Задание проверено",
		Intro:           "Ваше задание проверено.",
		Tutor:           "Преподаватель",
		Deadline:        "Срок",
		Action:          "Открыть задание",
		Note:            "Результат — на странице задания в OVC.",
		Unsubscribe:     "Отключить эти письма",
		UnsubscribeNote: "Вы получили это письмо, потому что в настройках писем OVC включены письма о результатах.",
	},
}

func SubmittedText(language domain.Language) (HomeworkEmailText, bool) {
	text, found := submittedCatalogue[language]
	return text, found
}

func SubmissionCheckedText(language domain.Language) (HomeworkEmailText, bool) {
	text, found := checkedCatalogue[language]
	return text, found
}
