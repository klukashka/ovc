-- name: InsertExport :one
INSERT INTO exports (id, user_id, state, language, timezone, requested_at, updated_at)
VALUES (@id, @user_id, 'queued', @language, @timezone, @requested_at, @requested_at)
RETURNING *;

-- name: LockExportOfUser :one
SELECT * FROM exports WHERE user_id = @user_id FOR UPDATE;

-- name: GetExportOfUser :one
SELECT * FROM exports WHERE user_id = @user_id;

-- name: LockExport :one
SELECT * FROM exports WHERE id = @id FOR UPDATE;

-- name: GetExport :one
SELECT * FROM exports WHERE id = @id;

-- name: LockExportByBuild :one
SELECT * FROM exports WHERE build_id = @build_id FOR UPDATE;

-- name: LockExportsOfUsers :many
SELECT * FROM exports WHERE user_id = ANY(sqlc.arg(user_ids)::uuid[]) ORDER BY user_id FOR UPDATE;

-- name: DeleteExport :execrows
DELETE FROM exports WHERE id = @id;

-- name: ClaimQueuedExport :one
SELECT * FROM exports WHERE state = 'queued' ORDER BY requested_at, id LIMIT 1 FOR UPDATE SKIP LOCKED;

-- name: StartExportBuild :execrows
UPDATE exports SET state = 'building', build_id = @build_id, storage_key = @storage_key, build_started_at = @now,
    progress_done = 0, progress_total = 0, updated_at = @now
WHERE id = @id AND state = 'queued';

-- name: RecordExportProgress :execrows
UPDATE exports SET progress_done = @progress_done, progress_total = @progress_total, updated_at = @now
WHERE id = @id AND build_id = @build_id AND state = 'building';

-- name: PublishExport :execrows
UPDATE exports SET state = 'ready', built_at = @now, size_bytes = @size_bytes, expires_at = @expires_at, updated_at = @now
WHERE id = @id AND build_id = @build_id AND state = 'building';

-- name: FailExport :execrows
UPDATE exports SET state = 'failed', failure = @failure, updated_at = @now
WHERE id = @id AND state IN ('building', 'ready');

-- name: ClearExportStorage :execrows
UPDATE exports SET storage_key = NULL, updated_at = @now WHERE id = @id;

-- name: ExpireExport :execrows
UPDATE exports SET state = 'expired', storage_key = NULL, updated_at = @now WHERE id = @id AND state = 'ready';

-- name: StaleBuildingExports :many
SELECT id FROM exports WHERE state = 'building' AND build_started_at <= sqlc.arg(cutoff)::timestamptz
ORDER BY build_started_at, id LIMIT sqlc.arg(batch_size)::integer;

-- name: DueReadyExports :many
SELECT id FROM exports WHERE state = 'ready' AND expires_at <= sqlc.arg(now)::timestamptz
ORDER BY expires_at, id LIMIT sqlc.arg(batch_size)::integer;

-- name: DueFinishedExports :many
SELECT id FROM exports
WHERE state IN ('failed', 'expired') AND coalesce(CASE WHEN state = 'expired' THEN expires_at END, built_at, requested_at) <= sqlc.arg(cutoff)::timestamptz
ORDER BY coalesce(CASE WHEN state = 'expired' THEN expires_at END, built_at, requested_at), id LIMIT sqlc.arg(batch_size)::integer;

-- name: GetExportLimit :one
SELECT * FROM export_limits WHERE user_id = @user_id;

-- name: LockExportLimit :one
SELECT * FROM export_limits WHERE user_id = @user_id FOR UPDATE;

-- name: UpsertExportLimit :exec
INSERT INTO export_limits (user_id, export_id, allowed_from) VALUES (@user_id, @export_id, @allowed_from)
ON CONFLICT (user_id) DO UPDATE SET export_id = EXCLUDED.export_id, allowed_from = EXCLUDED.allowed_from;

-- name: DeleteExportLimitOfExport :execrows
DELETE FROM export_limits WHERE user_id = @user_id AND export_id = @export_id;

-- name: DeleteExportLimit :execrows
DELETE FROM export_limits WHERE user_id = @user_id;
