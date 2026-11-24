package core

import "ovc/backend/internal/domain"

type ExportSheetText struct {
	Title   string
	Headers []string
}

type ExportText struct {
	Account   ExportSheetText
	RoleNames map[domain.Role]string
}

var exportTexts = map[domain.Language]ExportText{
	domain.LanguageEnglish: {
		Account: ExportSheetText{
			Title:   "Account",
			Headers: []string{"Name", "Email", "Role", "Language", "Timezone", "Created (%s)"},
		},
		RoleNames: map[domain.Role]string{domain.RoleTutor: "Tutor", domain.RoleStudent: "Student"},
	},
	domain.LanguageRussian: {
		Account: ExportSheetText{
			Title:   "Аккаунт",
			Headers: []string{"Имя", "Электронная почта", "Роль", "Язык", "Часовой пояс", "Создан (%s)"},
		},
		RoleNames: map[domain.Role]string{domain.RoleTutor: "Преподаватель", domain.RoleStudent: "Ученик"},
	},
}

func ExportTextFor(language domain.Language) (ExportText, bool) {
	text, found := exportTexts[language]
	return text, found
}

var exportRequestedTexts = map[domain.Language]EmailText{
	domain.LanguageEnglish: {
		Subject: "An export of your OVC data was requested",
		Lines: []string{
			"An export of all your OVC data was requested on %s.",
			"When it is ready, you can download it in Settings, under My data, for 3 days.",
		},
		Action: "Open My data",
		Note:   "If you did not request it, delete the export in My data and change your password at once.",
	},
	domain.LanguageRussian: {
		Subject: "Запрошена выгрузка ваших данных из OVC",
		Lines: []string{
			"%s была запрошена выгрузка всех ваших данных из OVC.",
			"Когда она будет готова, её можно будет скачать в настройках, в разделе «Мои данные», в течение 3 дней.",
		},
		Action: "Открыть «Мои данные»",
		Note:   "Если вы её не запрашивали, сразу удалите выгрузку в разделе «Мои данные» и смените пароль.",
	},
}

func ExportRequestedText(language domain.Language) (EmailText, bool) {
	text, found := exportRequestedTexts[language]
	return text, found
}
