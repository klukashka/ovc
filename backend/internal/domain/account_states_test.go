package domain_test

import (
	"testing"

	"ovc/backend/internal/domain"
)

func TestOperatorActionsFollowTheAccountStateMachine(t *testing.T) {
	states := []domain.AccountState{domain.AccountUnverified, domain.AccountActive, domain.AccountBlocked, domain.AccountDeleted}
	allowed := map[domain.OperatorAction][]domain.AccountState{
		domain.ActionBlock:   {domain.AccountUnverified, domain.AccountActive},
		domain.ActionUnblock: {domain.AccountBlocked},
		domain.ActionRestore: {domain.AccountDeleted},
	}

	for action, from := range allowed {
		for _, state := range states {
			want := false
			for _, permitted := range from {
				want = want || permitted == state
			}

			got := domain.AllowedFrom(action, state)

			if got != want {
				t.Errorf("%s from %s: got %v, want %v", action, state, got, want)
			}
		}
	}
}

func TestUnblockReturnsOnlyToARememberedLiveState(t *testing.T) {
	cases := map[domain.AccountState]bool{
		domain.AccountUnverified: true,
		domain.AccountActive:     true,
		domain.AccountBlocked:    false,
		domain.AccountDeleted:    false,
	}

	for remembered, want := range cases {
		target, ok := domain.UnblockTarget(remembered)

		if ok != want || (ok && target != remembered) {
			t.Errorf("unblock from remembered %s: %s %v", remembered, target, ok)
		}
	}
}
