package core

import "ovc/backend/internal/domain"

type ExportSheetText struct {
	Title   string
	Headers []string
}

type ExportSheet string

const (
	ExportSheetTutors   ExportSheet = "tutors"
	ExportSheetLessons  ExportSheet = "lessons"
	ExportSheetHomework ExportSheet = "homework"
	ExportSheetAnswers  ExportSheet = "answers"
	ExportSheetFeedback ExportSheet = "feedback"
	ExportSheetMessages ExportSheet = "messages"
	ExportSheetFiles    ExportSheet = "files"

	ExportSheetStudents     ExportSheet = "students"
	ExportSheetGroups       ExportSheet = "groups"
	ExportSheetGroupMembers ExportSheet = "group_members"
	ExportSheetSeries       ExportSheet = "series"
	ExportSheetIssues       ExportSheet = "issues"
	ExportSheetSubmissions  ExportSheet = "submissions"
	ExportSheetReviews      ExportSheet = "reviews"
	ExportSheetLibrary      ExportSheet = "library"
)

type ExportColumn struct {
	Key  string
	Date bool
}

type ExportText struct {
	Account        ExportSheetText
	RoleNames      map[domain.Role]string
	Sheets         map[ExportSheet]ExportSheetText
	TutorSheets    map[ExportSheet]ExportSheetText
	Labels         map[domain.LabelKind]string
	You            string
	MessageRemoved string
	NotAvailable   string
	Yes            string
	No             string
}

func StudentExportSheets() []ExportSheet {
	return []ExportSheet{
		ExportSheetTutors, ExportSheetLessons, ExportSheetHomework, ExportSheetAnswers, ExportSheetFeedback, ExportSheetMessages, ExportSheetFiles,
	}
}

func StudentExportColumns(sheet ExportSheet) []ExportColumn {
	switch sheet {
	case ExportSheetTutors:
		return []ExportColumn{
			{Key: "tutor.name"}, {Key: "state"}, {Key: "tutor.subjects"}, {Key: "tutor.bio"}, {Key: "ended_at", Date: true}, {Key: "ended_by"},
			{Key: "account_closed_until", Date: true},
		}
	case ExportSheetLessons:
		return []ExportColumn{
			{Key: "title"}, {Key: "starts_at", Date: true}, {Key: "ends_at", Date: true}, {Key: "status"}, {Key: "took_place"}, {Key: "timezone"},
			{Key: "audience.label"}, {Key: "tutor.name"}, {Key: "description"}, {Key: "cancelled_at", Date: true}, {Key: "materials.path"},
		}
	case ExportSheetHomework:
		return []ExportColumn{
			{Key: "title"}, {Key: "tutor.name"}, {Key: "status"}, {Key: "late"}, {Key: "deadline_at", Date: true}, {Key: "issued_at", Date: true},
			{Key: "submitted_at", Date: true}, {Key: "checked_at", Date: true}, {Key: "feedback_excerpt"}, {Key: "issued_before_reclaim"},
		}
	case ExportSheetAnswers:
		return []ExportColumn{
			{Key: "title"}, {Key: "blocks.prompt"}, {Key: "answers.value.text"}, {Key: "blocks.options.text"}, {Key: "answers.files.path"},
			{Key: "revealed.questions.accepted_answers"}, {Key: "revealed.questions.explanation"},
		}
	case ExportSheetFeedback:
		return []ExportColumn{
			{Key: "title"}, {Key: "review.feedback"}, {Key: "blocks.prompt"}, {Key: "review.items.comment"}, {Key: "review.items.corrected_files.path"},
		}
	case ExportSheetMessages:
		return []ExportColumn{
			{Key: "channel.label"}, {Key: "messages.created_at", Date: true}, {Key: "messages.sender_label"}, {Key: "messages.body"},
			{Key: "messages.edited_at", Date: true}, {Key: "messages.reply_to.preview"}, {Key: "messages.attachments.path"},
			{Key: "messages.reactions.reaction"},
		}
	case ExportSheetFiles:
		return []ExportColumn{{Key: "path"}, {Key: "filename"}, {Key: "media_type"}, {Key: "size_bytes"}, {Key: "sha256"}, {Key: "references"}}
	case ExportSheetStudents, ExportSheetGroups, ExportSheetGroupMembers, ExportSheetSeries, ExportSheetIssues, ExportSheetSubmissions,
		ExportSheetReviews, ExportSheetLibrary:
	}
	return nil
}

