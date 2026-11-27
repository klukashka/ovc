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
	PeerRead        *bool             `json:"peer_read" nullable:"true"`
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
	Pinned         *PinnedMessage   `json:"pinned"`
	PeerReadSeq    *int64           `json:"peer_read_seq" nullable:"true"`
}

func (ChannelDetail) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "tutor", "last_message", "pinned")
}

type PinnedMessage struct {
	MessageID         string            `json:"message_id" format:"uuid"`
	Seq               int64             `json:"seq"`
	SenderLabel       *string           `json:"sender_label" nullable:"true"`
	SenderLabelKind   *domain.LabelKind `json:"sender_label_kind" enum:"deleted_user" nullable:"true"`
	Own               bool              `json:"own"`
	Preview           *string           `json:"preview" nullable:"true"`
	SentBeforeReclaim bool              `json:"sent_before_reclaim"`
}

type MessageQuote struct {
	MessageID         string            `json:"message_id" format:"uuid"`
	Seq               int64             `json:"seq"`
	SenderLabel       *string           `json:"sender_label" nullable:"true"`
	SenderLabelKind   *domain.LabelKind `json:"sender_label_kind" enum:"deleted_user" nullable:"true"`
	Own               bool              `json:"own"`
	Preview           *string           `json:"preview" nullable:"true"`
	State             domain.QuoteState `json:"state" enum:"available,deleted,removed"`
	SentBeforeReclaim bool              `json:"sent_before_reclaim"`
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
	ReplyTo           *MessageQuote       `json:"reply_to"`
	Reactions         []MessageReaction   `json:"reactions" nullable:"false"`
}

type MessageReaction struct {
	Reaction domain.Reaction   `json:"reaction" enum:"thumbs_up,heart,joy,wow,sad,check"`
	Count    int64             `json:"count"`
	Mine     bool              `json:"mine"`
	Members  *[]ReactionMember `json:"members,omitempty" required:"false" nullable:"false"`
}

type ReactionMember struct {
	Label     *string           `json:"label" nullable:"true"`
	LabelKind *domain.LabelKind `json:"label_kind" enum:"deleted_user" nullable:"true"`
}

type ReactionRequest struct {
	Reaction domain.Reaction `json:"reaction" enum:"thumbs_up,heart,joy,wow,sad,check"`
}

func (Message) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "reply_to")
}

type MessagePage struct {
	Items       []Message `json:"items" nullable:"false"`
	NextCursor  *string   `json:"next_cursor" nullable:"true"`
	HasNewer    bool      `json:"has_newer"`
	PeerReadSeq *int64    `json:"peer_read_seq" nullable:"true"`
}

type NewMessage struct {
	Body             string   `json:"body"`
	AttachmentIDs    []string `json:"attachment_ids" format:"uuid" maxItems:"10" nullable:"false"`
	ReplyToMessageID *string  `json:"reply_to_message_id,omitempty" required:"false" nullable:"false"`
}

type PinRequest struct {
	MessageID string `json:"message_id"`
}

type MessagePatch struct {
	Body    string `json:"body"`
	Version int32  `json:"version" minimum:"1"`
}

type SearchRequest struct {
	Q      string  `json:"q" maxLength:"400"`
	Cursor *string `json:"cursor,omitempty" required:"false" nullable:"false" maxLength:"64"`
}

type SearchPage struct {
	Items      []Message `json:"items" nullable:"false"`
	NextCursor *string   `json:"next_cursor" nullable:"true"`
}

type ReadMarker struct {
	Seq int64 `json:"seq" minimum:"0"`
}

type ReadState struct {
	UnreadCount    int64 `json:"unread_count"`
	UnreadChannels int64 `json:"unread_channels"`
}
