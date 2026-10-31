package schemas

import (
	"encoding/json"

	"ovc/backend/internal/domain"
)

type ClientFrame struct {
	Type    domain.FrameType `json:"type"`
	Ref     *string          `json:"ref,omitempty"`
	Payload json.RawMessage  `json:"payload,omitempty"`
}

type AuthPayload struct {
	AccessToken string `json:"access_token"`
}

type ServerFrame[Payload any] struct {
	Type    domain.FrameType `json:"type"`
	Ref     *string          `json:"ref,omitempty"`
	Payload *Payload         `json:"payload,omitempty"`
}

type ReadyPayload struct{}

type NotificationNewPayload struct {
	ID string `json:"id"`
}

type EntityChangedPayload struct {
	Kind domain.EntityKind `json:"kind"`
	ID   string            `json:"id"`
}

type MessageNewPayload struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
	Seq       int64  `json:"seq"`
}

type MessageChangedPayload struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
}

type ReadUpdatedPayload struct {
	ChannelID string `json:"channel_id"`
}

type TypingPayload struct {
	ChannelID string  `json:"channel_id"`
	UserID    *string `json:"user_id"`
}

type PresencePayload struct {
	UserID string `json:"user_id"`
	Online bool   `json:"online"`
}

type TypingRequestPayload struct {
	ChannelID string `json:"channel_id"`
}

type FrameErrorPayload struct {
	Code      domain.ErrorCode `json:"code"`
	Message   string           `json:"message"`
	RequestID string           `json:"request_id"`
	Ref       *string          `json:"ref"`
}
