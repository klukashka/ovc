package domain

import (
	"time"
	"unicode/utf8"
)

const (
	EventEmailConfirmation OutboxKind = "email.confirmation"
	EventPasswordReset     OutboxKind = "password.reset"
	EventSignUpDuplicate   OutboxKind = "signup.duplicate_attempt"
)

type ConsentVersion string

const CurrentConsentVersion ConsentVersion = "2026-10"

const (
	PasswordMinRunes = 10
	PasswordMaxRunes = 128
	EmailTokenBytes  = 32
)

type FieldCode string

const (
	FieldInvalid    FieldCode = "invalid"
	FieldRequired   FieldCode = "required"
	FieldNotAllowed FieldCode = "not_allowed"
	FieldTooShort   FieldCode = "too_short"
	FieldTooLong    FieldCode = "too_long"
	FieldIncorrect  FieldCode = "incorrect"
)

func CheckPasswordLength(password string) (FieldCode, bool) {
	runes := utf8.RuneCountInString(password)
	switch {
	case runes < PasswordMinRunes:
		return FieldTooShort, false
	case runes > PasswordMaxRunes:
		return FieldTooLong, false
	}
	return "", true
}

func (p EmailPurpose) TokenLifetime() time.Duration {
	if p == EmailPurposeReset {
		return time.Hour
	}
	return 24 * time.Hour
}

func (p EmailPurpose) Event() OutboxKind {
	if p == EmailPurposeReset {
		return EventPasswordReset
	}
	return EventEmailConfirmation
}

type LinkOutcome string

const (
	LinkOutcomeChoiceRequired LinkOutcome = "choice_required"
	LinkOutcomeLinked         LinkOutcome = "linked"
	LinkOutcomeRequestSent    LinkOutcome = "request_sent"
	LinkOutcomeCodeInvalid    LinkOutcome = "code_invalid"
)

type EmailRequestPayload struct {
	Request string `json:"request"`
}

type AccountPayload struct {
	User string `json:"user"`
}
