-- name: CreateJoinRequest :exec
INSERT INTO join_requests (id, tutor_id, student_id, state, suppressed, owner_choice_pending, expires_at, created_at)
VALUES (@id, @tutor_id, @student_id, @state, @suppressed, @owner_choice_pending, @expires_at, @created_at);

-- name: DecideJoinRequest :execrows
UPDATE join_requests
SET state = sqlc.arg(decision)::text, decided_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND tutor_id = @tutor_id AND state = 'pending' AND suppressed = 'none';

-- name: AcceptWaitingJoinRequests :execrows
UPDATE join_requests
SET state = 'accepted', suppressed = 'none', owner_choice_pending = false, decided_at = sqlc.arg(now)::timestamptz
WHERE tutor_id = @tutor_id AND student_id = @student_id
    AND (
        (state IN ('held', 'pending') AND expires_at > sqlc.arg(now)::timestamptz)
        OR (state = 'expired' AND decided_at > sqlc.arg(now)::timestamptz)
    );

-- name: MarkHeldJoinRequestsForOwnerChoice :execrows
UPDATE join_requests SET owner_choice_pending = true
WHERE student_id = @student_id AND state = 'held' AND NOT owner_choice_pending;

-- name: ListOwnerChoiceJoinRequests :many
SELECT join_requests.id, join_requests.tutor_id, join_requests.expires_at, tutors.name, tutor_profiles.subjects
FROM join_requests
JOIN users AS tutors ON tutors.id = join_requests.tutor_id
JOIN tutor_profiles ON tutor_profiles.user_id = join_requests.tutor_id
WHERE join_requests.student_id = @student_id AND join_requests.state = 'held' AND join_requests.owner_choice_pending
ORDER BY join_requests.created_at, join_requests.id;

-- name: ListOwnerChoiceJoinRequestRows :many
SELECT * FROM join_requests
WHERE student_id = @student_id AND state = 'held' AND owner_choice_pending
ORDER BY id;

-- name: LockOwnerChoiceJoinRequests :many
SELECT * FROM join_requests
WHERE student_id = @student_id AND state = 'held' AND owner_choice_pending
ORDER BY id
FOR UPDATE;

-- name: ReleaseHeldJoinRequest :execrows
UPDATE join_requests SET state = 'pending', suppressed = sqlc.arg(suppressed)::text, owner_choice_pending = false
WHERE id = @id AND state = 'held';

-- name: ExpireHeldJoinRequest :execrows
UPDATE join_requests SET state = 'expired', owner_choice_pending = false, decided_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state = 'held';

-- name: CountVisiblePendingJoinRequests :one
SELECT count(id) FROM join_requests
WHERE tutor_id = @tutor_id AND state = 'pending' AND suppressed = 'none' AND expires_at > sqlc.arg(now)::timestamptz;

-- name: LockOpenJoinRequest :one
SELECT * FROM join_requests
WHERE tutor_id = @tutor_id AND student_id = @student_id AND state IN ('held', 'pending')
FOR UPDATE;

-- name: ListHeldJoinRequestTutors :many
SELECT tutor_id FROM join_requests
WHERE student_id = @student_id AND state = 'held' AND NOT owner_choice_pending
ORDER BY tutor_id;

-- name: LockHeldJoinRequests :many
SELECT * FROM join_requests
WHERE student_id = @student_id AND state = 'held' AND NOT owner_choice_pending
ORDER BY id
FOR UPDATE;

-- name: ListOpenJoinRequestTutors :many
SELECT tutor_id FROM join_requests
WHERE student_id = @student_id AND state IN ('held', 'pending')
ORDER BY tutor_id;

-- name: ExpireOpenJoinRequestsOfStudent :many
UPDATE join_requests
SET state = 'expired', suppressed = 'none', owner_choice_pending = false,
    decided_at = LEAST(join_requests.expires_at, sqlc.arg(now)::timestamptz)
WHERE student_id = @student_id AND state IN ('held', 'pending')
RETURNING tutor_id;

-- name: CloseUnblockedJoinRequestsOfStudent :execrows
UPDATE join_requests SET decided_at = sqlc.arg(now)::timestamptz
WHERE student_id = @student_id AND state = 'expired' AND decided_at > sqlc.arg(now)::timestamptz;

-- name: GetTutorJoinRequest :one
SELECT join_requests.* FROM join_requests
JOIN users AS students ON students.id = join_requests.student_id
WHERE join_requests.id = @id AND join_requests.tutor_id = @tutor_id AND students.state <> 'blocked'
    AND (
        join_requests.state IN ('accepted', 'declined')
        OR (join_requests.state = 'expired' AND join_requests.decided_at <= sqlc.arg(now)::timestamptz)
        OR (join_requests.state = 'pending' AND join_requests.suppressed = 'none' AND join_requests.expires_at > sqlc.arg(now)::timestamptz)
    );

