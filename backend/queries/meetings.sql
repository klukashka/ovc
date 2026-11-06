-- name: CreateMeetingRow :execrows
WITH reserved AS (
    UPDATE meeting_recovery_capacity SET unsettled = unsettled + 1 WHERE singleton AND unsettled < 4096 RETURNING singleton
), created AS (
    INSERT INTO meetings (id, lesson_id, provider, provider_meeting_id, routing_key, state, created_at, updated_at)
    SELECT $1, $2, $3, $4, $5, 'creating', $6, $6 FROM reserved
    RETURNING id, provider, provider_meeting_id, created_at, updated_at
)
INSERT INTO meeting_recovery (provider_meeting_id, provider, meeting_id, origin, create_state, created_at, updated_at)
SELECT provider_meeting_id, provider, id, 'attempt', 'prepared', created_at, updated_at FROM created;

-- name: GetMeetingRow :one
SELECT * FROM meetings WHERE lesson_id = $1 AND id = $2;

-- name: GetCurrentMeetingRow :one
SELECT * FROM meetings WHERE lesson_id = $1 AND state <> 'closed';

-- name: OpenMeetingRow :execrows
WITH current_meeting AS MATERIALIZED (
    SELECT meetings.id, meetings.provider_meeting_id FROM meetings
    WHERE meetings.lesson_id = sqlc.arg(lesson_id) AND meetings.id = sqlc.arg(id) AND meetings.provider_meeting_id = sqlc.arg(provider_meeting_id)
        AND meetings.state = 'creating' AND meetings.create_time IS NULL AND sqlc.arg(now) >= meetings.updated_at
    FOR UPDATE
), evidence AS MATERIALIZED (
    SELECT recovery.meeting_id FROM meeting_recovery AS recovery JOIN current_meeting
        ON recovery.meeting_id = current_meeting.id AND recovery.provider_meeting_id = current_meeting.provider_meeting_id
    WHERE recovery.create_state = 'returned' AND recovery.create_time = sqlc.arg(create_time) AND NOT recovery.cleanup_requested AND recovery.settled_at IS NULL
    FOR UPDATE OF recovery
)
UPDATE meetings SET state = 'open', create_time = sqlc.arg(create_time), updated_at = sqlc.arg(now)
FROM evidence WHERE meetings.id = evidence.meeting_id;

-- name: RecordMeetingFirstJoin :execrows
UPDATE meetings SET first_join_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE meetings.lesson_id = sqlc.arg(lesson_id) AND meetings.id = sqlc.arg(id) AND meetings.provider_meeting_id = sqlc.arg(provider_meeting_id)
    AND state = 'open' AND create_time = sqlc.arg(create_time) AND first_join_at IS NULL AND sqlc.arg(now) >= updated_at;

-- name: CloseMeetingRow :execrows
WITH closed AS (
    UPDATE meetings SET state = 'closed', close_reason = sqlc.arg(close_reason),
        ended_at = greatest(meetings.updated_at, sqlc.arg(now)), updated_at = greatest(meetings.updated_at, sqlc.arg(now))
    WHERE meetings.lesson_id = sqlc.arg(lesson_id) AND meetings.id = sqlc.arg(id) AND meetings.provider_meeting_id = sqlc.arg(provider_meeting_id)
        AND state = sqlc.arg(expected_state) AND state IN ('creating', 'open')
        AND create_time IS NOT DISTINCT FROM sqlc.narg(expected_create_time)::bigint
    RETURNING meetings.id, meetings.provider_meeting_id
)
UPDATE meeting_recovery AS recovery
SET cleanup_requested = true, revision = revision + 1, updated_at = greatest(recovery.updated_at, sqlc.arg(now))
FROM closed WHERE recovery.meeting_id = closed.id AND recovery.provider_meeting_id = closed.provider_meeting_id;

-- name: ListStaleCreatingMeetings :many
SELECT lesson_id, id, provider_meeting_id FROM meetings
WHERE state = 'creating' AND created_at <= sqlc.arg(before)
ORDER BY created_at, id LIMIT sqlc.arg(batch_size);

-- name: CountUnsettledMeetingsOfTutor :one
SELECT count(recovery.provider_meeting_id) FROM meeting_recovery AS recovery
JOIN meetings ON meetings.id = recovery.meeting_id
JOIN lessons ON lessons.id = meetings.lesson_id
WHERE recovery.settled_at IS NULL AND lessons.tutor_id = @tutor_id;

-- name: LessonHasFreshUncertainMeeting :one
SELECT EXISTS (
    SELECT 1 FROM meetings
    JOIN meeting_recovery AS recovery ON recovery.meeting_id = meetings.id
    WHERE meetings.lesson_id = @lesson_id AND meetings.state = 'closed' AND meetings.ended_at > sqlc.arg(since)::timestamptz
        AND meetings.ended_at <= sqlc.arg(now)::timestamptz
        AND recovery.create_state = 'uncertain' AND recovery.settled_at IS NULL
) AS fresh;
