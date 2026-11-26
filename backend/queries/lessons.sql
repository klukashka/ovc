-- name: CreateLessonSeries :exec
INSERT INTO lesson_series (
    id, tutor_id, relationship_id, group_id, weekdays, local_start, duration_minutes, timezone, first_date, end_date, count,
    title, description, materialised_until, create_key, created_at, updated_at
) VALUES (
    @id, @tutor_id, sqlc.narg(relationship_id), sqlc.narg(group_id), sqlc.arg(weekdays)::smallint[], @local_start, @duration_minutes,
    @timezone, @first_date, sqlc.narg(end_date), sqlc.narg(count), @title, sqlc.narg(description), @materialised_until,
    sqlc.narg(create_key), sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz
);

-- name: CreateLessons :execrows
INSERT INTO lessons (
    id, tutor_id, series_id, occurrence_date, relationship_id, group_id, title, description, starts_at, duration_minutes, status,
    timezone, create_key, created_at, updated_at
)
SELECT
    unnest(sqlc.arg(ids)::uuid[]), sqlc.arg(tutor_id)::uuid, sqlc.narg(series_id)::uuid, unnest(sqlc.arg(occurrence_dates)::date[]),
    sqlc.narg(relationship_id)::uuid, sqlc.narg(group_id)::uuid, sqlc.arg(title)::text, sqlc.narg(description)::text,
    unnest(sqlc.arg(starts)::timestamptz[]), sqlc.arg(duration_minutes)::smallint, 'scheduled', sqlc.arg(timezone)::text,
    sqlc.narg(create_key)::text, sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz;

-- name: GetTutorLessonByCreateKey :one
SELECT id FROM lessons WHERE tutor_id = @tutor_id AND create_key = @create_key;

-- name: GetTutorSeriesByCreateKey :one
SELECT * FROM lesson_series WHERE tutor_id = @tutor_id AND create_key = @create_key;

-- name: ListSeriesLessonIDs :many
SELECT id FROM lessons WHERE series_id = @series_id ORDER BY starts_at, id;

-- name: CountOpenSeriesOfTutor :one
SELECT count(series.id) FROM lesson_series AS series
WHERE series.tutor_id = @tutor_id AND series.ended_at IS NULL AND (
    (series.end_date IS NOT NULL AND series.end_date >= (sqlc.arg(now)::timestamptz AT TIME ZONE series.timezone)::date)
    OR (series.count IS NOT NULL AND EXISTS (
        SELECT 1 FROM lessons WHERE lessons.series_id = series.id AND lessons.starts_at > sqlc.arg(now)::timestamptz
    ))
);

-- name: CountFutureLessonsOfTutor :one
SELECT count(id) FROM lessons WHERE tutor_id = @tutor_id AND status = 'scheduled' AND starts_at > sqlc.arg(now)::timestamptz;

-- name: ListTutorLessonsForOverlap :many
SELECT id, title, starts_at, duration_minutes FROM lessons
WHERE tutor_id = @tutor_id AND status <> 'cancelled'
    AND starts_at < sqlc.arg(range_end)::timestamptz
    AND starts_at > sqlc.arg(range_start)::timestamptz - interval '240 minutes'
ORDER BY starts_at, id;

-- name: ListTutorSeriesBeyondHorizon :many
SELECT id, title, weekdays, local_start, duration_minutes, timezone, first_date, end_date, materialised_until FROM lesson_series
WHERE tutor_id = @tutor_id AND ended_at IS NULL AND end_date IS NOT NULL AND end_date > materialised_until
    AND end_date >= sqlc.arg(from_date)::date AND materialised_until < sqlc.arg(through_date)::date
ORDER BY id;

-- name: LockTutorRelationshipForLesson :one
SELECT id, state, student_id FROM relationships
WHERE id = @id AND tutor_id = @tutor_id AND state <> 'discarded'
FOR SHARE;

-- name: LockTutorRelationshipsForLesson :many
SELECT id, state, student_id FROM relationships
WHERE tutor_id = @tutor_id AND id = ANY (sqlc.arg(ids)::uuid[]) AND state <> 'discarded'
ORDER BY id
FOR SHARE;

