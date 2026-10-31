-- name: ListDigestCandidates :many
SELECT pending.user_id, pending.pending_since, pending.revision
FROM digest_pending AS pending
LEFT JOIN user_presence AS presence ON presence.user_id = pending.user_id
LEFT JOIN digest_state AS state ON state.user_id = pending.user_id
WHERE pending.pending_since <= sqlc.arg(now)::timestamptz - interval '10 minutes'
    AND (presence.online_until IS NULL OR presence.online_until <= sqlc.arg(now)::timestamptz - interval '10 minutes')
    AND (state.last_digest_at IS NULL OR state.last_digest_at <= sqlc.arg(now)::timestamptz - interval '30 minutes')
    AND (sqlc.narg(after_since)::timestamptz IS NULL OR (pending.pending_since, pending.user_id) > (sqlc.narg(after_since)::timestamptz, sqlc.narg(after_user)::uuid))
ORDER BY pending.pending_since, pending.user_id
LIMIT sqlc.arg(batch_size)::int;

-- name: GetDigestRevision :one
SELECT revision FROM digest_pending WHERE user_id = @user_id;

-- name: ListDigestChannels :many
SELECT channel_id FROM active_channel_members WHERE user_id = @user_id ORDER BY channel_id;

-- name: ShareDigestChannels :many
SELECT id FROM channels WHERE id = ANY(sqlc.arg(channel_ids)::uuid[]) ORDER BY id FOR SHARE;

-- name: LockDigestMembers :many
SELECT channel_members.channel_id FROM channel_members
JOIN active_channel_members AS active ON active.channel_id = channel_members.channel_id AND active.user_id = channel_members.user_id
WHERE channel_members.user_id = sqlc.arg(user_id)::uuid AND channel_members.channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
ORDER BY channel_members.channel_id FOR UPDATE OF channel_members;

-- name: EnsureDigestState :exec
INSERT INTO digest_state (user_id) VALUES (@user_id) ON CONFLICT DO NOTHING;

-- name: LockDigestState :one
SELECT user_id, last_digest_at FROM digest_state WHERE user_id = @user_id FOR UPDATE;

-- name: LockDigestPending :one
SELECT user_id, revision, pending_since FROM digest_pending WHERE user_id = @user_id FOR UPDATE;

-- name: DigestEligible :one
SELECT EXISTS (
    SELECT 1 FROM digest_pending AS pending
    JOIN users ON users.id = pending.user_id
    LEFT JOIN user_presence AS presence ON presence.user_id = pending.user_id
    LEFT JOIN digest_state AS state ON state.user_id = pending.user_id
    WHERE pending.user_id = @user_id
        AND pending.pending_since <= sqlc.arg(now)::timestamptz - interval '10 minutes'
        AND (presence.online_until IS NULL OR presence.online_until <= sqlc.arg(now)::timestamptz - interval '10 minutes')
        AND (state.last_digest_at IS NULL OR state.last_digest_at <= sqlc.arg(now)::timestamptz - interval '30 minutes')
) AS eligible;

-- name: ListDigestBounds :many
SELECT member.channel_id, GREATEST(member.last_read_seq, member.digested_through_seq)::bigint AS after_seq,
    member.last_seq AS through_seq,
    count(message.id)::bigint AS message_count
FROM active_channel_members AS member
LEFT JOIN messages AS message ON message.channel_id = member.channel_id
    AND message.seq > GREATEST(member.last_read_seq, member.digested_through_seq)
    AND (sqlc.narg(after_seen)::timestamptz IS NULL OR message.created_at > sqlc.narg(after_seen)::timestamptz)
    AND message.body IS NOT NULL AND message.deleted_at IS NULL AND message.erased_at IS NULL
WHERE member.user_id = @user_id AND member.channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
GROUP BY member.channel_id, member.last_read_seq, member.digested_through_seq, member.last_seq
ORDER BY member.channel_id;

-- name: AdvanceDigestedMarker :exec
UPDATE channel_members SET digested_through_seq = GREATEST(channel_members.digested_through_seq, sqlc.arg(through_seq)::bigint)
FROM active_channel_members AS active
WHERE active.channel_id = channel_members.channel_id AND active.user_id = channel_members.user_id
    AND channel_members.channel_id = sqlc.arg(channel_id)::uuid AND channel_members.user_id = sqlc.arg(user_id)::uuid;

-- name: RecordDigestTime :exec
UPDATE digest_state SET last_digest_at = sqlc.arg(now)::timestamptz WHERE user_id = @user_id;

-- name: DeleteDigestRevision :execrows
DELETE FROM digest_pending WHERE digest_pending.user_id = sqlc.arg(user_id)::uuid AND revision = @revision;

-- name: ClearReadDigestRevision :execrows
DELETE FROM digest_pending WHERE digest_pending.user_id = sqlc.arg(user_id)::uuid AND revision = @revision
    AND NOT EXISTS (SELECT 1 FROM active_channel_members WHERE active_channel_members.user_id = digest_pending.user_id AND last_seq > last_read_seq);

-- name: DeleteDigestStateOf :exec
DELETE FROM digest_state WHERE user_id = @user_id;

-- name: ListDigestChannelsAtDispatch :many
WITH bounds AS (
    SELECT unnest(sqlc.arg(channel_ids)::uuid[]) AS channel_id,
        unnest(sqlc.arg(after_seqs)::bigint[]) AS after_seq,
        unnest(sqlc.arg(through_seqs)::bigint[]) AS through_seq
), counted AS (
    SELECT member.channel_id, count(message.id)::bigint AS message_count
    FROM bounds
    JOIN active_channel_members AS member ON member.channel_id = bounds.channel_id AND member.user_id = sqlc.arg(user_id)::uuid
    JOIN messages AS message ON message.channel_id = member.channel_id
        AND message.seq > GREATEST(member.last_read_seq, bounds.after_seq)
        AND message.seq <= bounds.through_seq
        AND (member.user_id = member.tutor_id OR member.provisional_through_seq IS NULL OR message.seq > member.provisional_through_seq)
        AND (sqlc.narg(after_seen)::timestamptz IS NULL OR message.created_at > sqlc.narg(after_seen)::timestamptz)
        AND message.body IS NOT NULL AND message.deleted_at IS NULL AND message.erased_at IS NULL
    GROUP BY member.channel_id
), listed AS (
    SELECT counted.channel_id, counted.message_count, count(*) OVER () AS total_channels
    FROM counted ORDER BY counted.channel_id LIMIT sqlc.arg(channel_limit)::int
)
SELECT listed.channel_id, listed.message_count, listed.total_channels,
    (CASE WHEN member.user_id = member.tutor_id THEN COALESCE(relationships.display_name, groups.name)
        WHEN member.kind = 'group' THEN groups.name ELSE tutors.name END)::text AS label
FROM listed
JOIN active_channel_members AS member ON member.channel_id = listed.channel_id AND member.user_id = sqlc.arg(user_id)::uuid
JOIN users AS tutors ON tutors.id = member.tutor_id
LEFT JOIN relationships ON relationships.id = member.relationship_id
LEFT JOIN groups ON groups.id = member.group_id
ORDER BY listed.channel_id;

-- name: GetDigestLastSeen :one
SELECT last_seen_at FROM user_presence WHERE user_id = @user_id;
