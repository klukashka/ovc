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
SET create_state = 'started', attempt_id = sqlc.arg(attempt_id), dispatched_at = sqlc.arg(now)::timestamptz, revision = revision + 1, updated_at = sqlc.arg(now)
FROM current_meeting
WHERE recovery.provider_meeting_id = current_meeting.provider_meeting_id AND recovery.meeting_id = current_meeting.id
    AND recovery.origin = 'attempt' AND recovery.create_state = 'prepared' AND NOT recovery.cleanup_requested AND recovery.settled_at IS NULL
    AND recovery.revision = sqlc.arg(revision) AND sqlc.arg(now) >= recovery.updated_at;

-- name: RecordMeetingCreateReturned :execrows
UPDATE meeting_recovery
SET create_state = 'returned', create_time = sqlc.arg(create_time),
    final_cycle_requested = final_cycle_requested OR (create_state <> 'returned' AND cycle_number > 0),
    revision = CASE WHEN create_state = 'returned' THEN revision ELSE revision + 1 END,
    updated_at = CASE WHEN create_state = 'returned' THEN updated_at ELSE greatest(updated_at, sqlc.arg(now)) END
WHERE provider_meeting_id = sqlc.arg(provider_meeting_id) AND settled_at IS NULL AND origin = 'attempt' AND attempt_id = sqlc.arg(attempt_id)
    AND (create_state IN ('started', 'uncertain') OR (create_state = 'returned' AND create_time = sqlc.arg(create_time)));

-- name: MarkMeetingCreateUncertain :execrows
UPDATE meeting_recovery
SET create_state = 'uncertain', revision = revision + 1, updated_at = greatest(updated_at, sqlc.arg(now))
WHERE provider_meeting_id = sqlc.arg(provider_meeting_id) AND settled_at IS NULL AND origin = 'attempt' AND attempt_id = sqlc.arg(attempt_id)
    AND create_state = 'started';

-- name: ListMeetingRecoveryCandidates :many
SELECT * FROM meeting_recovery
WHERE settled_at IS NULL AND (cleanup_requested OR meeting_id IS NULL) AND provider_meeting_id > sqlc.arg(after_provider_meeting_id)
ORDER BY provider_meeting_id LIMIT sqlc.arg(batch_size);

-- name: GetMeetingRecoveryCapacity :one
SELECT unsettled FROM meeting_recovery_capacity WHERE singleton;

-- name: SettleMeetingRecovery :execrows
WITH capacity AS MATERIALIZED (
    SELECT singleton FROM meeting_recovery_capacity WHERE singleton FOR UPDATE
), settled AS (
    UPDATE meeting_recovery AS recovery
    SET settled_at = sqlc.arg(now), cleanup_requested = true, revision = recovery.revision + 1,
        settlement = CASE recovery.create_state WHEN 'prepared' THEN 'revoked' WHEN 'returned' THEN 'confirmed_end' ELSE 'quiet_absence' END,
        updated_at = greatest(recovery.updated_at, sqlc.arg(now))
    FROM capacity
    WHERE recovery.provider_meeting_id = sqlc.arg(provider_meeting_id) AND recovery.revision = sqlc.arg(revision)
        AND recovery.settled_at IS NULL AND (recovery.cleanup_requested OR recovery.meeting_id IS NULL)
        AND sqlc.arg(now) >= recovery.updated_at
        AND ((recovery.create_state = 'prepared' AND sqlc.narg(observed_create_time)::bigint IS NULL)
            OR (recovery.create_state = 'returned' AND recovery.create_time = sqlc.narg(observed_create_time)::bigint)
            OR (recovery.create_state IN ('started', 'uncertain') AND sqlc.narg(observed_create_time)::bigint IS NULL
                AND sqlc.narg(evaluated_at)::timestamptz <= sqlc.arg(now)
                AND recovery.dispatched_at <= sqlc.narg(evaluated_at)::timestamptz - interval '1 hour'))
    RETURNING recovery.provider_meeting_id
)
UPDATE meeting_recovery_capacity SET unsettled = unsettled - 1 WHERE singleton AND EXISTS (SELECT 1 FROM settled);

