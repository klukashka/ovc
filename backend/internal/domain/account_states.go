package domain

import "time"

const DeletedAccountRetention = 30 * 24 * time.Hour

type OperatorAction string

const (
	ActionBlock   OperatorAction = "block"
	ActionUnblock OperatorAction = "unblock"
	ActionRestore OperatorAction = "restore"
)

func AllowedFrom(action OperatorAction, state AccountState) bool {
	switch action {
	case ActionBlock:
		return state == AccountUnverified || state == AccountActive
	case ActionUnblock:
		return state == AccountBlocked
	case ActionRestore:
		return state == AccountDeleted
	}
	return false
}

func UnblockTarget(stateBeforeBlock AccountState) (AccountState, bool) {
	if stateBeforeBlock == AccountUnverified || stateBeforeBlock == AccountActive {
		return stateBeforeBlock, true
	}
	return "", false
}
