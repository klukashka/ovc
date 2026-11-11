package domain_test

import (
	"errors"
	"slices"
	"testing"

	"ovc/backend/internal/domain"
)

type scoreCase struct {
	name  string
	score func() (domain.AutoScore, error)
	want  domain.AutoScore
}

func full(points int32) domain.AutoScore {
	return domain.AutoScore{Points: points, Correct: true}
}

func runScoreCases(t *testing.T, cases []scoreCase) {
	t.Helper()
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := testCase.score()

			if err != nil || got != testCase.want {
				t.Errorf("got %+v, %v; want %+v", got, err, testCase.want)
			}
		})
	}
}

func runInvalidKeyCases(t *testing.T, cases map[string]func() (domain.AutoScore, error)) {
	t.Helper()
	for name, score := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := score()

			if !errors.Is(err, domain.ErrAnswerKeyInvalid) || got != (domain.AutoScore{}) {
				t.Errorf("got %+v, %v; want a refused answer key", got, err)
			}
		})
	}
}

func TestScoringRules(t *testing.T) {
	t.Run("single choice", func(t *testing.T) {
		single := func(selected ...string) func() (domain.AutoScore, error) {
			return func() (domain.AutoScore, error) { return domain.ScoreSingleChoice(7, "b", selected) }
		}

		runScoreCases(t, []scoreCase{
			{"the correct option", single("b"), full(7)},
			{"the correct option named twice", single("b", "b"), full(7)},
			{"another option", single("a"), domain.AutoScore{}},
			{"the correct option with another", single("a", "b"), domain.AutoScore{}},
			{"only unknown options", single("x", "y"), domain.AutoScore{}},
			{"unanswered", single(), domain.AutoScore{}},
		})
		runInvalidKeyCases(t, map[string]func() (domain.AutoScore, error){
			"no correct option": func() (domain.AutoScore, error) { return domain.ScoreSingleChoice(7, "", []string{""}) },
			"negative points":   func() (domain.AutoScore, error) { return domain.ScoreSingleChoice(-1, "b", []string{"b"}) },
			"points above 100":  func() (domain.AutoScore, error) { return domain.ScoreSingleChoice(101, "b", []string{"b"}) },
		})
	})

	t.Run("multiple choice all or nothing", func(t *testing.T) {
		allOrNothing := func(selected ...string) func() (domain.AutoScore, error) {
			return func() (domain.AutoScore, error) {
				return domain.ScoreMultipleChoice(9, domain.ScoringAllOrNothing, []string{"a", "b", "c"}, selected)
			}
		}

		runScoreCases(t, []scoreCase{
			{"exactly the correct set", allOrNothing("c", "a", "b"), full(9)},
			{"a subset", allOrNothing("a", "b"), domain.AutoScore{}},
			{"a superset", allOrNothing("a", "b", "c", "d"), domain.AutoScore{}},
			{"unanswered", allOrNothing(), domain.AutoScore{}},
		})
		runInvalidKeyCases(t, map[string]func() (domain.AutoScore, error){
			"no correct option": func() (domain.AutoScore, error) {
				return domain.ScoreMultipleChoice(9, domain.ScoringAllOrNothing, nil, nil)
			},
			"an empty correct option": func() (domain.AutoScore, error) {
				return domain.ScoreMultipleChoice(9, domain.ScoringAllOrNothing, []string{"a", ""}, []string{"a", ""})
			},
		})
	})

	t.Run("multiple choice partial", func(t *testing.T) {
		partial := func(selected ...string) func() (domain.AutoScore, error) {
			return func() (domain.AutoScore, error) {
				return domain.ScoreMultipleChoice(10, domain.ScoringPartial, []string{"a", "b", "c"}, selected)
			}
		}

		runScoreCases(t, []scoreCase{
			{"exactly the correct set", partial("a", "b", "c"), full(10)},
			{"two of three correct floors", partial("a", "b"), domain.AutoScore{Points: 6}},
			{"one correct named twice counts once", partial("a", "a"), domain.AutoScore{Points: 3}},
			{"one incorrect cancels one correct", partial("a", "b", "x"), domain.AutoScore{Points: 3}},
			{"all correct and one incorrect", partial("a", "b", "c", "x"), domain.AutoScore{Points: 6}},
			{"as many incorrect as correct", partial("a", "x"), domain.AutoScore{}},
			{"more incorrect than correct", partial("a", "x", "y"), domain.AutoScore{}},
			{"only incorrect", partial("x"), domain.AutoScore{}},
			{"unanswered", partial(), domain.AutoScore{}},
			{"a question worth zero", func() (domain.AutoScore, error) {
				return domain.ScoreMultipleChoice(0, domain.ScoringPartial, []string{"a"}, []string{"a"})
			}, full(0)},
		})
		runInvalidKeyCases(t, map[string]func() (domain.AutoScore, error){
			"no correct option": func() (domain.AutoScore, error) {
				return domain.ScoreMultipleChoice(10, domain.ScoringPartial, nil, []string{"a"})
			},
			"points above 100": func() (domain.AutoScore, error) {
				return domain.ScoreMultipleChoice(101, domain.ScoringPartial, []string{"a"}, []string{"a"})
			},
			"an unknown scoring mode": func() (domain.AutoScore, error) {
				return domain.ScoreMultipleChoice(10, domain.ScoringMode("most"), []string{"a"}, []string{"a"})
			},
		})
	})

	t.Run("short text", func(t *testing.T) {
		shortText := func(answer string) func() (domain.AutoScore, error) {
			return func() (domain.AutoScore, error) {
				return domain.ScoreShortText(4, []string{"go", "went", "«Ёлка» — ель"}, answer)
			}
		}

		runScoreCases(t, []scoreCase{
			{"an accepted answer after normalisation", shortText("Went "), full(4)},
			{"another accepted answer after normalisation", shortText(`  "ЕЛКА"  -  ЕЛЬ`), full(4)},
			{"a wrong answer", shortText("gone"), domain.AutoScore{}},
			{"unanswered", shortText(""), domain.AutoScore{}},
			{"only whitespace", shortText(" \t "), domain.AutoScore{}},
		})
		runInvalidKeyCases(t, map[string]func() (domain.AutoScore, error){
			"no accepted answer": func() (domain.AutoScore, error) { return domain.ScoreShortText(4, nil, "") },
			"negative points":    func() (domain.AutoScore, error) { return domain.ScoreShortText(-4, []string{"went"}, "went") },
			"an accepted answer normalising to nothing": func() (domain.AutoScore, error) {
				return domain.ScoreShortText(4, []string{"went", " "}, " ")
			},
		})
	})

	t.Run("short number", func(t *testing.T) {
		shortNumber := func(answer string) func() (domain.AutoScore, error) {
			return func() (domain.AutoScore, error) { return domain.ScoreShortNumber(5, "2.5", "0.1", answer) }
		}
		exactValue := func(answer string) func() (domain.AutoScore, error) {
			return func() (domain.AutoScore, error) { return domain.ScoreShortNumber(5, "-0.000001", "0", answer) }
		}

		runScoreCases(t, []scoreCase{
			{"the value", shortNumber("2.5"), full(5)},
			{"the upper tolerance boundary in exact arithmetic", shortNumber("2,6"), full(5)},
			{"the lower tolerance boundary in exact arithmetic", shortNumber("2.4"), full(5)},
			{"just past the upper boundary", shortNumber("2.6000001"), domain.AutoScore{}},
			{"just past the lower boundary", shortNumber("2.3999999"), domain.AutoScore{}},
			{"a text that does not parse", shortNumber("2.5e0"), domain.AutoScore{}},
			{"unanswered", shortNumber(""), domain.AutoScore{}},
			{"zero tolerance with the value", exactValue("−0,0000010"), full(5)},
			{"zero tolerance off by the smallest step", exactValue("-0.000002"), domain.AutoScore{}},
		})
		runInvalidKeyCases(t, map[string]func() (domain.AutoScore, error){
			"a negative tolerance": func() (domain.AutoScore, error) { return domain.ScoreShortNumber(5, "1", "-0.1", "1") },
			"a value with an exponent": func() (domain.AutoScore, error) {
				return domain.ScoreShortNumber(5, "1e3", "0", "1000")
			},
			"a value with seven fractional digits": func() (domain.AutoScore, error) {
				return domain.ScoreShortNumber(5, "0.0000001", "0", "0")
			},
			"an empty tolerance": func() (domain.AutoScore, error) { return domain.ScoreShortNumber(5, "1", "", "1") },
			"points above 100":   func() (domain.AutoScore, error) { return domain.ScoreShortNumber(101, "1", "0", "1") },
		})
	})

	t.Run("checked by the tutor", func(t *testing.T) {
		automatic := map[domain.BlockType]bool{
			domain.BlockSingleChoice: true, domain.BlockMultipleChoice: true, domain.BlockShortText: true, domain.BlockShortNumber: true,
		}
		manual := map[domain.BlockType]bool{domain.BlockLongText: true, domain.BlockFile: true}

		for _, blockType := range slices.Concat(domain.BlockTypes, []domain.BlockType{"essay"}) {
			gotAutomatic, gotManual := blockType.Automatic(), blockType.Manual()

			if gotAutomatic != automatic[blockType] || gotManual != manual[blockType] {
				t.Errorf("%s: automatic %v, manual %v", blockType, gotAutomatic, gotManual)
			}
		}
	})
}

