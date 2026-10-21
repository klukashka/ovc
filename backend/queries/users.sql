-- name: CreateUser :exec
INSERT INTO users (
    id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme,
    consent_version, consent_at, unverified_since, created_at, updated_at
) VALUES (
    @id, @role, @state, @email, @email_verified_at, @password_hash, @name, @language, @timezone, @theme,
    @consent_version, @consent_at, @created_at, @created_at, @created_at
);

-- name: GetUserByID :one
SELECT * FROM users WHERE id = @id;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = @email;

-- name: CreateTutorProfile :exec
INSERT INTO tutor_profiles (user_id, file_quota_bytes, recording_quota_bytes, created_at, updated_at)
VALUES (@user_id, @file_quota_bytes, @recording_quota_bytes, @created_at, @created_at);

-- name: GetTutorProfile :one
SELECT * FROM tutor_profiles WHERE user_id = @user_id;

-- name: UpdateUserPasswordHash :execrows
UPDATE users SET password_hash = @password_hash, updated_at = @now
WHERE id = @id AND password_hash = @current_password_hash;

-- name: BlockUser :execrows
UPDATE users
SET state_before_block = state, state = 'blocked', updated_at = @now
WHERE id = @id AND state IN ('unverified', 'active');

-- name: MarkUserDeleted :execrows
UPDATE users
SET state = 'deleted', deleted_at = @now, updated_at = @now
WHERE id = @id AND state IN ('unverified', 'active');

-- name: LockUserCredentials :one
SELECT password_hash, state FROM users WHERE id = @id FOR NO KEY UPDATE;

-- name: CreateUserIfEmailFree :execrows
INSERT INTO users (
    id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme,
    consent_version, consent_at, unverified_since, created_at, updated_at
) VALUES (
    @id, @role, 'unverified', @email, NULL, @password_hash, @name, @language, @timezone, @theme,
    @consent_version, @created_at, @created_at, @created_at, @created_at
)
ON CONFLICT (email) DO NOTHING;

-- name: ConfirmUserEmail :execrows
UPDATE users
SET state = 'active', email_verified_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state = 'unverified';

-- name: ReplaceUserPassword :execrows
UPDATE users
SET password_hash = @password_hash,
    state = CASE WHEN state = 'unverified' THEN 'active' ELSE state END,
    email_verified_at = COALESCE(email_verified_at, sqlc.arg(now)::timestamptz),
    updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state IN ('unverified', 'active');

-- name: UpdateUserProfile :execrows
UPDATE users
SET name = @name, language = @language, timezone = @timezone, theme = @theme, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state IN ('unverified', 'active');

-- name: UpdateTutorProfileText :execrows
UPDATE tutor_profiles SET bio = @bio, subjects = @subjects, updated_at = sqlc.arg(now)::timestamptz
WHERE user_id = @user_id;

-- name: ListDeletedBefore :many
SELECT id FROM users
WHERE state = 'deleted' AND deleted_at <= sqlc.arg(cutoff)::timestamptz
ORDER BY deleted_at
LIMIT sqlc.arg(batch_size);

-- name: ListUnverifiedSince :many
SELECT id FROM users
WHERE state = 'unverified' AND unverified_since <= sqlc.arg(cutoff)::timestamptz
ORDER BY unverified_since
LIMIT sqlc.arg(batch_size);

-- name: LockUserForPurge :one
SELECT state, deleted_at, unverified_since FROM users WHERE id = @id FOR UPDATE;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = @id;
