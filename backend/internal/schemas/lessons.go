package schemas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

type NewLesson struct {
	RelationshipID  *string  `json:"relationship_id,omitempty" format:"uuid" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
	GroupID         *string  `json:"group_id,omitempty" format:"uuid" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
	Title           string   `json:"title" minLength:"1" maxLength:"120"`
	Description     *string  `json:"description,omitempty" maxLength:"2000"`
	Date            string   `json:"date" pattern:"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"`
	Start           string   `json:"start" pattern:"^([01][0-9]|2[0-3]):[0-5][0-9]$"`
	DurationMinutes int16    `json:"duration_minutes" minimum:"15" maximum:"240" multipleOf:"5"`
	Weekdays        []int16  `json:"weekdays,omitempty" minItems:"1" maxItems:"7" uniqueItems:"true" nullable:"false"`
	EndDate         *string  `json:"end_date,omitempty" pattern:"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"`
	Count           *int16   `json:"count,omitempty" minimum:"1" maximum:"100"`
	AllowOverlap    bool     `json:"allow_overlap,omitempty"`
	MaterialIDs     []string `json:"material_ids,omitempty" format:"uuid" nullable:"false" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
}

func (NewLesson) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	if weekdays, found := schema.Properties["weekdays"]; found && weekdays.Items != nil {
		minimum, maximum := 1.0, float64(domain.MaxISOWeekday)
		weekdays.Items.Minimum, weekdays.Items.Maximum = &minimum, &maximum
	}
	return schema
}

type LessonMember struct {
	Label     *string           `json:"label" nullable:"true"`
	LabelKind *domain.LabelKind `json:"label_kind" enum:"deleted_account,removed_student,former_student" nullable:"true"`
}

type LessonAudience struct {
	Kind           domain.AudienceKind `json:"kind" enum:"one_to_one,group"`
	RelationshipID *string             `json:"relationship_id,omitempty" format:"uuid"`
	GroupID        *string             `json:"group_id,omitempty" format:"uuid"`
	Label          *string             `json:"label" nullable:"true"`
	LabelKind      *domain.LabelKind   `json:"label_kind" enum:"individual,group,removed_student,former_student" nullable:"true"`
	Colour         *domain.Colour      `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink" nullable:"true"`
	Members        []LessonMember      `json:"members" nullable:"true"`
}

type CalendarAudience struct {
	Kind      domain.AudienceKind `json:"kind" enum:"one_to_one,group"`
	Label     *string             `json:"label" nullable:"true"`
	LabelKind *domain.LabelKind   `json:"label_kind" enum:"individual,group,removed_student,former_student" nullable:"true"`
	Colour    *domain.Colour      `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink" nullable:"true"`
}

type LessonTutor struct {
	Name   string         `json:"name"`
	Colour *domain.Colour `json:"colour" enum:"clay,slate,moss,plum,ochre,teal,rust,ink" nullable:"true"`
}

type LessonDetail struct {
	ID              string              `json:"id" format:"uuid"`
	Title           string              `json:"title"`
	Description     *string             `json:"description" nullable:"true"`
	StartsAt        time.Time           `json:"starts_at"`
	EndsAt          time.Time           `json:"ends_at"`
	DurationMinutes int                 `json:"duration_minutes"`
	Status          domain.LessonStatus `json:"status" enum:"scheduled,live,ended,cancelled"`
	TookPlace       bool                `json:"took_place"`
	Timezone        string              `json:"timezone"`
	SeriesID        *string             `json:"series_id" format:"uuid" nullable:"true"`
	OccurrenceDate  *string             `json:"occurrence_date,omitempty" format:"date"`
	Series          *LessonSeries       `json:"series"`
	Version         int32               `json:"version"`
	CancelledAt     *time.Time          `json:"cancelled_at" nullable:"true"`
	OtherTimezone   *string             `json:"other_timezone" nullable:"true"`
	Audience        LessonAudience      `json:"audience"`
	Tutor           *LessonTutor        `json:"tutor"`
	ChannelID       *string             `json:"channel_id" format:"uuid" nullable:"true"`
	RoomOpen        bool                `json:"room_open"`
	EmbedAllowed    bool                `json:"embed_allowed"`
	Materials       []LessonMaterial    `json:"materials" nullable:"false"`
}

type LessonView struct {
	LessonDetail
	Issues []LessonIssue `json:"issues" nullable:"false" maxItems:"20"`
}

