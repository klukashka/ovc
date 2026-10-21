package domain

type UserID [16]byte

type SessionID [16]byte

type EmailRequestID [16]byte

type EmailTokenID [16]byte

type Role string

const (
	RoleTutor   Role = "tutor"
	RoleStudent Role = "student"
)

type AccountState string

const (
	AccountUnverified AccountState = "unverified"
	AccountActive     AccountState = "active"
	AccountBlocked    AccountState = "blocked"
	AccountDeleted    AccountState = "deleted"
)

type Language string

const (
	LanguageRussian Language = "ru"
	LanguageEnglish Language = "en"
)

type Theme string

const (
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
	ThemeSystem Theme = "system"
)

type EmailPurpose string

const (
	EmailPurposeVerify EmailPurpose = "verify_email"
	EmailPurposeReset  EmailPurpose = "reset_password"
)

type RateLimitPurpose string

type JobResult string

const (
	JobResultOK     JobResult = "ok"
	JobResultFailed JobResult = "failed"
)

func (id UserID) String() string {
	return formatUUID(id)
}

func formatUUID(id [16]byte) string {
	const digits = "0123456789abcdef"
	text := make([]byte, 0, 36)
	for index, value := range id {
		if index == 4 || index == 6 || index == 8 || index == 10 {
			text = append(text, '-')
		}
		text = append(text, digits[value>>4], digits[value&0x0f])
	}
	return string(text)
}

func (id AttachmentID) String() string {
	return formatUUID(id)
}

func ParseUUID[ID ~[16]byte](text string) (ID, bool) {
	var id ID
	if len(text) != 36 {
		return id, false
	}
	position := 0
	for index := 0; index < len(text); index++ {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if text[index] != '-' {
				return id, false
			}
			continue
		}
		value, ok := hexValue(text[index])
		if !ok {
			return id, false
		}
		if position%2 == 0 {
			id[position/2] = value << 4
		} else {
			id[position/2] |= value
		}
		position++
	}
	return id, true
}

func hexValue(character byte) (byte, bool) {
	switch {
	case character >= '0' && character <= '9':
		return character - '0', true
	case character >= 'a' && character <= 'f':
		return character - 'a' + 10, true
	case character >= 'A' && character <= 'F':
		return character - 'A' + 10, true
	}
	return 0, false
}
