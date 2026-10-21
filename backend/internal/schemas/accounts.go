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
}
