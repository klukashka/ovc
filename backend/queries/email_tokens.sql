-- name: CreateEmailRequest :exec
INSERT INTO email_requests (id, user_id, purpose, created_at)
VALUES (@id, @user_id, @purpose, @created_at);

-- name: CreateEmailToken :exec
INSERT INTO email_tokens (id, request_id, user_id, purpose, token_hash, expires_at, created_at)
VALUES (@id, @request_id, @user_id, @purpose, @token_hash, @expires_at, @created_at);

-- name: DeleteUnusedEmailTokensForPurpose :execrows
DELETE FROM email_tokens WHERE user_id = @user_id AND purpose = @purpose AND used_at IS NULL;

-- name: DeleteUnusedEmailTokens :execrows
DELETE FROM email_tokens WHERE user_id = @user_id AND used_at IS NULL;

-- name: GetUsableEmailToken :one
SELECT * FROM email_tokens
WHERE token_hash = @token_hash AND purpose = @purpose AND used_at IS NULL AND expires_at > @now;

-- name: ConsumeEmailToken :one
UPDATE email_tokens
SET used_at = @now
WHERE token_hash = @token_hash AND purpose = @purpose AND used_at IS NULL AND expires_at > @now
RETURNING id, request_id, user_id;

-- name: GetEmailRequest :one
SELECT * FROM email_requests WHERE id = @id;

-- name: HasNewerEmailRequest :one
SELECT EXISTS (
    SELECT 1 FROM email_requests AS newer
    WHERE newer.user_id = @user_id AND newer.purpose = @purpose AND newer.created_at > @created_at
);

-- name: DeleteOpenEmailRequests :execrows
DELETE FROM email_requests AS request
WHERE request.user_id = @user_id
    AND NOT EXISTS (
        SELECT 1 FROM email_tokens AS token WHERE token.request_id = request.id AND token.used_at IS NOT NULL
    );

-- name: EmailRequestHasUsedToken :one
SELECT EXISTS (SELECT 1 FROM email_tokens WHERE request_id = @request_id AND used_at IS NOT NULL);
