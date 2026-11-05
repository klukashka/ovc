-- name: GetMeetingRecovery :one
SELECT * FROM meeting_recovery WHERE provider_meeting_id = $1;

-- name: BeginMeetingCreate :execrows
WITH current_meeting AS MATERIALIZED (
    SELECT meetings.id, meetings.provider_meeting_id FROM meetings
    WHERE meetings.lesson_id = sqlc.arg(lesson_id) AND meetings.id = sqlc.arg(meeting_id)
        AND meetings.provider_meeting_id = sqlc.arg(provider_meeting_id) AND meetings.state = 'creating'
    FOR UPDATE
)
UPDATE meeting_recovery AS recovery
SET create_state = 'started', attempt_id = sqlc.arg(attempt_id), revision = revision + 1, updated_at = sqlc.arg(now)
FROM current_meeting
WHERE recovery.provider_meeting_id = current_meeting.provider_meeting_id AND recovery.meeting_id = current_meeting.id
    AND recovery.origin = 'attempt' AND recovery.create_state = 'prepared' AND NOT recovery.cleanup_requested
    AND recovery.revision = sqlc.arg(revision) AND sqlc.arg(now) >= recovery.updated_at;

-- name: RecordMeetingCreateReturned :execrows
UPDATE meeting_recovery
SET create_state = 'returned', create_time = sqlc.arg(create_time),
    revision = CASE WHEN create_state = 'returned' THEN revision ELSE revision + 1 END,
    updated_at = CASE WHEN create_state = 'returned' THEN updated_at ELSE greatest(updated_at, sqlc.arg(now)) END
WHERE provider_meeting_id = sqlc.arg(provider_meeting_id) AND origin = 'attempt' AND attempt_id = sqlc.arg(attempt_id)
    AND (create_state IN ('started', 'uncertain') OR (create_state = 'returned' AND create_time = sqlc.arg(create_time)));

-- name: MarkMeetingCreateUncertain :execrows
UPDATE meeting_recovery
SET create_state = 'uncertain', revision = revision + 1, updated_at = greatest(updated_at, sqlc.arg(now))
WHERE provider_meeting_id = sqlc.arg(provider_meeting_id) AND origin = 'attempt' AND attempt_id = sqlc.arg(attempt_id)
    AND create_state = 'started';

-- name: ListMeetingRecoveryCandidates :many
SELECT * FROM meeting_recovery
WHERE (cleanup_requested OR meeting_id IS NULL) AND provider_meeting_id > sqlc.arg(after_provider_meeting_id)
ORDER BY provider_meeting_id LIMIT sqlc.arg(batch_size);
