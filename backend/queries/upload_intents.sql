-- name: CreateUploadIntent :exec
INSERT INTO upload_intents (attachment_id, storage_key, capacity_class, subject_kind, phase, created_at, updated_at)
VALUES (@attachment_id, @storage_key, @capacity_class, @subject_kind, 'opening', @created_at, @created_at);

-- name: ReserveUploadIntentCapacity :execrows
UPDATE upload_intent_capacity SET used = used + 1, updated_at = @now
WHERE capacity_class = @capacity_class AND used < CASE capacity_class WHEN 'chat' THEN 4096 WHEN 'export' THEN 256 ELSE 1024 END;

-- name: ReleaseUploadIntentCapacity :execrows
UPDATE upload_intent_capacity SET used = used - 1, updated_at = @now WHERE capacity_class = @capacity_class AND used > 0;

-- name: LockUploadIntent :one
SELECT * FROM upload_intents WHERE attachment_id = @attachment_id FOR UPDATE;

-- name: GetUploadIntent :one
SELECT * FROM upload_intents WHERE attachment_id = @attachment_id;

-- name: RecordUploadIntentMultipart :execrows
UPDATE upload_intents SET multipart_upload_id = sqlc.arg(multipart_upload_id)::text,
    phase = CASE WHEN cleanup_requested THEN phase ELSE 'streaming' END, updated_at = @now
WHERE attachment_id = @attachment_id AND storage_key = @storage_key
    AND (multipart_upload_id IS NULL OR multipart_upload_id = sqlc.arg(multipart_upload_id)::text);

-- name: AuthorizeUploadIntentComplete :execrows
UPDATE upload_intents SET phase = 'completing', completion_started = true, updated_at = @now
WHERE attachment_id = @attachment_id AND storage_key = @storage_key
    AND multipart_upload_id = sqlc.arg(multipart_upload_id)::text
    AND phase = 'streaming' AND NOT cleanup_requested AND NOT completion_started;

-- name: ConfirmUploadIntentComplete :execrows
UPDATE upload_intents SET completion_confirmed = true,
    final_cleanup_required = final_cleanup_required OR (NOT completion_confirmed AND COALESCE(cleanup_generation = 0, false)),
    phase = CASE WHEN cleanup_requested THEN 'cleanup' ELSE phase END, updated_at = @now
WHERE attachment_id = @attachment_id AND storage_key = @storage_key AND completion_started;

-- name: RevokeUploadIntent :execrows
UPDATE upload_intents SET cleanup_requested = true,
    phase = CASE WHEN completion_started AND NOT completion_confirmed THEN 'uncertain' ELSE 'cleanup' END, updated_at = @now
WHERE attachment_id = @attachment_id;

-- name: QueueUploadIntentCleanup :execrows
UPDATE upload_intents SET cleanup_generation = sqlc.arg(generation)::smallint,
    cleanup_outbox_id = @outbox_id, cleanup_result = 'queued', error_code = NULL,
    final_cleanup_required = false, updated_at = @now
WHERE attachment_id = @attachment_id AND cleanup_requested
    AND (cleanup_generation IS NULL OR (cleanup_generation = 0 AND sqlc.arg(generation)::smallint = 1 AND final_cleanup_required));

-- name: FinishUploadIntentCleanup :execrows
UPDATE upload_intents SET cleanup_result = @cleanup_result, error_code = @error_code, updated_at = @now
WHERE attachment_id = @attachment_id AND cleanup_outbox_id = @outbox_id;

-- name: DeleteUploadIntent :many
DELETE FROM upload_intents WHERE attachment_id = @attachment_id AND storage_key = @storage_key RETURNING capacity_class;

-- name: UploadIntentCandidates :many
SELECT attachment_id, subject_kind FROM upload_intents
WHERE attachment_id > sqlc.arg(after_id)::uuid
    AND (cleanup_requested OR created_at <= sqlc.arg(cutoff)::timestamptz)
ORDER BY attachment_id LIMIT sqlc.arg(batch_size)::integer;

-- name: GetUploadCleanupReceipt :one
SELECT * FROM upload_cleanup_receipts WHERE attachment_id = @attachment_id AND attempt_id = @attempt_id;

-- name: ActiveUploadCleanupReceipt :one
SELECT * FROM upload_cleanup_receipts WHERE attachment_id = @attachment_id AND status IN ('pending', 'in_progress');

-- name: ListUploadCleanupReceipts :many
SELECT * FROM upload_cleanup_receipts WHERE attachment_id = @attachment_id AND attempt_id > sqlc.arg(after_id)::uuid
ORDER BY attempt_id LIMIT 100;

-- name: CountUploadCleanupReceipts :one
SELECT count(*) FROM upload_cleanup_receipts WHERE attachment_id = @attachment_id;

-- name: CreateUploadCleanupReceipt :exec
INSERT INTO upload_cleanup_receipts (attachment_id, attempt_id, outbox_id, status, subject_kind, created_at, updated_at)
VALUES (@attachment_id, @attempt_id, @outbox_id, 'pending', @subject_kind, @created_at, @created_at);

-- name: UpdateUploadCleanupReceipt :execrows
UPDATE upload_cleanup_receipts SET status = @status, updated_at = @now WHERE attachment_id = @attachment_id AND outbox_id = @outbox_id;

-- name: UploadIntentCapacityUsed :one
SELECT used FROM upload_intent_capacity WHERE capacity_class = @capacity_class;
