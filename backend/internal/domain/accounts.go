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
