-- name: EndLessonSeriesOfTutorForEffect :many
UPDATE lesson_series SET
    end_date = (sqlc.arg(now)::timestamptz AT TIME ZONE lesson_series.timezone)::date, count = NULL,
    ended_at = CASE
        WHEN NOT EXISTS (
            SELECT 1 FROM lessons
            WHERE lessons.series_id = lesson_series.id
                AND lessons.occurrence_date <= (sqlc.arg(now)::timestamptz AT TIME ZONE lesson_series.timezone)::date
        ) THEN sqlc.arg(now)::timestamptz
    END,
    updated_at = sqlc.arg(now)::timestamptz
FROM (
    SELECT locked.id FROM lesson_series AS locked
    WHERE locked.tutor_id = sqlc.arg(tutor_id)::uuid AND locked.ended_at IS NULL
        AND (locked.end_date IS NULL OR locked.end_date > (sqlc.arg(now)::timestamptz AT TIME ZONE locked.timezone)::date)
    ORDER BY locked.id
    FOR UPDATE
) AS ending
WHERE lesson_series.id = ending.id
RETURNING lesson_series.id;

-- name: EndLessonSeriesOfRelationshipsForEffect :many
UPDATE lesson_series SET
    end_date = (sqlc.arg(now)::timestamptz AT TIME ZONE lesson_series.timezone)::date, count = NULL,
    ended_at = CASE
        WHEN NOT EXISTS (
            SELECT 1 FROM lessons
            WHERE lessons.series_id = lesson_series.id
                AND lessons.occurrence_date <= (sqlc.arg(now)::timestamptz AT TIME ZONE lesson_series.timezone)::date
        ) THEN sqlc.arg(now)::timestamptz
    END,
    updated_at = sqlc.arg(now)::timestamptz
FROM (
    SELECT locked.id FROM lesson_series AS locked
    WHERE locked.relationship_id = ANY (sqlc.arg(relationship_ids)::uuid[]) AND locked.tutor_id = sqlc.arg(tutor_id)::uuid AND locked.ended_at IS NULL
        AND (locked.end_date IS NULL OR locked.end_date > (sqlc.arg(now)::timestamptz AT TIME ZONE locked.timezone)::date)
    ORDER BY locked.id
    FOR UPDATE
) AS ending
WHERE lesson_series.id = ending.id
RETURNING lesson_series.id;

-- name: EndLessonSeriesOfGroupsForEffect :many
UPDATE lesson_series SET
    end_date = (sqlc.arg(now)::timestamptz AT TIME ZONE lesson_series.timezone)::date, count = NULL,
    ended_at = CASE
        WHEN NOT EXISTS (
            SELECT 1 FROM lessons
            WHERE lessons.series_id = lesson_series.id
                AND lessons.occurrence_date <= (sqlc.arg(now)::timestamptz AT TIME ZONE lesson_series.timezone)::date
        ) THEN sqlc.arg(now)::timestamptz
    END,
    updated_at = sqlc.arg(now)::timestamptz
FROM (
    SELECT locked.id FROM lesson_series AS locked
    WHERE locked.group_id = ANY (sqlc.arg(group_ids)::uuid[]) AND locked.tutor_id = sqlc.arg(tutor_id)::uuid AND locked.ended_at IS NULL
        AND (locked.end_date IS NULL OR locked.end_date > (sqlc.arg(now)::timestamptz AT TIME ZONE locked.timezone)::date)
    ORDER BY locked.id
    FOR UPDATE
) AS ending
WHERE lesson_series.id = ending.id
RETURNING lesson_series.id;

-- name: CancelLessonsOfTutorForEffect :many
UPDATE lessons SET
    status = 'cancelled', cancelled_at = sqlc.arg(cancelled_at)::timestamptz, cancel_cause = sqlc.arg(cancel_cause)::text,
    version = version + 1, updated_at = sqlc.arg(cancelled_at)::timestamptz
