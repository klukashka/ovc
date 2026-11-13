-- name: InsertNotification :execrows
INSERT INTO notifications (id, user_id, tutor_id, student_id, event, subject_id, relationship_id, idempotency_key, created_at)
VALUES (@id, @user_id, @tutor_id, @student_id, @event, @subject_id, @relationship_id, @idempotency_key, @created_at)
ON CONFLICT (idempotency_key) DO NOTHING;

-- name: ListNotifications :many
SELECT id, event, tutor_id, student_id, subject_id, relationship_id, created_at, read_at
FROM notifications
WHERE user_id = @user_id AND (
    notifications.user_id IS DISTINCT FROM notifications.student_id
    OR notifications.event NOT IN ('issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned', 'submission.review_updated')
    OR EXISTS (
        SELECT 1 FROM submissions
        JOIN issues ON issues.id = submissions.issue_id
        JOIN relationships ON relationships.id = submissions.relationship_id
        WHERE submissions.id = notifications.subject_id AND submissions.student_id = notifications.user_id
            AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
    )
)
    AND (sqlc.narg(after_created_at)::timestamptz IS NULL
        OR (created_at, id) < (sqlc.narg(after_created_at)::timestamptz, sqlc.narg(after_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::int;

-- name: CountUnreadNotifications :one
SELECT count(*) FROM (
    SELECT 1 FROM notifications WHERE user_id = @user_id AND (
    notifications.user_id IS DISTINCT FROM notifications.student_id
    OR notifications.event NOT IN ('issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned', 'submission.review_updated')
    OR EXISTS (
        SELECT 1 FROM submissions
        JOIN issues ON issues.id = submissions.issue_id
        JOIN relationships ON relationships.id = submissions.relationship_id
        WHERE submissions.id = notifications.subject_id AND submissions.student_id = notifications.user_id
            AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
    )
) AND read_at IS NULL LIMIT sqlc.arg(cap)::int
) AS unread;

-- name: CountOwnNotifications :one
SELECT count(*) FROM notifications WHERE user_id = @user_id AND (
    notifications.user_id IS DISTINCT FROM notifications.student_id
    OR notifications.event NOT IN ('issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned', 'submission.review_updated')
    OR EXISTS (
        SELECT 1 FROM submissions
        JOIN issues ON issues.id = submissions.issue_id
        JOIN relationships ON relationships.id = submissions.relationship_id
        WHERE submissions.id = notifications.subject_id AND submissions.student_id = notifications.user_id
            AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
    )
) AND notifications.id = ANY (sqlc.arg(ids)::uuid[]);

-- name: GetOwnNotification :one
SELECT notifications.id, notifications.created_at FROM notifications WHERE user_id = @user_id AND (
    notifications.user_id IS DISTINCT FROM notifications.student_id
    OR notifications.event NOT IN ('issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned', 'submission.review_updated')
    OR EXISTS (
        SELECT 1 FROM submissions
        JOIN issues ON issues.id = submissions.issue_id
        JOIN relationships ON relationships.id = submissions.relationship_id
        WHERE submissions.id = notifications.subject_id AND submissions.student_id = notifications.user_id
            AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
    )
) AND notifications.id = @id;

-- name: MarkNotificationsRead :execrows
UPDATE notifications SET read_at = sqlc.arg(now)::timestamptz
WHERE user_id = @user_id AND (
    notifications.user_id IS DISTINCT FROM notifications.student_id
    OR notifications.event NOT IN ('issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned', 'submission.review_updated')
    OR EXISTS (
        SELECT 1 FROM submissions
        JOIN issues ON issues.id = submissions.issue_id
        JOIN relationships ON relationships.id = submissions.relationship_id
        WHERE submissions.id = notifications.subject_id AND submissions.student_id = notifications.user_id
            AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
    )
) AND notifications.id = ANY (sqlc.arg(ids)::uuid[]) AND read_at IS NULL;

-- name: MarkNotificationsReadThrough :execrows
UPDATE notifications SET read_at = sqlc.arg(now)::timestamptz
WHERE user_id = @user_id AND (
    notifications.user_id IS DISTINCT FROM notifications.student_id
    OR notifications.event NOT IN ('issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned', 'submission.review_updated')
    OR EXISTS (
        SELECT 1 FROM submissions
        JOIN issues ON issues.id = submissions.issue_id
        JOIN relationships ON relationships.id = submissions.relationship_id
        WHERE submissions.id = notifications.subject_id AND submissions.student_id = notifications.user_id
            AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
    )
) AND read_at IS NULL
    AND (created_at, id) <= (sqlc.arg(through_created_at)::timestamptz, sqlc.arg(through_id)::uuid);

-- name: DeleteNotificationsNaming :execrows
DELETE FROM notifications WHERE user_id = @account_id OR tutor_id = @account_id OR student_id = @account_id;

-- name: DeleteNotificationsNamingBatch :execrows
DELETE FROM notifications
WHERE id IN (
    SELECT doomed.id FROM notifications AS doomed
    WHERE doomed.user_id = @account_id OR doomed.tutor_id = @account_id OR doomed.student_id = @account_id
    LIMIT sqlc.arg(batch_size)::int
);

-- name: DeleteNotificationsBefore :many
DELETE FROM notifications
WHERE id IN (
    SELECT doomed.id FROM notifications AS doomed
    WHERE doomed.created_at < sqlc.arg(before)::timestamptz
    ORDER BY doomed.created_at
    LIMIT sqlc.arg(batch_size)::int
)
RETURNING id;

-- name: ListNotificationRelationships :many
SELECT relationships.id, relationships.tutor_id, relationships.student_id, relationships.state, relationships.display_name,
    relationships.colour, tutors.name AS tutor_name
FROM relationships
JOIN users AS tutors ON tutors.id = relationships.tutor_id
WHERE relationships.id = ANY (sqlc.arg(ids)::uuid[]);

-- name: ListNotificationJoinRequests :many
SELECT join_requests.id, join_requests.tutor_id, join_requests.student_id, join_requests.state, join_requests.suppressed,
    join_requests.expires_at, students.name AS student_name, students.state AS student_state, cards.display_name AS card_name
FROM join_requests
JOIN users AS students ON students.id = join_requests.student_id
LEFT JOIN relationships AS cards
    ON cards.tutor_id = join_requests.tutor_id AND cards.student_id = join_requests.student_id AND cards.state <> 'discarded'
WHERE join_requests.id = ANY (sqlc.arg(ids)::uuid[]);
