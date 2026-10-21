package schemas

import "ovc/backend/internal/domain"

type LoginRequest struct {
	Email    string `json:"email" minLength:"3" maxLength:"254"`
	Password string `json:"password" minLength:"1" maxLength:"128"`
}

type AccessToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type Me struct {
	ID            string              `json:"id" format:"uuid"`
	Role          domain.Role         `json:"role" enum:"tutor,student"`
	State         domain.AccountState `json:"state" enum:"unverified,active"`
	Email         string              `json:"email"`
	EmailVerified bool                `json:"email_verified"`
	Name          string              `json:"name"`
	Language      domain.Language     `json:"language" enum:"ru,en"`
	Timezone      string              `json:"timezone"`
	Theme         domain.Theme        `json:"theme" enum:"light,dark,system"`
	AvatarID      *string             `json:"avatar_id" format:"uuid" nullable:"true"`
	Bio           *string             `json:"bio,omitempty"`
	Subjects      *[]string           `json:"subjects,omitempty"`
}

type RegisterRequest struct {
	Role     domain.Role     `json:"role" enum:"tutor,student"`
	Name     string          `json:"name" maxLength:"400"`
	Email    string          `json:"email" maxLength:"254"`
	Password string          `json:"password" maxLength:"512"`
	Language domain.Language `json:"language" enum:"ru,en"`
	Timezone string          `json:"timezone"`
	Consent  bool            `json:"consent"`
}

type VerifyEmailRequest struct {
	Token       string  `json:"token" maxLength:"64"`
	NewPassword *string `json:"new_password,omitempty" maxLength:"512"`
}

type EmailConfirmed struct {
	Outcome     *domain.LinkOutcome `json:"outcome" enum:"linked,request_sent,code_invalid,invitation_invalid" nullable:"true"`
	AccessToken string              `json:"access_token,omitempty"`
	ExpiresIn   int64               `json:"expires_in,omitempty"`
}

type ResendConfirmationRequest struct {
	Email *string `json:"email,omitempty" maxLength:"254"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" maxLength:"254"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" maxLength:"64"`
	NewPassword string `json:"new_password" maxLength:"512"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" maxLength:"512"`
	NewPassword     string `json:"new_password" maxLength:"512"`
}

type DeleteAccountRequest struct {
	Password string `json:"password" maxLength:"512"`
}
