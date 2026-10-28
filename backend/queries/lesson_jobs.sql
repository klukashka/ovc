-- name: ListLessonsDueToEnd :many
SELECT id, timezone('UTC', timezone('UTC', starts_at) + make_interval(mins => duration_minutes::int))::timestamptz AS ends_at FROM lessons
WHERE status = 'scheduled' AND starts_at <= sqlc.arg(latest_start)::timestamptz
    AND timezone('UTC', starts_at) + make_interval(mins => duration_minutes::int) <= timezone('UTC', sqlc.arg(ended_before)::timestamptz)
    AND (timezone('UTC', starts_at) + make_interval(mins => duration_minutes::int), id)
        > (timezone('UTC', sqlc.arg(after_ends_at)::timestamptz), sqlc.arg(after_id)::uuid)
ORDER BY timezone('UTC', starts_at) + make_interval(mins => duration_minutes::int), id
LIMIT sqlc.arg(row_limit)::int;

-- name: GetLessonAudience :one
SELECT id, tutor_id, series_id, relationship_id, group_id FROM lessons WHERE id = @id;

-- name: LockSeriesOfLesson :one
SELECT id FROM lesson_series WHERE id = @id FOR SHARE;

-- name: LockLessonForJob :one
SELECT id, status, starts_at, duration_minutes, schedule_revision, reminded_24h_revision, reminded_1h_revision
FROM lessons WHERE id = @id
FOR UPDATE;

-- name: EndUnjoinedLesson :execrows
UPDATE lessons SET status = 'ended', ended_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND status = 'scheduled'
    AND starts_at + make_interval(mins => duration_minutes) <= sqlc.arg(ended_before)::timestamptz;

-- name: FreezeLessonRoster :execrows
INSERT INTO lesson_roster (lesson_id, student_id, lesson_starts_at, added_at)
SELECT lessons.id, frozen.student_id, lessons.starts_at, sqlc.arg(now)::timestamptz
FROM lessons
CROSS JOIN (SELECT unnest(sqlc.arg(students)::uuid[]) AS student_id) AS frozen
WHERE lessons.id = @lesson_id
ON CONFLICT (lesson_id, student_id) DO NOTHING;

-- name: ListReminderCandidates :many
SELECT id, starts_at FROM lessons
WHERE status = 'scheduled'
    AND starts_at > sqlc.arg(window_start)::timestamptz AND starts_at <= sqlc.arg(window_end)::timestamptz
    AND (starts_at, id) > (sqlc.arg(after_starts_at)::timestamptz, sqlc.arg(after_id)::uuid)
    AND (CASE WHEN sqlc.arg(day_before)::boolean THEN reminded_24h_revision ELSE reminded_1h_revision END)
        IS DISTINCT FROM schedule_revision
ORDER BY starts_at, id
LIMIT sqlc.arg(row_limit)::int;

-- name: MarkLessonReminded :execrows
UPDATE lessons SET
    reminded_24h_revision = CASE WHEN sqlc.arg(day_before)::boolean THEN schedule_revision ELSE reminded_24h_revision END,
    reminded_1h_revision = CASE WHEN sqlc.arg(day_before)::boolean THEN reminded_1h_revision ELSE schedule_revision END
WHERE id = @id AND status = 'scheduled' AND schedule_revision = @schedule_revision;

-- name: ListSeriesToExtend :many
SELECT series.id, series.materialised_until FROM lesson_series AS series
JOIN users AS tutor ON tutor.id = series.tutor_id
LEFT JOIN relationships AS card ON card.id = series.relationship_id
LEFT JOIN groups AS series_group ON series_group.id = series.group_id
WHERE series.ended_at IS NULL AND series.end_date IS NOT NULL AND series.end_date > series.materialised_until
    AND series.end_date >= (sqlc.arg(now)::timestamptz AT TIME ZONE series.timezone)::date
    AND EXISTS (
        SELECT 1 FROM generate_series(0, 6) AS step(days)
        WHERE extract(isodow FROM greatest(series.materialised_until + 1, series.first_date) + step.days)::smallint = ANY (series.weekdays)
            AND greatest(series.materialised_until + 1, series.first_date) + step.days
                <= least(series.end_date, (sqlc.arg(horizon)::timestamptz AT TIME ZONE series.timezone)::date)
    )
    AND tutor.state = 'active'
    AND (card.state IN ('active', 'provisional') OR (series_group.id IS NOT NULL AND series_group.archived_at IS NULL))
    AND (series.materialised_until, series.id) > (sqlc.arg(after_until)::date, sqlc.arg(after_id)::uuid)
