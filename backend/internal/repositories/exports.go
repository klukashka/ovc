package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type Exports struct{ queries *dbgen.Queries }
type Export = dbgen.Export
type ExportLimit = dbgen.ExportLimit

type NewExport struct {
	ID          domain.ExportID
	User        domain.UserID
	Language    domain.Language
	Timezone    string
	RequestedAt time.Time
}

func (r Exports) Insert(ctx context.Context, row NewExport) (Export, error) {
	created, err := r.queries.InsertExport(ctx, dbgen.InsertExportParams{ID: row.ID, UserID: row.User, Language: row.Language, Timezone: row.Timezone, RequestedAt: row.RequestedAt})
	return one(created, err, "insert export")
}

func (r Exports) LockOfUser(ctx context.Context, user domain.UserID) (Export, error) {
	row, err := r.queries.LockExportOfUser(ctx, user)
	return one(row, err, "lock export of user")
}

func (r Exports) OfUser(ctx context.Context, user domain.UserID) (Export, error) {
	row, err := r.queries.GetExportOfUser(ctx, user)
	return one(row, err, "export of user")
}

func (r Exports) Lock(ctx context.Context, id domain.ExportID) (Export, error) {
	row, err := r.queries.LockExport(ctx, id)
	return one(row, err, "lock export")
}

func (r Exports) ByID(ctx context.Context, id domain.ExportID) (Export, error) {
	row, err := r.queries.GetExport(ctx, id)
	return one(row, err, "get export")
}

func (r Exports) LockByBuild(ctx context.Context, build domain.ExportBuildID) (Export, error) {
	row, err := r.queries.LockExportByBuild(ctx, &build)
	return one(row, err, "lock export by build")
}

func (r Exports) LockUsersForChange(ctx context.Context, users ...domain.UserID) error {
	err := r.queries.LockExportUserBuckets(ctx, dbgen.LockExportUserBucketsParams{
		Namespace: domain.ExportUserLockNamespace, Buckets: domain.ExportUserLockBuckets(users...),
	})
	return done(err, "lock the export users")
}

func (r Exports) LockOfUsers(ctx context.Context, users []domain.UserID) ([]Export, error) {
	ids := make([]pgtype.UUID, 0, len(users))
	for _, user := range users {
		ids = append(ids, pgtype.UUID{Bytes: user, Valid: true})
	}
	rows, err := r.queries.LockExportsOfUsers(ctx, ids)
	return one(rows, err, "lock exports of users")
}

func (r Exports) Delete(ctx context.Context, id domain.ExportID) (bool, error) {
	rows, err := r.queries.DeleteExport(ctx, id)
	return applied(rows, err, "delete export")
}

func (r Exports) ClaimQueued(ctx context.Context) (Export, error) {
	row, err := r.queries.ClaimQueuedExport(ctx)
	return one(row, err, "claim queued export")
}

func (r Exports) StartBuild(ctx context.Context, id domain.ExportID, build domain.ExportBuildID, storageKey string, now time.Time) (bool, error) {
	rows, err := r.queries.StartExportBuild(ctx, dbgen.StartExportBuildParams{ID: id, BuildID: &build, StorageKey: &storageKey, Now: &now})
	return applied(rows, err, "start export build")
}

func (r Exports) RecordProgress(ctx context.Context, id domain.ExportID, build domain.ExportBuildID, done, total int32, now time.Time) (bool, error) {
	rows, err := r.queries.RecordExportProgress(ctx, dbgen.RecordExportProgressParams{ID: id, BuildID: &build, ProgressDone: done, ProgressTotal: total, Now: now})
	return applied(rows, err, "record export progress")
}

func (r Exports) Publish(ctx context.Context, id domain.ExportID, build domain.ExportBuildID, size int64, expires, now time.Time) (bool, error) {
	rows, err := r.queries.PublishExport(ctx, dbgen.PublishExportParams{ID: id, BuildID: &build, SizeBytes: &size, ExpiresAt: &expires, Now: &now})
	return applied(rows, err, "publish export")
}

