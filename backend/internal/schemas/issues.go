package schemas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

type NullableInstant struct {
	Set   bool
	Value *time.Time
}

func (n *NullableInstant) UnmarshalJSON(data []byte) error {
	n.Set = true
	if bytes.Equal(data, []byte("null")) {
		n.Value = nil
		return nil
	}
	var value time.Time
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode an instant: %w", err)
	}
	n.Value = &value
	return nil
}

func (NullableInstant) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: huma.TypeString, Format: "date-time", Nullable: true}
}

type IssueAudienceInput struct {
	RelationshipID *string `json:"relationship_id,omitempty" format:"uuid" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
	GroupID        *string `json:"group_id,omitempty" format:"uuid" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
}

type NewIssue struct {
	TemplateID   string              `json:"template_id" format:"uuid" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
	Audience     IssueAudienceInput  `json:"audience"`
	LessonID     *string             `json:"lesson_id" format:"uuid" nullable:"true" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
	DeadlineAt   *time.Time          `json:"deadline_at" nullable:"true"`
	AnswerPolicy domain.AnswerPolicy `json:"answer_policy" enum:"never,after_submission,after_check"`
}

type IssuePatch struct {
	DeadlineAt NullableInstant  `json:"deadline_at,omitempty"`
	LessonID   StrictNullableID `json:"lesson_id,omitempty"`
	Version    int64            `json:"version" minimum:"1"`
}

type IssueAudience struct {
	RelationshipID *string `json:"relationship_id,omitempty" format:"uuid"`
	DisplayName    *string `json:"display_name,omitempty"`
	GroupID        *string `json:"group_id,omitempty" format:"uuid"`
	Name           *string `json:"name,omitempty"`
}

type IssueProgress struct {
	Students      int64 `json:"students"`
	Submitted     int64 `json:"submitted"`
	Checked       int64 `json:"checked"`
	Returned      int64 `json:"returned"`
	Late          int64 `json:"late"`
	NeedsChecking int64 `json:"needs_checking"`
}

type IssueStudentSubmission struct {
	ID                  string                  `json:"id" format:"uuid"`
	Status              domain.SubmissionStatus `json:"status" enum:"not_started,draft,submitted,returned,checked,closed"`
	Late                bool                    `json:"late"`
	FinalPoints         *int32                  `json:"final_points" nullable:"true"`
	IssuedBeforeReclaim bool                    `json:"issued_before_reclaim"`
}

type IssueItem struct {
	ID                 string                  `json:"id" format:"uuid"`
	Title              string                  `json:"title"`
	Audience           IssueAudience           `json:"audience"`
	LessonID           *string                 `json:"lesson_id" format:"uuid" nullable:"true"`
	DeadlineAt         *time.Time              `json:"deadline_at" nullable:"true"`
	AnswerPolicy       domain.AnswerPolicy     `json:"answer_policy" enum:"never,after_submission,after_check"`
	Status             domain.IssueStatus      `json:"status" enum:"active,withdrawn"`
	IssuedAt           time.Time               `json:"issued_at"`
	WithdrawnAt        *time.Time              `json:"withdrawn_at" nullable:"true"`
	HasManualQuestions bool                    `json:"has_manual_questions"`
	MaxPoints          int32                   `json:"max_points"`
	Version            int64                   `json:"version"`
	Progress           IssueProgress           `json:"progress"`
	Submission         *IssueStudentSubmission `json:"submission,omitempty"`
}

type IssuePage struct {
	Items      []IssueItem `json:"items" nullable:"false"`
	NextCursor *string     `json:"next_cursor" nullable:"true"`
}

type IssueMaterial struct {
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

type IssueBlock struct {
	ID              string                   `json:"id" format:"uuid"`
	Type            domain.BlockType         `json:"type" enum:"instruction,material,single_choice,multiple_choice,short_text,short_number,long_text,file"`
	Text            *string                  `json:"text,omitempty"`
	Material        *IssueMaterial           `json:"material,omitempty"`
	Prompt          *string                  `json:"prompt,omitempty"`
	Images          *[]TemplateFile          `json:"images,omitempty"`
	Options         *[]TemplateOption        `json:"options,omitempty"`
	Scoring         *domain.ScoringMode      `json:"scoring,omitempty" enum:"all_or_nothing,partial"`
	AcceptedAnswers *[]string                `json:"accepted_answers,omitempty"`
	Value           *string                  `json:"value,omitempty"`
	Tolerance       *string                  `json:"tolerance,omitempty"`
	MaxLength       *int32                   `json:"max_length,omitempty"`
	AllowedKinds    *[]domain.AnswerFileKind `json:"allowed_kinds,omitempty"`
	MaxFiles        *int32                   `json:"max_files,omitempty"`
	Points          *int32                   `json:"points,omitempty"`
	Explanation     *string                  `json:"explanation,omitempty"`
}

type IssueSubmissionStudent struct {
	RelationshipID string `json:"relationship_id" format:"uuid"`
	DisplayName    string `json:"display_name"`
}

type IssueSubmission struct {
	ID                  string                  `json:"id" format:"uuid"`
	Student             IssueSubmissionStudent  `json:"student"`
	Status              domain.SubmissionStatus `json:"status" enum:"not_started,draft,submitted,returned,checked,closed"`
	Late                bool                    `json:"late"`
	FirstSubmittedAt    *time.Time              `json:"first_submitted_at" nullable:"true"`
	SubmittedAt         *time.Time              `json:"submitted_at" nullable:"true"`
	CheckedAt           *time.Time              `json:"checked_at" nullable:"true"`
	FinalPoints         *int32                  `json:"final_points" nullable:"true"`
	Percentage          *int64                  `json:"percentage" nullable:"true"`
	IssuedBeforeReclaim bool                    `json:"issued_before_reclaim"`
	HasDraft            bool                    `json:"has_draft"`
}

type Issue struct {
	IssueItem
	Blocks      []IssueBlock      `json:"blocks" nullable:"false"`
	Submissions []IssueSubmission `json:"submissions" nullable:"false"`
}
