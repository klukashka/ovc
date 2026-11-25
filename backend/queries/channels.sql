-- name: EnsureDirectChannels :execrows
INSERT INTO channels (id, kind, tutor_id, relationship_id, last_activity_at, created_at, updated_at)
SELECT candidate.id, 'direct', relationships.tutor_id, relationships.id, sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz,
    sqlc.arg(now)::timestamptz
FROM (SELECT unnest(sqlc.arg(ids)::uuid[]) AS id, unnest(sqlc.arg(relationship_ids)::uuid[]) AS relationship_id) AS candidate
JOIN relationships ON relationships.id = candidate.relationship_id
WHERE relationships.state IN ('provisional', 'active', 'ended', 'reclaim_pending')
ON CONFLICT (relationship_id) DO NOTHING;

-- name: EnsureGroupChannels :execrows
INSERT INTO channels (id, kind, tutor_id, group_id, last_activity_at, created_at, updated_at)
SELECT candidate.id, 'group', groups.tutor_id, groups.id, sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz
FROM (SELECT unnest(sqlc.arg(ids)::uuid[]) AS id, unnest(sqlc.arg(group_ids)::uuid[]) AS group_id) AS candidate
JOIN groups ON groups.id = candidate.group_id
ON CONFLICT (group_id) DO NOTHING;

-- name: LockChannelsOfAudiences :many
SELECT id, kind, tutor_id, relationship_id, group_id, last_seq, read_only_since, discarded_at FROM channels
WHERE relationship_id = ANY (sqlc.arg(relationship_ids)::uuid[]) OR group_id = ANY (sqlc.arg(group_ids)::uuid[])
ORDER BY id
FOR UPDATE;

-- name: LockChannelsOfTutor :many
SELECT id, kind, tutor_id, relationship_id, group_id, last_seq, read_only_since, discarded_at FROM channels
WHERE tutor_id = @tutor_id
ORDER BY id
FOR UPDATE;

-- name: LockChannelsOfTutorForShare :many
SELECT id FROM channels WHERE tutor_id = @tutor_id ORDER BY id FOR SHARE;

-- name: LockChannelOfRelationshipForShare :one
SELECT id FROM channels WHERE relationship_id = @relationship_id FOR SHARE;

-- name: LockChannelsOfMember :many
SELECT channels.id FROM channels
WHERE channels.id IN (SELECT channel_members.channel_id FROM channel_members WHERE channel_members.user_id = @user_id)
ORDER BY channels.id
FOR UPDATE;

-- name: ListActiveMembersOfChannels :many
SELECT channel_id, user_id, tutor_id FROM active_channel_members
WHERE channel_id = ANY (sqlc.arg(channel_ids)::uuid[])
ORDER BY channel_id, user_id;

-- name: SyncChannelReadOnly :many
UPDATE channels
SET read_only_since = derived.since, updated_at = sqlc.arg(now)::timestamptz
FROM (
    SELECT candidate.id,
        CASE
            WHEN candidate.kind = 'group' THEN groups.archived_at
            WHEN relationships.state = 'ended' THEN relationships.ended_at
            WHEN relationships.state = 'discarded' THEN candidate.read_only_since
        END AS since
    FROM channels AS candidate
    LEFT JOIN relationships ON relationships.id = candidate.relationship_id
    LEFT JOIN groups ON groups.id = candidate.group_id
    WHERE candidate.id = ANY (sqlc.arg(channel_ids)::uuid[]) AND candidate.discarded_at IS NULL
) AS derived
WHERE channels.id = derived.id AND channels.read_only_since IS DISTINCT FROM derived.since
RETURNING channels.id, channels.read_only_since;

-- name: DiscardChannelsOfDiscardedLinks :many
UPDATE channels
SET discarded_at = sqlc.arg(now)::timestamptz, erase_pending_since = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
FROM relationships
WHERE channels.id = ANY (sqlc.arg(channel_ids)::uuid[]) AND relationships.id = channels.relationship_id
    AND relationships.state = 'discarded' AND relationships.discard_reason <> 'student_purged' AND channels.discarded_at IS NULL
RETURNING channels.id, relationships.discard_reason;

-- name: DeleteMembersOfChannels :execrows
DELETE FROM channel_members WHERE channel_id = ANY (sqlc.arg(channel_ids)::uuid[]);