FROM (
    SELECT locked.id FROM lessons AS locked
    WHERE locked.tutor_id = sqlc.arg(tutor_id)::uuid AND locked.status = 'scheduled' AND locked.starts_at > sqlc.arg(cancelled_at)::timestamptz
    ORDER BY locked.id
    FOR UPDATE
) AS cancelling
WHERE lessons.id = cancelling.id
RETURNING lessons.id, lessons.relationship_id, lessons.group_id;

-- name: CancelLessonsOfRelationshipsForEffect :many
UPDATE lessons SET
    status = 'cancelled', cancelled_at = sqlc.arg(cancelled_at)::timestamptz, cancel_cause = sqlc.arg(cancel_cause)::text,
    version = version + 1, updated_at = sqlc.arg(cancelled_at)::timestamptz
FROM (
    SELECT locked.id FROM lessons AS locked
    WHERE locked.relationship_id = ANY (sqlc.arg(relationship_ids)::uuid[]) AND locked.tutor_id = sqlc.arg(tutor_id)::uuid AND locked.status = 'scheduled' AND locked.starts_at > sqlc.arg(cancelled_at)::timestamptz
    ORDER BY locked.id
    FOR UPDATE
) AS cancelling
WHERE lessons.id = cancelling.id
RETURNING lessons.id, lessons.relationship_id, lessons.group_id;

-- name: CancelLessonsOfGroupsForEffect :many
UPDATE lessons SET
    status = 'cancelled', cancelled_at = sqlc.arg(cancelled_at)::timestamptz, cancel_cause = sqlc.arg(cancel_cause)::text,
    version = version + 1, updated_at = sqlc.arg(cancelled_at)::timestamptz
FROM (
    SELECT locked.id FROM lessons AS locked
    WHERE locked.group_id = ANY (sqlc.arg(group_ids)::uuid[]) AND locked.tutor_id = sqlc.arg(tutor_id)::uuid AND locked.status = 'scheduled' AND locked.starts_at > sqlc.arg(cancelled_at)::timestamptz
    ORDER BY locked.id
    FOR UPDATE
) AS cancelling
WHERE lessons.id = cancelling.id
RETURNING lessons.id, lessons.relationship_id, lessons.group_id;

-- name: ListUnderWayLessonsOfTutor :many
SELECT id, relationship_id, group_id FROM lessons
WHERE tutor_id = @tutor_id AND status = 'scheduled' AND starts_at <= sqlc.arg(at)::timestamptz
ORDER BY id;

-- name: FreezeEffectRosters :execrows
INSERT INTO lesson_roster (lesson_id, student_id, lesson_starts_at, added_at)
SELECT lessons.id, frozen.student_id, lessons.starts_at, sqlc.arg(now)::timestamptz
FROM (
    SELECT unnest(sqlc.arg(lesson_ids)::uuid[]) AS lesson_id, unnest(sqlc.arg(student_ids)::uuid[]) AS student_id
) AS frozen
JOIN lessons ON lessons.id = frozen.lesson_id
ON CONFLICT (lesson_id, student_id) DO NOTHING;

-- name: DeleteRosterRowsOfLinks :execrows
DELETE FROM lesson_roster
USING lessons, (
    SELECT unnest(sqlc.arg(relationship_ids)::uuid[]) AS relationship_id, unnest(sqlc.arg(student_ids)::uuid[]) AS student_id
) AS links
WHERE lesson_roster.lesson_id = lessons.id AND lessons.relationship_id = links.relationship_id
    AND lesson_roster.student_id = links.student_id;

-- name: ListActiveAudienceOfTutor :many
SELECT id, student_id FROM relationships
WHERE tutor_id = @tutor_id AND state = 'active' AND student_id IS NOT NULL
ORDER BY id;