func (r Exports) Fail(ctx context.Context, id domain.ExportID, failure domain.ExportFailure, now time.Time) (bool, error) {
	rows, err := r.queries.FailExport(ctx, dbgen.FailExportParams{ID: id, Failure: &failure, Now: now})
	return applied(rows, err, "fail export")
}

func (r Exports) ClearStorage(ctx context.Context, id domain.ExportID, now time.Time) (bool, error) {
	rows, err := r.queries.ClearExportStorage(ctx, dbgen.ClearExportStorageParams{ID: id, Now: now})
	return applied(rows, err, "clear export storage")
}

func (r Exports) Expire(ctx context.Context, id domain.ExportID, now time.Time) (bool, error) {
	rows, err := r.queries.ExpireExport(ctx, dbgen.ExpireExportParams{ID: id, Now: now})
	return applied(rows, err, "expire export")
}

func (r Exports) StaleBuilding(ctx context.Context, cutoff time.Time, limit int32) ([]domain.ExportID, error) {
	rows, err := r.queries.StaleBuildingExports(ctx, dbgen.StaleBuildingExportsParams{Cutoff: cutoff, BatchSize: limit})
	return one(rows, err, "stale building exports")
}

func (r Exports) DueReady(ctx context.Context, now time.Time, limit int32) ([]domain.ExportID, error) {
	rows, err := r.queries.DueReadyExports(ctx, dbgen.DueReadyExportsParams{Now: now, BatchSize: limit})
	return one(rows, err, "due ready exports")
}

func (r Exports) DueFinished(ctx context.Context, cutoff time.Time, limit int32) ([]domain.ExportID, error) {
	rows, err := r.queries.DueFinishedExports(ctx, dbgen.DueFinishedExportsParams{Cutoff: cutoff, BatchSize: limit})
	return one(rows, err, "due finished exports")
}

func (r Exports) Limit(ctx context.Context, user domain.UserID) (ExportLimit, error) {
	row, err := r.queries.GetExportLimit(ctx, user)
	return one(row, err, "export limit")
}

func (r Exports) LockLimit(ctx context.Context, user domain.UserID) (ExportLimit, error) {
	row, err := r.queries.LockExportLimit(ctx, user)
	return one(row, err, "lock export limit")
}

func (r Exports) RecordLimit(ctx context.Context, user domain.UserID, export domain.ExportID, allowedFrom time.Time) error {
	return done(r.queries.UpsertExportLimit(ctx, dbgen.UpsertExportLimitParams{UserID: user, ExportID: export, AllowedFrom: allowedFrom}), "record export limit")
}

func (r Exports) DeleteLimitOf(ctx context.Context, user domain.UserID, export domain.ExportID) (bool, error) {
	rows, err := r.queries.DeleteExportLimitOfExport(ctx, dbgen.DeleteExportLimitOfExportParams{UserID: user, ExportID: export})
	return applied(rows, err, "delete export limit of export")
}

func (r Exports) DeleteLimit(ctx context.Context, user domain.UserID) (bool, error) {
	rows, err := r.queries.DeleteExportLimit(ctx, user)
	return applied(rows, err, "delete export limit")
}

func (r Exports) InvalidationAudience(ctx context.Context, user domain.UserID) ([]domain.UserID, error) {
	rows, err := r.queries.ExportInvalidationAudience(ctx, pgtype.UUID{Bytes: user, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("export invalidation audience: %w", err)
	}
	audience := make([]domain.UserID, 0, len(rows))
	for _, row := range rows {
		if row.Valid {
			audience = append(audience, domain.UserID(row.Bytes))
		}
	}
	return audience, nil
}

func (r Exports) EarliestDeletedTutorClosing(ctx context.Context, student domain.UserID) (*time.Time, error) {
	deletedAt, err := r.queries.EarliestDeletedTutorDeletion(ctx, &student)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("earliest deleted tutor of a student: %w", err)
	}
	if deletedAt == nil {
		return nil, nil
	}
	closing := deletedAt.Add(domain.DeletedAccountRetention)
	return &closing, nil
}
