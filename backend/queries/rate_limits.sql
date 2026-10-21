-- name: ReserveRateLimitSlot :one
INSERT INTO rate_limit_counters (purpose, subject, window_start, count)
VALUES (@purpose, @subject, @window_start, 1)
ON CONFLICT (purpose, subject, window_start) DO UPDATE SET count = rate_limit_counters.count + 1
RETURNING count;

-- name: ReleaseRateLimitSlot :execrows
UPDATE rate_limit_counters
SET count = count - 1
WHERE purpose = @purpose AND subject = @subject AND window_start = @window_start AND count > 0;

-- name: DeleteExpiredRateLimitCounters :execrows
DELETE FROM rate_limit_counters
WHERE (purpose, subject, window_start) IN (
    SELECT expired.purpose, expired.subject, expired.window_start
    FROM rate_limit_counters AS expired
    WHERE expired.window_start < @before
    LIMIT sqlc.arg(batch_size)
);
