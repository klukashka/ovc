-- name: CreateRelationship :exec
INSERT INTO relationships (
    id, tutor_id, student_id, state, discard_reason, ended_by, display_name, email, colour, notes, create_key,
    linked_at, ended_at, discarded_at, created_at, updated_at
) VALUES (
    @id, @tutor_id, @student_id, @state, @discard_reason, @ended_by, @display_name, @email, @colour, @notes, @create_key,
    @linked_at, @ended_at, @discarded_at, @created_at, @created_at
);

-- name: GetTutorRelationship :one
SELECT * FROM relationships WHERE id = @id AND tutor_id = @tutor_id AND state <> 'discarded';

-- name: ListTutorRelationships :many
SELECT * FROM relationships
WHERE relationships.tutor_id = @tutor_id
    AND state = ANY (sqlc.arg(states)::text[])
    AND state <> 'discarded'
    AND (
        sqlc.narg(group_id)::uuid IS NULL
        OR EXISTS (
            SELECT 1 FROM group_members
            WHERE group_members.relationship_id = relationships.id AND group_members.group_id = sqlc.narg(group_id)::uuid
        )
    )
    AND (
        sqlc.narg(pattern)::text IS NULL
        OR display_name ILIKE sqlc.narg(pattern)::text ESCAPE '\'
        OR email::text ILIKE sqlc.narg(pattern)::text ESCAPE '\'
    )
    AND (
        sqlc.narg(after_created_at)::timestamptz IS NULL
        OR (created_at, id) < (sqlc.narg(after_created_at)::timestamptz, sqlc.narg(after_id)::uuid)
    )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: RemoveRelationship :one
UPDATE relationships
SET state = 'discarded', discard_reason = 'removed', discarded_at = sqlc.arg(now)::timestamptz, notes = '', email = NULL,
    updated_at = sqlc.arg(now)::timestamptz
FROM (
    SELECT locked.id, locked.email, locked.notes FROM relationships AS locked
    WHERE locked.id = @id AND locked.tutor_id = @tutor_id
    FOR UPDATE
) AS removed
WHERE relationships.id = removed.id AND relationships.state IN ('provisional', 'reclaim_pending')
RETURNING relationships.id, relationships.tutor_id, relationships.student_id, relationships.display_name, removed.email, removed.notes;

-- name: GetTutorRelationshipByCreateKey :one
SELECT * FROM relationships WHERE tutor_id = @tutor_id AND create_key = @create_key;

-- name: GetTutorCardIDByEmail :one
SELECT id FROM relationships WHERE tutor_id = @tutor_id AND email = @email AND state <> 'discarded';

-- name: LockTutorRelationship :one
SELECT * FROM relationships WHERE id = @id AND tutor_id = @tutor_id FOR UPDATE;

-- name: UpdateCardText :execrows
UPDATE relationships
SET display_name = @display_name, email = @email, notes = @notes, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND tutor_id = @tutor_id AND state <> 'discarded';

-- name: WithdrawCard :execrows
UPDATE relationships
SET state = 'discarded', discard_reason = 'withdrawn', discarded_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND tutor_id = @tutor_id AND state = 'invited';

-- name: ListCardAccounts :many
SELECT id, name, email, email_verified_at FROM users WHERE id = ANY (sqlc.arg(ids)::uuid[]);

-- name: ListAvatarsOf :many
SELECT ref_id, attachment_id FROM attachment_references
WHERE ref_kind = 'avatar' AND ref_id = ANY (sqlc.arg(owner_ids)::uuid[]);

-- name: ListCardGroups :many
SELECT group_members.relationship_id, groups.id, groups.name, groups.colour
FROM group_members
JOIN groups ON groups.id = group_members.group_id AND groups.tutor_id = group_members.tutor_id
WHERE group_members.tutor_id = @tutor_id
    AND group_members.relationship_id = ANY (sqlc.arg(relationship_ids)::uuid[])
    AND groups.archived_at IS NULL
ORDER BY group_members.relationship_id, groups.name, groups.id;

-- name: LinkInvitedRelationship :execrows
UPDATE relationships
SET state = @state, student_id = @student_id, colour = @colour, linked_at = sqlc.arg(now)::timestamptz,
    updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state = 'invited';

-- name: ListStudentColours :many
SELECT colour FROM relationships WHERE student_id = @student_id AND state <> 'discarded' AND colour IS NOT NULL;

-- name: LockStudentRelationships :many
SELECT * FROM relationships WHERE student_id = @student_id AND state <> 'discarded' ORDER BY id FOR UPDATE;

-- name: ActivateProvisionalRelationship :execrows
UPDATE relationships SET state = 'active', updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state = 'provisional';

-- name: DeleteStudentMemberships :execrows
DELETE FROM group_members
WHERE relationship_id IN (SELECT relationships.id FROM relationships WHERE relationships.student_id = @student_id);

-- name: DiscardRelationshipsOfPurgedStudent :execrows
UPDATE relationships
SET state = 'discarded', discard_reason = 'student_purged', discarded_at = sqlc.arg(now)::timestamptz,
    updated_at = sqlc.arg(now)::timestamptz
WHERE student_id = @student_id AND state <> 'discarded';

-- name: ClearPurgedStudentFromRelationships :execrows
UPDATE relationships SET student_id = NULL, email = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE student_id = @student_id;