-- name: LockTutorGroupForLesson :one
SELECT id, archived_at FROM groups WHERE id = @id AND tutor_id = @tutor_id FOR SHARE;

-- name: GetStudentListedRelationship :one
SELECT relationships.tutor_id FROM relationships
JOIN users AS tutor ON tutor.id = relationships.tutor_id
WHERE relationships.id = @id AND relationships.student_id = @student_id AND relationships.state IN ('active', 'provisional', 'ended')
    AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz);

-- name: ListTutorLessonViews :many
SELECT
    lessons.id, lessons.title, lessons.description, lessons.starts_at, lessons.duration_minutes, lessons.status, (lessons.live_at IS NOT NULL)::boolean AS took_place, lessons.timezone,
    lessons.series_id, lessons.occurrence_date, lessons.version, lessons.cancelled_at, lessons.relationship_id, lessons.group_id,
    card.display_name AS card_name, coalesce(card.state, '')::text AS card_state, card.discard_reason AS card_discard_reason,
    card.colour AS card_colour, student.timezone AS student_timezone,
    lesson_group.name AS group_name, coalesce(lesson_group.colour, '')::text AS group_colour,
    EXISTS (
        SELECT 1 FROM meetings WHERE meetings.lesson_id = lessons.id AND meetings.state = 'open' AND lessons.status = 'live'
    ) AS room_open
FROM lessons
LEFT JOIN relationships AS card ON card.id = lessons.relationship_id
LEFT JOIN users AS student ON student.id = card.student_id
LEFT JOIN groups AS lesson_group ON lesson_group.id = lessons.group_id
WHERE lessons.tutor_id = @tutor_id AND lessons.id = ANY (sqlc.arg(ids)::uuid[])
ORDER BY lessons.starts_at, lessons.id;

-- name: ListTutorCalendar :many
SELECT
    lessons.id, lessons.title, lessons.starts_at, lessons.duration_minutes, lessons.status, (lessons.live_at IS NOT NULL)::boolean AS took_place, lessons.timezone,
    lessons.relationship_id, lessons.group_id,
    card.display_name AS card_name, coalesce(card.state, '')::text AS card_state, card.discard_reason AS card_discard_reason,
    card.colour AS card_colour, lesson_group.name AS group_name, coalesce(lesson_group.colour, '')::text AS group_colour
FROM lessons
LEFT JOIN relationships AS card ON card.id = lessons.relationship_id
LEFT JOIN groups AS lesson_group ON lesson_group.id = lessons.group_id
WHERE lessons.tutor_id = @tutor_id
    AND lessons.starts_at < sqlc.arg(range_end)::timestamptz
    AND lessons.starts_at > sqlc.arg(range_start)::timestamptz - interval '240 minutes'
    AND lessons.starts_at + make_interval(mins => lessons.duration_minutes) > sqlc.arg(range_start)::timestamptz
ORDER BY lessons.starts_at, lessons.id
LIMIT sqlc.arg(row_limit)::int;

-- name: ListGroupLessonMembers :many
SELECT member.group_id, card.display_name, coalesce(account.state, '')::text AS account_state
FROM group_members AS member
JOIN relationships AS card ON card.id = member.relationship_id
LEFT JOIN users AS account ON account.id = card.student_id
WHERE member.tutor_id = @tutor_id AND member.group_id = ANY (sqlc.arg(group_ids)::uuid[])
ORDER BY member.group_id, member.added_at, member.relationship_id;

-- name: ListRosterLessonMembers :many
SELECT roster.lesson_id, card.display_name, coalesce(account.state, '')::text AS account_state
FROM lesson_roster AS roster
JOIN lessons ON lessons.id = roster.lesson_id
JOIN LATERAL (
    SELECT relationships.display_name FROM relationships
    WHERE relationships.tutor_id = lessons.tutor_id AND relationships.student_id = roster.student_id
    ORDER BY relationships.state = 'discarded', relationships.updated_at DESC, relationships.id
    LIMIT 1
) AS card ON true
LEFT JOIN users AS account ON account.id = roster.student_id
WHERE lessons.tutor_id = @tutor_id AND roster.lesson_id = ANY (sqlc.arg(lesson_ids)::uuid[])
ORDER BY roster.lesson_id, roster.added_at, roster.student_id;

