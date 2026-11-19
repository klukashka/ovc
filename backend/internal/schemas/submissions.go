package schemas

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

var errSubmissionDetailEmpty = errors.New("a submission detail holds neither schema")

type StudentAssignmentTutor struct {
	RelationshipID string         `json:"relationship_id" format:"uuid"`
	Name           string         `json:"name"`
	Colour         *domain.Colour `json:"colour" nullable:"true"`
}

type StudentAssignmentItem struct {
	SubmissionID        string                  `json:"submission_id" format:"uuid"`
	Title               string                  `json:"title"`
	Tutor               StudentAssignmentTutor  `json:"tutor"`
	Status              domain.SubmissionStatus `json:"status" enum:"not_started,draft,submitted,returned,checked,closed"`
	Late                bool                    `json:"late"`
	IssuedBeforeReclaim bool                    `json:"issued_before_reclaim"`
	DeadlineAt          *time.Time              `json:"deadline_at" nullable:"true"`
	IssuedAt            time.Time               `json:"issued_at"`
	SubmittedAt         *time.Time              `json:"submitted_at" nullable:"true"`
	CheckedAt           *time.Time              `json:"checked_at" nullable:"true"`
	FeedbackExcerpt     *string                 `json:"feedback_excerpt" nullable:"true"`
	HasCorrectedFiles   bool                    `json:"has_corrected_files"`
	FeedbackNew         bool                    `json:"feedback_new"`
}

type StudentAssignmentCounts struct {
	Todo   int64 `json:"todo"`
	Closed int64 `json:"closed"`
}

type StudentAssignmentPage struct {
	Items      []StudentAssignmentItem `json:"items" nullable:"false"`
	NextCursor *string                 `json:"next_cursor" nullable:"true"`
	Counts     StudentAssignmentCounts `json:"counts"`
}

type StudentAnswerValue struct {
	OptionIDs     *[]string `json:"option_ids,omitempty" maxItems:"10"`
	Text          *string   `json:"text,omitempty" maxLength:"20000"`
	AttachmentIDs *[]string `json:"attachment_ids,omitempty" maxItems:"5"`
}

type SubmissionFile struct {
	AttachmentID string    `json:"attachment_id" format:"uuid"`
	Filename     string    `json:"filename"`
	MediaType    string    `json:"media_type"`
	SizeBytes    int64     `json:"size_bytes"`
	Width        *int32    `json:"width" nullable:"true"`
	Height       *int32    `json:"height" nullable:"true"`
	URL          string    `json:"url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type StudentAnswer struct {
	QuestionID string             `json:"question_id" format:"uuid"`
	Value      StudentAnswerValue `json:"value"`
	Files      []SubmissionFile   `json:"files" nullable:"false"`
}

type StudentRevealedQuestion struct {
	QuestionID       string    `json:"question_id" format:"uuid"`
	CorrectOptionIDs *[]string `json:"correct_option_ids,omitempty"`
	AcceptedAnswers  *[]string `json:"accepted_answers,omitempty"`
	CorrectValue     *string   `json:"correct_value,omitempty"`
	Tolerance        *string   `json:"tolerance,omitempty"`
	Explanation      *string   `json:"explanation,omitempty"`
}

type StudentRevealed struct {
	Questions []StudentRevealedQuestion `json:"questions" nullable:"false"`
}

type StudentReviewItem struct {
	QuestionID     string           `json:"question_id" format:"uuid"`
	Comment        string           `json:"comment"`
	CorrectedFiles []SubmissionFile `json:"corrected_files" nullable:"false"`
}

type StudentReview struct {
	Feedback string              `json:"feedback"`
	Items    []StudentReviewItem `json:"items" nullable:"false"`
}

type StudentSubmission struct {
	ID                  string                  `json:"id" format:"uuid"`
	Title               string                  `json:"title"`
	Tutor               StudentAssignmentTutor  `json:"tutor"`
	Status              domain.SubmissionStatus `json:"status" enum:"not_started,draft,submitted,returned,checked,closed"`
	Late                bool                    `json:"late"`
	IssuedBeforeReclaim bool                    `json:"issued_before_reclaim"`
	DeadlineAt          *time.Time              `json:"deadline_at" nullable:"true"`
	Version             int64                   `json:"version"`
	Blocks              []StudentTemplateBlock  `json:"blocks" nullable:"false"`
	Answers             []StudentAnswer         `json:"answers" nullable:"false"`
	EditableQuestionIDs []string                `json:"editable_question_ids" nullable:"false"`
	Review              *StudentReview          `json:"review"`
	Revealed            *StudentRevealed        `json:"revealed,omitempty"`
}

func (StudentSubmission) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	return nullableReferences(schema, "review")
}

type StudentAnswerInput struct {
	QuestionID string             `json:"question_id" maxLength:"64"`
	Value      StudentAnswerValue `json:"value"`
}

type StudentAnswersSave struct {
	Version int64                `json:"version" minimum:"1"`
	SaveID  string               `json:"save_id" format:"uuid" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$"`
	Answers []StudentAnswerInput `json:"answers" maxItems:"100" nullable:"false"`
}