-- name: QueueMeetingRecoveryCycle :execrows
WITH queued AS (
    UPDATE meeting_recovery
    SET cycle_number = cycle_number + 1, cycle_outbox_id = sqlc.arg(outbox_id), cycle_result = 'pending',
        final_cycle_used = final_cycle_used OR (sqlc.arg(mode)::text = 'automatic' AND cycle_number > 0 AND create_state NOT IN ('started', 'uncertain')),
        reconcile_cycle_used = reconcile_cycle_used OR (sqlc.arg(mode)::text = 'automatic' AND cycle_number > 0 AND create_state IN ('started', 'uncertain')),
        revision = revision + 1, updated_at = greatest(updated_at, sqlc.arg(now))
    WHERE meeting_recovery.provider_meeting_id = sqlc.arg(provider_meeting_id) AND revision = sqlc.arg(revision)
        AND settled_at IS NULL AND (cleanup_requested OR meeting_id IS NULL) AND cycle_result <> 'pending'
        AND (sqlc.arg(mode)::text = 'operator' OR cycle_number = 0
            OR (create_state NOT IN ('started', 'uncertain') AND final_cycle_requested AND NOT final_cycle_used)
            OR (create_state IN ('started', 'uncertain') AND NOT reconcile_cycle_used AND dispatched_at <= sqlc.arg(now)::timestamptz - interval '1 hour'))
        AND NOT EXISTS (SELECT 1 FROM meeting_recovery_cycles WHERE outbox_id = sqlc.arg(outbox_id))
    RETURNING meeting_recovery.provider_meeting_id, meeting_recovery.cycle_number, meeting_recovery.cycle_outbox_id
), receipt AS (
    INSERT INTO meeting_recovery_cycles (outbox_id, provider_meeting_id, cycle_number, mode, result, created_at)
    SELECT cycle_outbox_id, provider_meeting_id, cycle_number, sqlc.arg(mode), 'pending', sqlc.arg(now) FROM queued
    RETURNING outbox_id, provider_meeting_id, cycle_number
)
INSERT INTO outbox (id, lane, priority, kind, payload, idempotency_key, status, next_attempt_at, created_at)
SELECT outbox_id, 'provider_call', 1, 'meeting_cleanup',
    jsonb_build_object('provider_meeting_id', provider_meeting_id, 'cycle', cycle_number),
    'meeting-cleanup:' || outbox_id::text, 'pending', sqlc.arg(now), sqlc.arg(now) FROM receipt;

-- name: CompleteMeetingRecoveryCycle :execrows
WITH completed AS (
    UPDATE meeting_recovery SET cycle_result = sqlc.arg(result), revision = revision + 1,
        updated_at = greatest(updated_at, sqlc.arg(now))
    WHERE meeting_recovery.provider_meeting_id = sqlc.arg(provider_meeting_id) AND cycle_outbox_id = sqlc.arg(outbox_id)
        AND cycle_result = 'pending' AND (sqlc.arg(result)::text <> 'settled' OR settled_at IS NOT NULL)
    RETURNING cycle_outbox_id
)
UPDATE meeting_recovery_cycles SET result = sqlc.arg(result), completed_at = greatest(created_at, sqlc.arg(now))
FROM completed WHERE outbox_id = completed.cycle_outbox_id AND result = 'pending';

-- name: ListMeetingRecovery :many
SELECT * FROM meeting_recovery WHERE provider_meeting_id > sqlc.arg(after_provider_meeting_id)
ORDER BY provider_meeting_id LIMIT sqlc.arg(batch_size);

-- name: GetMeetingRecoveryCycle :one
SELECT * FROM meeting_recovery_cycles WHERE outbox_id = sqlc.arg(outbox_id);

