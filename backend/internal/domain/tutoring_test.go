package domain_test

import (
	"testing"

	"ovc/backend/internal/domain"
)

type relationshipStep struct {
	from  domain.RelationshipState
	cause domain.RelationshipCause
}

func removalStep() domain.RelationshipTransition {
	return domain.RelationshipTransition{To: domain.RelationshipDiscarded, DiscardReason: domain.DiscardRemoved, NewCard: true}
}

func activeStep() domain.RelationshipTransition {
	return domain.RelationshipTransition{To: domain.RelationshipActive}
}

func tutorDeletedStep() domain.RelationshipTransition {
	return domain.RelationshipTransition{To: domain.RelationshipDiscarded, DiscardReason: domain.DiscardTutorDeleted}
}

func endedStep(by domain.EndedBy) domain.RelationshipTransition {
	return domain.RelationshipTransition{To: domain.RelationshipEnded, EndedBy: by}
}

func expectedRelationshipTransitions() map[relationshipStep]domain.RelationshipTransition {
	return map[relationshipStep]domain.RelationshipTransition{
		{domain.RelationshipInvited, domain.CauseWithdrawn}:           {To: domain.RelationshipDiscarded, DiscardReason: domain.DiscardWithdrawn},
		{domain.RelationshipInvited, domain.CauseMerged}:              {To: domain.RelationshipDiscarded, DiscardReason: domain.DiscardMerged},
		{domain.RelationshipInvited, domain.CauseTutorDeleted}:        tutorDeletedStep(),
		{domain.RelationshipInvited, domain.CauseAcceptedUnconfirmed}: {To: domain.RelationshipProvisional},
		{domain.RelationshipInvited, domain.CauseAcceptedConfirmed}:   activeStep(),
		{domain.RelationshipInvited, domain.CauseJoinRequestAccepted}: activeStep(),

		{domain.RelationshipProvisional, domain.CauseConfirmedInOwnBrowser}: activeStep(),
		{domain.RelationshipProvisional, domain.CauseReclaimed}:             {To: domain.RelationshipReclaimPending},
		{domain.RelationshipProvisional, domain.CauseTutorRemoved}:          removalStep(),
		{domain.RelationshipProvisional, domain.CauseStudentStopped}:        removalStep(),
		{domain.RelationshipProvisional, domain.CauseUnverifiedCleanup}:     removalStep(),
		{domain.RelationshipProvisional, domain.CauseStudentDeleted}:        removalStep(),
		{domain.RelationshipProvisional, domain.CauseStudentPurged}:         removalStep(),
		{domain.RelationshipProvisional, domain.CauseTutorDeleted}:          tutorDeletedStep(),

		{domain.RelationshipReclaimPending, domain.CauseOwnerKept}:              activeStep(),
		{domain.RelationshipReclaimPending, domain.CauseAcceptedConfirmed}:      activeStep(),
		{domain.RelationshipReclaimPending, domain.CauseOwnerKeptInactiveTutor}: removalStep(),
		{domain.RelationshipReclaimPending, domain.CauseOwnerDropped}:           removalStep(),
		{domain.RelationshipReclaimPending, domain.CauseReclaimExpired}:         removalStep(),
		{domain.RelationshipReclaimPending, domain.CauseTutorRemoved}:           removalStep(),
		{domain.RelationshipReclaimPending, domain.CauseStudentDeleted}:         removalStep(),
		{domain.RelationshipReclaimPending, domain.CauseStudentPurged}:          removalStep(),
		{domain.RelationshipReclaimPending, domain.CauseTutorDeleted}:           tutorDeletedStep(),

		{domain.RelationshipActive, domain.CauseTutorEnded}:     endedStep(domain.EndedByTutor),
		{domain.RelationshipActive, domain.CauseStudentStopped}: endedStep(domain.EndedByStudent),
		{domain.RelationshipActive, domain.CauseStudentDeleted}: endedStep(domain.EndedByDeletion),
		{domain.RelationshipActive, domain.CauseTutorDeleted}:   endedStep(domain.EndedByDeletion),

		{domain.RelationshipEnded, domain.CauseAcceptedConfirmed}:   activeStep(),
		{domain.RelationshipEnded, domain.CauseJoinRequestAccepted}: activeStep(),
		{domain.RelationshipEnded, domain.CauseStudentPurged}:       {To: domain.RelationshipDiscarded, DiscardReason: domain.DiscardStudentPurged},
	}
}

func TestRelationshipTransitions(t *testing.T) {
	expected := expectedRelationshipTransitions()

	checked := 0
	for _, from := range domain.RelationshipStates {
		for _, cause := range domain.RelationshipCauses {
			want, wantAllowed := expected[relationshipStep{from: from, cause: cause}]

			got, allowed := domain.NextRelationshipState(from, cause)

			if allowed != wantAllowed || got != want {
				t.Errorf("%s by %s: got %+v %v, want %+v %v", from, cause, got, allowed, want, wantAllowed)
			}
			checked++
		}
	}
	if checked != len(domain.RelationshipStates)*len(domain.RelationshipCauses) || len(domain.RelationshipStates) != 6 {
		t.Fatalf("checked %d combinations", checked)
	}
}

