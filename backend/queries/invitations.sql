-- name: CreateInvitation :exec
INSERT INTO invitations (id, relationship_id, token_hash, token_encrypted, bound_student_id, expires_at, created_at)
VALUES (@id, @relationship_id, @token_hash, @token_encrypted, @bound_student_id, @expires_at, @created_at);

-- name: ConsumeInvitation :one
UPDATE invitations
SET used_at = sqlc.arg(now)::timestamptz, used_by = @used_by, token_encrypted = NULL
WHERE token_hash = @token_hash AND used_at IS NULL AND revoked_at IS NULL AND expires_at > sqlc.arg(now)::timestamptz
    AND (bound_student_id IS NULL OR bound_student_id = @used_by)
RETURNING id, relationship_id, bound_student_id;

-- name: CountInvitationMismatch :one
UPDATE invitations
SET mismatch_count = mismatch_count + 1,
    revoked_at = CASE WHEN mismatch_count + 1 >= sqlc.arg(mismatch_limit)::smallint THEN sqlc.arg(now)::timestamptz END,
    revoke_reason = CASE WHEN mismatch_count + 1 >= sqlc.arg(mismatch_limit)::smallint THEN sqlc.arg(limit_reason)::text END,
    token_encrypted = CASE WHEN mismatch_count + 1 >= sqlc.arg(mismatch_limit)::smallint THEN NULL ELSE token_encrypted END
WHERE token_hash = @token_hash AND used_at IS NULL AND revoked_at IS NULL AND expires_at > sqlc.arg(now)::timestamptz
RETURNING mismatch_count;

-- name: RevokeOpenInvitation :execrows
UPDATE invitations
SET revoked_at = sqlc.arg(now)::timestamptz, revoke_reason = sqlc.arg(reason)::text, token_encrypted = NULL
WHERE relationship_id = @relationship_id
    AND relationship_id IN (SELECT id FROM relationships WHERE tutor_id = @tutor_id)
    AND used_at IS NULL AND revoked_at IS NULL;
