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
