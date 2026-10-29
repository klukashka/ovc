package schemas

import (
	"time"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

type GroupMember struct {
	RelationshipID string    `json:"relationship_id" format:"uuid"`
	DisplayName    string    `json:"display_name"`
	Name           string    `json:"name"`
	AvatarID       *string   `json:"avatar_id" format:"uuid" nullable:"true"`
	AddedAt        time.Time `json:"added_at"`
}

type Group struct {
	ID         string        `json:"id" format:"uuid"`
	Name       string        `json:"name"`
	Colour     domain.Colour `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink"`
	ArchivedAt *time.Time    `json:"archived_at" nullable:"true"`
	Members    []GroupMember `json:"members" nullable:"false"`
}

type GroupDetail struct {
	Group
	NextLessons       []CalendarItem `json:"next_lessons" nullable:"false"`
	FutureLessonCount int64          `json:"future_lesson_count"`
	HasOpenSeries     bool           `json:"has_open_series"`
	ChannelID         *string        `json:"channel_id" format:"uuid" nullable:"true"`
}

type GroupSummary struct {
	ID          string        `json:"id" format:"uuid"`
	Name        string        `json:"name"`
	Colour      domain.Colour `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink"`
	ArchivedAt  *time.Time    `json:"archived_at" nullable:"true"`
	MemberCount int64         `json:"member_count"`
	Members     []GroupMember `json:"members" nullable:"false"`
	NextLesson  *CalendarItem `json:"next_lesson"`
}

func (GroupSummary) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "next_lesson")
}

type GroupList struct {
	Groups []GroupSummary `json:"groups" nullable:"false"`
}

type NewGroup struct {
	Name      string        `json:"name" minLength:"1" maxLength:"60"`
	Colour    domain.Colour `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink"`
	MemberIDs []string      `json:"member_ids" format:"uuid" maxItems:"30" nullable:"false"`
}

type GroupPatch struct {
	Name   *string        `json:"name,omitempty" minLength:"1" maxLength:"60"`
	Colour *domain.Colour `json:"colour,omitempty" enum:"clay,slate,moss,plum,ochre,teal,rust,ink"`
	Add    []string       `json:"add,omitempty" format:"uuid" maxItems:"30" nullable:"false"`
	Remove []string       `json:"remove,omitempty" format:"uuid" maxItems:"30" nullable:"false"`
}
