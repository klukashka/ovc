package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

var (
	ErrUploadCapacity   = errors.New("the upload cleanup journal is full")
	ErrUploadTransition = errors.New("the upload intent transition was refused")
)

type UploadIntents struct{ queries *dbgen.Queries }
type UploadIntent = dbgen.UploadIntent
type UploadCleanupReceipt = dbgen.UploadCleanupReceipt

func (r UploadIntents) Create(ctx context.Context, id domain.AttachmentID, key string, now time.Time) error {
	if err := r.queries.CreateUploadIntent(ctx, dbgen.CreateUploadIntentParams{AttachmentID: id, StorageKey: key, CreatedAt: now}); err != nil {
		return done(err, "create upload intent")
	}
	rows, err := r.queries.ReserveUploadIntentCapacity(ctx, now)
	if err != nil {
		return done(err, "reserve upload intent capacity")
	}
	if rows != 1 {
		return ErrUploadCapacity
	}
	return nil
}

func (r UploadIntents) Lock(ctx context.Context, id domain.AttachmentID) (UploadIntent, error) {
	row, err := r.queries.LockUploadIntent(ctx, id)
	return one(row, err, "lock upload intent")
}

func (r UploadIntents) ByID(ctx context.Context, id domain.AttachmentID) (UploadIntent, error) {
	row, err := r.queries.GetUploadIntent(ctx, id)
	return one(row, err, "get upload intent")
}

func (r UploadIntents) RecordMultipart(ctx context.Context, id domain.AttachmentID, key string, upload string, now time.Time) error {
	rows, err := r.queries.RecordUploadIntentMultipart(ctx, dbgen.RecordUploadIntentMultipartParams{AttachmentID: id, StorageKey: key, MultipartUploadID: upload, Now: now})
	return intentApplied(rows, err)
}

func (r UploadIntents) AuthorizeComplete(ctx context.Context, id domain.AttachmentID, key string, upload string, now time.Time) error {
	rows, err := r.queries.AuthorizeUploadIntentComplete(ctx, dbgen.AuthorizeUploadIntentCompleteParams{AttachmentID: id, StorageKey: key, MultipartUploadID: upload, Now: now})
	return intentApplied(rows, err)
}

func (r UploadIntents) ConfirmComplete(ctx context.Context, id domain.AttachmentID, key string, now time.Time) error {
	rows, err := r.queries.ConfirmUploadIntentComplete(ctx, dbgen.ConfirmUploadIntentCompleteParams{AttachmentID: id, StorageKey: key, Now: now})
	return intentApplied(rows, err)
}

func (r UploadIntents) Revoke(ctx context.Context, id domain.AttachmentID, now time.Time) error {
	rows, err := r.queries.RevokeUploadIntent(ctx, dbgen.RevokeUploadIntentParams{AttachmentID: id, Now: now})
	return intentApplied(rows, err)
}

func (r UploadIntents) Queue(ctx context.Context, id domain.AttachmentID, generation domain.UploadCleanupGeneration, outbox domain.OutboxID, now time.Time) error {
	rows, err := r.queries.QueueUploadIntentCleanup(ctx, dbgen.QueueUploadIntentCleanupParams{AttachmentID: id, Generation: int16(generation), OutboxID: &outbox, Now: now})
	return intentApplied(rows, err)
}

func (r UploadIntents) Finish(ctx context.Context, id domain.AttachmentID, outbox domain.OutboxID, result domain.UploadCleanupResult, code *string, now time.Time) error {
	_, err := r.queries.FinishUploadIntentCleanup(ctx, dbgen.FinishUploadIntentCleanupParams{AttachmentID: id, OutboxID: &outbox, CleanupResult: result, ErrorCode: code, Now: now})
	return done(err, "finish upload cleanup")
}

func (r UploadIntents) Remove(ctx context.Context, id domain.AttachmentID, key string, now time.Time) error {
	rows, err := r.queries.DeleteUploadIntent(ctx, dbgen.DeleteUploadIntentParams{AttachmentID: id, StorageKey: key})
	if err != nil {
		return done(err, "delete upload intent")
	}
	if rows == 0 {
		return nil
	}
	released, err := r.queries.ReleaseUploadIntentCapacity(ctx, now)
	return intentApplied(released, err)
}

func (r UploadIntents) Candidates(ctx context.Context, after domain.AttachmentID, cutoff time.Time, limit int32) ([]domain.AttachmentID, error) {
	rows, err := r.queries.UploadIntentCandidates(ctx, dbgen.UploadIntentCandidatesParams{AfterID: pgtype.UUID{Bytes: after, Valid: true}, Cutoff: cutoff, BatchSize: limit})
	return one(rows, err, "upload recovery candidates")
}

func (r UploadIntents) Receipt(ctx context.Context, id domain.AttachmentID, attempt domain.UploadAttemptID) (UploadCleanupReceipt, error) {
	row, err := r.queries.GetUploadCleanupReceipt(ctx, dbgen.GetUploadCleanupReceiptParams{AttachmentID: id, AttemptID: attempt})
	return one(row, err, "upload cleanup receipt")
}

func (r UploadIntents) ActiveReceipt(ctx context.Context, id domain.AttachmentID) (UploadCleanupReceipt, error) {
	row, err := r.queries.ActiveUploadCleanupReceipt(ctx, id)
	return one(row, err, "active upload cleanup receipt")
}

func (r UploadIntents) Receipts(ctx context.Context, id domain.AttachmentID, after domain.UploadAttemptID) ([]UploadCleanupReceipt, error) {
	rows, err := r.queries.ListUploadCleanupReceipts(ctx, dbgen.ListUploadCleanupReceiptsParams{AttachmentID: id, AfterID: pgtype.UUID{Bytes: after, Valid: true}})
	return one(rows, err, "upload cleanup receipts")
}

func (r UploadIntents) ReceiptCount(ctx context.Context, id domain.AttachmentID) (int64, error) {
	count, err := r.queries.CountUploadCleanupReceipts(ctx, id)
	return one(count, err, "upload cleanup receipt count")
}

func (r UploadIntents) CreateReceipt(ctx context.Context, id domain.AttachmentID, attempt domain.UploadAttemptID, outbox domain.OutboxID, now time.Time) error {
	return done(r.queries.CreateUploadCleanupReceipt(ctx, dbgen.CreateUploadCleanupReceiptParams{AttachmentID: id, AttemptID: attempt, OutboxID: outbox, CreatedAt: now}), "create upload cleanup receipt")
}

func (r UploadIntents) UpdateReceipt(ctx context.Context, id domain.AttachmentID, outbox domain.OutboxID, status domain.OutboxStatus, now time.Time) error {
	_, err := r.queries.UpdateUploadCleanupReceipt(ctx, dbgen.UpdateUploadCleanupReceiptParams{AttachmentID: id, OutboxID: outbox, Status: status, Now: now})
	return done(err, "update upload cleanup receipt")
}

func intentApplied(rows int64, err error) error {
	if err != nil {
		return done(err, "transition upload intent")
	}
	if rows != 1 {
		return ErrUploadTransition
	}
	return nil
}
