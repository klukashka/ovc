package domain

import (
	"errors"
	"time"
)

type AttachmentID [16]byte

type EntityID [16]byte

type ScopeKind string

const (
	ScopeChannel    ScopeKind = "channel"
	ScopeSubmission ScopeKind = "submission"
)

type AttachmentContext string

const (
	ContextAvatar      AttachmentContext = "avatar"
	ContextPromptImage AttachmentContext = "prompt_image"
	ContextLibrary     AttachmentContext = "library"
	ContextChat        AttachmentContext = "chat"
	ContextAnswer      AttachmentContext = "answer"
	ContextCorrected   AttachmentContext = "corrected"
)

type AttachmentState string

const (
	AttachmentUploading AttachmentState = "uploading"
	AttachmentPending   AttachmentState = "pending"
	AttachmentBound     AttachmentState = "bound"
	AttachmentDeleted   AttachmentState = "deleted"
)

type ReferenceKind string

const (
	ReferenceMessage       ReferenceKind = "message"
	ReferenceAnswer        ReferenceKind = "answer"
	ReferenceReviewItem    ReferenceKind = "review_item"
	ReferenceMaterial      ReferenceKind = "material"
	ReferenceTemplate      ReferenceKind = "template"
	ReferenceIssueSnapshot ReferenceKind = "issue_snapshot"
	ReferenceLesson        ReferenceKind = "lesson"
	ReferenceAvatar        ReferenceKind = "avatar"
)

var ErrInvalidAvatar = errors.New("the attachment cannot become this account's avatar")

type AttachmentPayload struct {
	Attachment string `json:"attachment"`
	Key        string `json:"key"`
	Upload     string `json:"upload,omitempty"`
}

const (
	AvatarMaxBytes       = 5_000_000
	AvatarMaxStoredBytes = 10_000_000
	AvatarMaxSide        = 4096
	AvatarKeyPrefix      = "avatars/"
)

const (
	ChatMaxBytes            = 25_000_000
	ChatWebPMaxBytes        = 8 << 20
	ChatDailyBytesPerTutor  = 200_000_000
	ChatOpenUploadsPerUser  = 20
	ChatUploadsPerHour      = 60
	ChatKeyPrefix           = "chat/"
	UploadPartBytes         = 8 << 20
	UploadOverheadBytes     = 64 << 10
	UploadPartHeaderBytes   = 8 << 10
	PreviewMaxPixels        = 50_000_000
	PendingUploadLifetime   = 24 * time.Hour
	UploadDeadlineCeiling   = 10 * time.Minute
	UploadAgeAfterDeadline  = 10 * time.Minute
	DownloadAddressWindow   = 5 * time.Minute
	DownloadAddressMaxAhead = 10*time.Minute + 30*time.Second
)

type Disposition string

const (
	DispositionAttachment Disposition = "attachment"
	DispositionInline     Disposition = "inline"
)

type UploadRejection string

const (
	RejectedBusy           UploadRejection = "busy"
	RejectedTooLarge       UploadRejection = "too_large"
	RejectedUnsupported    UploadRejection = "unsupported"
	RejectedSessionRevoked UploadRejection = "session_revoked"
	RejectedScope          UploadRejection = "scope"
	RejectedReadOnly       UploadRejection = "read_only"
	RejectedQuota          UploadRejection = "quota"
	RejectedDailyCap       UploadRejection = "daily_cap"
	RejectedPendingCap     UploadRejection = "pending_cap"
	RejectedHourlyCap      UploadRejection = "hourly_cap"
	RejectedContainer      UploadRejection = "container"
	RejectedFilename       UploadRejection = "filename"
	RejectedParts          UploadRejection = "parts"
	RejectedStalled        UploadRejection = "stalled"
)

type ChatLimit string

const (
	LimitAttachmentBytes ChatLimit = "attachment_bytes"
	LimitPendingUploads  ChatLimit = "pending_uploads"
	LimitHourlyUploads   ChatLimit = "hourly_uploads"
)

type DownloadRefusal string

const (
	RefusedToken        DownloadRefusal = "token"
	RefusedExpired      DownloadRefusal = "expired"
	RefusedUnreferenced DownloadRefusal = "unreferenced"
)

const FieldWebPTooLarge FieldCode = "webp_too_large"