-- name: ListMeetingRecoveryCycles :many
SELECT * FROM meeting_recovery_cycles
WHERE provider_meeting_id = sqlc.arg(provider_meeting_id) AND cycle_number > sqlc.arg(after_cycle_number)
ORDER BY cycle_number LIMIT sqlc.arg(batch_size);

-- name: GetMeetingRecoveryCursor :one
SELECT after_provider_meeting_id, revision FROM meeting_recovery_cursor WHERE singleton;

-- name: AdvanceMeetingRecoveryCursor :execrows
UPDATE meeting_recovery_cursor SET after_provider_meeting_id = sqlc.arg(after_provider_meeting_id), revision = revision + 1
WHERE singleton AND revision = sqlc.arg(revision);

-- name: DeleteMeetingRecoveryReceipts :execrows
DELETE FROM meeting_recovery WHERE provider_meeting_id IN (
    SELECT provider_meeting_id FROM meeting_recovery
    WHERE settled_at < sqlc.arg(now)::timestamptz - interval '7 days' AND cycle_result <> 'pending'
    ORDER BY settled_at, provider_meeting_id LIMIT sqlc.arg(batch_size)
    FOR UPDATE SKIP LOCKED
);

-- name: LockMeetingRecoveryCapacity :one
SELECT unsettled FROM meeting_recovery_capacity WHERE singleton FOR UPDATE;

-- name: LockMeetingRecoveryForOutbox :one
SELECT recovery.* FROM meeting_recovery AS recovery
JOIN meeting_recovery_cycles AS cycle ON cycle.provider_meeting_id = recovery.provider_meeting_id
WHERE cycle.outbox_id = sqlc.arg(outbox_id)
FOR UPDATE OF recovery;

-- name: LockMeetingRecoveryOutbox :one
SELECT * FROM outbox WHERE id = sqlc.arg(id) AND kind = 'meeting_cleanup' FOR UPDATE;

-- name: FinishMeetingRecoveryOutbox :execrows
UPDATE outbox SET status = sqlc.arg(status), done_reason = sqlc.narg(done_reason), done_at = sqlc.arg(now),
    last_error_code = sqlc.narg(error_code), lease_token = NULL, lease_expires_at = NULL
WHERE id = sqlc.arg(id) AND kind = 'meeting_cleanup' AND status = 'in_progress' AND lease_token = sqlc.arg(lease_token);

-- name: ListExpiredMeetingRecoveryOutbox :many
SELECT id, lease_token FROM outbox
WHERE kind = 'meeting_cleanup' AND status = 'in_progress' AND lease_expires_at <= sqlc.arg(now)
ORDER BY lease_expires_at, id LIMIT sqlc.arg(batch_size);

-- name: RequeueMeetingRecoveryOutbox :execrows
UPDATE outbox SET status = 'pending', lease_token = NULL, lease_expires_at = NULL
WHERE id = sqlc.arg(id) AND kind = 'meeting_cleanup' AND status = 'in_progress' AND lease_token = sqlc.arg(lease_token);

-- name: TryLockMeetingRecoveryCapacity :one
SELECT unsettled FROM meeting_recovery_capacity WHERE singleton FOR UPDATE SKIP LOCKED;

-- name: TryLockMeetingRecoveryForOutbox :one
SELECT recovery.* FROM meeting_recovery AS recovery
JOIN meeting_recovery_cycles AS cycle ON cycle.provider_meeting_id = recovery.provider_meeting_id
WHERE cycle.outbox_id = sqlc.arg(outbox_id)
FOR UPDATE OF recovery SKIP LOCKED;

-- name: TryLockMeetingRecoveryOutbox :one
SELECT * FROM outbox WHERE id = sqlc.arg(id) AND kind = 'meeting_cleanup' FOR UPDATE SKIP LOCKED;