-- name: AddDirectChannelMembers :many
INSERT INTO channel_members (channel_id, user_id, last_read_seq, digested_through_seq, suspended_since, created_at, updated_at)
SELECT channels.id, member.user_id, channels.last_seq, channels.last_seq,
    CASE WHEN member.student AND relationships.state = 'reclaim_pending' THEN sqlc.arg(now)::timestamptz END,
    sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz
FROM channels
JOIN relationships ON relationships.id = channels.relationship_id
CROSS JOIN LATERAL (VALUES (relationships.tutor_id, false), (relationships.student_id, true)) AS member (user_id, student)
WHERE channels.id = ANY (sqlc.arg(channel_ids)::uuid[]) AND channels.discarded_at IS NULL AND member.user_id IS NOT NULL
    AND relationships.state IN ('provisional', 'active', 'ended', 'reclaim_pending')
ON CONFLICT (channel_id, user_id) DO NOTHING
RETURNING channel_id, user_id;

-- name: SuspendReclaimedMembers :many
WITH suspended AS (
    UPDATE channel_members
    SET suspended_since = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
    FROM channels
    JOIN relationships ON relationships.id = channels.relationship_id
    WHERE channel_members.channel_id = channels.id AND channel_members.user_id = relationships.student_id
        AND channels.id = ANY (sqlc.arg(channel_ids)::uuid[]) AND channels.discarded_at IS NULL
        AND relationships.state = 'reclaim_pending' AND channel_members.suspended_since IS NULL
    RETURNING channel_members.channel_id, channel_members.user_id
)
UPDATE channels
SET provisional_through_seq = channels.last_seq, updated_at = sqlc.arg(now)::timestamptz
FROM suspended
WHERE channels.id = suspended.channel_id
RETURNING suspended.channel_id, suspended.user_id;

-- name: RestoreKeptMembers :many
UPDATE channel_members
SET suspended_since = NULL,
    last_read_seq = CASE
        WHEN channels.provisional_through_seq IS NULL THEN channel_members.last_read_seq
        ELSE COALESCE((
            SELECT min(messages.seq) - 1 FROM messages
            WHERE messages.channel_id = channels.id AND messages.seq <= channels.provisional_through_seq
                AND messages.sender_id IS DISTINCT FROM channel_members.user_id
        ), channels.provisional_through_seq)
    END,
    digested_through_seq = channels.last_seq, updated_at = sqlc.arg(now)::timestamptz
FROM channels
JOIN relationships ON relationships.id = channels.relationship_id
WHERE channel_members.channel_id = channels.id AND channel_members.user_id = relationships.student_id
    AND channels.id = ANY (sqlc.arg(channel_ids)::uuid[]) AND channels.discarded_at IS NULL
    AND relationships.state IN ('provisional', 'active', 'ended') AND channel_members.suspended_since IS NOT NULL
RETURNING channel_members.channel_id, channel_members.user_id;

-- name: RemoveFormerGroupMembers :many
DELETE FROM channel_members
USING channels
WHERE channel_members.channel_id = channels.id AND channels.id = ANY (sqlc.arg(channel_ids)::uuid[]) AND channels.kind = 'group'
    AND channel_members.user_id <> channels.tutor_id
    AND NOT EXISTS (
        SELECT 1 FROM group_members
        JOIN relationships ON relationships.id = group_members.relationship_id AND relationships.tutor_id = group_members.tutor_id
        WHERE group_members.group_id = channels.group_id AND relationships.student_id = channel_members.user_id
    )
RETURNING channel_members.channel_id, channel_members.user_id;

-- name: AddGroupChannelMembers :many
INSERT INTO channel_members (channel_id, user_id, last_read_seq, digested_through_seq, created_at, updated_at)
SELECT channels.id, member.user_id, channels.last_seq, channels.last_seq, sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz
FROM channels
CROSS JOIN LATERAL (
    SELECT channels.tutor_id AS user_id
    UNION
    SELECT relationships.student_id FROM group_members
    JOIN relationships ON relationships.id = group_members.relationship_id AND relationships.tutor_id = group_members.tutor_id
    WHERE group_members.group_id = channels.group_id AND relationships.student_id IS NOT NULL
) AS member
WHERE channels.id = ANY (sqlc.arg(channel_ids)::uuid[]) AND channels.kind = 'group'
ON CONFLICT (channel_id, user_id) DO NOTHING
RETURNING channel_id, user_id;

-- name: MarkPendingUploadsOfChannels :many
UPDATE attachments SET state = 'deleted', expires_at = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE context = 'chat' AND state = 'pending' AND scope_kind = 'channel' AND scope_id = ANY (sqlc.arg(channel_ids)::uuid[])
RETURNING id, storage_key;

