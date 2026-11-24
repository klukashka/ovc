package schemas

import (
	"time"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

type ExportRequest struct {
	Password string `json:"password" maxLength:"512"`
}

type ExportProgress struct {
	Done  int32 `json:"done"`
	Total int32 `json:"total"`
}

type Export struct {
	ID            string                `json:"id"`
	State         domain.ExportState    `json:"state" enum:"queued,building,ready,failed,expired"`
	Failure       *domain.ExportFailure `json:"failure" enum:"error,too_long,too_large,withdrawn" nullable:"true"`
	RequestedAt   time.Time             `json:"requested_at"`
	BuiltAt       *time.Time            `json:"built_at" nullable:"true"`
	ExpiresAt     *time.Time            `json:"expires_at" nullable:"true"`
	SizeBytes     *int64                `json:"size_bytes" nullable:"true"`
	Progress      *ExportProgress       `json:"progress"`
	NextAllowedAt *time.Time            `json:"next_allowed_at" nullable:"true"`
}

func (Export) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "progress")
}

type CurrentExport struct {
	Export        *Export    `json:"export"`
	NextAllowedAt *time.Time `json:"next_allowed_at" nullable:"true"`
}

func (CurrentExport) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "export")
}

type ExportDownload struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}
