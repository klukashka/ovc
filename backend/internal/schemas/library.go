package schemas

import (
	"time"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

type LibraryFolder struct {
	ID       string  `json:"id" format:"uuid"`
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id" format:"uuid" nullable:"true"`
}

type LibraryFolderList struct {
	Folders []LibraryFolder `json:"folders" nullable:"false"`
}

type LibraryPathEntry struct {
	ID   string `json:"id" format:"uuid"`
	Name string `json:"name"`
}

type LibraryItem struct {
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
	URL           *string                `json:"url" nullable:"true"`
	ExpiresAt     *time.Time             `json:"expires_at" nullable:"true"`
	QuestionCount *int32                 `json:"question_count,omitempty"`
	MaxPoints     *int32                 `json:"max_points,omitempty"`
	Ready         *bool                  `json:"ready,omitempty"`
	Version       *int64                 `json:"version,omitempty"`
}

type Library struct {
	Folder        *LibraryFolder     `json:"folder"`
	Path          []LibraryPathEntry `json:"path" nullable:"false"`
	Folders       []LibraryFolder    `json:"folders" nullable:"false"`
	Items         []LibraryItem      `json:"items" nullable:"false"`
	NextCursor    *string            `json:"next_cursor" nullable:"true"`
	TemplateCount int64              `json:"template_count"`
}

func (Library) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "folder")
}

type NewLibraryFolder struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id" format:"uuid" nullable:"true"`
}

type LibraryFolderPatch struct {
	Name     *string    `json:"name,omitempty"`
	ParentID NullableID `json:"parent_id,omitempty"`
}

type NewLibraryMaterial struct {
	AttachmentID string  `json:"attachment_id" format:"uuid"`
	Title        string  `json:"title"`
	FolderID     *string `json:"folder_id" format:"uuid" nullable:"true"`
}

type LibraryMaterialPatch struct {
	Title    *string    `json:"title,omitempty"`
	FolderID NullableID `json:"folder_id,omitempty"`
}
