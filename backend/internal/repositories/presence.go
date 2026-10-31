package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type PresenceSummary = dbgen.UserPresence

type ExpiredPresence = dbgen.ListExpiredPresenceRow

type Presence struct {
	queries *dbgen.Queries
}

func (r Presence) LockUsers(ctx context.Context, users []domain.UserID) ([]domain.UserID, error) {
	if len(users) == 0 {
		return nil, nil
	}
	found, err := r.queries.LockPresenceUsers(ctx, uuids(users))
	return one(found, err, "lock presence users")
}

func (r Presence) LockSummaries(ctx context.Context, users []domain.UserID, now time.Time) ([]PresenceSummary, error) {
	if err := r.queries.InitializePresence(ctx, dbgen.InitializePresenceParams{Now: now, UserIds: uuids(users)}); err != nil {
		return nil, done(err, "initialize presence summaries")
	}
	return r.ExistingSummaries(ctx, users)
}

func (r Presence) ExistingSummaries(ctx context.Context, users []domain.UserID) ([]PresenceSummary, error) {
	rows, err := r.queries.LockPresenceSummaries(ctx, uuids(users))
	return one(rows, err, "lock presence summaries")
}

func (r Presence) Extend(ctx context.Context, instance domain.PresenceInstanceID, users []domain.UserID, until, now time.Time) error {
	err := r.queries.ExtendPresenceInstances(ctx, dbgen.ExtendPresenceInstancesParams{InstanceID: pgtype.UUID{Bytes: instance, Valid: true}, OnlineUntil: until, Now: now, UserIds: uuids(users)})
	return done(err, "extend presence instances")
}

func (r Presence) Release(ctx context.Context, instance domain.PresenceInstanceID, users []domain.UserID) error {
	err := r.queries.DeletePresenceInstances(ctx, dbgen.DeletePresenceInstancesParams{InstanceID: pgtype.UUID{Bytes: instance, Valid: true}, UserIds: uuids(users)})
	return done(err, "delete presence instances")
}

func (r Presence) Recompute(ctx context.Context, users []domain.UserID, now time.Time) error {
	err := r.queries.RecomputePresence(ctx, dbgen.RecomputePresenceParams{UserIds: uuids(users), FallbackUntil: now, Now: now})
	return done(err, "recompute presence summaries")
}

func (r Presence) Expired(ctx context.Context, now time.Time, limit int32) ([]ExpiredPresence, error) {
	rows, err := r.queries.ListExpiredPresence(ctx, dbgen.ListExpiredPresenceParams{Now: now, RowLimit: limit})
	return one(rows, err, "list expired presence instances")
}

func (r Presence) DeleteExpired(ctx context.Context, lease ExpiredPresence, now time.Time) (int64, error) {
	count, err := r.queries.DeleteExpiredPresence(ctx, dbgen.DeleteExpiredPresenceParams{UserID: pgtype.UUID{Bytes: lease.UserID, Valid: true}, InstanceID: pgtype.UUID{Bytes: lease.InstanceID, Valid: true}, Now: now})
	return one(count, err, "delete expired presence instance")
}

func (r Presence) RecomputeExpired(ctx context.Context, users []domain.UserID) error {
	return done(r.queries.RecomputeExpiredPresence(ctx, uuids(users)), "recompute expired presence summaries")
}

func (r Presence) Recipients(ctx context.Context, user domain.UserID) ([]domain.UserID, error) {
	recipients, err := r.queries.ListPresenceRecipients(ctx, user)
	return one(recipients, err, "presence recipients")
}
