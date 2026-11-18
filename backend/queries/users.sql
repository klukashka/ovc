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

-- name: CreateTutorProfileIfCodeFree :execrows
INSERT INTO tutor_profiles (user_id, code, file_quota_bytes, recording_quota_bytes, created_at, updated_at)
VALUES (@user_id, @code, @file_quota_bytes, @recording_quota_bytes, @created_at, @created_at)
ON CONFLICT (code) DO NOTHING;

-- name: GetTutorProfile :one
SELECT * FROM tutor_profiles WHERE user_id = @user_id;

-- name: GetTutorCode :one
SELECT tutor_profiles.code, users.state FROM tutor_profiles
JOIN users ON users.id = tutor_profiles.user_id
WHERE tutor_profiles.user_id = @user_id;

-- name: DismissTutorOnboarding :execrows
UPDATE tutor_profiles SET onboarding_dismissed_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE user_id = @user_id AND onboarding_dismissed_at IS NULL;

-- name: LockTutorProfile :one
SELECT code FROM tutor_profiles WHERE user_id = @user_id FOR NO KEY UPDATE;

-- name: LockTutorProfiles :many
SELECT user_id FROM tutor_profiles WHERE user_id = ANY (sqlc.arg(user_ids)::uuid[]) ORDER BY user_id FOR NO KEY UPDATE;

-- name: ReplaceTutorCodeIfFree :execrows
UPDATE tutor_profiles SET code = @code, updated_at = sqlc.arg(now)::timestamptz
WHERE tutor_profiles.user_id = @user_id
    AND NOT EXISTS (SELECT 1 FROM tutor_profiles AS taken WHERE taken.code = @code);

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
ORDER BY deleted_at, id
OFFSET sqlc.arg(skip)
LIMIT sqlc.arg(batch_size);

-- name: ListUnverifiedSince :many
SELECT id FROM users
WHERE state = 'unverified' AND unverified_since <= sqlc.arg(cutoff)::timestamptz
ORDER BY unverified_since, id
OFFSET sqlc.arg(skip)
LIMIT sqlc.arg(batch_size);

-- name: LockUserForPurge :one
SELECT state, role, deleted_at, unverified_since FROM users WHERE id = @id FOR UPDATE;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = @id;

-- name: ListUsersForOperator :many
SELECT id, role, state, email, created_at FROM users
WHERE (sqlc.narg(role)::text IS NULL OR role = sqlc.narg(role)::text)
    AND (sqlc.narg(state)::text IS NULL OR state = sqlc.narg(state)::text)
    AND (sqlc.narg(email_pattern)::text IS NULL OR email::text ILIKE sqlc.narg(email_pattern)::text ESCAPE '\')
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit);

-- name: LockUserForOperator :one
SELECT id, role, state, state_before_block FROM users WHERE email = @email FOR UPDATE;

-- name: UnblockUser :execrows
UPDATE users
SET state = state_before_block,
    state_before_block = NULL,
    unverified_since = CASE WHEN state_before_block = 'unverified' THEN sqlc.arg(now)::timestamptz ELSE unverified_since END,
    updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state = 'blocked';

-- name: RestoreUser :execrows
UPDATE users SET state = 'active', deleted_at = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state = 'deleted' AND email_verified_at IS NOT NULL AND deleted_at > sqlc.arg(purge_cutoff)::timestamptz;

-- name: SetTutorQuotas :execrows
UPDATE tutor_profiles
SET file_quota_bytes = @file_quota_bytes, recording_quota_bytes = @recording_quota_bytes, updated_at = sqlc.arg(now)::timestamptz
WHERE user_id = @user_id;

-- name: SetUserReclaimDeadline :execrows
UPDATE users SET reclaim_until = sqlc.arg(reclaim_until)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id;

-- name: ClearUserReclaimDeadline :execrows
UPDATE users SET reclaim_until = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND reclaim_until IS NOT NULL;

-- name: ListReclaimsDue :many
SELECT id, reclaim_until::timestamptz AS reclaim_until FROM users
WHERE reclaim_until <= sqlc.arg(now)::timestamptz
    AND (
        sqlc.narg(after_until)::timestamptz IS NULL
        OR (reclaim_until, id) > (sqlc.narg(after_until)::timestamptz, sqlc.narg(after_id)::uuid)
    )
ORDER BY reclaim_until, id
LIMIT sqlc.arg(batch_size);

-- name: LockUserReclaim :one
SELECT state, reclaim_until FROM users WHERE id = @id FOR NO KEY UPDATE;

-- name: GetTutorByCode :one
SELECT tutor_profiles.user_id, users.state FROM tutor_profiles
JOIN users ON users.id = tutor_profiles.user_id
WHERE tutor_profiles.code = @code;

-- name: ListConfirmedUsers :many
SELECT id FROM users WHERE id = ANY (sqlc.arg(ids)::uuid[]) AND email_verified_at IS NOT NULL ORDER BY id;

-- name: GetTutorStanding :one
SELECT state, deleted_at FROM users WHERE id = @id;
