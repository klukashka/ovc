-- name: BlockRequester :execrows
INSERT INTO blocked_requesters (tutor_id, student_id, created_at)
VALUES (@tutor_id, @student_id, @created_at)
ON CONFLICT (tutor_id, student_id) DO NOTHING;

-- name: UnblockRequester :execrows
DELETE FROM blocked_requesters WHERE tutor_id = @tutor_id AND student_id = @student_id;

-- name: ListBlockedRequesters :many
SELECT blocked_requesters.student_id, blocked_requesters.created_at, students.name, students.state
FROM blocked_requesters
JOIN users AS students ON students.id = blocked_requesters.student_id
WHERE blocked_requesters.tutor_id = @tutor_id
    AND (
        sqlc.narg(after_created_at)::timestamptz IS NULL
        OR (
            blocked_requesters.created_at <= sqlc.narg(after_created_at)::timestamptz
            AND (
                blocked_requesters.created_at < sqlc.narg(after_created_at)::timestamptz
                OR blocked_requesters.student_id > sqlc.narg(after_id)::uuid
            )
        )
    )
ORDER BY blocked_requesters.created_at DESC, blocked_requesters.student_id
LIMIT sqlc.arg(row_limit);

-- name: StudentBlockedByTutor :one
SELECT EXISTS (SELECT 1 FROM blocked_requesters WHERE tutor_id = @tutor_id AND student_id = @student_id);

-- name: DeleteTutorBlockedRequestersBatch :execrows
DELETE FROM blocked_requesters
WHERE (tutor_id, student_id) IN (
    SELECT doomed.tutor_id, doomed.student_id FROM blocked_requesters AS doomed
    WHERE doomed.tutor_id = @tutor_id
    ORDER BY doomed.student_id
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE
);
