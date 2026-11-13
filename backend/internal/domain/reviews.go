package domain

import (
	"encoding/hex"
	"fmt"
	"slices"
	"time"
)

type ReviewRevisionID [16]byte

func (id ReviewRevisionID) String() string {
	return formatUUID(id)
}

type ReviewState string

const (
	ReviewDraft     ReviewState = "draft"
	ReviewPublished ReviewState = "published"
)

type ReviewAction string

const (
	ReviewCheck  ReviewAction = "check"
	ReviewReturn ReviewAction = "return"
)

const (
	ReviewFeedbackMaxRunes       = 10_000
	ReviewCommentMaxRunes        = 10_000
	OverrideReasonMaxRunes       = 500
	CorrectedFilesPerAnswer      = 5
	ReviewRevisionsPerSubmission = 100
	ReviewSavesPerSecond         = 2
	ReviewSavesPerHour           = 3_600
	ReviewSaveSecondWindow       = time.Second
	ReviewSaveHourWindow         = time.Hour
	ReviewQueueCountCap          = 100
	PurposeReviewSavesSecond     = RateLimitPurpose("review_saves_second")
	PurposeReviewSavesHour       = RateLimitPurpose("review_saves_hour")
	EventSubmissionReturned      = OutboxKind("submission.returned")
	EventReviewUpdated           = OutboxKind("submission.review_updated")
)

type ReviewItem struct {
	QuestionID             string   `json:"question_id"`
	Points                 *int32   `json:"points"`
	OverridePoints         *int32   `json:"override_points"`
	OverrideReason         *string  `json:"override_reason"`
	Comment                string   `json:"comment"`
	CorrectedAttachmentIDs []string `json:"corrected_attachment_ids"`
}

type VisibleReview struct {
	Feedback string
	Items    []ReviewItem
}

func (r VisibleReview) Shown() []ReviewItem {
	shown := []ReviewItem{}
	for _, item := range r.Items {
		if item.Comment != "" || len(item.CorrectedAttachmentIDs) > 0 {
			shown = append(shown, item)
		}
	}
	return shown
}

func (r VisibleReview) Empty() bool {
	return r.Feedback == "" && len(r.Shown()) == 0
}

func ReviewVisiblyChanged(previous VisibleReview, next VisibleReview) bool {
	if previous.Feedback != next.Feedback {
		return true
	}
	before, after := visibleItems(previous), visibleItems(next)
	if len(before) != len(after) {
		return true
	}
	for question, item := range after {
		earlier, found := before[question]
		if !found || earlier.comment != item.comment || !slices.Equal(earlier.files, item.files) {
			return true
		}
	}
	return false
}

type visibleItem struct {
	comment string
	files   []string
}

func visibleItems(review VisibleReview) map[string]visibleItem {
	items := map[string]visibleItem{}
	for _, item := range review.Shown() {
		files := slices.Clone(item.CorrectedAttachmentIDs)
		slices.Sort(files)
		items[item.QuestionID] = visibleItem{comment: item.Comment, files: files}
	}
	return items
}

func SubmissionReturnedKey(submission SubmissionID, revision int32) string {
	return fmt.Sprintf("%s:%s:%d", NotificationSubmissionReturned, hex.EncodeToString(submission[:]), revision)
}

func ReviewUpdatedKey(submission SubmissionID, revision int32) string {
	return fmt.Sprintf("%s:%s:%d", NotificationReviewUpdated, hex.EncodeToString(submission[:]), revision)
}
