-- name: CreateAttachment :exec
INSERT INTO attachments (
    id, uploader_id, context, quota_tutor_id, storage_key, filename, media_type,
    size_bytes, sha256, state, expires_at, created_at, updated_at
) VALUES (
    @id, @uploader_id, @context, @quota_tutor_id, @storage_key, @filename, sqlc.arg(media_type)::text,
    sqlc.arg(size_bytes)::bigint, sqlc.arg(sha256)::bytea, 'pending', @expires_at, @created_at, @created_at
);

-- name: GetAttachment :one
SELECT * FROM attachments WHERE id = @id;

-- name: CreateAttachmentReference :exec
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, question_id, created_at)
VALUES (@attachment_id, @ref_kind, @ref_id, @question_id, @created_at);

-- name: CountPendingUploads :one
SELECT count(id) FROM attachments
WHERE uploader_id = @uploader_id AND context = @context AND state = 'pending' AND expires_at > sqlc.arg(now)::timestamptz;

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
WHERE id IN (SELECT candidate.id FROM attachments AS candidate WHERE candidate.state = 'pending' AND candidate.expires_at <= sqlc.arg(now)::timestamptz ORDER BY candidate.expires_at LIMIT sqlc.arg(batch_size))
    AND state = 'pending' AND expires_at <= sqlc.arg(now)::timestamptz
ORDER BY id FOR UPDATE SKIP LOCKED;

-- name: DeleteAttachmentRow :execrows
DELETE FROM attachments WHERE id = @id AND state = 'deleted';

-- name: ListUploadsOf :many
SELECT id, storage_key, multipart_upload_id FROM attachments WHERE uploader_id = @user_id OR (quota_tutor_id = @user_id AND state <> 'bound') ORDER BY id;

-- name: DeleteUploadsOf :execrows
DELETE FROM attachments WHERE uploader_id = @user_id OR (quota_tutor_id = @user_id AND state <> 'bound');

-- name: CreateUploadingAttachment :exec
INSERT INTO attachments (
    id, uploader_id, context, scope_kind, scope_id, scope_question_id, quota_tutor_id, storage_key, filename, state, reserved_bytes, created_at,
    updated_at
) VALUES (
    @id, @uploader_id, @context, @scope_kind, @scope_id, @scope_question_id, @quota_tutor_id, @storage_key, @filename, 'uploading',
    sqlc.arg(reserved_bytes)::bigint, @created_at, @created_at
);

-- name: RecordMultipartUpload :execrows
UPDATE attachments SET multipart_upload_id = sqlc.arg(multipart_upload_id)::text, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state = 'uploading';

-- name: CompleteUpload :execrows
UPDATE attachments
SET state = 'pending', media_type = sqlc.arg(media_type)::text, size_bytes = sqlc.arg(size_bytes)::bigint,
    sha256 = sqlc.arg(sha256)::bytea, width = sqlc.narg(width)::integer, height = sqlc.narg(height)::integer,
    expires_at = sqlc.arg(expires_at)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state = 'uploading';

-- name: CountOpenUploads :one
SELECT count(id) FROM attachments
WHERE uploader_id = @uploader_id AND context = ANY (sqlc.arg(contexts)::text[])
    AND ((state = 'pending' AND expires_at > sqlc.arg(now)::timestamptz)
        OR (state = 'uploading' AND created_at > sqlc.arg(uploading_since)::timestamptz));

-- name: GetQuotaUse :one
SELECT tutor_profiles.file_quota_bytes, tutor_profiles.file_usage_bytes,
    COALESCE((
        SELECT sum(CASE WHEN open.state = 'pending' THEN open.size_bytes ELSE open.reserved_bytes END)
        FROM attachments AS open
        WHERE open.quota_tutor_id = tutor_profiles.user_id
            AND (sqlc.narg(excluded)::uuid IS NULL OR open.id <> sqlc.narg(excluded)::uuid)
            AND ((open.state = 'pending' AND open.expires_at > sqlc.arg(now)::timestamptz)
                OR (open.state = 'uploading' AND open.created_at > sqlc.arg(uploading_since)::timestamptz))
    ), 0)::bigint AS pending_bytes
FROM tutor_profiles
WHERE tutor_profiles.user_id = @tutor_id;

-- name: ClaimAgedUploads :many
SELECT id, storage_key, multipart_upload_id FROM attachments
WHERE id IN (SELECT candidate.id FROM attachments AS candidate WHERE candidate.state = 'uploading' AND candidate.created_at <= sqlc.arg(cutoff)::timestamptz ORDER BY candidate.created_at, candidate.id LIMIT sqlc.arg(batch_size))
    AND state = 'uploading' AND created_at <= sqlc.arg(cutoff)::timestamptz
ORDER BY id FOR UPDATE SKIP LOCKED;

-- name: ClaimUnbindableChatUploads :many
SELECT attachments.id FROM attachments
WHERE attachments.state = 'pending' AND attachments.context = 'chat' AND attachments.scope_kind = 'channel'
    AND NOT EXISTS (
        SELECT 1 FROM channels
        JOIN channel_members ON channel_members.channel_id = channels.id
        WHERE channels.id = attachments.scope_id AND channels.discarded_at IS NULL AND channels.erase_pending_since IS NULL
            AND channel_members.user_id = attachments.uploader_id
    )
ORDER BY attachments.id
LIMIT sqlc.arg(batch_size)
FOR UPDATE OF attachments SKIP LOCKED;

-- name: GetDownloadableChatAttachment :one
SELECT attachments.id, attachment_references.ref_id AS message_id, attachments.width, attachments.height
FROM attachments
JOIN attachment_references ON attachment_references.attachment_id = attachments.id AND attachment_references.ref_kind = 'message'
JOIN messages
    ON messages.id = attachment_references.ref_id AND messages.channel_id = attachments.scope_id AND messages.deleted_at IS NULL
        AND messages.erased_at IS NULL
JOIN active_channel_members AS members ON members.channel_id = attachments.scope_id AND members.user_id = sqlc.arg(viewer)::uuid
WHERE attachments.id = @id AND attachments.context = 'chat' AND attachments.state = 'bound' AND attachments.scope_kind = 'channel'
ORDER BY attachment_references.ref_id
LIMIT 1;

-- name: GetServableChatAttachment :one
SELECT attachments.storage_key, attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes,
    attachments.filename, attachments.width, attachments.context
FROM attachments
JOIN attachment_references
    ON attachment_references.attachment_id = attachments.id AND attachment_references.ref_kind = 'message'
        AND attachment_references.ref_id = @message_id
JOIN messages
    ON messages.id = attachment_references.ref_id AND messages.channel_id = attachments.scope_id AND messages.deleted_at IS NULL
        AND messages.erased_at IS NULL
JOIN channels ON channels.id = attachments.scope_id
WHERE attachments.id = @id AND attachments.context = 'chat' AND attachments.state = 'bound' AND attachments.scope_kind = 'channel'
    AND channels.discarded_at IS NULL AND channels.erase_pending_since IS NULL;

-- name: NameUploadingAttachment :execrows
UPDATE attachments SET filename = @filename, updated_at = @now WHERE id = @id AND state = 'uploading';

-- name: ClaimUnbindableLibraryUploads :many
SELECT attachments.id FROM attachments
JOIN users ON users.id = attachments.uploader_id
WHERE attachments.state = 'pending' AND attachments.context IN ('library', 'prompt_image') AND users.state IN ('blocked', 'deleted')
ORDER BY attachments.id
LIMIT sqlc.arg(batch_size)
FOR UPDATE OF attachments SKIP LOCKED;
