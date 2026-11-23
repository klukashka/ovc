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
	LabelKind   *domain.LabelKind   `json:"label_kind" enum:"deleted_user,former_student,removed_student" nullable:"true"`
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

type MessageAttachment struct {
	ID        string    `json:"id" format:"uuid"`
	Filename  string    `json:"filename"`
	MediaType string    `json:"media_type"`
	SizeBytes int64     `json:"size_bytes"`
	Width     *int32    `json:"width" nullable:"true"`
	Height    *int32    `json:"height" nullable:"true"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Message struct {
	ID                string              `json:"id" format:"uuid"`
	Seq               int64               `json:"seq"`
	SenderLabel       *string             `json:"sender_label" nullable:"true"`
	SenderLabelKind   *domain.LabelKind   `json:"sender_label_kind" enum:"deleted_user" nullable:"true"`
	Own               bool                `json:"own"`
	Body              *string             `json:"body" nullable:"true"`
	CreatedAt         time.Time           `json:"created_at"`
	EditedAt          *time.Time          `json:"edited_at" nullable:"true"`
	Deleted           bool                `json:"deleted"`
	Erased            bool                `json:"erased"`
	Version           int32               `json:"version"`
	SentBeforeReclaim bool                `json:"sent_before_reclaim"`
	Attachments       []MessageAttachment `json:"attachments" nullable:"false"`
}

type MessagePage struct {
	Items      []Message `json:"items" nullable:"false"`
	NextCursor *string   `json:"next_cursor" nullable:"true"`
}

type NewMessage struct {
	Body          string   `json:"body"`
	AttachmentIDs []string `json:"attachment_ids" format:"uuid" maxItems:"10" nullable:"false"`
}

type MessagePatch struct {
	Body    string `json:"body"`
	Version int32  `json:"version" minimum:"1"`
}

type ReadMarker struct {
	Seq int64 `json:"seq" minimum:"0"`
}

type ReadState struct {
	UnreadCount    int64 `json:"unread_count"`
	UnreadChannels int64 `json:"unread_channels"`
}