func TestColourChoiceAvoidsTheStudentsOtherColours(t *testing.T) {
	cases := map[string]struct {
		taken []domain.Colour
		want  domain.Colour
	}{
		"no other tutor":                {nil, domain.ColourClay},
		"first key taken":               {[]domain.Colour{domain.ColourClay}, domain.ColourSlate},
		"gaps are filled in order":      {[]domain.Colour{domain.ColourClay, domain.ColourMoss, domain.ColourSlate}, domain.ColourPlum},
		"the last free key":             {[]domain.Colour{domain.ColourClay, domain.ColourSlate, domain.ColourMoss, domain.ColourPlum, domain.ColourOchre, domain.ColourTeal, domain.ColourRust}, domain.ColourInk},
		"every key once, palette order": {domain.Palette, domain.ColourClay},
		"least used after a full round": {
			append(append([]domain.Colour{}, domain.Palette...), domain.ColourClay, domain.ColourSlate, domain.ColourMoss),
			domain.ColourPlum,
		},
		"least used wins over palette order": {
			[]domain.Colour{domain.ColourClay, domain.ColourClay, domain.ColourSlate, domain.ColourMoss, domain.ColourPlum, domain.ColourOchre, domain.ColourTeal, domain.ColourRust, domain.ColourInk, domain.ColourSlate},
			domain.ColourMoss,
		},
	}

	for name, testCase := range cases {
		got := domain.ChooseColour(testCase.taken)

		if got != testCase.want {
			t.Errorf("%s: got %s, want %s", name, got, testCase.want)
		}
	}
}

func TestTutorCodeNormalisation(t *testing.T) {
	cases := map[string]struct {
		input string
		want  domain.TutorCode
		valid bool
	}{
		"canonical":                    {"ABCD2345", "ABCD2345", true},
		"displayed with a hyphen":      {"ABCD-2345", "ABCD2345", true},
		"lower case":                   {"abcd-2345", "ABCD2345", true},
		"spaces and hyphens ignored":   {" ab cd - 23 45 ", "ABCD2345", true},
		"tabs and line breaks":         {"\tABCD\n2345", "ABCD2345", true},
		"every letter of the alphabet": {"WXYZ-HJKM", "WXYZHJKM", true},
		"zero is not in the alphabet":  {"ABCD-0345", "", false},
		"one is not in the alphabet":   {"ABCD-1345", "", false},
		"letter I":                     {"ABCI-2345", "", false},
		"letter L":                     {"ABCL-2345", "", false},
		"letter O":                     {"ABCO-2345", "", false},
		"lower case l":                 {"abcl-2345", "", false},
		"too short":                    {"ABCD-234", "", false},
		"too long":                     {"ABCD-23456", "", false},
		"two codes pasted together":    {"ABCD-2345 ABCD-2345", "", false},
		"empty":                        {"", "", false},
		"other punctuation":            {"ABCD_2345", "", false},
		"non-latin look-alike":         {"АВСD-2345", "", false},
	}

	for name, testCase := range cases {
		got, valid := domain.NormaliseTutorCode(testCase.input)

		if got != testCase.want || valid != testCase.valid {
			t.Errorf("%s: got %q %v, want %q %v", name, got, valid, testCase.want, testCase.valid)
		}
	}
	if display := domain.TutorCode("ABCD2345").Display(); display != "ABCD-2345" {
		t.Errorf("display: %s", display)
	}
}

func TestOutcomePrecedence(t *testing.T) {
	cases := map[string]struct {
		outcomes []domain.LinkOutcome
		want     domain.LinkOutcome
		set      bool
	}{
		"nothing happened": {nil, "", false},
		"choice required over everything": {
			[]domain.LinkOutcome{domain.LinkOutcomeCodeInvalid, domain.LinkOutcomeRequestSent, domain.LinkOutcomeLinked, domain.LinkOutcomeChoiceRequired},
			domain.LinkOutcomeChoiceRequired, true,
		},
		"linked over a request":        {[]domain.LinkOutcome{domain.LinkOutcomeRequestSent, domain.LinkOutcomeLinked}, domain.LinkOutcomeLinked, true},
		"linked over an invalid code":  {[]domain.LinkOutcome{domain.LinkOutcomeCodeInvalid, domain.LinkOutcomeLinked}, domain.LinkOutcomeLinked, true},
		"request over an invalid code": {[]domain.LinkOutcome{domain.LinkOutcomeCodeInvalid, domain.LinkOutcomeRequestSent}, domain.LinkOutcomeRequestSent, true},
		"invalid code alone":           {[]domain.LinkOutcome{domain.LinkOutcomeCodeInvalid}, domain.LinkOutcomeCodeInvalid, true},
		"repeated outcomes":            {[]domain.LinkOutcome{domain.LinkOutcomeRequestSent, domain.LinkOutcomeRequestSent}, domain.LinkOutcomeRequestSent, true},
		"unknown outcome is ignored":   {[]domain.LinkOutcome{"invitation_invalid"}, "", false},
	}

	for name, testCase := range cases {
		got, set := domain.CombineOutcomes(testCase.outcomes...)

		if got != testCase.want || set != testCase.set {
			t.Errorf("%s: got %q %v, want %q %v", name, got, set, testCase.want, testCase.set)
		}
	}
}