func points(value int32) *int32 {
	return &value
}

func TestFinalScoreAndPercentage(t *testing.T) {
	t.Run("final score", func(t *testing.T) {
		cases := []struct {
			name      string
			questions []domain.QuestionScore
			want      int64
		}{
			{"no questions", nil, 0},
			{"automatic points", []domain.QuestionScore{
				{Type: domain.BlockSingleChoice, MaxPoints: 3, AutoPoints: 3}, {Type: domain.BlockShortNumber, MaxPoints: 5, AutoPoints: 4},
			}, 7},
			{"an override replaces the automatic points", []domain.QuestionScore{
				{Type: domain.BlockShortText, MaxPoints: 5, AutoPoints: 0, OverridePoints: points(5)},
				{Type: domain.BlockMultipleChoice, MaxPoints: 6, AutoPoints: 6, OverridePoints: points(2)},
			}, 7},
			{"an override of zero", []domain.QuestionScore{
				{Type: domain.BlockSingleChoice, MaxPoints: 9, AutoPoints: 9, OverridePoints: points(0)},
			}, 0},
			{"manual questions score their tutor points and 0 while unscored", []domain.QuestionScore{
				{Type: domain.BlockLongText, MaxPoints: 10, ManualPoints: points(8)},
				{Type: domain.BlockFile, MaxPoints: 50},
			}, 8},
			{"questions worth zero", []domain.QuestionScore{
				{Type: domain.BlockShortText, MaxPoints: 0}, {Type: domain.BlockFile, MaxPoints: 0, ManualPoints: points(0)},
			}, 0},
			{"a mix", []domain.QuestionScore{
				{Type: domain.BlockLongText, MaxPoints: 100, ManualPoints: points(100)},
				{Type: domain.BlockShortText, MaxPoints: 100, AutoPoints: 100, OverridePoints: points(40)},
				{Type: domain.BlockShortNumber, MaxPoints: 100, AutoPoints: 100},
			}, 240},
		}
		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				got, err := domain.FinalPoints(testCase.questions)

				if err != nil || got != testCase.want {
					t.Errorf("got %d, %v; want %d", got, err, testCase.want)
				}
			})
		}
	})

	t.Run("refused question scores", func(t *testing.T) {
		cases := map[string]struct {
			question domain.QuestionScore
			want     error
		}{
			"a block that is not a question":        {domain.QuestionScore{Type: domain.BlockInstruction, MaxPoints: 3}, domain.ErrNotAQuestion},
			"an unknown block type":                 {domain.QuestionScore{Type: domain.BlockType("essay"), MaxPoints: 3}, domain.ErrNotAQuestion},
			"an override on a manual question":      {domain.QuestionScore{Type: domain.BlockLongText, MaxPoints: 5, OverridePoints: points(1)}, domain.ErrPointsInvalid},
			"tutor points on an automatic question": {domain.QuestionScore{Type: domain.BlockShortText, MaxPoints: 5, ManualPoints: points(1)}, domain.ErrPointsInvalid},
			"automatic points on a manual question": {domain.QuestionScore{Type: domain.BlockFile, MaxPoints: 50, AutoPoints: 50}, domain.ErrPointsInvalid},
			"tutor points above the maximum":        {domain.QuestionScore{Type: domain.BlockLongText, MaxPoints: 5, ManualPoints: points(6)}, domain.ErrPointsInvalid},
			"an override above the maximum":         {domain.QuestionScore{Type: domain.BlockSingleChoice, MaxPoints: 5, OverridePoints: points(6)}, domain.ErrPointsInvalid},
			"negative automatic points":             {domain.QuestionScore{Type: domain.BlockSingleChoice, MaxPoints: 5, AutoPoints: -1}, domain.ErrPointsInvalid},
			"a maximum above 100":                   {domain.QuestionScore{Type: domain.BlockShortNumber, MaxPoints: 101, AutoPoints: 101}, domain.ErrPointsInvalid},
		}
		for name, testCase := range cases {
			t.Run(name, func(t *testing.T) {
				questions := []domain.QuestionScore{{Type: domain.BlockShortText, MaxPoints: 3, AutoPoints: 3}, testCase.question}

				got, err := domain.FinalPoints(questions)

				if !errors.Is(err, testCase.want) || got != 0 {
					t.Errorf("got %d, %v; want %v", got, err, testCase.want)
				}
			})
		}
	})

	t.Run("percentage rounded half up", func(t *testing.T) {
		cases := []struct {
			final, maximum, want int64
		}{
			{0, 10, 0}, {10, 10, 100}, {1, 3, 33}, {2, 3, 67}, {1, 8, 13}, {3, 8, 38}, {1, 200, 1}, {1, 201, 0},
			{199, 400, 50}, {9_999, 10_000, 100}, {19_800, 40_000, 50}, {19_799, 40_000, 49}, {19_999, 40_000, 50},
		}
		for _, testCase := range cases {
			got, scored := domain.ScorePercentage(testCase.final, testCase.maximum)

			if !scored || got != testCase.want {
				t.Errorf("%d of %d: got %d, %v; want %d", testCase.final, testCase.maximum, got, scored, testCase.want)
			}
		}
	})

	t.Run("not scored when the maximum is zero", func(t *testing.T) {
		maximum := int64(0)

		got, scored := domain.ScorePercentage(0, maximum)

		if scored || got != 0 {
			t.Errorf("got %d, %v; want not scored", got, scored)
		}
	})
}

