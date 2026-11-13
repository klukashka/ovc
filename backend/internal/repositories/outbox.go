package repositories

import (
	"context"
	"fmt"
	"time"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	Attachment         = dbgen.Attachment
	NewAttachment      = dbgen.CreateAttachmentParams
	NewAttachmentRef   = dbgen.CreateAttachmentReferenceParams
	NewOutboxRow       = dbgen.InsertOutboxParams
	ClaimedOutboxRow   = dbgen.ClaimOutboxRow
	ReclaimedOutboxRow = dbgen.ReclaimExpiredOutboxRow
	OutboxRow          = dbgen.Outbox
)

type OutboxCompletion struct {
	ID     domain.OutboxID
	Lease  domain.LeaseToken
	Reason domain.OutboxDoneReason
	Now    time.Time
}

type OutboxRetry struct {
	ID            domain.OutboxID
	Lease         domain.LeaseToken
	NextAttemptAt time.Time
	ErrorCode     string
}

type OutboxFailure struct {
	ID        domain.OutboxID
	Lease     domain.LeaseToken
	Now       time.Time
	ErrorCode string
}

type Attachments struct {
	queries *dbgen.Queries
}

func (r Attachments) Create(ctx context.Context, attachment NewAttachment) error {
	if attachment.UploaderID == nil {
		return ErrMissingUploader
	}
	return done(r.queries.CreateAttachment(ctx, attachment), "create attachment")
}

func (r Attachments) ByID(ctx context.Context, id domain.AttachmentID) (Attachment, error) {
	attachment, err := r.queries.GetAttachment(ctx, id)
	return one(attachment, err, "attachment by id")
}

func (r Attachments) Reference(ctx context.Context, reference NewAttachmentRef) error {
	return done(r.queries.CreateAttachmentReference(ctx, reference), "create attachment reference")
}

type OutboxClaim struct {
	Lane      domain.OutboxLane
	Now       time.Time
	Limit     int32
	Lease     domain.LeaseToken
	LeaseEnds time.Time
}

type QueuedOutboxRow struct {
	ID             domain.OutboxID
	Payload        []byte
	IdempotencyKey string
}

type NewOutboxRows struct {
	Lane          domain.OutboxLane
	Priority      domain.OutboxPriority
	Kind          domain.OutboxKind
	NextAttemptAt time.Time
	CreatedAt     time.Time
	Rows          []QueuedOutboxRow
}

type Outbox struct {
	queries *dbgen.Queries
}

func (r Outbox) Insert(ctx context.Context, row NewOutboxRow) (bool, error) {
	rows, err := r.queries.InsertOutbox(ctx, row)
	return applied(rows, err, "insert outbox row")
}

func (r Outbox) InsertAll(ctx context.Context, batch NewOutboxRows) (int64, error) {
	params := dbgen.InsertOutboxRowsParams{
		Lane: string(batch.Lane), Priority: int16(batch.Priority), Kind: string(batch.Kind), NextAttemptAt: batch.NextAttemptAt, CreatedAt: batch.CreatedAt,
		Payloads: make([]string, 0, len(batch.Rows)), IdempotencyKeys: make([]string, 0, len(batch.Rows)),
	}
	ids := make([]domain.OutboxID, 0, len(batch.Rows))
	for _, row := range batch.Rows {
		ids = append(ids, row.ID)
		params.Payloads = append(params.Payloads, string(row.Payload))
		params.IdempotencyKeys = append(params.IdempotencyKeys, row.IdempotencyKey)
	}
	params.Ids = uuids(ids)
	rows, err := r.queries.InsertOutboxRows(ctx, params)
	return counted(rows, err, "insert outbox rows")
}

func (r Outbox) Claim(ctx context.Context, claim OutboxClaim) ([]ClaimedOutboxRow, error) {
	claimed, err := r.queries.ClaimOutbox(ctx, dbgen.ClaimOutboxParams{
		LeaseToken:     &claim.Lease,
		LeaseExpiresAt: &claim.LeaseEnds,
		Lane:           claim.Lane,
		Now:            claim.Now,
		BatchSize:      claim.Limit,
	})
	return one(claimed, err, "claim outbox rows")
}

func (r Outbox) Complete(ctx context.Context, completion OutboxCompletion) (bool, error) {
	rows, err := r.queries.CompleteOutbox(ctx, dbgen.CompleteOutboxParams{
		DoneReason: &completion.Reason, Now: &completion.Now, ID: completion.ID, LeaseToken: &completion.Lease,
	})
	return applied(rows, err, "complete outbox row")
}

func (r Outbox) Retry(ctx context.Context, retry OutboxRetry) (bool, error) {
	rows, err := r.queries.RetryOutbox(ctx, dbgen.RetryOutboxParams{
		NextAttemptAt: retry.NextAttemptAt, LastErrorCode: &retry.ErrorCode, ID: retry.ID, LeaseToken: &retry.Lease,
	})
	return applied(rows, err, "retry outbox row")
}

func (r Outbox) Fail(ctx context.Context, failure OutboxFailure) (bool, error) {
	rows, err := r.queries.KillOutbox(ctx, dbgen.KillOutboxParams{
		Now: &failure.Now, LastErrorCode: &failure.ErrorCode, ID: failure.ID, LeaseToken: &failure.Lease,
	})
	return applied(rows, err, "mark outbox row dead")
}

func (r Outbox) ReclaimExpired(ctx context.Context, now time.Time) ([]ReclaimedOutboxRow, error) {
	rows, err := r.queries.ReclaimExpiredOutbox(ctx, dbgen.ReclaimExpiredOutboxParams{MaxAttempts: domain.OutboxMaxAttempts, Now: now})
	return one(rows, err, "reclaim expired outbox leases")
}

func (r Outbox) ByID(ctx context.Context, id domain.OutboxID) (OutboxRow, error) {
	row, err := r.queries.GetOutboxRow(ctx, id)
	return one(row, err, "outbox row by id")
}

type FinishedOutboxPurge struct {
	DoneBefore time.Time
	DeadBefore time.Time
	BatchSize  int32
}

func (r Outbox) DeleteFinished(ctx context.Context, purge FinishedOutboxPurge) (int64, error) {
	done, err := r.queries.DeleteDoneOutbox(ctx, dbgen.DeleteDoneOutboxParams{Before: purge.DoneBefore, BatchSize: purge.BatchSize})
	if err != nil {
		return 0, fmt.Errorf("delete done outbox rows: %w", err)
	}
	dead, err := r.queries.DeleteDeadOutbox(ctx, dbgen.DeleteDeadOutboxParams{Before: purge.DeadBefore, BatchSize: purge.BatchSize})
	if err != nil {
		return done, fmt.Errorf("delete dead outbox rows: %w", err)
	}
	return done + dead, nil
}

func (r Outbox) ByKey(ctx context.Context, key string) (OutboxRow, error) {
	row, err := r.queries.GetOutboxByKey(ctx, key)
	return one(row, err, "outbox row by key")
}