type LessonIssue struct {
	IssueID      *string                  `json:"issue_id,omitempty" format:"uuid"`
	SubmissionID *string                  `json:"submission_id,omitempty" format:"uuid"`
	Title        string                   `json:"title"`
	DeadlineAt   *time.Time               `json:"deadline_at" nullable:"true"`
	Students     *int64                   `json:"students,omitempty"`
	Submitted    *int64                   `json:"submitted,omitempty"`
	Status       *domain.SubmissionStatus `json:"status,omitempty" enum:"not_started,draft,submitted,returned,checked,closed"`
	Late         *bool                    `json:"late,omitempty"`
}

type LessonMaterial struct {
	MaterialID   *string   `json:"material_id,omitempty" format:"uuid"`
	AttachmentID string    `json:"attachment_id" format:"uuid"`
	Title        string    `json:"title"`
	Filename     string    `json:"filename"`
	MediaType    string    `json:"media_type"`
	SizeBytes    int64     `json:"size_bytes"`
	Width        *int32    `json:"width" nullable:"true"`
	Height       *int32    `json:"height" nullable:"true"`
	URL          string    `json:"url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type LessonMaterialsUpdate struct {
	MaterialIDs []string `json:"material_ids" format:"uuid" nullable:"false" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
	Version     int32    `json:"version" minimum:"1"`
}

type LessonJoin struct {
	URL          string `json:"url"`
	EmbedAllowed bool   `json:"embed_allowed"`
}

func (LessonDetail) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "tutor", "series")
}

type LessonSeries struct {
	ID              string  `json:"id" format:"uuid"`
	Weekdays        []int   `json:"weekdays" nullable:"false"`
	Start           string  `json:"start"`
	DurationMinutes int     `json:"duration_minutes"`
	Timezone        string  `json:"timezone"`
	FirstDate       string  `json:"first_date"`
	EndDate         *string `json:"end_date" nullable:"true"`
	Count           *int    `json:"count" nullable:"true"`
}

type CreatedLessons struct {
	Series  *LessonSeries  `json:"series"`
	Lessons []LessonDetail `json:"lessons" nullable:"false"`
}

func (CreatedLessons) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "series")
}

type CalendarItem struct {
	ID        string              `json:"id" format:"uuid"`
	Title     string              `json:"title"`
	StartsAt  time.Time           `json:"starts_at"`
	EndsAt    time.Time           `json:"ends_at"`
	Status    domain.LessonStatus `json:"status" enum:"scheduled,live,ended,cancelled"`
	Moved     bool                `json:"moved"`
	TookPlace bool                `json:"took_place"`
	Timezone  string              `json:"timezone"`
	Audience  CalendarAudience    `json:"audience"`
	Tutor     *LessonTutor        `json:"tutor"`
}

func (CalendarItem) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "tutor")
}

type Calendar struct {
	Items     []CalendarItem `json:"items" nullable:"false"`
	Truncated bool           `json:"truncated"`
}

type OverlappingLesson struct {
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

type LessonPatch struct {
	Scope           domain.EditScope    `json:"scope" enum:"this,following"`
	Version         int32               `json:"version" minimum:"1"`
	Date            *string             `json:"date,omitempty" pattern:"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"`
	Start           *string             `json:"start,omitempty" pattern:"^([01][0-9]|2[0-3]):[0-5][0-9]$"`
	DurationMinutes *int16              `json:"duration_minutes,omitempty" minimum:"15" maximum:"240" multipleOf:"5"`
	Title           *string             `json:"title,omitempty" minLength:"1" maxLength:"120"`
	Description     NullableDescription `json:"description,omitempty"`
	AllowOverlap    bool                `json:"allow_overlap,omitempty"`
	MaterialIDs     []string            `json:"material_ids,omitempty" format:"uuid" nullable:"false" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
}

type NullableDescription struct {
	Set   bool
	Value *string
}

func (n *NullableDescription) UnmarshalJSON(data []byte) error {
	n.Set = true
	if bytes.Equal(data, []byte("null")) {
		n.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode a description: %w", err)
	}
	n.Value = &value
	return nil
}

func (NullableDescription) Schema(huma.Registry) *huma.Schema {
	maxLength := domain.LessonDescriptionMaxRunes
	return &huma.Schema{Type: huma.TypeString, MaxLength: &maxLength, Nullable: true}
}

type LessonCancellation struct {
	Scope   domain.EditScope `json:"scope" enum:"this,following"`
	Version int32            `json:"version" minimum:"1"`
}