type StudentSubmit struct {
	Version int64 `json:"version" minimum:"1"`
}

type StudentFeedbackSeen struct {
	Version int64 `json:"version" minimum:"0"`
}

type TutorSubmissionStudent struct {
	RelationshipID string `json:"relationship_id" format:"uuid"`
	DisplayName    string `json:"display_name"`
}

type TutorAnswer struct {
	QuestionID        string             `json:"question_id" format:"uuid"`
	Value             StudentAnswerValue `json:"value"`
	Normalised        *string            `json:"normalised" nullable:"true"`
	NormaliserVersion *int32             `json:"normaliser_version" nullable:"true"`
	AutoPoints        *int32             `json:"auto_points" nullable:"true"`
	AutoCorrect       *bool              `json:"auto_correct" nullable:"true"`
	FrozenAt          *time.Time         `json:"frozen_at" nullable:"true"`
	Files             []SubmissionFile   `json:"files" nullable:"false"`
}

type TutorReviewItem struct {
	QuestionID     string           `json:"question_id" format:"uuid"`
	Points         *int32           `json:"points" nullable:"true"`
	OverridePoints *int32           `json:"override_points" nullable:"true"`
	OverrideReason *string          `json:"override_reason" nullable:"true"`
	Comment        string           `json:"comment"`
	CorrectedFiles []SubmissionFile `json:"corrected_files" nullable:"false"`
}

type TutorReviewRevision struct {
	Revision    int32             `json:"revision"`
	State       string            `json:"state" enum:"draft,published"`
	Action      *string           `json:"action" nullable:"true" enum:"check,return"`
	Feedback    string            `json:"feedback"`
	Items       []TutorReviewItem `json:"items" nullable:"false"`
	Version     int64             `json:"version"`
	PublishedAt *time.Time        `json:"published_at" nullable:"true"`
}

type TutorSubmission struct {
	ID                  string                  `json:"id" format:"uuid"`
	IssueID             string                  `json:"issue_id" format:"uuid"`
	Title               string                  `json:"title"`
	Student             TutorSubmissionStudent  `json:"student"`
	Status              domain.SubmissionStatus `json:"status" enum:"not_started,draft,submitted,returned,checked,closed"`
	Late                bool                    `json:"late"`
	IssuedBeforeReclaim bool                    `json:"issued_before_reclaim"`
	DeadlineAt          *time.Time              `json:"deadline_at" nullable:"true"`
	FirstSubmittedAt    *time.Time              `json:"first_submitted_at" nullable:"true"`
	SubmittedAt         *time.Time              `json:"submitted_at" nullable:"true"`
	CheckedAt           *time.Time              `json:"checked_at" nullable:"true"`
	Version             int64                   `json:"version"`
	AnswerPolicy        domain.AnswerPolicy     `json:"answer_policy" enum:"never,after_submission,after_check"`
	Blocks              []IssueBlock            `json:"blocks" nullable:"false"`
	Answers             []TutorAnswer           `json:"answers" nullable:"false"`
	Revisions           []TutorReviewRevision   `json:"revisions" nullable:"false"`
	AutoPoints          *int32                  `json:"auto_points" nullable:"true"`
	FinalPoints         *int32                  `json:"final_points" nullable:"true"`
	MaxPoints           int32                   `json:"max_points"`
	Percentage          *int64                  `json:"percentage" nullable:"true"`
}

type SubmissionDetail struct {
	Student *StudentSubmission `json:"student,omitempty"`
	Tutor   *TutorSubmission   `json:"tutor,omitempty"`
}

func (d SubmissionDetail) MarshalJSON() ([]byte, error) {
	var encoded []byte
	var err error
	switch {
	case d.Student != nil:
		encoded, err = json.Marshal(d.Student)
	case d.Tutor != nil:
		encoded, err = json.Marshal(d.Tutor)
	default:
		return nil, errSubmissionDetailEmpty
	}
	if err != nil {
		return nil, fmt.Errorf("encode a submission detail: %w", err)
	}
	return encoded, nil
}

func (SubmissionDetail) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	student, tutor := schema.Properties["student"], schema.Properties["tutor"]
	if student == nil || tutor == nil || student.Ref == "" || tutor.Ref == "" {
		panic("a submission detail schema lost its references")
	}
	return &huma.Schema{OneOf: []*huma.Schema{{Ref: student.Ref}, {Ref: tutor.Ref}}}
}