func TutorExportSheets() []ExportSheet {
	return []ExportSheet{
		ExportSheetStudents, ExportSheetGroups, ExportSheetGroupMembers, ExportSheetLessons, ExportSheetSeries, ExportSheetIssues,
		ExportSheetSubmissions, ExportSheetAnswers, ExportSheetReviews, ExportSheetLibrary, ExportSheetMessages, ExportSheetFiles,
	}
}

func TutorExportColumns(sheet ExportSheet) []ExportColumn {
	switch sheet {
	case ExportSheetStudents:
		return []ExportColumn{
			{Key: "display_name"}, {Key: "state"}, {Key: "email"}, {Key: "email_erased"}, {Key: "account.name"}, {Key: "account.email"},
			{Key: "groups.name"}, {Key: "notes"}, {Key: "invitation.status"}, {Key: "invitation.expires_at", Date: true},
			{Key: "linked_at", Date: true}, {Key: "ended_at", Date: true}, {Key: "ended_by"}, {Key: "created_at", Date: true},
		}
	case ExportSheetGroups:
		return []ExportColumn{{Key: "name"}, {Key: "colour"}, {Key: "archived_at", Date: true}, {Key: "members.display_name"}}
	case ExportSheetGroupMembers:
		return []ExportColumn{{Key: "name"}, {Key: "members.display_name"}, {Key: "members.name"}, {Key: "members.added_at", Date: true}}
	case ExportSheetLessons:
		return []ExportColumn{
			{Key: "title"}, {Key: "starts_at", Date: true}, {Key: "ends_at", Date: true}, {Key: "status"}, {Key: "took_place"}, {Key: "timezone"},
			{Key: "audience.label"}, {Key: "audience.members.label"}, {Key: "description"}, {Key: "cancelled_at", Date: true}, {Key: "series_id"},
			{Key: "materials.path"},
		}
	case ExportSheetSeries:
		return []ExportColumn{
			{Key: "id"}, {Key: "weekdays"}, {Key: "start"}, {Key: "duration_minutes"}, {Key: "timezone"}, {Key: "first_date"}, {Key: "end_date"},
			{Key: "count"},
		}
	case ExportSheetIssues:
		return []ExportColumn{
			{Key: "title"}, {Key: "audience.display_name"}, {Key: "status"}, {Key: "answer_policy"}, {Key: "deadline_at", Date: true},
			{Key: "issued_at", Date: true}, {Key: "withdrawn_at", Date: true}, {Key: "max_points"}, {Key: "blocks.material.path"},
		}
	case ExportSheetSubmissions:
		return []ExportColumn{
			{Key: "title"}, {Key: "student.display_name"}, {Key: "status"}, {Key: "late"}, {Key: "issued_before_reclaim"},
			{Key: "submitted_at", Date: true}, {Key: "checked_at", Date: true}, {Key: "auto_points"}, {Key: "final_points"}, {Key: "max_points"},
			{Key: "percentage"},
		}
	case ExportSheetAnswers:
		return []ExportColumn{
			{Key: "title"}, {Key: "student.display_name"}, {Key: "blocks.prompt"}, {Key: "answers.value.text"}, {Key: "blocks.options.text"},
			{Key: "answers.files.path"}, {Key: "answers.auto_points"}, {Key: "blocks.accepted_answers"},
		}
	case ExportSheetReviews:
		return []ExportColumn{
			{Key: "title"}, {Key: "student.display_name"}, {Key: "revisions.published_at", Date: true}, {Key: "revisions.feedback"},
			{Key: "blocks.prompt"}, {Key: "revisions.items.points"}, {Key: "revisions.items.comment"}, {Key: "revisions.items.corrected_files.path"},
		}
	case ExportSheetLibrary:
		return []ExportColumn{
			{Key: "kind"}, {Key: "title"}, {Key: "path.name"}, {Key: "filename"}, {Key: "file_path"}, {Key: "question_count"}, {Key: "max_points"},
			{Key: "updated_at", Date: true},
		}
	case ExportSheetMessages, ExportSheetFiles:
		return StudentExportColumns(sheet)
	case ExportSheetTutors, ExportSheetHomework, ExportSheetFeedback:
	}
	return nil
}

