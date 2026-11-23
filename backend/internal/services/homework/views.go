package homework

import (
	"time"

	"ovc/backend/internal/domain"
)

type Audience struct {
	Relationship *domain.RelationshipID
	DisplayName  string
	LabelKind    *domain.LabelKind
	Group        *domain.GroupID
	Name         string
}

type Progress struct {
	Students      int64
	Submitted     int64
	Checked       int64
	Returned      int64
	Late          int64
	NeedsChecking int64
}

type StudentSubmission struct {
	ID                  domain.SubmissionID
	Status              domain.SubmissionStatus
	Late                bool
	FinalPoints         *int32
	IssuedBeforeReclaim bool
}

type IssueItem struct {
	ID                 domain.IssueID
	Title              string
	Audience           Audience
	Lesson             *domain.LessonID
	Deadline           *time.Time
	Policy             domain.AnswerPolicy
	Status             domain.IssueStatus
	IssuedAt           time.Time
	WithdrawnAt        *time.Time
	HasManualQuestions bool
	MaxPoints          int32
	Version            int64
	Progress           Progress
	Submission         *StudentSubmission
}

type IssueFile struct {
	Attachment domain.AttachmentID
	Filename   string
	MediaType  string
	SizeBytes  int64
	Width      *int32
	Height     *int32
	URL        string
	ExpiresAt  time.Time
}

type IssueMaterial struct {
	Title string
	File  IssueFile
}

type IssueBlock struct {
	domain.TemplateBlock
	Material *IssueMaterial
	Images   []IssueFile
}

type IssueSubmission struct {
	ID                  domain.SubmissionID
	Relationship        domain.RelationshipID
	DisplayName         string
	Status              domain.SubmissionStatus
	Late                bool
	FirstSubmittedAt    *time.Time
	SubmittedAt         *time.Time
	CheckedAt           *time.Time
	FinalPoints         *int32
	Percentage          *int64
	IssuedBeforeReclaim bool
	HasDraft            bool
}

type Issue struct {
	IssueItem
	Blocks      []IssueBlock
	Submissions []IssueSubmission
}

type IssueAudienceRequest struct {
	Relationship *domain.RelationshipID
	Group        *domain.GroupID
}

type NewIssue struct {
	Template  domain.TemplateID
	Audience  IssueAudienceRequest
	Lesson    *domain.LessonID
	Deadline  *time.Time
	Policy    domain.AnswerPolicy
	CreateKey *string
}

type OptionalLesson struct {
	Set   bool
	Value *domain.LessonID
}

type OptionalDeadline struct {
	Set   bool
	Value *time.Time
}

type IssueChange struct {
	Deadline OptionalDeadline
	Lesson   OptionalLesson
	Version  int64
}

type IssueListQuery struct {
	Status         domain.IssueStatus
	Group          *domain.GroupID
	Student        *domain.RelationshipID
	NeedsChecking  bool
	DeadlineBefore *time.Time
	Cursor         string
	Limit          int32
}

type IssuePage struct {
	Items      []IssueItem
	NextCursor *string
}