-- name: MarkPendingUploadsOfMembers :many
UPDATE attachments SET state = 'deleted', expires_at = NULL, updated_at = sqlc.arg(now)::timestamptz
FROM (SELECT unnest(sqlc.arg(channel_ids)::uuid[]) AS channel_id, unnest(sqlc.arg(user_ids)::uuid[]) AS user_id) AS former
WHERE attachments.context = 'chat' AND attachments.state = 'pending' AND attachments.scope_kind = 'channel'
    AND attachments.scope_id = former.channel_id AND attachments.uploader_id = former.user_id
RETURNING attachments.id, attachments.storage_key;

-- name: DeleteMembershipsOfUser :execrows
DELETE FROM channel_members WHERE user_id = @user_id;

-- name: ListChannelsOfMember :many
SELECT members.channel_id, members.kind, members.tutor_id, members.relationship_id, members.group_id, members.last_seq,
    members.last_read_seq, members.last_activity_at, members.read_only_since,
    (CASE WHEN members.tutor_id = members.user_id THEN COALESCE(relationships.display_name, groups.name, '') ELSE '' END)::text AS tutor_label,
    (relationships.id IS NOT NULL AND relationships.display_name IS NULL)::boolean AS card_name_erased,
    relationships.discard_reason AS card_discard_reason,
    (CASE WHEN members.tutor_id = members.user_id THEN COALESCE(groups.colour, relationships.colour) ELSE '' END)::text AS tutor_colour,
    COALESCE(relationships.state = 'provisional', false)::boolean AS provisional,
    relationships.student_id AS card_student_id, groups.name AS group_name, tutors.name AS tutor_name,
    own_link.colour AS own_colour,
    EXISTS (
        SELECT 1 FROM active_channel_members AS other
        WHERE other.channel_id = members.channel_id AND other.user_id <> members.user_id
    ) AS other_party_active,
    EXISTS (
        SELECT 1 FROM active_channel_members AS other
        JOIN user_presence ON user_presence.user_id = other.user_id
        WHERE other.channel_id = members.channel_id AND other.user_id <> members.user_id
            AND user_presence.online_until > sqlc.arg(now)::timestamptz
    ) AS other_party_online
FROM active_channel_members AS members
JOIN users AS tutors ON tutors.id = members.tutor_id
LEFT JOIN relationships ON relationships.id = members.relationship_id
LEFT JOIN groups ON groups.id = members.group_id
LEFT JOIN relationships AS own_link
    ON own_link.tutor_id = members.tutor_id AND own_link.student_id = members.user_id AND own_link.state <> 'discarded'
