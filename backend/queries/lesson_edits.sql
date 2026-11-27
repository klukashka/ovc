-- name: GetTutorLessonAudience :one
SELECT id, tutor_id, series_id, occurrence_date, relationship_id, group_id FROM lessons WHERE id = @id AND tutor_id = @tutor_id;

-- name: LockTutorSeriesForEdit :one
SELECT id, weekdays, local_start, duration_minutes, timezone, first_date, end_date, count, title, description, materialised_until, ended_at
FROM lesson_series
WHERE id = @id AND tutor_id = @tutor_id
FOR UPDATE;

-- name: LockLessonsForEdit :many
SELECT id, series_id, occurrence_date, status, title, description, starts_at, duration_minutes, timezone, version, overridden_fields
FROM lessons
WHERE tutor_id = @tutor_id AND (
    id = sqlc.arg(id)::uuid
    OR (sqlc.narg(series_id)::uuid IS NOT NULL AND series_id = sqlc.narg(series_id)::uuid AND occurrence_date >= sqlc.arg(from_date)::date)
)
ORDER BY id
FOR UPDATE;

-- name: UpdateEditedLesson :one
UPDATE lessons SET
    starts_at = @starts_at, duration_minutes = @duration_minutes, title = @title, description = sqlc.narg(description),
    overridden_fields = sqlc.arg(overridden_fields)::text[], version = version + 1,
    schedule_revision = schedule_revision + CASE WHEN sqlc.arg(moved)::boolean THEN 1 ELSE 0 END,
    moved_at = CASE WHEN sqlc.arg(moved)::boolean THEN sqlc.arg(now)::timestamptz ELSE moved_at END,
    updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND status = 'scheduled' AND starts_at > sqlc.arg(now)::timestamptz
RETURNING schedule_revision;

-- name: EndSeriesBefore :one
UPDATE lesson_series SET
    end_date = sqlc.arg(end_date)::date, count = NULL,
    ended_at = CASE
        WHEN ended_at IS NULL AND NOT EXISTS (
            SELECT 1 FROM lessons WHERE lessons.series_id = lesson_series.id AND lessons.occurrence_date < sqlc.arg(from_date)::date
        ) THEN sqlc.arg(now)::timestamptz
        ELSE ended_at
    END,
    updated_at = sqlc.arg(now)::timestamptz
WHERE lesson_series.id = sqlc.arg(id)::uuid
RETURNING lesson_series.id;

-- name: CreateSplitLessonSeries :exec
INSERT INTO lesson_series (
    id, tutor_id, relationship_id, group_id, weekdays, local_start, duration_minutes, timezone, first_date, end_date, count,
    title, description, materialised_until, split_from_series_id, created_at, updated_at
)
SELECT
    sqlc.arg(id)::uuid, original.tutor_id, original.relationship_id, original.group_id, original.weekdays, sqlc.arg(local_start)::time,
    sqlc.arg(duration_minutes)::smallint, original.timezone, sqlc.arg(first_date)::date, sqlc.narg(end_date)::date, sqlc.narg(count)::smallint,
    sqlc.arg(title)::text, sqlc.narg(description)::text, original.materialised_until, original.id,
    sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz
FROM lesson_series AS original
WHERE original.id = sqlc.arg(original_id)::uuid;

-- name: MoveLessonsToSeries :execrows
UPDATE lessons SET series_id = sqlc.arg(new_series_id)::uuid, updated_at = sqlc.arg(now)::timestamptz
WHERE series_id = sqlc.arg(original_id)::uuid AND occurrence_date >= sqlc.arg(from_date)::date;

-- name: CancelLessonsByTutor :many
UPDATE lessons SET
    status = 'cancelled', cancelled_at = sqlc.arg(now)::timestamptz, cancel_cause = 'tutor', version = version + 1,
    updated_at = sqlc.arg(now)::timestamptz
WHERE id = ANY (sqlc.arg(ids)::uuid[]) AND status = 'scheduled' AND starts_at > sqlc.arg(now)::timestamptz
RETURNING id;

-- name: ListLessonSeriesViews :many
SELECT id, weekdays, local_start, duration_minutes, timezone, first_date, end_date, count FROM lesson_series
WHERE id = ANY (sqlc.arg(ids)::uuid[]);

-- name: ListLessonsOfSeriesFrom :many
SELECT id FROM lessons
WHERE series_id = sqlc.arg(series_id)::uuid AND occurrence_date >= sqlc.arg(from_date)::date
ORDER BY starts_at, id;

-- name: IsOnLessonRoster :one
SELECT EXISTS (
    SELECT 1 FROM lesson_roster
    JOIN lessons ON lessons.id = lesson_roster.lesson_id
    WHERE lesson_roster.lesson_id = @lesson_id AND lesson_roster.student_id = @student_id
        AND NOT EXISTS (
            SELECT 1 FROM relationships AS waiting
            WHERE waiting.id = lessons.relationship_id AND waiting.state = 'reclaim_pending'
        )
) AS listed;

-- name: ListLessonsForChangeEmail :many
SELECT id, title, starts_at, timezone, status FROM lessons
WHERE id = ANY (sqlc.arg(ids)::uuid[])
ORDER BY starts_at, id;
