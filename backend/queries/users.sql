-- name: CreateUser :exec
INSERT INTO users (
    id, role, state, email, password_hash, name, language, timezone, theme,
    consent_version, consent_at, unverified_since, created_at, updated_at
) VALUES (
    @id, @role, @state, @email, @password_hash, @name, @language, @timezone, @theme,
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
