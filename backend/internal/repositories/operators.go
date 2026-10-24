package repositories

import (
	"context"
	"time"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	OperatorFilter  = dbgen.ListUsersForOperatorParams
	OperatorListing = dbgen.ListUsersForOperatorRow
	OperatorTarget  = dbgen.LockUserForOperatorRow
)

func (r Users) ForOperator(ctx context.Context, filter OperatorFilter) ([]OperatorListing, error) {
	users, err := r.queries.ListUsersForOperator(ctx, filter)
	return one(users, err, "users for operator")
}

func (r Users) LockByEmail(ctx context.Context, email string) (OperatorTarget, error) {
	target, err := r.queries.LockUserForOperator(ctx, email)
	return one(target, err, "lock user by email")
}

func (r Users) Unblock(ctx context.Context, id domain.UserID, now time.Time) (bool, error) {
	rows, err := r.queries.UnblockUser(ctx, dbgen.UnblockUserParams{Now: now, ID: id})
	return applied(rows, err, "unblock user")
}

func (r Users) Restore(ctx context.Context, id domain.UserID, now time.Time, purgeCutoff time.Time) (bool, error) {
	rows, err := r.queries.RestoreUser(ctx, dbgen.RestoreUserParams{Now: now, ID: id, PurgeCutoff: purgeCutoff})
	return applied(rows, err, "restore user")
}

func (r TutorProfiles) SetQuotas(ctx context.Context, user domain.UserID, files int64, recordings int64, now time.Time) (bool, error) {
	rows, err := r.queries.SetTutorQuotas(ctx, dbgen.SetTutorQuotasParams{FileQuotaBytes: files, RecordingQuotaBytes: recordings, Now: now, UserID: user})
	return applied(rows, err, "set tutor quotas")
}
