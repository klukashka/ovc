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
