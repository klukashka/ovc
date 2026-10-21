-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, generation, refresh_hash, expires_at, last_used_at, created_at)
VALUES (@id, @user_id, 1, @refresh_hash, @expires_at, @created_at, @created_at);

-- name: GetSession :one
SELECT * FROM sessions WHERE id = @id;

-- name: RotateSession :one
UPDATE sessions
SET generation = generation + 1, refresh_hash = @new_refresh_hash, last_used_at = @now
WHERE id = @id
    AND generation = @generation
    AND refresh_hash = @refresh_hash
    AND revoked_at IS NULL
    AND expires_at > @now
RETURNING generation;

-- name: RevokeSession :execrows
UPDATE sessions SET revoked_at = @now WHERE id = @id AND revoked_at IS NULL;

-- name: RevokeUserSessions :execrows
UPDATE sessions SET revoked_at = @now WHERE user_id = @user_id AND revoked_at IS NULL;

-- name: GetSessionPrincipal :one
SELECT sessions.id, sessions.user_id, sessions.revoked_at, sessions.expires_at, users.role, users.state
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.id = @id;

-- name: RevokeOtherUserSessions :execrows
UPDATE sessions SET revoked_at = sqlc.arg(now)::timestamptz
WHERE user_id = @user_id AND id <> sqlc.arg(kept) AND revoked_at IS NULL;
