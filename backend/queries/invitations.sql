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

-- name: GetCardInvitation :one
SELECT id, token_encrypted, expires_at, used_at, revoked_at, revoke_reason FROM invitations
WHERE relationship_id = @relationship_id
ORDER BY (used_at IS NULL AND revoked_at IS NULL) DESC, created_at DESC, id DESC
LIMIT 1;

-- name: GetInvitationTutor :one
SELECT tutors.id, tutors.name, tutor_profiles.subjects
FROM invitations
JOIN relationships ON relationships.id = invitations.relationship_id
JOIN users AS tutors ON tutors.id = relationships.tutor_id
JOIN tutor_profiles ON tutor_profiles.user_id = relationships.tutor_id
WHERE invitations.token_hash = @token_hash
    AND invitations.used_at IS NULL
    AND invitations.revoked_at IS NULL
    AND invitations.expires_at > sqlc.arg(now)::timestamptz
    AND relationships.state IN ('invited', 'ended')
    AND tutors.state = 'active';

-- name: ClearExpiredInvitationCiphertext :execrows
UPDATE invitations
SET token_encrypted = NULL
WHERE id IN (
    SELECT expired.id FROM invitations AS expired
    WHERE expired.token_encrypted IS NOT NULL AND expired.expires_at <= sqlc.arg(now)::timestamptz
    ORDER BY expired.expires_at
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE SKIP LOCKED
);

-- name: CountInvitationSend :one
UPDATE invitations
SET send_count = send_count + 1
WHERE id = @id AND used_at IS NULL AND revoked_at IS NULL AND expires_at > sqlc.arg(now)::timestamptz
RETURNING send_count;

-- name: GetInvitationForEmail :one
SELECT invitations.id, invitations.token_encrypted, invitations.expires_at, invitations.used_at, invitations.revoked_at,
    invitations.send_count, relationships.tutor_id, relationships.state, relationships.email
FROM invitations
JOIN relationships ON relationships.id = invitations.relationship_id
WHERE invitations.id = @id;