-- name: ListStudentCalendar :many
WITH visible AS (
    SELECT lessons.id FROM relationships
    JOIN lessons ON lessons.relationship_id = relationships.id
    WHERE relationships.student_id = sqlc.arg(student_id)::uuid AND relationships.state IN ('active', 'provisional')
        AND lessons.status = 'scheduled'
        AND lessons.starts_at < sqlc.arg(range_end)::timestamptz
        AND lessons.starts_at > sqlc.arg(range_start)::timestamptz - interval '240 minutes'
    UNION ALL
    SELECT lessons.id FROM relationships
    JOIN group_members ON group_members.relationship_id = relationships.id
    JOIN lessons ON lessons.group_id = group_members.group_id
    WHERE relationships.student_id = sqlc.arg(student_id)::uuid AND lessons.status = 'scheduled'
        AND lessons.starts_at < sqlc.arg(range_end)::timestamptz
        AND lessons.starts_at > sqlc.arg(range_start)::timestamptz - interval '240 minutes'
    UNION ALL
    SELECT lessons.id FROM lesson_roster
    JOIN lessons ON lessons.id = lesson_roster.lesson_id
    WHERE lesson_roster.student_id = sqlc.arg(student_id)::uuid AND lessons.status <> 'scheduled'
        AND lesson_roster.lesson_starts_at < sqlc.arg(range_end)::timestamptz
        AND lesson_roster.lesson_starts_at > sqlc.arg(range_start)::timestamptz - interval '240 minutes'
        AND NOT EXISTS (
            SELECT 1 FROM relationships AS waiting
            WHERE waiting.id = lessons.relationship_id AND waiting.state = 'reclaim_pending'
        )
)
SELECT
    lessons.id, lessons.title, lessons.starts_at, lessons.duration_minutes, lessons.status, (lessons.live_at IS NOT NULL)::boolean AS took_place, lessons.timezone,
    lessons.group_id, lesson_group.name AS group_name,
    EXISTS (
        SELECT 1 FROM group_members
        JOIN relationships AS membership ON membership.id = group_members.relationship_id
        WHERE group_members.group_id = lessons.group_id AND membership.student_id = sqlc.arg(student_id)::uuid
    ) AS current_member,
    teacher.name AS tutor_name, coalesce(mine.state, '')::text AS my_state, mine.colour AS my_colour
FROM visible
JOIN lessons ON lessons.id = visible.id
JOIN users AS teacher ON teacher.id = lessons.tutor_id
LEFT JOIN groups AS lesson_group ON lesson_group.id = lessons.group_id
LEFT JOIN LATERAL (
    SELECT relationships.state, relationships.colour FROM relationships
    WHERE relationships.tutor_id = lessons.tutor_id AND relationships.student_id = sqlc.arg(student_id)::uuid
        AND relationships.state <> 'discarded'
    LIMIT 1
) AS mine ON true
WHERE lessons.starts_at + make_interval(mins => lessons.duration_minutes) > sqlc.arg(range_start)::timestamptz
    AND (sqlc.narg(tutor_id)::uuid IS NULL OR lessons.tutor_id = sqlc.narg(tutor_id)::uuid)
ORDER BY lessons.starts_at, lessons.id
LIMIT sqlc.arg(row_limit)::int;

