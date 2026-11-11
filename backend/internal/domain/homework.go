package domain

import (
	"errors"
	"math/big"
	"slices"
)

type AnswerPolicy string

const (
	AnswerPolicyNever           AnswerPolicy = "never"
	AnswerPolicyAfterSubmission AnswerPolicy = "after_submission"
	AnswerPolicyAfterCheck      AnswerPolicy = "after_check"
)

var AnswerPolicies = []AnswerPolicy{AnswerPolicyNever, AnswerPolicyAfterSubmission, AnswerPolicyAfterCheck}

type SubmissionStatus string

const (
	SubmissionNotStarted SubmissionStatus = "not_started"
	SubmissionDraft      SubmissionStatus = "draft"
	SubmissionSubmitted  SubmissionStatus = "submitted"
	SubmissionReturned   SubmissionStatus = "returned"
	SubmissionChecked    SubmissionStatus = "checked"
	SubmissionClosed     SubmissionStatus = "closed"
)

var SubmissionStatuses = []SubmissionStatus{
	SubmissionNotStarted, SubmissionDraft, SubmissionSubmitted, SubmissionReturned, SubmissionChecked, SubmissionClosed,
}

var (
	ErrAnswerKeyInvalid = errors.New("the answer key of an automatic question is not valid")
	ErrNotAQuestion     = errors.New("the block is not a question")
	ErrPointsInvalid    = errors.New("the points of a question are outside its range")
)

func AnswersRevealed(policy AnswerPolicy, status SubmissionStatus) bool {
	switch policy {
	case AnswerPolicyAfterSubmission:
		return status == SubmissionSubmitted || status == SubmissionChecked
	case AnswerPolicyAfterCheck:
		return status == SubmissionChecked
	case AnswerPolicyNever:
		return false
	}
	return false
}

func ReturnAllowed(
	status SubmissionStatus, policy AnswerPolicy, hasManualQuestions bool, relationship RelationshipState,
) bool {
	policyAllows := policy == AnswerPolicyNever || policy == AnswerPolicyAfterCheck
	relationshipOpen := relationship == RelationshipActive || relationship == RelationshipProvisional
	return status == SubmissionSubmitted && policyAllows && hasManualQuestions && relationshipOpen
}

var submissionTransitions = map[SubmissionStatus][]SubmissionStatus{
	SubmissionNotStarted: {SubmissionDraft, SubmissionSubmitted, SubmissionClosed},
	SubmissionDraft:      {SubmissionSubmitted, SubmissionClosed},
	SubmissionSubmitted:  {SubmissionChecked, SubmissionReturned},
	SubmissionReturned:   {SubmissionSubmitted, SubmissionClosed},
	SubmissionChecked:    {SubmissionChecked},
}

func SubmissionTransitionAllowed(from SubmissionStatus, to SubmissionStatus) bool {
	return slices.Contains(submissionTransitions[from], to)
}

func (t BlockType) Automatic() bool {
	return t == BlockSingleChoice || t == BlockMultipleChoice || t == BlockShortText || t == BlockShortNumber
}

func (t BlockType) Manual() bool {
	return t == BlockLongText || t == BlockFile
}

type AutoScore struct {
	Points  int32
	Correct bool
}

func ScoreSingleChoice(points int32, correctOptionID string, selectedOptionIDs []string) (AutoScore, error) {
	if !questionPointsValid(points) || correctOptionID == "" {
		return AutoScore{}, ErrAnswerKeyInvalid
	}
	selected := distinct(selectedOptionIDs)
	if _, chosen := selected[correctOptionID]; !chosen || len(selected) != 1 {
		return AutoScore{}, nil
	}
	return AutoScore{Points: points, Correct: true}, nil
}