func TestRevealRule(t *testing.T) {
	revealing := map[domain.AnswerPolicy][]domain.SubmissionStatus{
		domain.AnswerPolicyNever:           nil,
		domain.AnswerPolicyAfterSubmission: {domain.SubmissionSubmitted, domain.SubmissionChecked},
		domain.AnswerPolicyAfterCheck:      {domain.SubmissionChecked},
		domain.AnswerPolicy(""):            nil,
		domain.AnswerPolicy("always"):      nil,
	}
	statuses := slices.Concat(domain.SubmissionStatuses, []domain.SubmissionStatus{"", "graded"})

	for policy, revealedFor := range revealing {
		for _, status := range statuses {
			want := slices.Contains(revealedFor, status)

			got := domain.AnswersRevealed(policy, status)

			if got != want {
				t.Errorf("policy %q, status %q: got %v, want %v", policy, status, got, want)
			}
		}
	}
}

func TestReturnAllowedOnlyBeforeAnswersAreSeen(t *testing.T) {
	type returnCase struct {
		status       domain.SubmissionStatus
		policy       domain.AnswerPolicy
		relationship domain.RelationshipState
	}
	allowed := map[returnCase]bool{
		{domain.SubmissionSubmitted, domain.AnswerPolicyNever, domain.RelationshipActive}:           true,
		{domain.SubmissionSubmitted, domain.AnswerPolicyNever, domain.RelationshipProvisional}:      true,
		{domain.SubmissionSubmitted, domain.AnswerPolicyAfterCheck, domain.RelationshipActive}:      true,
		{domain.SubmissionSubmitted, domain.AnswerPolicyAfterCheck, domain.RelationshipProvisional}: true,
	}
	relationships := []domain.RelationshipState{
		domain.RelationshipInvited, domain.RelationshipProvisional, domain.RelationshipReclaimPending,
		domain.RelationshipActive, domain.RelationshipEnded, domain.RelationshipDiscarded,
	}
	policies := slices.Concat(domain.AnswerPolicies, []domain.AnswerPolicy{"", "always"})
	statuses := slices.Concat(domain.SubmissionStatuses, []domain.SubmissionStatus{""})

	for _, status := range statuses {
		for _, policy := range policies {
			for _, hasManualQuestions := range []bool{false, true} {
				for _, relationship := range relationships {
					want := hasManualQuestions && allowed[returnCase{status, policy, relationship}]

					got := domain.ReturnAllowed(status, policy, hasManualQuestions, relationship)

					if got != want {
						t.Errorf("%s, policy %q, manual %v, %s: got %v, want %v", status, policy, hasManualQuestions, relationship, got, want)
					}
				}
			}
		}
	}
}

