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