ORDER BY series.materialised_until, series.id
LIMIT sqlc.arg(row_limit)::int;

-- name: GetSeriesAudience :one
SELECT id, tutor_id, relationship_id, group_id FROM lesson_series WHERE id = @id;

-- name: LockSeriesForExtension :one
SELECT series.id, series.weekdays, series.local_start, series.duration_minutes, series.timezone, series.first_date, series.end_date,
    series.title, series.description, series.materialised_until, series.ended_at, tutor.state AS tutor_state
FROM lesson_series AS series
JOIN users AS tutor ON tutor.id = series.tutor_id
WHERE series.id = @id
FOR UPDATE OF series;

-- name: InsertSeriesOccurrences :many
INSERT INTO lessons (
    id, tutor_id, series_id, occurrence_date, relationship_id, group_id, title, description, starts_at, duration_minutes, status,
    timezone, created_at, updated_at
)
SELECT
    occurrence.id, series.tutor_id, series.id, occurrence.occurrence_date, series.relationship_id, series.group_id, series.title,
    series.description, occurrence.starts_at, series.duration_minutes, 'scheduled', series.timezone,
    sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz
FROM lesson_series AS series
CROSS JOIN (
    SELECT unnest(sqlc.arg(ids)::uuid[]) AS id, unnest(sqlc.arg(occurrence_dates)::date[]) AS occurrence_date,
        unnest(sqlc.arg(starts)::timestamptz[]) AS starts_at
) AS occurrence
WHERE series.id = @series_id
ON CONFLICT (series_id, occurrence_date) DO NOTHING
RETURNING id;

-- name: SetSeriesMaterialisedUntil :exec
UPDATE lesson_series SET materialised_until = @materialised_until, updated_at = sqlc.arg(now)::timestamptz WHERE id = @id;

-- name: GetLessonForEmail :one
SELECT
    lessons.id, lessons.tutor_id, lessons.title, lessons.starts_at, lessons.status, lessons.timezone, lessons.schedule_revision,
    lessons.relationship_id, lessons.group_id, tutor.state AS tutor_state, tutor.name AS tutor_name,
    card.student_id AS card_student_id, coalesce(card.state, '')::text AS card_state, card.display_name AS card_name,
    card.discard_reason AS card_discard_reason, lesson_group.name AS group_name
FROM lessons
JOIN users AS tutor ON tutor.id = lessons.tutor_id
LEFT JOIN relationships AS card ON card.id = lessons.relationship_id
LEFT JOIN groups AS lesson_group ON lesson_group.id = lessons.group_id
WHERE lessons.id = @id;

-- name: IsCurrentGroupMember :one
SELECT EXISTS (
    SELECT 1 FROM group_members
    JOIN relationships ON relationships.id = group_members.relationship_id
    WHERE group_members.group_id = @group_id AND relationships.student_id = sqlc.arg(student_id)::uuid
) AS member;

-- name: ListTutorNotificationLessons :many
SELECT id, title FROM lessons WHERE tutor_id = @tutor_id AND id = ANY (sqlc.arg(ids)::uuid[]);

-- name: ListStudentNotificationLessons :many
SELECT lessons.id, lessons.title FROM lessons
WHERE lessons.id = ANY (sqlc.arg(ids)::uuid[]) AND (
    (lessons.status = 'scheduled' AND EXISTS (
        SELECT 1 FROM relationships
        WHERE relationships.id = lessons.relationship_id AND relationships.student_id = sqlc.arg(student_id)::uuid
            AND relationships.state IN ('active', 'provisional')
    ))
    OR (lessons.status = 'scheduled' AND EXISTS (
        SELECT 1 FROM group_members
        JOIN relationships AS membership ON membership.id = group_members.relationship_id
        WHERE group_members.group_id = lessons.group_id AND membership.student_id = sqlc.arg(student_id)::uuid
    ))
    OR (lessons.status <> 'scheduled' AND EXISTS (
        SELECT 1 FROM lesson_roster
        WHERE lesson_roster.lesson_id = lessons.id AND lesson_roster.student_id = sqlc.arg(student_id)::uuid
    ) AND NOT EXISTS (
        SELECT 1 FROM relationships AS waiting
        WHERE waiting.id = lessons.relationship_id AND waiting.state = 'reclaim_pending'
    ))
);
