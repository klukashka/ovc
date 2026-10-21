package domain

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
	AttachmentPending AttachmentState = "pending"
	AttachmentBound   AttachmentState = "bound"
	AttachmentDeleted AttachmentState = "deleted"
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
