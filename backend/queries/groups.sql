-- name: CreateGroup :exec
INSERT INTO groups (id, tutor_id, name, colour, create_key, created_at, updated_at)
VALUES (@id, @tutor_id, @name, @colour, @create_key, @created_at, @created_at);

-- name: GetTutorGroup :one
SELECT * FROM groups WHERE id = @id AND tutor_id = @tutor_id;

-- name: LockTutorGroup :one
SELECT * FROM groups WHERE id = @id AND tutor_id = @tutor_id FOR UPDATE;

-- name: GetTutorGroupByCreateKey :one
SELECT * FROM groups WHERE tutor_id = @tutor_id AND create_key = @create_key;

-- name: CountTutorGroups :one
SELECT count(id) FROM groups WHERE tutor_id = @tutor_id;

-- name: ListTutorGroups :many
SELECT * FROM groups WHERE tutor_id = @tutor_id ORDER BY archived_at IS NOT NULL, created_at, id;

-- name: UpdateGroupDetails :execrows
UPDATE groups SET name = @name, colour = @colour, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND tutor_id = @tutor_id;

-- name: ArchiveGroup :one
UPDATE groups SET archived_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND tutor_id = @tutor_id AND archived_at IS NULL
RETURNING archived_at;

-- name: UnarchiveGroup :execrows
UPDATE groups SET archived_at = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND tutor_id = @tutor_id AND archived_at IS NOT NULL;

-- name: AddGroupMembers :execrows
INSERT INTO group_members (group_id, relationship_id, tutor_id, added_at)
SELECT sqlc.arg(group_id)::uuid, member.id, sqlc.arg(tutor_id)::uuid, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(relationship_ids)::uuid[]) AS member (id)
ON CONFLICT (group_id, relationship_id) DO NOTHING;

-- name: RemoveGroupMembers :many
WITH removed AS (
    DELETE FROM group_members
    WHERE group_members.group_id = @group_id AND group_members.tutor_id = @tutor_id
        AND group_members.relationship_id = ANY (sqlc.arg(relationship_ids)::uuid[])
    RETURNING group_members.relationship_id, group_members.tutor_id
)
SELECT relationships.student_id FROM removed
JOIN relationships ON relationships.id = removed.relationship_id AND relationships.tutor_id = removed.tutor_id;

-- name: ListGroupMemberIDs :many
SELECT relationship_id FROM group_members WHERE group_id = @group_id AND tutor_id = @tutor_id;

-- name: ListGroupMembers :many
SELECT group_members.relationship_id, relationships.display_name, relationships.student_id, users.name, group_members.added_at,
    relationships.state AS card_state, users.state AS account_state
FROM group_members
JOIN relationships ON relationships.id = group_members.relationship_id AND relationships.tutor_id = group_members.tutor_id
JOIN users ON users.id = relationships.student_id
WHERE group_members.group_id = @group_id AND group_members.tutor_id = @tutor_id
ORDER BY group_members.added_at, group_members.relationship_id;

-- name: ListFirstGroupMembers :many
SELECT ranked.group_id, ranked.relationship_id, ranked.display_name, ranked.student_id, ranked.name, ranked.added_at,
    ranked.member_count, card.state AS card_state, account.state AS account_state
FROM (
    SELECT group_members.group_id, group_members.relationship_id, relationships.display_name, relationships.student_id,
        users.name, group_members.added_at,
        row_number() OVER (
            PARTITION BY group_members.group_id ORDER BY group_members.added_at, group_members.relationship_id
        ) AS position,
        count(group_members.relationship_id) OVER (PARTITION BY group_members.group_id) AS member_count
    FROM group_members
    JOIN relationships ON relationships.id = group_members.relationship_id AND relationships.tutor_id = group_members.tutor_id
    JOIN users ON users.id = relationships.student_id
    WHERE group_members.tutor_id = @tutor_id AND group_members.group_id = ANY (sqlc.arg(group_ids)::uuid[])
) AS ranked
JOIN relationships AS card ON card.id = ranked.relationship_id
JOIN users AS account ON account.id = ranked.student_id
WHERE ranked.position <= sqlc.arg(first_members)::bigint
ORDER BY ranked.group_id, ranked.position;

-- name: ArchiveTutorGroups :many
UPDATE groups SET archived_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE tutor_id = @tutor_id AND archived_at IS NULL
RETURNING id;

-- name: DeleteTutorGroupMembersBatch :execrows
DELETE FROM group_members
WHERE (group_id, relationship_id) IN (
    SELECT doomed.group_id, doomed.relationship_id FROM groups
    JOIN group_members AS doomed ON doomed.group_id = groups.id
    WHERE groups.tutor_id = @tutor_id
    ORDER BY doomed.group_id, doomed.relationship_id
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE OF doomed
);

-- name: DeleteTutorGroupsBatch :execrows
DELETE FROM groups
WHERE id IN (
    SELECT doomed.id FROM groups AS doomed
    WHERE doomed.tutor_id = @tutor_id
    ORDER BY doomed.id
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE
);

-- name: LockStudentDeletionGroups :many
SELECT groups.id FROM groups
WHERE groups.id IN (
    SELECT group_members.group_id FROM group_members JOIN relationships ON relationships.id = group_members.relationship_id
    WHERE relationships.student_id = @student_id
) OR groups.id IN (
    SELECT channels.group_id FROM channels JOIN channel_members ON channel_members.channel_id = channels.id
    WHERE channel_members.user_id = @student_id
)
ORDER BY groups.id
FOR UPDATE;
