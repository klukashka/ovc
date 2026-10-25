package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

const foreignKeyViolation = "23503"

type (
	EmailPreference      = dbgen.ListEmailPreferencesRow
	EmailPreferenceWrite = dbgen.SaveEmailPreferenceParams
)

type EmailPreferences struct {
	queries *dbgen.Queries
}

func (r EmailPreferences) Of(ctx context.Context, user domain.UserID) ([]EmailPreference, error) {
	rows, err := r.queries.ListEmailPreferences(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("list email preferences: %w", err)
	}
	return rows, nil
}

func (r EmailPreferences) Save(ctx context.Context, write EmailPreferenceWrite) error {
	return done(r.queries.SaveEmailPreference(ctx, write), "save email preference")
}

func (r EmailPreferences) DisableForUsableAccount(ctx context.Context, user domain.UserID, group domain.EmailGroup, now time.Time) (bool, error) {
	roles := make([]string, 0, 2)
	for _, role := range group.RolesAllowed() {
		roles = append(roles, string(role))
	}
	rows, err := r.queries.DisableEmailPreferenceOfUsableAccount(ctx, dbgen.DisableEmailPreferenceOfUsableAccountParams{
		EmailGroup: group, Now: now, UserID: user, Roles: roles,
	})
	var violation *pgconn.PgError
	if errors.As(err, &violation) && violation.Code == foreignKeyViolation {
		return false, nil
	}
	return applied(rows, err, "disable email preference")
}
