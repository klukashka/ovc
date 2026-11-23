package schemas

import (
	"time"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

type NotificationLink struct {
	Kind domain.NotificationLinkKind `json:"kind" enum:"student_card,join_requests,my_tutors,lesson,assignment,review"`
	ID   string                      `json:"id" format:"uuid"`
}

type Notification struct {
	ID           string                   `json:"id" format:"uuid"`
	Event        domain.NotificationEvent `json:"event" enum:"invitation.accepted,join_request.created,join_request.accepted,join_request.declined,relationship.ended,lesson.reminder_1h,lesson.moved,lesson.cancelled,lesson.series_changed,group.archived,issue.created,issue.withdrawn,submission.submitted,submission.checked,submission.returned,submission.review_updated"`
	CreatedAt    time.Time                `json:"created_at"`
	ReadAt       *time.Time               `json:"read_at" nullable:"true"`
	Label        *string                  `json:"label" nullable:"true"`
	LabelDeleted bool                     `json:"label_deleted"`
	LabelKind    *domain.LabelKind        `json:"label_kind" enum:"group,removed_student,former_student" nullable:"true"`
	Colour       *domain.Colour           `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink" nullable:"true"`
	Link         *NotificationLink        `json:"link"`
}

func (Notification) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "link")
}

type NotificationPage struct {
	Items       []Notification `json:"items" nullable:"false"`
	NextCursor  *string        `json:"next_cursor" nullable:"true"`
	UnreadCount int64          `json:"unread_count"`
}

type NotificationRead struct {
	IDs     []string `json:"ids,omitempty" maxItems:"100" required:"false"`
	Through *string  `json:"through,omitempty" required:"false"`
}

type NotificationUnread struct {
	UnreadCount int64 `json:"unread_count"`
}