func ScoreMultipleChoice(points int32, mode ScoringMode, correctOptionIDs []string, selectedOptionIDs []string) (AutoScore, error) {
	correct := distinct(correctOptionIDs)
	if _, empty := correct[""]; empty || len(correct) == 0 || !questionPointsValid(points) {
		return AutoScore{}, ErrAnswerKeyInvalid
	}
	var correctOptions, correctSelected, incorrectSelected int32
	for range correct {
		correctOptions++
	}
	for option := range distinct(selectedOptionIDs) {
		if _, isCorrect := correct[option]; isCorrect {
			correctSelected++
		} else {
			incorrectSelected++
		}
	}
	exact := correctSelected == correctOptions && incorrectSelected == 0
	switch mode {
	case ScoringAllOrNothing:
		if exact {
			return AutoScore{Points: points, Correct: true}, nil
		}
		return AutoScore{}, nil
	case ScoringPartial:
		credited := max(0, correctSelected-incorrectSelected)
		return AutoScore{Points: points * credited / correctOptions, Correct: exact}, nil
	}
	return AutoScore{}, ErrAnswerKeyInvalid
}

func ScoreShortText(points int32, acceptedAnswers []string, answer string) (AutoScore, error) {
	if len(acceptedAnswers) == 0 || !questionPointsValid(points) {
		return AutoScore{}, ErrAnswerKeyInvalid
	}
	normalised := NormaliseTextV1(answer)
	matched := false
	for _, accepted := range acceptedAnswers {
		normalisedAccepted := NormaliseTextV1(accepted)
		if normalisedAccepted == "" {
			return AutoScore{}, ErrAnswerKeyInvalid
		}
		matched = matched || normalisedAccepted == normalised
	}
	if !matched {
		return AutoScore{}, nil
	}
	return AutoScore{Points: points, Correct: true}, nil
}

func ScoreShortNumber(points int32, value string, tolerance string, answer string) (AutoScore, error) {
	if !questionPointsValid(points) || !templateDecimal.MatchString(value) || !validTolerance(tolerance) {
		return AutoScore{}, ErrAnswerKeyInvalid
	}
	expected, expectedParsed := new(big.Rat).SetString(value)
	allowed, allowedParsed := new(big.Rat).SetString(tolerance)
	if !expectedParsed || !allowedParsed {
		return AutoScore{}, ErrAnswerKeyInvalid
	}
	given, err := ParseNumberV1(answer)
	if err != nil {
		return AutoScore{}, nil
	}
	distance := new(big.Rat).Sub(given, expected)
	if distance.Abs(distance).Cmp(allowed) > 0 {
		return AutoScore{}, nil
	}
	return AutoScore{Points: points, Correct: true}, nil
}

func questionPointsValid(points int32) bool {
	return points >= 0 && points <= TemplateMaxPoints
}

func distinct(identifiers []string) map[string]struct{} {
	set := make(map[string]struct{}, len(identifiers))
	for _, identifier := range identifiers {
		set[identifier] = struct{}{}
	}
	return set
}

type QuestionScore struct {
	Type           BlockType
	MaxPoints      int32
	AutoPoints     int32
	ManualPoints   *int32
	OverridePoints *int32
}

func FinalPoints(questions []QuestionScore) (int64, error) {
	var total int64
	for _, question := range questions {
		points, err := question.points()
		if err != nil {
			return 0, err
		}
		total += int64(points)
	}
	return total, nil
}

func (q QuestionScore) points() (int32, error) {
	if !q.Type.Question() {
		return 0, ErrNotAQuestion
	}
	if !questionPointsValid(q.MaxPoints) || !q.within(&q.AutoPoints) || !q.within(q.ManualPoints) || !q.within(q.OverridePoints) {
		return 0, ErrPointsInvalid
	}
	if q.Type.Manual() {
		if q.AutoPoints != 0 || q.OverridePoints != nil {
			return 0, ErrPointsInvalid
		}
		if q.ManualPoints == nil {
			return 0, nil
		}
		return *q.ManualPoints, nil
	}
	if q.ManualPoints != nil {
		return 0, ErrPointsInvalid
	}
	if q.OverridePoints != nil {
		return *q.OverridePoints, nil
	}
	return q.AutoPoints, nil
}

func (q QuestionScore) within(points *int32) bool {
	return points == nil || (*points >= 0 && *points <= q.MaxPoints)
}

func ScorePercentage(finalPoints int64, maximumPoints int64) (int64, bool) {
	if maximumPoints <= 0 {
		return 0, false
	}
	numerator := 200*finalPoints + maximumPoints
	denominator := 2 * maximumPoints
	quotient := numerator / denominator
	if numerator%denominator != 0 && numerator < 0 {
		quotient--
	}
	return quotient, true
}
