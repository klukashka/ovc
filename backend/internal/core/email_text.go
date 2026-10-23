package core

import "ovc/backend/internal/domain"

type EmailText struct {
	Subject string
	Lines   []string
	Action  string
	Note    string
}

var accountEmailTexts = map[domain.Language]map[domain.OutboxKind]EmailText{
	domain.LanguageEnglish: {
		domain.EventEmailConfirmation: {
			Subject: "Confirm your email for OVC",
			Lines:   []string{"Welcome to OVC.", "Confirm your email address to finish setting up your account."},
			Action:  "Confirm email",
			Note:    "The link works for 24 hours and only once. If you did not sign up for OVC, ignore this email.",
		},
		domain.EventPasswordReset: {
			Subject: "Reset your OVC password",
			Lines:   []string{"We received a request to reset the password of your OVC account."},
			Action:  "Set a new password",
			Note:    "The link works for 1 hour and only once. If you did not ask for this, ignore this email: your password stays the same.",
		},
		domain.EventSignUpDuplicate: {
			Subject: "Someone tried to sign up with your email",
			Lines: []string{
				"Someone tried to create an OVC account with your email address, but an account with it already exists.",
				"If it was you, sign in with your password, or reset it if you have forgotten it.",
			},
			Action: "Reset password",
			Note:   "If it was not you, you do not need to do anything: nobody can use your account without your password.",
		},
	},
	domain.LanguageRussian: {
		domain.EventEmailConfirmation: {
			Subject: "Подтвердите email в OVC",
			Lines:   []string{"Добро пожаловать в OVC.", "Подтвердите адрес электронной почты, чтобы завершить настройку аккаунта."},
			Action:  "Подтвердить email",
			Note:    "Ссылка действует 24 часа и только один раз. Если вы не регистрировались в OVC, просто проигнорируйте это письмо.",
		},
		domain.EventPasswordReset: {
			Subject: "Сброс пароля в OVC",
			Lines:   []string{"Мы получили запрос на сброс пароля вашего аккаунта OVC."},
			Action:  "Задать новый пароль",
			Note:    "Ссылка действует 1 час и только один раз. Если вы не запрашивали сброс, проигнорируйте письмо: пароль останется прежним.",
		},
		domain.EventSignUpDuplicate: {
			Subject: "Попытка регистрации с вашим email",
			Lines: []string{
				"Кто-то пытался создать аккаунт OVC с вашим адресом электронной почты, но аккаунт с этим адресом уже существует.",
				"Если это были вы, войдите со своим паролем или сбросьте его, если забыли.",
			},
			Action: "Сбросить пароль",
			Note:   "Если это были не вы, ничего делать не нужно: без пароля никто не сможет войти в ваш аккаунт.",
		},
	},
}

func AccountEmailText(language domain.Language, event domain.OutboxKind) (EmailText, bool) {
	text, found := accountEmailTexts[language][event]
	return text, found
}

type RelationshipText struct {
	Subject string
	Intro   string
	Lines   []string
	Action  string
	Note    string
}

var relationshipEmailTexts = map[domain.Language]map[domain.OutboxKind]RelationshipText{
	domain.LanguageEnglish: {
		domain.EventInvitationCreated: {
			Subject: "You are invited to study on OVC",
			Intro:   "You have been invited to study on OVC. The invitation is from the tutor:",
			Lines:   []string{"Open the invitation to see the tutor's profile, then create your account or sign in to accept it."},
			Action:  "Open the invitation",
			Note: "The link works only once and stops working when it expires or the tutor replaces it. " +
				"If you do not expect lessons with this person, ignore this email.",
		},
	},
	domain.LanguageRussian: {
		domain.EventInvitationCreated: {
			Subject: "Приглашение на занятия в OVC",
			Intro:   "Вас пригласили на занятия в OVC. Приглашение от преподавателя:",
			Lines:   []string{"Откройте приглашение, чтобы посмотреть профиль преподавателя, а затем создайте аккаунт или войдите, чтобы принять его."},
			Action:  "Открыть приглашение",
			Note: "Ссылка действует только один раз и перестаёт работать, когда истекает её срок или преподаватель заменяет её. " +
				"Если вы не ждёте занятий с этим человеком, просто проигнорируйте письмо.",
		},
	},
}

func RelationshipEmailText(language domain.Language, event domain.OutboxKind) (RelationshipText, bool) {
	text, found := relationshipEmailTexts[language][event]
	return text, found
}
