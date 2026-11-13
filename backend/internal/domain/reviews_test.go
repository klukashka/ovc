package domain_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"ovc/backend/internal/domain"
)

func TestFeedbackExcerptFollowsTheHomeRule(t *testing.T) {
	t.Parallel()
	longWords := strings.Repeat("word ", 40)
	cases := []struct {
		name     string
		feedback string
		want     string
	}{
		{name: "markers removed", feedback: "**Well done**, _nearly_ *all* right.", want: "Well done, nearly all right."},
		{name: "link text kept", feedback: "See [the rule](https://example.com/rule) again.", want: "See the rule again."},
		{name: "paragraphs and items joined", feedback: "First.\n\n- one\n- two\n1. three\n\nLast\nline.", want: "First. one two three Last line."},
		{name: "code kept as text", feedback: "Use `**x**` here.\n\n```\nlet *y* = 1\n```", want: "Use **x** here. let *y* = 1"},
		{name: "escapes and literal markers", feedback: `2 * 3 is \*six\* and snake_case stays.`, want: "2 * 3 is *six* and snake_case stays."},
		{name: "exactly the limit", feedback: strings.Repeat("a", 140), want: strings.Repeat("a", 140)},
		{name: "cut at a word boundary", feedback: longWords, want: strings.TrimSpace(strings.Repeat("word ", 28)) + "…"},
		{name: "one long first word", feedback: strings.Repeat("b", 200), want: strings.Repeat("b", 139) + "…"},
	}
	var wrong []string

	for _, sample := range cases {
		got := domain.FeedbackExcerpt(sample.feedback)
		if got == nil || *got != sample.want || utf8.RuneCountInString(*got) > domain.FeedbackExcerptMaxRunes {
			wrong = append(wrong, sample.name)
		}
	}

	if len(wrong) != 0 {
		t.Fatalf("the excerpt rule disagrees for %v", wrong)
	}
}

func TestFeedbackExcerptIsAbsentWithoutText(t *testing.T) {
	t.Parallel()
	inputs := []string{"", "   \n\n  ", "```\n```"}
	var present []string

	for _, input := range inputs {
		if domain.FeedbackExcerpt(input) != nil {
			present = append(present, input)
		}
	}

	if len(present) != 0 {
		t.Fatalf("an excerpt without text was produced for %q", present)
	}
}

func TestReviewVisiblyChangedComparesCommentsFilesAndFeedback(t *testing.T) {
	t.Parallel()
	points, other := int32(3), int32(1)
	base := domain.VisibleReview{Feedback: "Good", Items: []domain.ReviewItem{
		{QuestionID: "q1", Points: &points, Comment: "ok", CorrectedAttachmentIDs: []string{"f2", "f1"}},
		{QuestionID: "q2", Points: &points},
	}}
	cases := []struct {
		name    string
		next    domain.VisibleReview
		changed bool
	}{
		{name: "points only", next: domain.VisibleReview{Feedback: "Good", Items: []domain.ReviewItem{
			{QuestionID: "q1", Points: &other, OverridePoints: &other, Comment: "ok", CorrectedAttachmentIDs: []string{"f1", "f2"}},
		}}, changed: false},
		{name: "feedback", next: domain.VisibleReview{Feedback: "Better", Items: base.Items}, changed: true},
		{name: "comment", next: domain.VisibleReview{Feedback: "Good", Items: []domain.ReviewItem{
			{QuestionID: "q1", Comment: "fine", CorrectedAttachmentIDs: []string{"f1", "f2"}},
		}}, changed: true},
		{name: "file removed", next: domain.VisibleReview{Feedback: "Good", Items: []domain.ReviewItem{
			{QuestionID: "q1", Comment: "ok", CorrectedAttachmentIDs: []string{"f1"}},
		}}, changed: true},
		{name: "comment on another question", next: domain.VisibleReview{Feedback: "Good", Items: append([]domain.ReviewItem{
			{QuestionID: "q2", Comment: "new"},
		}, base.Items[0])}, changed: true},
	}
	var wrong []string

	for _, sample := range cases {
		if domain.ReviewVisiblyChanged(base, sample.next) != sample.changed {
			wrong = append(wrong, sample.name)
		}
	}

	if len(wrong) != 0 {
		t.Fatalf("the visible-change rule disagrees for %v", wrong)
	}
}

func TestOverridesAfterTheAutomaticRevisionAreInvisible(t *testing.T) {
	t.Parallel()
	override, reason := int32(0), "Copied"
	overridesOnly := domain.VisibleReview{Items: []domain.ReviewItem{{QuestionID: "q1", OverridePoints: &override, OverrideReason: &reason}}}

	changed := domain.ReviewVisiblyChanged(domain.VisibleReview{}, overridesOnly)

	if changed || !overridesOnly.Empty() {
		t.Fatal("a revision holding only overrides counted as a visible change after revision 0")
	}
}