var exportTexts = map[domain.Language]ExportText{
	domain.LanguageEnglish: {
		Account: ExportSheetText{
			Title:   "Account",
			Headers: []string{"Name", "Email", "Role", "Language", "Timezone", "Created (%s)"},
		},
		RoleNames: map[domain.Role]string{domain.RoleTutor: "Tutor", domain.RoleStudent: "Student"},
		Sheets: map[ExportSheet]ExportSheetText{
			ExportSheetTutors: {Title: "Tutors", Headers: []string{
				"Tutor", "State", "Subjects", "About", "Ended (%s)", "Ended by", "Account closed until (%s)",
			}},
			ExportSheetLessons: {Title: "Lessons", Headers: []string{
				"Title", "Starts (%s)", "Ends (%s)", "Status", "Took place", "Lesson timezone", "Audience", "Tutor", "Description", "Cancelled (%s)",
				"Materials",
			}},
			ExportSheetHomework: {Title: "Homework", Headers: []string{
				"Title", "Tutor", "Status", "Late", "Deadline (%s)", "Issued (%s)", "Submitted (%s)", "Checked (%s)", "Feedback", "Issued before reclaim",
			}},
			ExportSheetAnswers: {Title: "Answers", Headers: []string{
				"Assignment", "Question", "Answer", "Chosen options", "Files", "Correct answer", "Explanation",
			}},
			ExportSheetFeedback: {Title: "Feedback", Headers: []string{
				"Assignment", "Feedback", "Question", "Comment", "Corrected files",
			}},
			ExportSheetMessages: {Title: "Messages", Headers: []string{
				"Chat", "Sent (%s)", "Sender", "Text", "Edited (%s)", "Reply to", "Files", "Reactions",
			}},
			ExportSheetFiles: {Title: "Files", Headers: []string{
				"Path", "Original name", "Media type", "Size in bytes", "SHA-256", "Referenced by",
			}},
		},
		TutorSheets: map[ExportSheet]ExportSheetText{
			ExportSheetStudents: {Title: "Students", Headers: []string{
				"Name on the card", "State", "Card email", "Email erased", "Account name", "Account email", "Groups", "Notes", "Invitation",
				"Invitation expires (%s)", "Linked (%s)", "Ended (%s)", "Ended by", "Created (%s)",
			}},
			ExportSheetGroups:       {Title: "Groups", Headers: []string{"Group", "Colour", "Archived (%s)", "Members"}},
			ExportSheetGroupMembers: {Title: "Group members", Headers: []string{"Group", "Name on the card", "Account name", "Added (%s)"}},
			ExportSheetLessons: {Title: "Lessons", Headers: []string{
				"Title", "Starts (%s)", "Ends (%s)", "Status", "Took place", "Lesson timezone", "Audience", "Members", "Description", "Cancelled (%s)",
				"Series", "Materials",
			}},
			ExportSheetSeries: {Title: "Series", Headers: []string{
				"Series", "Weekdays", "Start", "Duration in minutes", "Timezone", "First date", "End date", "Count",
			}},
			ExportSheetIssues: {Title: "Issued work", Headers: []string{
				"Title", "Audience", "Status", "Answers shown", "Deadline (%s)", "Issued (%s)", "Withdrawn (%s)", "Maximum points", "Materials",
			}},
			ExportSheetSubmissions: {Title: "Submissions", Headers: []string{
				"Assignment", "Student", "Status", "Late", "Issued before reclaim", "Submitted (%s)", "Checked (%s)", "Automatic points", "Final points",
				"Maximum points", "Percentage",
			}},
			ExportSheetAnswers: {Title: "Answers", Headers: []string{
				"Assignment", "Student", "Question", "Answer", "Chosen options", "Files", "Automatic points", "Accepted answers",
			}},
			ExportSheetReviews: {Title: "Reviews", Headers: []string{
				"Assignment", "Student", "Published (%s)", "Feedback", "Question", "Points", "Comment", "Corrected files",
			}},
			ExportSheetLibrary: {Title: "Library", Headers: []string{
				"Kind", "Title", "Folder", "File name", "File", "Questions", "Maximum points", "Updated (%s)",
			}},
			ExportSheetMessages: {Title: "Messages", Headers: []string{
				"Chat", "Sent (%s)", "Sender", "Text", "Edited (%s)", "Reply to", "Files", "Reactions",
			}},
			ExportSheetFiles: {Title: "Files", Headers: []string{
				"Path", "Original name", "Media type", "Size in bytes", "SHA-256", "Referenced by",
			}},
		},
		Labels: map[domain.LabelKind]string{
			domain.LabelIndividual: "Individual lesson", domain.LabelGroup: "Group lesson", domain.LabelRemovedStudent: "Removed student",
			domain.LabelFormerStudent: "Former student", domain.LabelDeletedAccount: "Deleted account", domain.LabelDeletedUser: "Deleted user",
		},
		You: "You", MessageRemoved: "Message removed", NotAvailable: "not available", Yes: "Yes", No: "No",
	},
	domain.LanguageRussian: {
		Account: ExportSheetText{
			Title:   "Аккаунт",
			Headers: []string{"Имя", "Электронная почта", "Роль", "Язык", "Часовой пояс", "Создан (%s)"},
		},
		RoleNames: map[domain.Role]string{domain.RoleTutor: "Преподаватель", domain.RoleStudent: "Ученик"},
		Sheets: map[ExportSheet]ExportSheetText{
			ExportSheetTutors: {Title: "Преподаватели", Headers: []string{
				"Преподаватель", "Состояние", "Предметы", "О себе", "Завершено (%s)", "Кем завершено", "Аккаунт закрыт до (%s)",
			}},
			ExportSheetLessons: {Title: "Уроки", Headers: []string{
				"Название", "Начало (%s)", "Конец (%s)", "Статус", "Состоялся", "Часовой пояс урока", "Участники", "Преподаватель", "Описание",
				"Отменён (%s)", "Материалы",
			}},
			ExportSheetHomework: {Title: "Домашние задания", Headers: []string{
				"Название", "Преподаватель", "Статус", "С опозданием", "Срок (%s)", "Выдано (%s)", "Сдано (%s)", "Проверено (%s)", "Отзыв",
				"Выдано до восстановления",
			}},
			ExportSheetAnswers: {Title: "Ответы", Headers: []string{
				"Задание", "Вопрос", "Ответ", "Выбранные варианты", "Файлы", "Правильный ответ", "Пояснение",
			}},
			ExportSheetFeedback: {Title: "Отзывы", Headers: []string{
				"Задание", "Отзыв", "Вопрос", "Комментарий", "Исправленные файлы",
			}},
			ExportSheetMessages: {Title: "Сообщения", Headers: []string{
				"Чат", "Отправлено (%s)", "Отправитель", "Текст", "Изменено (%s)", "Ответ на", "Файлы", "Реакции",
			}},
			ExportSheetFiles: {Title: "Файлы", Headers: []string{
				"Путь", "Исходное имя", "Тип", "Размер в байтах", "SHA-256", "Где используется",
			}},
		},
		TutorSheets: map[ExportSheet]ExportSheetText{
			ExportSheetStudents: {Title: "Ученики", Headers: []string{
				"Имя в карточке", "Состояние", "Почта в карточке", "Почта стёрта", "Имя аккаунта", "Почта аккаунта", "Группы", "Заметки",
				"Приглашение", "Приглашение действует до (%s)", "Связан (%s)", "Завершено (%s)", "Кем завершено", "Создана (%s)",
			}},
			ExportSheetGroups:       {Title: "Группы", Headers: []string{"Группа", "Цвет", "В архиве с (%s)", "Участники"}},
			ExportSheetGroupMembers: {Title: "Участники групп", Headers: []string{"Группа", "Имя в карточке", "Имя аккаунта", "Добавлен (%s)"}},
			ExportSheetLessons: {Title: "Уроки", Headers: []string{
				"Название", "Начало (%s)", "Конец (%s)", "Статус", "Состоялся", "Часовой пояс урока", "Участники", "Состав", "Описание",
				"Отменён (%s)", "Серия", "Материалы",
			}},
			ExportSheetSeries: {Title: "Серии", Headers: []string{
				"Серия", "Дни недели", "Начало", "Длительность в минутах", "Часовой пояс", "Первая дата", "Последняя дата", "Количество",
			}},
			ExportSheetIssues: {Title: "Выданные задания", Headers: []string{
				"Название", "Кому", "Статус", "Показ ответов", "Срок (%s)", "Выдано (%s)", "Отозвано (%s)", "Максимум баллов", "Материалы",
			}},
			ExportSheetSubmissions: {Title: "Работы", Headers: []string{
				"Задание", "Ученик", "Статус", "С опозданием", "Выдано до восстановления", "Сдано (%s)", "Проверено (%s)", "Автоматические баллы",
				"Итоговые баллы", "Максимум баллов", "Процент",
			}},
			ExportSheetAnswers: {Title: "Ответы", Headers: []string{
				"Задание", "Ученик", "Вопрос", "Ответ", "Выбранные варианты", "Файлы", "Автоматические баллы", "Принимаемые ответы",
			}},
			ExportSheetReviews: {Title: "Проверки", Headers: []string{
				"Задание", "Ученик", "Опубликовано (%s)", "Отзыв", "Вопрос", "Баллы", "Комментарий", "Исправленные файлы",
			}},
			ExportSheetLibrary: {Title: "Библиотека", Headers: []string{
				"Вид", "Название", "Папка", "Имя файла", "Файл", "Вопросов", "Максимум баллов", "Изменено (%s)",
			}},
			ExportSheetMessages: {Title: "Сообщения", Headers: []string{
				"Чат", "Отправлено (%s)", "Отправитель", "Текст", "Изменено (%s)", "Ответ на", "Файлы", "Реакции",
			}},
			ExportSheetFiles: {Title: "Файлы", Headers: []string{
				"Путь", "Исходное имя", "Тип", "Размер в байтах", "SHA-256", "Где используется",
			}},
		},
		Labels: map[domain.LabelKind]string{
			domain.LabelIndividual: "Индивидуальный урок", domain.LabelGroup: "Групповой урок", domain.LabelRemovedStudent: "Удалённый ученик",
			domain.LabelFormerStudent: "Бывший ученик", domain.LabelDeletedAccount: "Удалённый аккаунт", domain.LabelDeletedUser: "Удалённый пользователь",
		},
		You: "Вы", MessageRemoved: "Сообщение удалено", NotAvailable: "недоступно", Yes: "Да", No: "Нет",
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
