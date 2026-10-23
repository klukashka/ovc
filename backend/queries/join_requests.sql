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
WHERE tutor_id = @tutor_id AND student_id = @student_id AND state IN ('held', 'pending');

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
SELECT count(id) FROM join_requests WHERE tutor_id = @tutor_id AND state = 'pending' AND suppressed = 'none';

-- name: StudentBlockedByTutor :one
SELECT EXISTS (SELECT 1 FROM blocked_requesters WHERE tutor_id = @tutor_id AND student_id = @student_id);

-- name: LockTutorOwnerChoiceJoinRequests :many
SELECT * FROM join_requests
WHERE tutor_id = @tutor_id AND state = 'held' AND owner_choice_pending
ORDER BY id
FOR UPDATE;