WHERE members.user_id = @user_id
    AND (sqlc.narg(channel_id)::uuid IS NULL OR members.channel_id = sqlc.narg(channel_id)::uuid)
    AND (
        sqlc.narg(pattern)::text IS NULL
        OR (members.tutor_id = members.user_id AND COALESCE(relationships.display_name, groups.name) ILIKE sqlc.narg(pattern)::text ESCAPE '\')
        OR (members.tutor_id <> members.user_id AND members.kind = 'direct' AND tutors.name ILIKE sqlc.narg(pattern)::text ESCAPE '\')
        OR (members.tutor_id <> members.user_id AND members.kind = 'group' AND groups.name ILIKE sqlc.narg(pattern)::text ESCAPE '\')
    )
    AND (
        sqlc.narg(after_activity_at)::timestamptz IS NULL
        OR (members.last_activity_at, members.channel_id) < (sqlc.narg(after_activity_at)::timestamptz, sqlc.narg(after_id)::uuid)
    )
ORDER BY members.last_activity_at DESC, members.channel_id DESC
LIMIT sqlc.arg(row_limit);

-- name: CountUnreadChannels :one
SELECT count(*) FROM (
    SELECT 1 FROM active_channel_members
    WHERE user_id = @user_id AND last_seq > last_read_seq
    LIMIT sqlc.arg(row_cap)::int
) AS unread;

-- name: ListChannelMemberLabels :many
SELECT members.user_id, relationships.display_name,
    COALESCE(user_presence.online_until > sqlc.arg(now)::timestamptz, false)::boolean AS online
FROM active_channel_members AS members
JOIN relationships
    ON relationships.tutor_id = members.tutor_id AND relationships.student_id = members.user_id AND relationships.state <> 'discarded'
LEFT JOIN user_presence ON user_presence.user_id = members.user_id
WHERE members.channel_id = @channel_id AND members.user_id <> members.tutor_id
ORDER BY relationships.display_name, members.user_id;

-- name: ListRelationshipChannelsOf :many
SELECT relationship_id, channel_id FROM active_channel_members
WHERE user_id = @user_id AND relationship_id = ANY (sqlc.arg(relationship_ids)::uuid[]);

-- name: GetGroupChannelOf :one
SELECT channel_id FROM active_channel_members WHERE user_id = @user_id AND group_id = @group_id;

-- name: ListLessonChannelsOf :many
SELECT lessons.id AS lesson_id, members.channel_id
FROM lessons
JOIN active_channel_members AS members
    ON members.relationship_id = lessons.relationship_id OR members.group_id = lessons.group_id
WHERE lessons.id = ANY (sqlc.arg(lesson_ids)::uuid[]) AND members.user_id = @user_id;

-- name: ListChannelsPendingErasure :many
SELECT channels.id, channels.tutor_id, relationships.discard_reason
FROM channels
JOIN relationships ON relationships.id = channels.relationship_id
WHERE channels.erase_pending_since IS NOT NULL
ORDER BY channels.erase_pending_since, channels.id
LIMIT sqlc.arg(batch_size);

-- name: LockChannelPendingErasure :one
SELECT id FROM channels WHERE id = @id AND erase_pending_since IS NOT NULL FOR UPDATE;

-- name: ClearChannelErasure :execrows
UPDATE channels SET erase_pending_since = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND erase_pending_since IS NOT NULL;

-- name: DeleteTutorChannelMembersBatch :execrows
DELETE FROM channel_members
WHERE (channel_id, user_id) IN (
    SELECT doomed.channel_id, doomed.user_id FROM channels
    JOIN channel_members AS doomed ON doomed.channel_id = channels.id
    WHERE channels.tutor_id = @tutor_id
    ORDER BY doomed.channel_id, doomed.user_id
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE OF doomed
);

-- name: DeleteTutorChannelsBatch :execrows
DELETE FROM channels
WHERE id IN (
    SELECT doomed.id FROM channels AS doomed
    WHERE doomed.tutor_id = @tutor_id
    ORDER BY doomed.id
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE
);

-- name: LockChannelForShare :one
SELECT id FROM channels WHERE id = @id FOR SHARE;

-- name: GetChannelOfActiveMember :one
SELECT channel_id, tutor_id, read_only_since FROM active_channel_members
WHERE channel_id = @channel_id AND user_id = @user_id;

-- name: ListStudentDeletionTutors :many
SELECT tutor_id FROM relationships WHERE student_id = @student_id
UNION
SELECT channels.tutor_id FROM channels
JOIN channel_members ON channel_members.channel_id = channels.id
WHERE channel_members.user_id = @student_id
ORDER BY tutor_id;

-- name: LockStudentDeletionChannels :many
SELECT channels.id FROM channels
WHERE channels.relationship_id = ANY (sqlc.arg(relationship_ids)::uuid[])
    OR channels.group_id = ANY (sqlc.arg(group_ids)::uuid[])
    OR channels.id IN (SELECT channel_id FROM channel_members WHERE channel_members.user_id = @user_id)
ORDER BY channels.id
FOR UPDATE;

-- name: GetChannelPin :one
SELECT pinned_message_id FROM channels WHERE id = @id;

-- name: SetChannelPin :exec
UPDATE channels SET pinned_message_id = @message_id, updated_at = sqlc.arg(now)::timestamptz WHERE id = @id;

-- name: ClearChannelPin :execrows
UPDATE channels SET pinned_message_id = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND pinned_message_id IS NOT NULL;

-- name: ClearChannelPinOfMessage :execrows
UPDATE channels SET pinned_message_id = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND pinned_message_id = @message_id;

-- name: ClearPinsOfMessages :many
WITH pinned AS (
    SELECT channels.id, channels.pinned_message_id AS message_id, channels.tutor_id FROM channels
    WHERE channels.pinned_message_id = ANY (sqlc.arg(message_ids)::uuid[])
)
UPDATE channels SET pinned_message_id = NULL, updated_at = sqlc.arg(now)::timestamptz
FROM pinned
WHERE channels.id = pinned.id
RETURNING channels.id, pinned.message_id::uuid AS message_id, pinned.tutor_id;

-- name: ClearPinsOfTutorChannels :execrows
UPDATE channels SET pinned_message_id = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE tutor_id = @tutor_id AND pinned_message_id IS NOT NULL;