-- name: GetJoinRequestOfTutor :one
SELECT * FROM join_requests WHERE id = @id AND tutor_id = @tutor_id;

-- name: LockTutorJoinRequest :one
SELECT * FROM join_requests WHERE id = @id AND tutor_id = @tutor_id FOR UPDATE;

-- name: ListTutorJoinRequests :many
SELECT join_requests.id, join_requests.student_id, join_requests.created_at, join_requests.expires_at, students.name, students.email
FROM join_requests
JOIN users AS students ON students.id = join_requests.student_id
WHERE join_requests.tutor_id = @tutor_id AND join_requests.state = 'pending' AND join_requests.suppressed = 'none'
    AND students.state <> 'blocked' AND join_requests.expires_at > sqlc.arg(now)::timestamptz
    AND (
        sqlc.narg(after_created_at)::timestamptz IS NULL
        OR (join_requests.created_at, join_requests.id) > (sqlc.narg(after_created_at)::timestamptz, sqlc.narg(after_id)::uuid)
    )
ORDER BY join_requests.created_at, join_requests.id
LIMIT sqlc.arg(row_limit);

-- name: ListStudentJoinRequests :many
SELECT * FROM join_requests
WHERE student_id = @student_id
    AND (
        sqlc.narg(after_created_at)::timestamptz IS NULL
        OR (created_at, id) < (sqlc.narg(after_created_at)::timestamptz, sqlc.narg(after_id)::uuid)
    )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: UnsuppressJoinRequests :many
UPDATE join_requests SET suppressed = 'none'
WHERE join_requests.id IN (
    SELECT waiting.id FROM join_requests AS waiting
    WHERE waiting.tutor_id = @tutor_id AND waiting.state = 'pending' AND waiting.suppressed = 'limit'
        AND waiting.expires_at > sqlc.arg(now)::timestamptz
    ORDER BY waiting.created_at, waiting.id
    LIMIT sqlc.arg(slots)::bigint
    FOR UPDATE
)
RETURNING join_requests.id, join_requests.student_id;

-- name: ListDueJoinRequests :many
SELECT id, tutor_id, student_id, expires_at FROM join_requests
WHERE state IN ('held', 'pending') AND expires_at <= sqlc.arg(now)::timestamptz
    AND (
        sqlc.narg(after_expires_at)::timestamptz IS NULL
        OR (expires_at, id) > (sqlc.narg(after_expires_at)::timestamptz, sqlc.narg(after_id)::uuid)
    )
ORDER BY expires_at, id
LIMIT sqlc.arg(batch_size);

-- name: ExpireDueJoinRequest :execrows
UPDATE join_requests
SET state = 'expired', suppressed = 'none', owner_choice_pending = false, decided_at = expires_at
WHERE id = @id AND state IN ('held', 'pending') AND expires_at <= sqlc.arg(now)::timestamptz;

-- name: CloseBlockedJoinRequests :execrows
UPDATE join_requests
SET state = 'expired', suppressed = 'none', decided_at = expires_at
WHERE tutor_id = @tutor_id AND student_id = @student_id AND state = 'pending' AND suppressed = 'blocked';

-- name: LockJoinRequestClosedByUnblock :one
SELECT * FROM join_requests
WHERE tutor_id = @tutor_id AND student_id = @student_id AND state = 'expired' AND decided_at > sqlc.arg(now)::timestamptz
ORDER BY created_at DESC, id DESC
LIMIT 1
FOR UPDATE;

-- name: ReopenJoinRequest :execrows
UPDATE join_requests SET state = 'pending', suppressed = sqlc.arg(suppressed)::text, decided_at = NULL
WHERE id = @id AND state = 'expired' AND decided_at > sqlc.arg(now)::timestamptz;

-- name: ExpireJoinRequestsOfTutor :execrows
UPDATE join_requests
SET state = 'expired', suppressed = 'none', owner_choice_pending = false,
    decided_at = LEAST(join_requests.expires_at, sqlc.arg(now)::timestamptz)
WHERE tutor_id = @tutor_id
    AND (state IN ('held', 'pending') OR (state = 'expired' AND decided_at > sqlc.arg(now)::timestamptz));

-- name: DeleteTutorJoinRequestsBatch :execrows
DELETE FROM join_requests
WHERE id IN (
    SELECT doomed.id FROM join_requests AS doomed
    WHERE doomed.tutor_id = @tutor_id
    ORDER BY doomed.id
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE
);
