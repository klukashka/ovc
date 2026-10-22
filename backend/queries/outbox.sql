-- name: InsertOutbox :execrows
INSERT INTO outbox (id, lane, priority, kind, payload, idempotency_key, status, next_attempt_at, created_at)
VALUES (@id, @lane, @priority, @kind, @payload, @idempotency_key, 'pending', @next_attempt_at, @created_at)
ON CONFLICT (idempotency_key) DO NOTHING;

-- name: ClaimOutbox :many
WITH claimable AS MATERIALIZED (
    SELECT candidate.id
    FROM outbox AS candidate
    WHERE candidate.lane = @lane AND candidate.status = 'pending' AND candidate.next_attempt_at <= @now
    ORDER BY candidate.priority, candidate.next_attempt_at
    LIMIT sqlc.arg(batch_size)
    FOR UPDATE SKIP LOCKED
), claimed AS (
    UPDATE outbox
    SET status = 'in_progress', attempts = outbox.attempts + 1, lease_token = @lease_token, lease_expires_at = @lease_expires_at
    FROM claimable
    WHERE outbox.id = claimable.id
    RETURNING outbox.id, outbox.attempts
)
SELECT claimed_row.id, claimed_row.priority, claimed_row.kind, claimed_row.payload, claimed_row.idempotency_key, claimed.attempts
FROM claimed
JOIN outbox AS claimed_row ON claimed_row.id = claimed.id
ORDER BY claimed_row.priority, claimed_row.next_attempt_at;

-- name: GetOutboxRow :one
SELECT * FROM outbox WHERE id = @id;

-- name: CompleteOutbox :execrows
UPDATE outbox
SET status = 'done', done_reason = @done_reason, done_at = @now, lease_token = NULL, lease_expires_at = NULL
WHERE id = @id AND lease_token = @lease_token AND status = 'in_progress';

-- name: RetryOutbox :execrows
UPDATE outbox
SET status = 'pending', next_attempt_at = @next_attempt_at, last_error_code = @last_error_code,
    lease_token = NULL, lease_expires_at = NULL
WHERE id = @id AND lease_token = @lease_token AND status = 'in_progress';

-- name: KillOutbox :execrows
UPDATE outbox
SET status = 'dead', done_at = @now, last_error_code = @last_error_code, lease_token = NULL, lease_expires_at = NULL
WHERE id = @id AND lease_token = @lease_token AND status = 'in_progress';

-- name: ReclaimExpiredOutbox :many
UPDATE outbox
SET status = CASE WHEN attempts >= sqlc.arg(max_attempts)::integer THEN 'dead' ELSE 'pending' END,
    done_at = CASE WHEN attempts >= sqlc.arg(max_attempts)::integer THEN sqlc.arg(now)::timestamptz END,
    last_error_code = CASE WHEN attempts >= sqlc.arg(max_attempts)::integer THEN 'lease_expired' ELSE last_error_code END,
    lease_token = NULL,
    lease_expires_at = NULL
WHERE status = 'in_progress' AND lease_expires_at <= sqlc.arg(now)::timestamptz
RETURNING id, kind, status;

-- name: DeleteDoneOutbox :execrows
DELETE FROM outbox
WHERE id IN (
    SELECT finished.id FROM outbox AS finished
    WHERE finished.status = 'done' AND finished.done_at < sqlc.arg(before)::timestamptz
    LIMIT sqlc.arg(batch_size)
);

-- name: DeleteDeadOutbox :execrows
DELETE FROM outbox
WHERE id IN (
    SELECT finished.id FROM outbox AS finished
    WHERE finished.status = 'dead' AND finished.done_at < sqlc.arg(before)::timestamptz
    LIMIT sqlc.arg(batch_size)
);
