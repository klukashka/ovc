package core

import "ovc/backend/internal/domain"

type DigestText struct {
	Subject     string
	Intro       string
	Action      string
	More        string
	Unsubscribe string
}

func DigestEmailText(language domain.Language) (DigestText, bool) {
	switch language {
	case domain.LanguageEnglish:
		return DigestText{Subject: "Unread messages", Intro: "A summary of messages you have not read.", Action: "Open messages", More: "and %d more", Unsubscribe: "Turn off these emails"}, true
	case domain.LanguageRussian:
		return DigestText{Subject: "Непрочитанные сообщения", Intro: "Сводка сообщений, которые вы не прочитали.", Action: "Открыть сообщения", More: "и ещё %d", Unsubscribe: "Отключить эти письма"}, true
	}
	return DigestText{}, false
}