-- name: ListStudentCalendarAfter :many
WITH visible AS (
    SELECT lessons.id FROM relationships
    JOIN lessons ON lessons.relationship_id = relationships.id
    WHERE relationships.student_id = sqlc.arg(student_id)::uuid AND relationships.state IN ('active', 'provisional')
        AND lessons.status = 'scheduled'
    UNION ALL
    SELECT lessons.id FROM relationships
    JOIN group_members ON group_members.relationship_id = relationships.id
    JOIN lessons ON lessons.group_id = group_members.group_id
    WHERE relationships.student_id = sqlc.arg(student_id)::uuid AND lessons.status = 'scheduled'
    UNION ALL
    SELECT lessons.id FROM lesson_roster
    JOIN lessons ON lessons.id = lesson_roster.lesson_id
    WHERE lesson_roster.student_id = sqlc.arg(student_id)::uuid AND lessons.status <> 'scheduled'
        AND NOT EXISTS (
            SELECT 1 FROM relationships AS waiting
            WHERE waiting.id = lessons.relationship_id AND waiting.state = 'reclaim_pending'
        )
)
SELECT
    lessons.id, lessons.title, lessons.starts_at, lessons.duration_minutes, lessons.status, (lessons.live_at IS NOT NULL)::boolean AS took_place, lessons.timezone,
    lessons.group_id, lesson_group.name AS group_name,
    EXISTS (
        SELECT 1 FROM group_members
        JOIN relationships AS membership ON membership.id = group_members.relationship_id
        WHERE group_members.group_id = lessons.group_id AND membership.student_id = sqlc.arg(student_id)::uuid
    ) AS current_member,
    teacher.name AS tutor_name, coalesce(mine.state, '')::text AS my_state, mine.colour AS my_colour, lessons.tutor_id
FROM visible
JOIN lessons ON lessons.id = visible.id
JOIN users AS teacher ON teacher.id = lessons.tutor_id
LEFT JOIN groups AS lesson_group ON lesson_group.id = lessons.group_id
LEFT JOIN LATERAL (
    SELECT relationships.state, relationships.colour FROM relationships
    WHERE relationships.tutor_id = lessons.tutor_id AND relationships.student_id = sqlc.arg(student_id)::uuid
        AND relationships.state <> 'discarded'
    LIMIT 1
) AS mine ON true
WHERE (lessons.starts_at, lessons.id) > (sqlc.arg(after_starts_at)::timestamptz, sqlc.arg(after_id)::uuid)
ORDER BY lessons.starts_at, lessons.id
LIMIT sqlc.arg(row_limit)::int;

-- name: ListTutorCalendarAfter :many
SELECT
    lessons.id, lessons.starts_at,
    ARRAY(
        SELECT DISTINCT referenced.student_id FROM (
            SELECT card.student_id FROM relationships AS card WHERE card.id = lessons.relationship_id
            UNION ALL
            SELECT membership.student_id FROM group_members
            JOIN relationships AS membership ON membership.id = group_members.relationship_id
            WHERE group_members.group_id = lessons.group_id
            UNION ALL
            SELECT lesson_roster.student_id FROM lesson_roster WHERE lesson_roster.lesson_id = lessons.id
        ) AS referenced
        WHERE referenced.student_id IS NOT NULL
    )::uuid[] AS account_ids
FROM lessons
WHERE lessons.tutor_id = @tutor_id
    AND (lessons.starts_at, lessons.id) > (sqlc.arg(after_starts_at)::timestamptz, sqlc.arg(after_id)::uuid)
ORDER BY lessons.starts_at, lessons.id
LIMIT sqlc.arg(row_limit)::int;

-- name: GetStudentLessonView :one
SELECT
    lessons.id, lessons.title, lessons.description, lessons.starts_at, lessons.duration_minutes, lessons.status, (lessons.live_at IS NOT NULL)::boolean AS took_place, lessons.timezone,
    lessons.series_id, lessons.version, lessons.cancelled_at, lessons.group_id, lesson_group.name AS group_name,
    EXISTS (
        SELECT 1 FROM group_members
        JOIN relationships AS membership ON membership.id = group_members.relationship_id
        WHERE group_members.group_id = lessons.group_id AND membership.student_id = sqlc.arg(student_id)::uuid
    ) AS current_member,
    EXISTS (
        SELECT 1 FROM relationships AS own_link
        WHERE own_link.id = lessons.relationship_id AND own_link.student_id = sqlc.arg(student_id)::uuid
            AND own_link.state IN ('active', 'provisional')
    ) AS current_link,
    teacher.name AS tutor_name, coalesce(mine.state, '')::text AS my_state, mine.colour AS my_colour,
    EXISTS (
        SELECT 1 FROM meetings WHERE meetings.lesson_id = lessons.id AND meetings.state = 'open' AND lessons.status = 'live'
    ) AS room_open
