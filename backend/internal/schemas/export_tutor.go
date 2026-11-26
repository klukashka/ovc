package schemas

import (
	"time"

	"ovc/backend/internal/domain"
)

type TutorExportFile struct {
	AttachmentID string  `json:"attachment_id" format:"uuid"`
	Filename     string  `json:"filename"`
	MediaType    string  `json:"media_type"`
	SizeBytes    int64   `json:"size_bytes"`
	Width        *int32  `json:"width" nullable:"true"`
	Height       *int32  `json:"height" nullable:"true"`
	Path         *string `json:"path" nullable:"true"`
}

type TutorExportCardInvitation struct {
	ExpiresAt time.Time               `json:"expires_at"`
	Status    domain.InvitationStatus `json:"status" enum:"open,expired,revoked_after_mismatches"`
}

type TutorExportCard struct {
	StudentCard
	Invitation *TutorExportCardInvitation `json:"invitation"`
	ChannelID  *string                    `json:"channel_id" format:"uuid" nullable:"true"`
}

type TutorExportGroup struct {
	Group
	ChannelID *string `json:"channel_id" format:"uuid" nullable:"true"`
}

type TutorExportLessonMaterial struct {
	MaterialID   *string `json:"material_id,omitempty" format:"uuid"`
	AttachmentID string  `json:"attachment_id" format:"uuid"`
	Title        string  `json:"title"`
	Filename     string  `json:"filename"`
	MediaType    string  `json:"media_type"`
	SizeBytes    int64   `json:"size_bytes"`
	Width        *int32  `json:"width" nullable:"true"`
	Height       *int32  `json:"height" nullable:"true"`
	Path         *string `json:"path" nullable:"true"`
}

type TutorExportLesson struct {
	LessonDetail
	Materials []TutorExportLessonMaterial `json:"materials" nullable:"false"`
}

type TutorExportTemplateFile struct {
	AttachmentID string  `json:"attachment_id" format:"uuid"`
	Filename     string  `json:"filename"`
	MediaType    string  `json:"media_type"`
	SizeBytes    int64   `json:"size_bytes"`
	Width        *int32  `json:"width" nullable:"true"`
	Height       *int32  `json:"height" nullable:"true"`
	Path         *string `json:"path" nullable:"true"`
}

type TutorExportIssueMaterial struct {
	AttachmentID string  `json:"attachment_id" format:"uuid"`
	Title        string  `json:"title"`
	Filename     string  `json:"filename"`
	MediaType    string  `json:"media_type"`
	SizeBytes    int64   `json:"size_bytes"`
	Width        *int32  `json:"width" nullable:"true"`
	Height       *int32  `json:"height" nullable:"true"`
	Path         *string `json:"path" nullable:"true"`
}

type TutorExportIssueBlock struct {
	IssueBlock
	Material *TutorExportIssueMaterial  `json:"material,omitempty"`
	Images   *[]TutorExportTemplateFile `json:"images,omitempty"`
}

type TutorExportIssue struct {
	Issue
	Blocks []TutorExportIssueBlock `json:"blocks" nullable:"false"`
}

type TutorExportAnswer struct {
	TutorAnswer
	Files []TutorExportFile `json:"files" nullable:"false"`
}

type TutorExportReviewItem struct {
	TutorReviewItem
	CorrectedFiles []TutorExportFile `json:"corrected_files" nullable:"false"`
}

type TutorExportRevision struct {
	TutorReviewRevision
	Items []TutorExportReviewItem `json:"items" nullable:"false"`
}

type TutorExportSubmission struct {
	TutorSubmission
	Blocks    []TutorExportIssueBlock `json:"blocks" nullable:"false"`
	Answers   []TutorExportAnswer     `json:"answers" nullable:"false"`
	Revisions []TutorExportRevision   `json:"revisions" nullable:"false"`
}

type TutorExportLibraryItem struct {
	Kind          domain.LibraryItemKind `json:"kind" enum:"material,template"`
	ID            string                 `json:"id" format:"uuid"`
	Title         string                 `json:"title"`
	FolderID      *string                `json:"folder_id" format:"uuid" nullable:"true"`
	Path          []LibraryPathEntry     `json:"path" nullable:"true"`
	UpdatedAt     time.Time              `json:"updated_at"`
	AttachmentID  *string                `json:"attachment_id,omitempty" format:"uuid"`
	Filename      *string                `json:"filename,omitempty"`
	MediaType     *string                `json:"media_type,omitempty"`
	SizeBytes     *int64                 `json:"size_bytes,omitempty"`
	Width         *int32                 `json:"width" nullable:"true"`
	Height        *int32                 `json:"height" nullable:"true"`
	FilePath      *string                `json:"file_path" nullable:"true"`
	QuestionCount *int32                 `json:"question_count,omitempty"`
	MaxPoints     *int32                 `json:"max_points,omitempty"`
	Ready         *bool                  `json:"ready,omitempty"`
	Version       *int64                 `json:"version,omitempty"`
}

type TutorExportTemplateMaterial struct {
	MaterialID   string  `json:"material_id" format:"uuid"`
	AttachmentID string  `json:"attachment_id" format:"uuid"`
	Title        string  `json:"title"`
	Filename     string  `json:"filename"`
	MediaType    string  `json:"media_type"`
	SizeBytes    int64   `json:"size_bytes"`
	Width        *int32  `json:"width" nullable:"true"`
	Height       *int32  `json:"height" nullable:"true"`
	Deleted      bool    `json:"deleted"`
	Path         *string `json:"path" nullable:"true"`
}

type TutorExportTemplateBlock struct {
	TemplateBlock
	Material *TutorExportTemplateMaterial `json:"material,omitempty"`
	Images   *[]TutorExportTemplateFile   `json:"images,omitempty"`
}

type TutorExportTemplate struct {
	Template
	Blocks []TutorExportTemplateBlock `json:"blocks" nullable:"false"`
}

type TutorExportMessageAttachment struct {
	ID        string  `json:"id" format:"uuid"`
	Filename  string  `json:"filename"`
	MediaType string  `json:"media_type"`
	SizeBytes int64   `json:"size_bytes"`
	Width     *int32  `json:"width" nullable:"true"`
	Height    *int32  `json:"height" nullable:"true"`
	Path      *string `json:"path" nullable:"true"`
}

type TutorExportMessage struct {
	Message
	Attachments []TutorExportMessageAttachment `json:"attachments" nullable:"false"`
}

type TutorExportChannelHistory struct {
	Channel  ChannelItem          `json:"channel"`
	Messages []TutorExportMessage `json:"messages" nullable:"false"`
}

type TutorExportFileEntry struct {
	Path       string   `json:"path"`
	Filename   string   `json:"filename"`
	MediaType  string   `json:"media_type"`
	SizeBytes  int64    `json:"size_bytes"`
	SHA256     string   `json:"sha256"`
	References []string `json:"references" nullable:"false"`
}
