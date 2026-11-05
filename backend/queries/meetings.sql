-- name: CreateMeetingRow :exec
INSERT INTO meetings (id, lesson_id, provider, provider_meeting_id, routing_key, state, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, 'creating', $6, $6);

-- name: GetMeetingRow :one
SELECT * FROM meetings WHERE lesson_id = $1 AND id = $2;

-- name: GetCurrentMeetingRow :one
SELECT * FROM meetings WHERE lesson_id = $1 AND state <> 'closed';

-- name: OpenMeetingRow :execrows
UPDATE meetings SET state = 'open', create_time = sqlc.arg(create_time), updated_at = sqlc.arg(now)
WHERE lesson_id = sqlc.arg(lesson_id) AND id = sqlc.arg(id) AND provider_meeting_id = sqlc.arg(provider_meeting_id)
    AND state = 'creating' AND create_time IS NULL AND sqlc.arg(now) >= updated_at;

-- name: RecordMeetingFirstJoin :execrows
UPDATE meetings SET first_join_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE lesson_id = sqlc.arg(lesson_id) AND id = sqlc.arg(id) AND provider_meeting_id = sqlc.arg(provider_meeting_id)
    AND state = 'open' AND create_time = sqlc.arg(create_time) AND first_join_at IS NULL AND sqlc.arg(now) >= updated_at;

-- name: CloseMeetingRow :execrows
UPDATE meetings SET state = 'closed', close_reason = sqlc.arg(close_reason), ended_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE lesson_id = sqlc.arg(lesson_id) AND id = sqlc.arg(id) AND provider_meeting_id = sqlc.arg(provider_meeting_id)
    AND state = sqlc.arg(expected_state) AND state IN ('creating', 'open')
    AND create_time IS NOT DISTINCT FROM sqlc.narg(expected_create_time)::bigint AND sqlc.arg(now) >= updated_at;
