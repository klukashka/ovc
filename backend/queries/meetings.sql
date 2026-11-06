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
SELECT id, lesson_id, provider, provider_meeting_id, routing_key, state, create_time, created_at, updated_at, first_join_at, ended_at, close_reason FROM meetings WHERE lesson_id = $1 AND id = $2;

-- name: GetCurrentMeetingRow :one
SELECT id, lesson_id, provider, provider_meeting_id, routing_key, state, create_time, created_at, updated_at, first_join_at, ended_at, close_reason FROM meetings WHERE lesson_id = $1 AND state <> 'closed';

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

-- name: InsertLessonParticipant :exec
INSERT INTO lesson_participants (lesson_id, account_id, participant_id, created_at)
VALUES (@lesson_id, @account_id, @participant_id, sqlc.arg(now)::timestamptz)
ON CONFLICT (lesson_id, account_id) DO NOTHING;

-- name: GetLessonParticipant :one
SELECT participant_id FROM lesson_participants WHERE lesson_id = @lesson_id AND account_id = @account_id;

-- name: DeleteAccountParticipants :execrows
DELETE FROM lesson_participants WHERE account_id = @account_id;

-- name: ListCurrentMeetingsOfTutor :many
SELECT meetings.lesson_id, meetings.id, meetings.provider_meeting_id, meetings.state, meetings.create_time FROM meetings
JOIN lessons ON lessons.id = meetings.lesson_id
WHERE lessons.tutor_id = @tutor_id AND meetings.state <> 'closed'
ORDER BY meetings.lesson_id;

-- name: ListUnconfirmedMeetingRevocations :many
SELECT meetings.id, meetings.provider_meeting_id, meetings.create_time FROM meetings
LEFT JOIN meeting_recovery AS recovery ON recovery.meeting_id = meetings.id
WHERE meetings.lesson_id = @lesson_id AND meetings.state = 'closed' AND meetings.close_reason = 'tutor_restart'
    AND meetings.revocation_confirmed_at IS NULL AND meetings.create_time IS NOT NULL AND recovery.settled_at IS NULL
ORDER BY meetings.created_at, meetings.id
LIMIT 16;

-- name: ConfirmMeetingRevocation :execrows
UPDATE meetings SET revocation_confirmed_at = sqlc.arg(now)::timestamptz
WHERE lesson_id = @lesson_id AND id = @id AND state = 'closed' AND close_reason = 'tutor_restart' AND revocation_confirmed_at IS NULL;

-- name: AcceptMeetingEndedHint :execrows
WITH accepted AS (
    UPDATE meetings SET ended_hint_at = sqlc.arg(now)::timestamptz, ended_hint_until = sqlc.arg(chain_until)::timestamptz, ended_hint_checks = 0
    WHERE routing_key = @routing_key AND state = 'open' AND first_join_at IS NOT NULL AND created_at <= sqlc.arg(created_before)::timestamptz
        AND (ended_hint_at IS NULL OR ended_hint_at <= sqlc.arg(debounced_before)::timestamptz)
        AND (ended_hint_until IS NULL OR ended_hint_until <= sqlc.arg(now)::timestamptz)
    RETURNING meetings.id AS meeting_id, meetings.lesson_id AS meeting_lesson
)
INSERT INTO outbox (id, lane, priority, kind, payload, idempotency_key, status, next_attempt_at, created_at)
SELECT sqlc.arg(outbox_id)::uuid, 'provider_call', 1, 'meeting_ended_hint',
    jsonb_build_object('lesson_id', meeting_lesson, 'meeting_id', meeting_id, 'check', 0),
    'meeting-hint:' || sqlc.arg(outbox_id)::uuid::text, 'pending', sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz FROM accepted;

-- name: AdvanceMeetingEndedHint :execrows
WITH advanced AS (
    UPDATE meetings SET ended_hint_checks = ended_hint_checks + 1
    WHERE meetings.lesson_id = @lesson_id AND meetings.id = @id AND state = 'open' AND ended_hint_checks = sqlc.arg(expected_check)::smallint AND ended_hint_checks < 3
    RETURNING meetings.id AS meeting_id, meetings.lesson_id AS meeting_lesson, meetings.ended_hint_checks AS next_check
)
INSERT INTO outbox (id, lane, priority, kind, payload, idempotency_key, status, next_attempt_at, created_at)
SELECT sqlc.arg(outbox_id)::uuid, 'provider_call', 1, 'meeting_ended_hint',
    jsonb_build_object('lesson_id', meeting_lesson, 'meeting_id', meeting_id, 'check', next_check),
    'meeting-hint:' || sqlc.arg(outbox_id)::uuid::text, 'pending', sqlc.arg(next_attempt_at)::timestamptz, sqlc.arg(now)::timestamptz FROM advanced;

-- name: GetMeetingEndedHintChecks :one
SELECT ended_hint_checks FROM meetings WHERE lesson_id = @lesson_id AND id = @id;

-- name: FinishMeetingEndedHint :execrows
UPDATE meetings SET ended_hint_until = NULL
WHERE lesson_id = @lesson_id AND id = @id AND state = 'open' AND ended_hint_checks = sqlc.arg(expected_check)::smallint;

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
