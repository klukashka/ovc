package schemas

import "time"

type ReviewQueueItem struct {
	SubmissionID        string                 `json:"submission_id" format:"uuid"`
	IssueID             string                 `json:"issue_id" format:"uuid"`
	Title               string                 `json:"title"`
	Student             TutorSubmissionStudent `json:"student"`
	SubmittedAt         *time.Time             `json:"submitted_at" nullable:"true"`
	FirstSubmittedAt    *time.Time             `json:"first_submitted_at" nullable:"true"`
	Late                bool                   `json:"late"`
	HasDraft            bool                   `json:"has_draft"`
	IssuedBeforeReclaim bool                   `json:"issued_before_reclaim"`
}

type ReviewQueuePage struct {
	Items      []ReviewQueueItem `json:"items" nullable:"false"`
	NextCursor *string           `json:"next_cursor" nullable:"true"`
	Count      int64             `json:"count"`
}

type ReviewItemInput struct {
	QuestionID             string   `json:"question_id" maxLength:"64"`
	Points                 *int32   `json:"points,omitempty" nullable:"true"`
	OverridePoints         *int32   `json:"override_points,omitempty" nullable:"true"`
	OverrideReason         *string  `json:"override_reason,omitempty" nullable:"true" maxLength:"2000"`
	Comment                string   `json:"comment,omitempty" maxLength:"40000"`
	CorrectedAttachmentIDs []string `json:"corrected_attachment_ids,omitempty" maxItems:"20" nullable:"false"`
}

type ReviewSave struct {
	Revision int32             `json:"revision" minimum:"1" maximum:"100"`
	Version  int64             `json:"version" minimum:"0"`
	SaveID   string            `json:"save_id" format:"uuid" pattern:"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$"`
	Feedback string            `json:"feedback" maxLength:"40000"`
	Items    []ReviewItemInput `json:"items" maxItems:"100" nullable:"false"`
}

type ReviewPublish struct {
	Revision int32 `json:"revision" minimum:"1" maximum:"100"`
}
