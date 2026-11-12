package domain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	AnswerSavesPerSecond       = 2
	AnswerSavesPerHour         = 3_600
	AnswerSaveSecondWindow     = time.Second
	AnswerSaveHourWindow       = time.Hour
	SubmissionFileBytesCap     = 250_000_000
	AnswerShortTextMaxRunes    = 500
	AnswerNumberMaxRunes       = 64
	AssignmentCountCap         = 100
	HomeworkCleanupBudget      = 20 * time.Second
	HomeworkCleanupBatch       = 500
	PurposeAnswerSavesSecond   = RateLimitPurpose("answer_saves_second")
	PurposeAnswerSavesHour     = RateLimitPurpose("answer_saves_hour")
	JobHomeworkCleanup         = JobName("homework.cleanup")
	EventSubmissionSubmitted   = OutboxKind("submission.submitted")
	EventSubmissionChecked     = OutboxKind("submission.checked")
	AutomaticReviewRevision    = 0
	SubmissionCloseCauseEnding = "ending"
)

var ErrAnswerShapeInvalid = errors.New("the answer value does not fit the question type")

type AnswerValue struct {
	OptionIDs     *[]string `json:"option_ids,omitempty"`
	Text          *string   `json:"text,omitempty"`
	AttachmentIDs *[]string `json:"attachment_ids,omitempty"`
}

type AssignmentGroup string

const (
	AssignmentsTodo      AssignmentGroup = "todo"
	AssignmentsSubmitted AssignmentGroup = "submitted"
	AssignmentsChecked   AssignmentGroup = "checked"
	AssignmentsClosed    AssignmentGroup = "closed"
)

func (v AnswerValue) FitsQuestion(question BlockType) bool {
	switch question {
	case BlockSingleChoice, BlockMultipleChoice:
		return v.OptionIDs != nil && v.Text == nil && v.AttachmentIDs == nil
	case BlockShortText, BlockShortNumber, BlockLongText:
		return v.Text != nil && v.OptionIDs == nil && v.AttachmentIDs == nil
	case BlockFile:
		return v.AttachmentIDs != nil && v.OptionIDs == nil && v.Text == nil
	case BlockInstruction, BlockMaterial:
		return false
	}
	return false
}

func (v AnswerValue) Answered() bool {
	switch {
	case v.OptionIDs != nil:
		return len(*v.OptionIDs) > 0
	case v.Text != nil:
		return *v.Text != ""
	case v.AttachmentIDs != nil:
		return len(*v.AttachmentIDs) > 0
	}
	return false
}

func EditableQuestions(status SubmissionStatus, blocks []IssueBlock) []string {
	editable := []string{}
	for _, block := range blocks {
		if !block.Type.Question() {
			continue
		}
		switch status {
		case SubmissionNotStarted, SubmissionDraft:
			editable = append(editable, block.ID)
		case SubmissionReturned:
			if block.Type.Manual() {
				editable = append(editable, block.ID)
			}
		case SubmissionSubmitted, SubmissionChecked, SubmissionClosed:
		}
	}
	return editable
}

type StoredScore struct {
	Points     int32
	Correct    bool
	Normalised *string
	Version    *NormalisationVersion
}

func ScoreAnswerV1(block IssueBlock, value AnswerValue) (StoredScore, error) {
	if !block.Type.Automatic() || block.Points == nil {
		return StoredScore{}, ErrNotAQuestion
	}
	if value.Answered() && !value.FitsQuestion(block.Type) {
		return StoredScore{}, ErrAnswerShapeInvalid
	}
	points := *block.Points
	switch block.Type {
	case BlockSingleChoice:
		correct := correctOptions(block)
		if len(correct) != 1 {
			return StoredScore{}, ErrAnswerKeyInvalid
		}
		score, err := ScoreSingleChoice(points, correct[0], optionsOf(value))
		return StoredScore{Points: score.Points, Correct: score.Correct}, err
	case BlockMultipleChoice:
		if block.Scoring == nil {
			return StoredScore{}, ErrAnswerKeyInvalid
		}
		score, err := ScoreMultipleChoice(points, *block.Scoring, correctOptions(block), optionsOf(value))
		return StoredScore{Points: score.Points, Correct: score.Correct}, err
	case BlockShortText:
		if block.AcceptedAnswers == nil {
			return StoredScore{}, ErrAnswerKeyInvalid
		}
		text := textOf(value)
		score, err := ScoreShortText(points, *block.AcceptedAnswers, text)
		normalised := string(NormaliseTextV1(text))
		version := NormalisationVersion1
		return StoredScore{Points: score.Points, Correct: score.Correct, Normalised: &normalised, Version: &version}, err
	case BlockShortNumber:
		if block.Value == nil || block.Tolerance == nil {
			return StoredScore{}, ErrAnswerKeyInvalid
		}
		text := textOf(value)
		score, err := ScoreShortNumber(points, *block.Value, *block.Tolerance, text)
		version := NormalisationVersion1
		return StoredScore{Points: score.Points, Correct: score.Correct, Normalised: parsedNumberV1(text), Version: &version}, err
	case BlockInstruction, BlockMaterial, BlockLongText, BlockFile:
	}
	return StoredScore{}, ErrNotAQuestion
}

func parsedNumberV1(text string) *string {
	parsed, err := ParseNumberV1(text)
	if err != nil {
		return nil
	}
	compact := strings.ReplaceAll(string(NormaliseTextV1(text)), " ", "")
	digits := 0
	if index := strings.IndexAny(compact, ".,"); index >= 0 {
		digits = len(compact) - index - 1
	}
	form := parsed.FloatString(digits)
	return &form
}

func correctOptions(block IssueBlock) []string {
	var correct []string
	if block.Options == nil {
		return correct
	}
	for _, option := range *block.Options {
		if option.Correct != nil && *option.Correct {
			correct = append(correct, option.ID)
		}
	}
	return correct
}

func optionsOf(value AnswerValue) []string {
	if value.OptionIDs == nil {
		return nil
	}
	return *value.OptionIDs
}

func textOf(value AnswerValue) string {
	if value.Text == nil {
		return ""
	}
	return *value.Text
}

type SubmittedPayload struct {
	Submission        string `json:"submission"`
	SubmittedAtMicros int64  `json:"submitted_at_micros"`
}

type SubmissionCheckedPayload struct {
	Submission string `json:"submission"`
	Revision   int32  `json:"revision"`
}

func SubmittedKey(submission SubmissionID, submittedAt time.Time) string {
	return fmt.Sprintf("%s:%s:%d", NotificationSubmitted, hex.EncodeToString(submission[:]), submittedAt.UnixMicro())
}

func SubmissionCheckedKey(submission SubmissionID, revision int32) string {
	return fmt.Sprintf("%s:%s:%d", NotificationSubmissionChecked, hex.EncodeToString(submission[:]), revision)
}
