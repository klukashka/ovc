package domain

import (
	"encoding/hex"
	"fmt"
	"time"
)

type IssueID [16]byte

type SubmissionID [16]byte

type QuestionID [16]byte

func (id IssueID) String() string {
	return formatUUID(id)
}

func (id SubmissionID) String() string {
	return formatUUID(id)
}

func (id QuestionID) String() string {
	return formatUUID(id)
}

type IssueStatus string

const (
	IssueActive    IssueStatus = "active"
	IssueWithdrawn IssueStatus = "withdrawn"
)

type IssueAudienceKind string

const (
	IssueAudienceRelationship IssueAudienceKind = "relationship"
	IssueAudienceGroup        IssueAudienceKind = "group"
)

type IssueChange string

const (
	IssueChangeDeadline IssueChange = "deadline"
	IssueChangeLesson   IssueChange = "lesson"
)

const (
	IssuesPerHour              = 100
	HomeworkWritesPerHour      = 600
	IssueSnapshotBudgetBytes   = 512 << 20
	IssuesPerLesson            = 20
	IssueWindow                = time.Hour
	IssuePageDefault           = 30
	IssuePageMaximum           = 100
	HomeworkEmailsPerDay       = 20
	SubmissionEmailsPerDay     = 50
	PurposeIssuesHour          = RateLimitPurpose("issues_hour")
	PurposeHomeworkWritesHour  = RateLimitPurpose("homework_writes_hour")
	PurposeHomeworkEmails      = RateLimitPurpose("homework_emails")
	PurposeSubmissionEmails    = RateLimitPurpose("submission_emails")
	EventIssueCreated          = OutboxKind("issue.created")
	HomeworkAssignmentPagePath = "/homework/"
	HomeworkReviewPagePath     = "/homework/review/"
)

type HomeworkCaps struct {
	IssuesPerHour       int64
	WritesPerHour       int64
	SnapshotBudgetBytes int64
	IssuesPerLesson     int64
	SubmissionFileBytes int64
	RevisionsPerReview  int64
	ReviewQueueCount    int32
}

func (c HomeworkCaps) WithDefaults() HomeworkCaps {
	if c.IssuesPerHour <= 0 {
		c.IssuesPerHour = IssuesPerHour
	}
	if c.WritesPerHour <= 0 {
		c.WritesPerHour = HomeworkWritesPerHour
	}
	if c.SnapshotBudgetBytes <= 0 {
		c.SnapshotBudgetBytes = IssueSnapshotBudgetBytes
	}
	if c.IssuesPerLesson <= 0 {
		c.IssuesPerLesson = IssuesPerLesson
	}
	if c.SubmissionFileBytes <= 0 {
		c.SubmissionFileBytes = SubmissionFileBytesCap
	}
	if c.RevisionsPerReview <= 0 {
		c.RevisionsPerReview = ReviewRevisionsPerSubmission
	}
	if c.ReviewQueueCount <= 0 {
		c.ReviewQueueCount = ReviewQueueCountCap
	}
	return c
}

type IssueMaterial struct {
	Title        string `json:"title"`
	AttachmentID string `json:"attachment_id"`
}

type IssueBlock struct {
	TemplateBlock
	Material *IssueMaterial `json:"material,omitempty"`
}

func ReadableStudentSubmission(status IssueStatus, markedForErasure bool, relationship RelationshipState) bool {
	if status != IssueActive || markedForErasure {
		return false
	}
	switch relationship {
	case RelationshipActive, RelationshipProvisional, RelationshipEnded:
		return true
	case RelationshipInvited, RelationshipReclaimPending, RelationshipDiscarded:
		return false
	}
	return false
}

type TutorStanding struct {
	State     AccountState
	DeletedAt *time.Time
}

func (t TutorStanding) ReadableAt(now time.Time) bool {
	if t.State != AccountDeleted {
		return true
	}
	return t.DeletedAt != nil && now.Before(t.DeletedAt.Add(DeletedAccountRetention))
}

func ReadableTutorWorkCutoff(now time.Time) time.Time {
	return now.Add(-DeletedAccountRetention)
}

func ReadableStudentSubmissionAt(
	status IssueStatus, markedForErasure bool, relationship RelationshipState, tutor TutorStanding, now time.Time,
) bool {
	return ReadableStudentSubmission(status, markedForErasure, relationship) && tutor.ReadableAt(now)
}

func WritableStudentSubmission(status SubmissionStatus, relationship RelationshipState) bool {
	if relationship != RelationshipActive && relationship != RelationshipProvisional {
		return false
	}
	return status == SubmissionNotStarted || status == SubmissionDraft || status == SubmissionReturned
}

func ReviewableSubmission(issue IssueStatus, markedForErasure bool, status SubmissionStatus) bool {
	return issue == IssueActive && !markedForErasure && (status == SubmissionSubmitted || status == SubmissionChecked)
}

func IssuableRelationship(state RelationshipState) bool {
	return state == RelationshipActive || state == RelationshipProvisional
}

type IssueCreatedPayload struct {
	Submission string `json:"submission"`
}

func IssueCreatedKey(submission SubmissionID) string {
	return fmt.Sprintf("%s:%s", NotificationIssueCreated, hex.EncodeToString(submission[:]))
}

func IssueWithdrawnKey(submission SubmissionID) string {
	return fmt.Sprintf("%s:%s", NotificationIssueWithdrawn, hex.EncodeToString(submission[:]))
}

func DailyEmailCap(group EmailGroup) (RateLimitPurpose, int32) {
	switch group {
	case EmailGroupHomework, EmailGroupResults:
		return PurposeHomeworkEmails, HomeworkEmailsPerDay
	case EmailGroupSubmissions:
		return PurposeSubmissionEmails, SubmissionEmailsPerDay
	case EmailGroupLessonReminders, EmailGroupLessonChanges, EmailGroupMessageDigest:
		return PurposeLessonEmails, LessonEmailsPerDay
	}
	return PurposeLessonEmails, LessonEmailsPerDay
}
