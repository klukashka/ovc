package domain_test

import (
	"errors"
	"slices"
	"testing"

	"ovc/backend/internal/domain"
)

func stringPointer(value string) *string {
	return &value
}

func pointsOf(value int32) *int32 {
	return &value
}

func choiceBlock(kind domain.BlockType, correct ...string) domain.IssueBlock {
	options := []domain.TemplateOption{}
	for _, id := range []string{"a", "b", "c"} {
		isCorrect := slices.Contains(correct, id)
		options = append(options, domain.TemplateOption{ID: id, Text: id, Correct: &isCorrect})
	}
	scoring := domain.ScoringPartial
	block := domain.IssueBlock{TemplateBlock: domain.TemplateBlock{ID: "q", Type: kind, Options: &options, Points: pointsOf(6)}}
	if kind == domain.BlockMultipleChoice {
		block.Scoring = &scoring
	}
	return block
}

func selected(ids ...string) domain.AnswerValue {
	return domain.AnswerValue{OptionIDs: &ids}
}

func typed(text string) domain.AnswerValue {
	return domain.AnswerValue{Text: &text}
}

func TestScoreAnswerV1ReturnsTheStoredForm(t *testing.T) {
	textBlock := domain.IssueBlock{TemplateBlock: domain.TemplateBlock{
		Type: domain.BlockShortText, AcceptedAnswers: &[]string{"Ёлка"}, Points: pointsOf(3),
	}}
	numberBlock := domain.IssueBlock{TemplateBlock: domain.TemplateBlock{
		Type: domain.BlockShortNumber, Value: stringPointer("2.5"), Tolerance: stringPointer("0.1"), Points: pointsOf(2),
	}}
	cases := []struct {
		name       string
		block      domain.IssueBlock
		value      domain.AnswerValue
		points     int32
		correct    bool
		normalised *string
		versioned  bool
	}{
		{name: "single choice", block: choiceBlock(domain.BlockSingleChoice, "b"), value: selected("b"), points: 6, correct: true},
		{name: "single choice wrong", block: choiceBlock(domain.BlockSingleChoice, "b"), value: selected("a")},
		{name: "partial multiple choice", block: choiceBlock(domain.BlockMultipleChoice, "a", "b"), value: selected("a"), points: 3},
		{name: "unanswered choice", block: choiceBlock(domain.BlockMultipleChoice, "a"), value: selected()},
		{name: "short text", block: textBlock, value: typed("  ЕЛКА "), points: 3, correct: true, normalised: stringPointer("елка"), versioned: true},
		{name: "short text wrong", block: textBlock, value: typed("сосна"), normalised: stringPointer("сосна"), versioned: true},
		{name: "number with a comma", block: numberBlock, value: typed("2,55"), points: 2, correct: true, normalised: stringPointer("2.55"), versioned: true},
		{name: "number outside the tolerance", block: numberBlock, value: typed("3"), normalised: stringPointer("3"), versioned: true},
		{name: "unparsed number", block: numberBlock, value: typed("two"), versioned: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			score, err := domain.ScoreAnswerV1(testCase.block, testCase.value)

			if err != nil || score.Points != testCase.points || score.Correct != testCase.correct {
				t.Fatalf("got %+v, %v", score, err)
			}
			if (score.Normalised == nil) != (testCase.normalised == nil) || (score.Normalised != nil && *score.Normalised != *testCase.normalised) {
				t.Fatalf("stored the form %v, want %v", score.Normalised, testCase.normalised)
			}
			if (score.Version != nil) != testCase.versioned || (score.Version != nil && *score.Version != domain.NormalisationVersion1) {
				t.Fatalf("stored the version %v", score.Version)
			}
		})
	}
}

func TestScoreAnswerV1RefusesManualQuestionsAndWrongShapes(t *testing.T) {
	manual := domain.IssueBlock{TemplateBlock: domain.TemplateBlock{Type: domain.BlockLongText, Points: pointsOf(5)}}

	_, manualErr := domain.ScoreAnswerV1(manual, typed("essay"))
	_, shapeErr := domain.ScoreAnswerV1(choiceBlock(domain.BlockSingleChoice, "a"), typed("a"))

	if !errors.Is(manualErr, domain.ErrNotAQuestion) || !errors.Is(shapeErr, domain.ErrAnswerShapeInvalid) {
		t.Fatalf("got %v and %v", manualErr, shapeErr)
	}
}

func TestEditableQuestionsFollowTheStatus(t *testing.T) {
	blocks := []domain.IssueBlock{
		{TemplateBlock: domain.TemplateBlock{ID: "intro", Type: domain.BlockInstruction}},
		{TemplateBlock: domain.TemplateBlock{ID: "auto", Type: domain.BlockShortText}},
		{TemplateBlock: domain.TemplateBlock{ID: "manual", Type: domain.BlockFile}},
	}
	want := map[domain.SubmissionStatus][]string{
		domain.SubmissionNotStarted: {"auto", "manual"}, domain.SubmissionDraft: {"auto", "manual"}, domain.SubmissionReturned: {"manual"},
		domain.SubmissionSubmitted: {}, domain.SubmissionChecked: {}, domain.SubmissionClosed: {},
	}

	for status, expected := range want {
		if got := domain.EditableQuestions(status, blocks); !slices.Equal(got, expected) {
			t.Errorf("%s: got %v, want %v", status, got, expected)
		}
	}
}
