package domain

import (
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	EventEmailConfirmation OutboxKind = "email.confirmation"
	EventPasswordReset     OutboxKind = "password.reset"
	EventSignUpDuplicate   OutboxKind = "signup.duplicate_attempt"
	EventInvitationCreated OutboxKind = "invitation.created"
	EventRelationshipEnded OutboxKind = "relationship.ended"
	EventTutorDeleted      OutboxKind = "tutor.deleted"
)

func EmailReachesRecipient(kind OutboxKind, recipient AccountState) bool {
	switch recipient {
	case AccountActive:
		return true
	case AccountUnverified:
		return unverifiedRecipientKinds[kind]
	case AccountBlocked, AccountDeleted:
		return false
	}
	return false
}

var unverifiedRecipientKinds = map[OutboxKind]bool{
	EventEmailConfirmation: true,
	EventPasswordReset:     true,
	EventSignUpDuplicate:   true,
	EventInvitationCreated: true,
}

type ConsentVersion string

const CurrentConsentVersion ConsentVersion = "2026-10"

const (
	PasswordMinRunes = 10
	PasswordMaxRunes = 128
	EmailTokenBytes  = 32
	EmailMaxBytes    = 254
)

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func ValidEmail(email string) bool {
	if email == "" || len(email) > EmailMaxBytes || strings.ContainsFunc(email, unicode.IsControl) {
		return false
	}
	parsed, err := mail.ParseAddress(email)
	return err == nil && parsed.Address == email && parsed.Name == ""
}

type FieldCode string

const (
	FieldInvalid    FieldCode = "invalid"
	FieldRequired   FieldCode = "required"
	FieldNotAllowed FieldCode = "not_allowed"
	FieldTooShort   FieldCode = "too_short"
	FieldTooLong    FieldCode = "too_long"
	FieldIncorrect  FieldCode = "incorrect"
	FieldTooMany    FieldCode = "too_many"
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

type InvitationEmailPayload struct {
	Invitation string `json:"invitation"`
	Send       int16  `json:"send"`
}

type RelationshipEndedPayload struct {
	Relationship  string `json:"relationship"`
	EndedAtMicros int64  `json:"ended_at_micros"`
}

type TutorDeletedPayload struct {
	Tutor           string `json:"tutor"`
	DeletedAtMicros int64  `json:"deleted_at_micros"`
	Recipient       string `json:"recipient"`
}