func TestSubmissionTransitions(t *testing.T) {
	type transition struct {
		from, to domain.SubmissionStatus
	}
	diagram := map[transition]bool{
		{domain.SubmissionNotStarted, domain.SubmissionDraft}:     true,
		{domain.SubmissionNotStarted, domain.SubmissionSubmitted}: true,
		{domain.SubmissionDraft, domain.SubmissionSubmitted}:      true,
		{domain.SubmissionSubmitted, domain.SubmissionChecked}:    true,
		{domain.SubmissionSubmitted, domain.SubmissionReturned}:   true,
		{domain.SubmissionReturned, domain.SubmissionSubmitted}:   true,
		{domain.SubmissionChecked, domain.SubmissionChecked}:      true,
		{domain.SubmissionNotStarted, domain.SubmissionClosed}:    true,
		{domain.SubmissionDraft, domain.SubmissionClosed}:         true,
		{domain.SubmissionReturned, domain.SubmissionClosed}:      true,
	}
	statuses := slices.Concat(domain.SubmissionStatuses, []domain.SubmissionStatus{"", "graded"})

	for _, from := range statuses {
		for _, to := range statuses {
			want := diagram[transition{from, to}]

			got := domain.SubmissionTransitionAllowed(from, to)

			if got != want {
				t.Errorf("%q to %q: got %v, want %v", from, to, got, want)
			}
		}
	}
}
