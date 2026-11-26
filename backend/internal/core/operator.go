package core

import (
	"errors"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	varOperatorName  = "OVC_OPERATOR_NAME"
	varOperatorEmail = "OVC_OPERATOR_EMAIL"

	maximumOperatorNameRunes  = 200
	maximumOperatorEmailBytes = 254
)

type Operator struct {
	name  string
	email string
}

func NewOperator(name string, email string) (Operator, error) {
	var problems []error
	trimmedName := strings.TrimSpace(name)
	if !validOperatorName(trimmedName) {
		problems = append(problems, &SettingsError{
			Variable: varOperatorName,
			Problem:  "is required and must be 1 to 200 characters of valid UTF-8 with no control, format, line or paragraph separator characters",
		})
	}
	trimmedEmail := strings.TrimSpace(email)
	if !validOperatorEmail(trimmedEmail) {
		problems = append(problems, &SettingsError{
			Variable: varOperatorEmail,
			Problem:  "is required and must be one plain email address of at most 254 bytes, with no display name",
		})
	}
	if len(problems) > 0 {
		return Operator{}, errors.Join(problems...)
	}
	return Operator{name: trimmedName, email: trimmedEmail}, nil
}

func (o Operator) Name() string {
	return o.name
}

func (o Operator) Email() string {
	return o.email
}

func (o Operator) IsZero() bool {
	return o.name == "" || o.email == ""
}

func validOperatorName(name string) bool {
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > maximumOperatorNameRunes {
		return false
	}
	for _, character := range name {
		if unicode.Is(unicode.Cc, character) || unicode.Is(unicode.Cf, character) ||
			unicode.Is(unicode.Zl, character) || unicode.Is(unicode.Zp, character) {
			return false
		}
	}
	return true
}

func validOperatorEmail(email string) bool {
	if email == "" || len(email) > maximumOperatorEmailBytes || strings.ContainsAny(email, "\r\n") {
		return false
	}
	address, err := mail.ParseAddress(email)
	return err == nil && address.Name == "" && address.Address == email
}

func operatorFrom(lookup Lookup) (Operator, error) {
	name, _ := lookup(varOperatorName)
	email, _ := lookup(varOperatorEmail)
	return NewOperator(name, email)
}
