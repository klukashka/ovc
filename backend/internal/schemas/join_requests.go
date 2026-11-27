package schemas

import (
	"time"

	"ovc/backend/internal/domain"
)

type JoinRequestEntry struct {
	Code string `json:"code" minLength:"1" maxLength:"64"`
}

type SentJoinRequest struct {
	ID        string                  `json:"id" format:"uuid"`
	State     domain.SentRequestState `json:"state" enum:"pending,accepted,not_accepted,expired"`
	CreatedAt time.Time               `json:"created_at"`
	DecidedAt *time.Time              `json:"decided_at" nullable:"true"`
}

type JoinRequestSent struct {
	Request SentJoinRequest `json:"request"`
}

type SentJoinRequestPage struct {
	Items      []SentJoinRequest `json:"items" nullable:"false"`
	NextCursor *string           `json:"next_cursor" nullable:"true"`
}

type RequestingStudent struct {
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	AvatarID *string `json:"avatar_id" format:"uuid" nullable:"true"`
}

type TutorJoinRequest struct {
	ID        string            `json:"id" format:"uuid"`
	Student   RequestingStudent `json:"student"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type TutorJoinRequestPage struct {
	Items      []TutorJoinRequest `json:"items" nullable:"false"`
	NextCursor *string            `json:"next_cursor" nullable:"true"`
}

type JoinRequestDecline struct {
	Block *bool `json:"block,omitempty"`
}

type JoinRequestDecision struct {
	ID    string                  `json:"id" format:"uuid"`
	State domain.JoinRequestState `json:"state" enum:"declined"`
}

type BlockedRequester struct {
	StudentID string    `json:"student_id" format:"uuid"`
	Name      *string   `json:"name" nullable:"true"`
	AvatarID  *string   `json:"avatar_id" format:"uuid" nullable:"true"`
	BlockedAt time.Time `json:"blocked_at"`
}

type BlockedRequesterPage struct {
	Items      []BlockedRequester `json:"items" nullable:"false"`
	NextCursor *string            `json:"next_cursor" nullable:"true"`
}

type JoinRequestCount struct {
	Count int64 `json:"count"`
}
