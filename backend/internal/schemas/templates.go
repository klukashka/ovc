package schemas

import (
	"time"

	"ovc/backend/internal/domain"
)

type TemplateOption struct {
	ID      string `json:"id" format:"uuid"`
	Text    string `json:"text"`
	Correct *bool  `json:"correct,omitempty"`
}

type TemplateBlockInput struct {
	ID              string                   `json:"id" format:"uuid"`
	Type            domain.BlockType         `json:"type" enum:"instruction,material,single_choice,multiple_choice,short_text,short_number,long_text,file"`
	Text            *string                  `json:"text,omitempty"`
	MaterialID      *string                  `json:"material_id,omitempty" format:"uuid"`
	Prompt          *string                  `json:"prompt,omitempty"`
	ImageIDs        *[]string                `json:"image_ids,omitempty"`
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

type TemplateFile struct {
	AttachmentID string    `json:"attachment_id" format:"uuid"`
	Filename     string    `json:"filename"`
	MediaType    string    `json:"media_type"`
	SizeBytes    int64     `json:"size_bytes"`
	Width        *int32    `json:"width" nullable:"true"`
	Height       *int32    `json:"height" nullable:"true"`
	URL          string    `json:"url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type TemplateMaterial struct {
	MaterialID   string    `json:"material_id" format:"uuid"`
	AttachmentID string    `json:"attachment_id" format:"uuid"`
	Title        string    `json:"title"`
	Filename     string    `json:"filename"`
	MediaType    string    `json:"media_type"`
	SizeBytes    int64     `json:"size_bytes"`
	Width        *int32    `json:"width" nullable:"true"`
	Height       *int32    `json:"height" nullable:"true"`
	Deleted      bool      `json:"deleted"`
	URL          string    `json:"url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type TemplateBlock struct {
	ID              string                   `json:"id" format:"uuid"`
	Type            domain.BlockType         `json:"type" enum:"instruction,material,single_choice,multiple_choice,short_text,short_number,long_text,file"`
	Text            *string                  `json:"text,omitempty"`
	MaterialID      *string                  `json:"material_id,omitempty" format:"uuid"`
	Material        *TemplateMaterial        `json:"material,omitempty"`
	Prompt          *string                  `json:"prompt,omitempty"`
	ImageIDs        *[]string                `json:"image_ids,omitempty"`
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

type TemplateProblem struct {
	BlockID *string            `json:"block_id" format:"uuid" nullable:"true"`
	Code    domain.ProblemCode `json:"code" enum:"no_blocks,text_missing,prompt_missing,too_few_options,option_empty,correct_count,no_correct_option,no_accepted_answer,accepted_answer_empty,value_invalid,tolerance_invalid"`
}

type Template struct {
	ID            string            `json:"id" format:"uuid"`
	Title         string            `json:"title"`
	FolderID      *string           `json:"folder_id" format:"uuid" nullable:"true"`
	Blocks        []TemplateBlock   `json:"blocks" nullable:"false"`
	Version       int64             `json:"version"`
	Ready         bool              `json:"ready"`
	Problems      []TemplateProblem `json:"problems" nullable:"false"`
	QuestionCount int32             `json:"question_count"`
	MaxPoints     int32             `json:"max_points"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type NewTemplate struct {
	Title    string               `json:"title"`
	FolderID *string              `json:"folder_id" format:"uuid" nullable:"true" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
	Blocks   []TemplateBlockInput `json:"blocks" nullable:"false"`
}

type TemplateSave struct {
	Title   string               `json:"title"`
	Blocks  []TemplateBlockInput `json:"blocks" nullable:"false"`
	Version int64                `json:"version" minimum:"1"`
	SaveID  string               `json:"save_id" format:"uuid" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
}

type TemplateMove struct {
	FolderID *string `json:"folder_id" format:"uuid" nullable:"true" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
	Version  int64   `json:"version" minimum:"1"`
}

type TemplateDuplicate struct {
	Title    string  `json:"title"`
	FolderID *string `json:"folder_id" format:"uuid" nullable:"true" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"`
}

type StudentTemplateFile struct {
	Filename  string    `json:"filename"`
	MediaType string    `json:"media_type"`
	SizeBytes int64     `json:"size_bytes"`
	Width     *int32    `json:"width" nullable:"true"`
	Height    *int32    `json:"height" nullable:"true"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type StudentTemplateMaterial struct {
	Title     string    `json:"title"`
	Filename  string    `json:"filename"`
	MediaType string    `json:"media_type"`
	SizeBytes int64     `json:"size_bytes"`
	Width     *int32    `json:"width" nullable:"true"`
	Height    *int32    `json:"height" nullable:"true"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type StudentTemplateOption struct {
	ID   string `json:"id" format:"uuid"`
	Text string `json:"text"`
}

type StudentTemplateBlock struct {
	ID           string                   `json:"id" format:"uuid"`
	Type         domain.BlockType         `json:"type" enum:"instruction,material,single_choice,multiple_choice,short_text,short_number,long_text,file"`
	Text         *string                  `json:"text,omitempty"`
	Prompt       *string                  `json:"prompt,omitempty"`
	Options      *[]StudentTemplateOption `json:"options,omitempty"`
	MaxLength    *int32                   `json:"max_length,omitempty"`
	AllowedKinds *[]domain.AnswerFileKind `json:"allowed_kinds,omitempty"`
	MaxFiles     *int32                   `json:"max_files,omitempty"`
	Material     *StudentTemplateMaterial `json:"material,omitempty"`
	Images       *[]StudentTemplateFile   `json:"images,omitempty"`
}

type StudentTemplate struct {
	Title  string                 `json:"title"`
	Blocks []StudentTemplateBlock `json:"blocks" nullable:"false"`
}
