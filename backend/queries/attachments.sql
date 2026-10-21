-- name: CreateAttachment :exec
INSERT INTO attachments (
    id, uploader_id, context, quota_tutor_id, storage_key, filename, media_type,
    size_bytes, sha256, state, expires_at, created_at, updated_at
) VALUES (
    @id, @uploader_id, @context, @quota_tutor_id, @storage_key, @filename, @media_type,
    @size_bytes, @sha256, 'pending', @expires_at, @created_at, @created_at
);

-- name: GetAttachment :one
SELECT * FROM attachments WHERE id = @id;

-- name: CreateAttachmentReference :exec
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, question_id, created_at)
VALUES (@attachment_id, @ref_kind, @ref_id, @question_id, @created_at);

-- name: CountPendingUploads :one
SELECT count(id) FROM attachments WHERE uploader_id = @uploader_id AND context = @context AND state = 'pending';

-- name: LockAttachment :one
SELECT * FROM attachments WHERE id = @id FOR UPDATE;

-- name: BindAttachment :execrows
UPDATE attachments SET state = 'bound', expires_at = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state = 'pending';

-- name: RemoveAvatarReference :many
DELETE FROM attachment_references
WHERE ref_kind = 'avatar' AND ref_id = @ref_id
RETURNING attachment_id;

-- name: GetAvatarOf :one
SELECT attachments.id
FROM attachment_references
JOIN attachments ON attachments.id = attachment_references.attachment_id
WHERE attachment_references.ref_kind = 'avatar' AND attachment_references.ref_id = @ref_id;

-- name: GetServableAvatar :one
SELECT attachments.storage_key, attachments.media_type, attachments.size_bytes
FROM attachments
JOIN attachment_references ON attachment_references.attachment_id = attachments.id AND attachment_references.ref_kind = 'avatar'
JOIN users ON users.id = attachment_references.ref_id
WHERE attachments.id = @id AND attachments.state = 'bound' AND users.state IN ('unverified', 'active');

-- name: MarkAttachmentDeleted :execrows
UPDATE attachments SET state = 'deleted', expires_at = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state <> 'deleted';

-- name: ClaimExpiredUploads :many
SELECT id FROM attachments
WHERE state = 'pending' AND expires_at <= sqlc.arg(now)::timestamptz
ORDER BY expires_at
LIMIT sqlc.arg(batch_size)
FOR UPDATE SKIP LOCKED;

-- name: DeleteAttachmentRow :execrows
DELETE FROM attachments WHERE id = @id AND state = 'deleted';
