package api

import (
	"testing"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/accounts"
)

type caller struct {
	role  domain.Role
	state domain.AccountState
}

var (
	activeTutor       = caller{role: domain.RoleTutor, state: domain.AccountActive}
	unverifiedTutor   = caller{role: domain.RoleTutor, state: domain.AccountUnverified}
	activeStudent     = caller{role: domain.RoleStudent, state: domain.AccountActive}
	unverifiedStudent = caller{role: domain.RoleStudent, state: domain.AccountUnverified}
)

const admittedOutcome = "admitted"

func outcomeFor(rule audienceRule, who caller) string {
	principal := accounts.Principal{UserID: domain.UserID{1}, Role: who.role, State: who.state}

	code, refused := rule.refusal(principal)

	if refused {
		return string(code)
	}
	return admittedOutcome
}

func TestAudiencesRefuseTheOtherRoleBeforeAnUnconfirmedEmail(t *testing.T) {
	t.Parallel()
	expected := map[Audience]map[caller]string{
		AudienceUser: {
			activeTutor: admittedOutcome, unverifiedTutor: "email_not_verified",
			activeStudent: admittedOutcome, unverifiedStudent: "email_not_verified",
		},
		AudienceUnverifiedOK: {
			activeTutor: admittedOutcome, unverifiedTutor: admittedOutcome,
			activeStudent: admittedOutcome, unverifiedStudent: admittedOutcome,
		},
		AudienceTutor: {
			activeTutor: admittedOutcome, unverifiedTutor: "email_not_verified",
			activeStudent: "not_found", unverifiedStudent: "not_found",
		},
		AudienceTutorUnverifiedOK: {
			activeTutor: admittedOutcome, unverifiedTutor: admittedOutcome,
			activeStudent: "not_found", unverifiedStudent: "not_found",
		},
		AudienceStudent: {
			activeTutor: "not_found", unverifiedTutor: "not_found",
			activeStudent: admittedOutcome, unverifiedStudent: "email_not_verified",
		},
		AudienceStudentUnverifiedOK: {
			activeTutor: "not_found", unverifiedTutor: "not_found",
			activeStudent: admittedOutcome, unverifiedStudent: admittedOutcome,
		},
	}
	if len(expected) != len(authenticatedAudiences) {
		t.Fatalf("the table covers %d of %d authenticated audiences", len(expected), len(authenticatedAudiences))
	}

	for audience, callers := range expected {
		rule, found := authenticatedAudiences[audience]
		if !found {
			t.Fatalf("%q is not an authenticated audience", audience)
		}
		for who, want := range callers {
			got := outcomeFor(rule, who)

			if got != want {
				t.Errorf("%q for a %s %s: got %s, want %s", audience, who.state, who.role, got, want)
			}
		}
	}
}
