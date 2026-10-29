package schemas

import (
	"time"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

type ChannelTutor struct {
	Name   string         `json:"name"`
	Colour *domain.Colour `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink" nullable:"true"`
}

type ChannelLastMessage struct {
	Seq             int64             `json:"seq"`
	CreatedAt       time.Time         `json:"created_at"`
	Own             bool              `json:"own"`
	SenderLabel     *string           `json:"sender_label" nullable:"true"`
	SenderLabelKind *domain.LabelKind `json:"sender_label_kind" enum:"deleted_user" nullable:"true"`
	Preview         *string           `json:"preview" nullable:"true"`
	AttachmentCount int64             `json:"attachment_count"`
}

type ChannelItem struct {
	ID          string              `json:"id" format:"uuid"`
	Kind        domain.ChannelKind  `json:"kind" enum:"direct,group"`
	Label       *string             `json:"label" nullable:"true"`
	LabelKind   *domain.LabelKind   `json:"label_kind" enum:"deleted_user" nullable:"true"`
	Colour      *domain.Colour      `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink" nullable:"true"`
	Tutor       *ChannelTutor       `json:"tutor"`
	UserID      *string             `json:"user_id" format:"uuid" nullable:"true"`
	LastMessage *ChannelLastMessage `json:"last_message"`
	UnreadCount int64               `json:"unread_count"`
	Writable    bool                `json:"writable"`
	Online      *bool               `json:"online" nullable:"true"`
	Unconfirmed bool                `json:"unconfirmed"`
}

func (ChannelItem) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "tutor", "last_message")
}

type ChannelPage struct {
	Items          []ChannelItem `json:"items" nullable:"false"`
	NextCursor     *string       `json:"next_cursor" nullable:"true"`
	UnreadChannels int64         `json:"unread_channels"`
}

type UnreadChannels struct {
	UnreadChannels int64 `json:"unread_channels"`
}

type ChannelMember struct {
	UserID string `json:"user_id" format:"uuid"`
	Label  string `json:"label"`
	Online *bool  `json:"online" nullable:"true"`
}

type ChannelDetail struct {
	ChannelItem
	RelationshipID *string          `json:"relationship_id,omitempty" format:"uuid"`
	GroupID        *string          `json:"group_id,omitempty" format:"uuid"`
	Members        *[]ChannelMember `json:"members,omitempty" nullable:"false"`
}

func (ChannelDetail) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "tutor", "last_message")
}
