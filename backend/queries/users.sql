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
