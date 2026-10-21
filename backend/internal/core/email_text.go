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
