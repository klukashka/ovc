-- name: LockPresenceUsers :many
SELECT id FROM users WHERE id = ANY (sqlc.arg(user_ids)::uuid[]) ORDER BY id FOR KEY SHARE;

-- name: InitializePresence :exec
INSERT INTO user_presence (user_id, online_until, last_seen_at)
SELECT present.user_id, sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(user_ids)::uuid[]) AS present (user_id)
ORDER BY present.user_id
ON CONFLICT (user_id) DO NOTHING;

-- name: LockPresenceSummaries :many
SELECT user_id, online_until, last_seen_at FROM user_presence
WHERE user_id = ANY (sqlc.arg(user_ids)::uuid[]) ORDER BY user_id FOR UPDATE;

-- name: ExtendPresenceInstances :exec
INSERT INTO user_presence_instances (user_id, instance_id, online_until, last_seen_at)
SELECT present.user_id, sqlc.arg(instance_id)::uuid, sqlc.arg(online_until)::timestamptz, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(user_ids)::uuid[]) AS present (user_id)
ORDER BY present.user_id
ON CONFLICT (user_id, instance_id) DO UPDATE SET
    online_until = greatest(user_presence_instances.online_until, EXCLUDED.online_until),
    last_seen_at = greatest(user_presence_instances.last_seen_at, EXCLUDED.last_seen_at);

-- name: DeletePresenceInstances :exec
DELETE FROM user_presence_instances
WHERE user_id = ANY (sqlc.arg(user_ids)::uuid[]) AND instance_id = sqlc.arg(instance_id)::uuid;

-- name: RecomputePresence :exec
UPDATE user_presence AS summary SET
    online_until = greatest(
        coalesce((SELECT max(lease.online_until) FROM user_presence_instances AS lease WHERE lease.user_id = summary.user_id), sqlc.arg(fallback_until)::timestamptz),
        sqlc.arg(fallback_until)::timestamptz, summary.last_seen_at),
    last_seen_at = greatest(summary.last_seen_at, sqlc.arg(now)::timestamptz,
        (SELECT max(lease.last_seen_at) FROM user_presence_instances AS lease WHERE lease.user_id = summary.user_id))
WHERE summary.user_id = ANY (sqlc.arg(user_ids)::uuid[]);

-- name: ListExpiredPresence :many
SELECT user_id, instance_id FROM user_presence_instances
WHERE online_until <= sqlc.arg(now)::timestamptz
ORDER BY online_until, user_id, instance_id LIMIT sqlc.arg(row_limit)::int;

-- name: DeleteExpiredPresence :execrows
DELETE FROM user_presence_instances
WHERE user_id = sqlc.arg(user_id)::uuid AND instance_id = sqlc.arg(instance_id)::uuid
    AND online_until <= sqlc.arg(now)::timestamptz;

-- name: RecomputeExpiredPresence :exec
UPDATE user_presence AS summary SET
    online_until = greatest(coalesce((SELECT max(lease.online_until) FROM user_presence_instances AS lease WHERE lease.user_id = summary.user_id), summary.online_until), summary.last_seen_at),
    last_seen_at = greatest(summary.last_seen_at,
        (SELECT max(lease.last_seen_at) FROM user_presence_instances AS lease WHERE lease.user_id = summary.user_id))
WHERE summary.user_id = ANY (sqlc.arg(user_ids)::uuid[]);

-- name: ListPresenceRecipients :many
SELECT DISTINCT other.user_id
FROM active_channel_members AS mine
JOIN active_channel_members AS other ON other.channel_id = mine.channel_id AND other.user_id <> mine.user_id
WHERE mine.user_id = @user_id AND mine.read_only_since IS NULL
    AND (mine.user_id = mine.tutor_id OR other.user_id = other.tutor_id)
ORDER BY other.user_id;