FROM lessons
JOIN users AS teacher ON teacher.id = lessons.tutor_id
LEFT JOIN groups AS lesson_group ON lesson_group.id = lessons.group_id
LEFT JOIN LATERAL (
    SELECT relationships.state, relationships.colour FROM relationships
    WHERE relationships.tutor_id = lessons.tutor_id AND relationships.student_id = sqlc.arg(student_id)::uuid
        AND relationships.state <> 'discarded'
    LIMIT 1
) AS mine ON true
WHERE lessons.id = sqlc.arg(id)::uuid AND (
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

-- name: DeleteTutorRosterBatch :execrows
DELETE FROM lesson_roster
WHERE (lesson_id, student_id) IN (
    SELECT doomed.lesson_id, doomed.student_id FROM lessons
    JOIN lesson_roster AS doomed ON doomed.lesson_id = lessons.id
    WHERE lessons.tutor_id = @tutor_id
    ORDER BY lessons.starts_at, lessons.id, doomed.student_id
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE OF doomed
);

-- name: DeleteTutorLessonsBatch :execrows
DELETE FROM lessons
WHERE id IN (
    SELECT doomed.id FROM lessons AS doomed
    WHERE doomed.tutor_id = @tutor_id
    ORDER BY doomed.starts_at, doomed.id
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE
);

-- name: DeleteTutorSeriesBatch :execrows
DELETE FROM lesson_series
WHERE id IN (
    SELECT doomed.id FROM lesson_series AS doomed
    WHERE doomed.tutor_id = @tutor_id
    ORDER BY doomed.created_at DESC, doomed.id
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE
);

-- name: DeleteStudentRosterRows :execrows
DELETE FROM lesson_roster WHERE student_id = @student_id;

-- name: ListCardNextLessons :many
WITH candidates AS (
    (SELECT lessons.id FROM lessons
    WHERE lessons.relationship_id = @relationship_id AND lessons.tutor_id = @tutor_id AND lessons.status = 'scheduled'
        AND lessons.starts_at > sqlc.arg(now)::timestamptz
    ORDER BY lessons.starts_at, lessons.id
    LIMIT sqlc.arg(row_limit)::int)
    UNION ALL
    (SELECT lessons.id FROM group_members
    JOIN lessons ON lessons.group_id = group_members.group_id
    WHERE group_members.relationship_id = @relationship_id AND group_members.tutor_id = @tutor_id AND lessons.status = 'scheduled'
        AND lessons.starts_at > sqlc.arg(now)::timestamptz
    ORDER BY lessons.starts_at, lessons.id
    LIMIT sqlc.arg(row_limit)::int)
)
SELECT
    lessons.id, lessons.title, lessons.starts_at, lessons.duration_minutes, lessons.status, (lessons.live_at IS NOT NULL)::boolean AS took_place, lessons.timezone,
    lessons.relationship_id, lessons.group_id,
    card.display_name AS card_name, coalesce(card.state, '')::text AS card_state, card.discard_reason AS card_discard_reason,
    card.colour AS card_colour, lesson_group.name AS group_name, coalesce(lesson_group.colour, '')::text AS group_colour
FROM candidates
JOIN lessons ON lessons.id = candidates.id
LEFT JOIN relationships AS card ON card.id = lessons.relationship_id
LEFT JOIN groups AS lesson_group ON lesson_group.id = lessons.group_id
ORDER BY lessons.starts_at, lessons.id
LIMIT sqlc.arg(row_limit)::int;

-- name: ListCardLessonHistory :many
WITH visible AS (
    (SELECT lessons.id FROM lessons
    WHERE lessons.relationship_id = @relationship_id AND lessons.tutor_id = @tutor_id
        AND (sqlc.narg(after_starts_at)::timestamptz IS NULL
            OR (lessons.starts_at, lessons.id) < (sqlc.narg(after_starts_at)::timestamptz, sqlc.narg(after_id)::uuid))
    ORDER BY lessons.starts_at DESC, lessons.id DESC
    LIMIT sqlc.arg(row_limit)::int)
    UNION ALL
    (SELECT lessons.id FROM group_members
    JOIN lessons ON lessons.group_id = group_members.group_id
    WHERE group_members.relationship_id = @relationship_id AND group_members.tutor_id = @tutor_id AND lessons.status = 'scheduled'
        AND (sqlc.narg(after_starts_at)::timestamptz IS NULL
            OR (lessons.starts_at, lessons.id) < (sqlc.narg(after_starts_at)::timestamptz, sqlc.narg(after_id)::uuid))
    ORDER BY lessons.starts_at DESC, lessons.id DESC
    LIMIT sqlc.arg(row_limit)::int)
    UNION ALL
    (SELECT lessons.id FROM relationships AS owner
    JOIN lesson_roster ON lesson_roster.student_id = owner.student_id
    JOIN lessons ON lessons.id = lesson_roster.lesson_id
    WHERE owner.id = @relationship_id AND owner.tutor_id = @tutor_id
        AND lessons.tutor_id = @tutor_id AND lessons.group_id IS NOT NULL AND lessons.status <> 'scheduled'
        AND (sqlc.narg(after_starts_at)::timestamptz IS NULL
            OR (lesson_roster.lesson_starts_at <= sqlc.narg(after_starts_at)::timestamptz
                AND (lesson_roster.lesson_starts_at, lesson_roster.lesson_id) < (sqlc.narg(after_starts_at)::timestamptz, sqlc.narg(after_id)::uuid)))
    ORDER BY lesson_roster.lesson_starts_at DESC, lesson_roster.lesson_id DESC
    LIMIT sqlc.arg(row_limit)::int)
)
SELECT
    lessons.id, lessons.title, lessons.starts_at, lessons.duration_minutes, lessons.status, (lessons.live_at IS NOT NULL)::boolean AS took_place, lessons.timezone,
    lessons.relationship_id, lessons.group_id,
    card.display_name AS card_name, coalesce(card.state, '')::text AS card_state, card.discard_reason AS card_discard_reason,
    card.colour AS card_colour, lesson_group.name AS group_name, coalesce(lesson_group.colour, '')::text AS group_colour
FROM visible
JOIN lessons ON lessons.id = visible.id
LEFT JOIN relationships AS card ON card.id = lessons.relationship_id
LEFT JOIN groups AS lesson_group ON lesson_group.id = lessons.group_id
ORDER BY lessons.starts_at DESC, lessons.id DESC
LIMIT sqlc.arg(row_limit)::int;

-- name: ListCardsNextLesson :many
SELECT
    requested.id AS card_id, lessons.id, lessons.title, lessons.starts_at, lessons.duration_minutes, lessons.status, lessons.timezone,
    lessons.relationship_id, lessons.group_id,
    card.display_name AS card_name, coalesce(card.state, '')::text AS card_state, card.discard_reason AS card_discard_reason,
    card.colour AS card_colour, lesson_group.name AS group_name, coalesce(lesson_group.colour, '')::text AS group_colour
FROM relationships AS requested
JOIN LATERAL (
    SELECT candidates.id FROM (
        (SELECT own.id, own.starts_at FROM lessons AS own
        WHERE own.relationship_id = requested.id AND own.tutor_id = requested.tutor_id AND own.status = 'scheduled'
            AND own.starts_at > sqlc.arg(now)::timestamptz
        ORDER BY own.starts_at, own.id
        LIMIT 1)
        UNION ALL
        (SELECT shared.id, shared.starts_at FROM group_members
        JOIN lessons AS shared ON shared.group_id = group_members.group_id
        WHERE group_members.relationship_id = requested.id AND group_members.tutor_id = requested.tutor_id
            AND shared.status = 'scheduled' AND shared.starts_at > sqlc.arg(now)::timestamptz
        ORDER BY shared.starts_at, shared.id
        LIMIT 1)
    ) AS candidates
    ORDER BY candidates.starts_at, candidates.id
    LIMIT 1
) AS next ON true
JOIN lessons ON lessons.id = next.id
LEFT JOIN relationships AS card ON card.id = lessons.relationship_id
LEFT JOIN groups AS lesson_group ON lesson_group.id = lessons.group_id
WHERE requested.tutor_id = @tutor_id AND requested.id = ANY (sqlc.arg(ids)::uuid[]);

-- name: ListGroupNextLessons :many
SELECT
    lessons.id, lessons.title, lessons.starts_at, lessons.duration_minutes, lessons.status, (lessons.live_at IS NOT NULL)::boolean AS took_place, lessons.timezone,
    lessons.relationship_id, lessons.group_id,
    card.display_name AS card_name, coalesce(card.state, '')::text AS card_state, card.discard_reason AS card_discard_reason,
    card.colour AS card_colour, lesson_group.name AS group_name, coalesce(lesson_group.colour, '')::text AS group_colour
FROM lessons
LEFT JOIN relationships AS card ON card.id = lessons.relationship_id
LEFT JOIN groups AS lesson_group ON lesson_group.id = lessons.group_id
WHERE lessons.group_id = @group_id AND lessons.tutor_id = @tutor_id AND lessons.status = 'scheduled'
    AND lessons.starts_at > sqlc.arg(now)::timestamptz
ORDER BY lessons.starts_at, lessons.id
LIMIT sqlc.arg(row_limit)::int;

-- name: ListGroupsNextLesson :many
SELECT
    requested.id AS requested_group_id, lessons.id, lessons.title, lessons.starts_at, lessons.duration_minutes, lessons.status,
    lessons.timezone, lessons.relationship_id, lessons.group_id,
    card.display_name AS card_name, coalesce(card.state, '')::text AS card_state, card.discard_reason AS card_discard_reason,
    card.colour AS card_colour, lesson_group.name AS group_name, coalesce(lesson_group.colour, '')::text AS group_colour
FROM groups AS requested
JOIN LATERAL (
    SELECT upcoming.id FROM lessons AS upcoming
    WHERE upcoming.group_id = requested.id AND upcoming.tutor_id = requested.tutor_id AND upcoming.status = 'scheduled'
        AND upcoming.starts_at > sqlc.arg(now)::timestamptz
    ORDER BY upcoming.starts_at, upcoming.id
    LIMIT 1
) AS next ON true
JOIN lessons ON lessons.id = next.id
LEFT JOIN relationships AS card ON card.id = lessons.relationship_id
LEFT JOIN groups AS lesson_group ON lesson_group.id = lessons.group_id
WHERE requested.tutor_id = @tutor_id AND requested.id = ANY (sqlc.arg(ids)::uuid[]);

-- name: CountFutureLessonsOfGroup :one
SELECT count(id) FROM lessons
WHERE group_id = @group_id AND tutor_id = @tutor_id AND status = 'scheduled' AND starts_at > sqlc.arg(now)::timestamptz;

-- name: GroupHasOpenSeries :one
SELECT EXISTS (
    SELECT 1 FROM lesson_series AS series
    WHERE series.group_id = @group_id AND series.tutor_id = @tutor_id AND series.ended_at IS NULL AND (
        (series.end_date IS NOT NULL AND series.end_date >= (sqlc.arg(now)::timestamptz AT TIME ZONE series.timezone)::date)
        OR (series.count IS NOT NULL AND EXISTS (
            SELECT 1 FROM lessons WHERE lessons.series_id = series.id AND lessons.starts_at > sqlc.arg(now)::timestamptz
        ))
    )
);

-- name: TutorHasLessonStarting :one
SELECT EXISTS (
    SELECT 1 FROM lessons
    WHERE tutor_id = @tutor_id AND starts_at >= sqlc.arg(range_start)::timestamptz AND starts_at < sqlc.arg(range_end)::timestamptz
)::boolean;