-- name: ListGroupMemberStudentsOfTutor :many
SELECT group_members.group_id, relationships.student_id
FROM groups
JOIN group_members ON group_members.group_id = groups.id AND group_members.tutor_id = groups.tutor_id
JOIN relationships ON relationships.id = group_members.relationship_id AND relationships.tutor_id = groups.tutor_id
WHERE groups.tutor_id = @tutor_id AND relationships.student_id IS NOT NULL
ORDER BY group_members.group_id, relationships.student_id;

-- name: ListLessonsCancelledOfLink :many
SELECT lessons.id, lessons.title, lessons.starts_at, lessons.timezone, count(lessons.id) OVER () AS total
FROM lessons
WHERE lessons.relationship_id = sqlc.arg(relationship_id)::uuid AND lessons.starts_at > sqlc.arg(cancelled_at)::timestamptz
    AND lessons.tutor_id = sqlc.arg(tutor_id)::uuid AND lessons.cancel_cause = sqlc.arg(cancel_cause)::text
    AND lessons.cancelled_at = sqlc.arg(cancelled_at)::timestamptz
ORDER BY lessons.starts_at, lessons.id
LIMIT sqlc.arg(row_limit)::int;

-- name: ListLessonsCancelledOnRoster :many
SELECT cancelled.id, cancelled.title, cancelled.starts_at, cancelled.timezone, count(cancelled.id) OVER () AS total
FROM lesson_roster
CROSS JOIN LATERAL (
    SELECT lessons.id, lessons.title, lessons.starts_at, lessons.timezone FROM lessons
    WHERE lessons.id = lesson_roster.lesson_id AND lessons.tutor_id = sqlc.arg(tutor_id)::uuid
        AND lessons.cancel_cause = sqlc.arg(cancel_cause)::text AND lessons.cancelled_at = sqlc.arg(cancelled_at)::timestamptz
    LIMIT 1
) AS cancelled
WHERE lesson_roster.student_id = sqlc.arg(student_id)::uuid AND lesson_roster.lesson_starts_at > sqlc.arg(cancelled_at)::timestamptz
ORDER BY cancelled.starts_at, cancelled.id
LIMIT sqlc.arg(row_limit)::int;

-- name: ListGroupLessonsCancelledOnRoster :many
SELECT cancelled.id, cancelled.title, cancelled.starts_at, cancelled.timezone, count(cancelled.id) OVER () AS total
FROM lesson_roster
CROSS JOIN LATERAL (
    SELECT lessons.id, lessons.title, lessons.starts_at, lessons.timezone FROM lessons
    WHERE lessons.id = lesson_roster.lesson_id AND lessons.group_id = sqlc.arg(group_id)::uuid AND lessons.tutor_id = sqlc.arg(tutor_id)::uuid
        AND lessons.cancel_cause = sqlc.arg(cancel_cause)::text AND lessons.cancelled_at = sqlc.arg(cancelled_at)::timestamptz
    LIMIT 1
) AS cancelled
WHERE lesson_roster.student_id = sqlc.arg(student_id)::uuid AND lesson_roster.lesson_starts_at > sqlc.arg(cancelled_at)::timestamptz
ORDER BY cancelled.starts_at, cancelled.id
LIMIT sqlc.arg(row_limit)::int;

-- name: GetGroupForArchivedEmail :one
SELECT groups.id, groups.tutor_id, groups.name, groups.archived_at, tutor.state AS tutor_state, tutor.name AS tutor_name
FROM groups
JOIN users AS tutor ON tutor.id = groups.tutor_id
WHERE groups.id = @id;

-- name: ListNotificationGroups :many
SELECT groups.id, groups.tutor_id, groups.name, EXISTS (
    SELECT 1 FROM group_members
    JOIN relationships ON relationships.id = group_members.relationship_id
    WHERE group_members.group_id = groups.id AND relationships.student_id = sqlc.arg(student_id)::uuid
) AS member
FROM groups
WHERE groups.id = ANY (sqlc.arg(ids)::uuid[]);
