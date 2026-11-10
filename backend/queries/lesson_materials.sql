-- name: ListTutorMaterialsByID :many
SELECT id, attachment_id, deleted_at FROM materials
WHERE tutor_id = @tutor_id AND id = ANY (sqlc.arg(ids)::uuid[]);

-- name: ListMaterialIDsOfLessons :many
SELECT lesson_id, material_id FROM lesson_materials
WHERE lesson_id = ANY (sqlc.arg(lesson_ids)::uuid[])
ORDER BY lesson_id, position;

-- name: ListMaterialIDsOfSeries :many
SELECT material_id FROM series_materials WHERE series_id = @series_id ORDER BY position;

-- name: ListLessonMaterialViews :many
SELECT lesson_materials.lesson_id, materials.id AS material_id, materials.attachment_id, materials.title, attachments.filename,
    attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes, attachments.width, attachments.height
FROM lesson_materials
JOIN materials ON materials.id = lesson_materials.material_id AND materials.tutor_id = lesson_materials.tutor_id
JOIN attachments ON attachments.id = materials.attachment_id AND attachments.state = 'bound'
WHERE lesson_materials.lesson_id = ANY (sqlc.arg(lesson_ids)::uuid[])
ORDER BY lesson_materials.lesson_id, lesson_materials.position;

-- name: RemoveLessonMaterialReferences :many
DELETE FROM attachment_references
WHERE ref_kind = 'lesson' AND ref_id = ANY (sqlc.arg(lesson_ids)::uuid[])
RETURNING attachment_id;

-- name: DeleteLessonMaterials :execrows
DELETE FROM lesson_materials WHERE lesson_id = ANY (sqlc.arg(lesson_ids)::uuid[]);

-- name: InsertLessonMaterials :execrows
INSERT INTO lesson_materials (lesson_id, material_id, tutor_id, position, created_at)
SELECT lesson.id, chosen.material_id, sqlc.arg(tutor_id)::uuid, (chosen.ordinality - 1)::smallint, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(lesson_ids)::uuid[]) AS lesson (id)
CROSS JOIN unnest(sqlc.arg(material_ids)::uuid[]) WITH ORDINALITY AS chosen (material_id, ordinality);

-- name: CopySeriesMaterialsToLessons :execrows
INSERT INTO lesson_materials (lesson_id, material_id, tutor_id, position, created_at)
SELECT lesson.id, series_materials.material_id, series_materials.tutor_id, series_materials.position, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(lesson_ids)::uuid[]) AS lesson (id)
CROSS JOIN series_materials
WHERE series_materials.series_id = sqlc.arg(series_id)::uuid;

-- name: InsertLessonMaterialReferences :many
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, created_at)
SELECT materials.attachment_id, 'lesson', lesson_materials.lesson_id, sqlc.arg(now)::timestamptz
FROM lesson_materials
JOIN materials ON materials.id = lesson_materials.material_id AND materials.tutor_id = lesson_materials.tutor_id
WHERE lesson_materials.lesson_id = ANY (sqlc.arg(lesson_ids)::uuid[])
ON CONFLICT DO NOTHING
RETURNING attachment_id;

-- name: RemoveSeriesMaterialReferences :many
DELETE FROM attachment_references WHERE ref_kind = 'series' AND ref_id = sqlc.arg(series_id)::uuid RETURNING attachment_id;

-- name: DeleteSeriesMaterials :execrows
DELETE FROM series_materials WHERE series_id = @series_id;

-- name: InsertSeriesMaterials :execrows
INSERT INTO series_materials (series_id, material_id, tutor_id, position, created_at)
SELECT sqlc.arg(series_id)::uuid, chosen.material_id, sqlc.arg(tutor_id)::uuid, (chosen.ordinality - 1)::smallint, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(material_ids)::uuid[]) WITH ORDINALITY AS chosen (material_id, ordinality);

-- name: CopySeriesMaterials :execrows
INSERT INTO series_materials (series_id, material_id, tutor_id, position, created_at)
SELECT sqlc.arg(to_series_id)::uuid, material_id, tutor_id, position, sqlc.arg(now)::timestamptz
FROM series_materials
WHERE series_id = sqlc.arg(from_series_id)::uuid;

-- name: InsertSeriesMaterialReferences :many
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, created_at)
SELECT materials.attachment_id, 'series', series_materials.series_id, sqlc.arg(now)::timestamptz
FROM series_materials
JOIN materials ON materials.id = series_materials.material_id AND materials.tutor_id = series_materials.tutor_id
WHERE series_materials.series_id = sqlc.arg(series_id)::uuid
ON CONFLICT DO NOTHING
RETURNING attachment_id;

-- name: ListLessonRosterStudents :many
SELECT student_id FROM lesson_roster WHERE lesson_id = @lesson_id ORDER BY student_id;

-- name: LockLessonForMaterials :one
SELECT id, series_id, status, version, overridden_fields FROM lessons WHERE id = @id AND tutor_id = @tutor_id FOR UPDATE;

-- name: RecordLessonMaterialsChange :execrows
UPDATE lessons SET overridden_fields = sqlc.arg(overridden_fields)::text[], version = version + 1, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND version = @version;

-- name: VisibleLessonReferences :many
SELECT lessons.id, attachments.width
FROM attachment_references
JOIN lessons ON lessons.id = attachment_references.ref_id
JOIN attachments ON attachments.id = attachment_references.attachment_id AND attachments.state = 'bound'
WHERE attachment_references.attachment_id = @attachment_id AND attachment_references.ref_kind = 'lesson' AND (
    lessons.tutor_id = sqlc.arg(viewer)::uuid
    OR (lessons.status = 'scheduled' AND EXISTS (
        SELECT 1 FROM relationships
        WHERE relationships.id = lessons.relationship_id AND relationships.student_id = sqlc.arg(viewer)::uuid
            AND relationships.state IN ('active', 'provisional')
    ))
    OR (lessons.status = 'scheduled' AND EXISTS (
        SELECT 1 FROM group_members
        JOIN relationships AS membership ON membership.id = group_members.relationship_id
        WHERE group_members.group_id = lessons.group_id AND membership.student_id = sqlc.arg(viewer)::uuid
    ))
    OR (lessons.status <> 'scheduled' AND EXISTS (
        SELECT 1 FROM lesson_roster
        WHERE lesson_roster.lesson_id = lessons.id AND lesson_roster.student_id = sqlc.arg(viewer)::uuid
    ) AND NOT EXISTS (
        SELECT 1 FROM relationships AS waiting
        WHERE waiting.id = lessons.relationship_id AND waiting.state = 'reclaim_pending'
    ))
)
ORDER BY lessons.id;

-- name: GetServableLessonFile :one
SELECT attachments.storage_key, attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes,
    attachments.filename, attachments.width
FROM attachments
JOIN attachment_references
    ON attachment_references.attachment_id = attachments.id AND attachment_references.ref_kind = 'lesson'
        AND attachment_references.ref_id = sqlc.arg(lesson_id)::uuid
WHERE attachments.id = @id AND attachments.state = 'bound';

-- name: ListTutorLessonMaterialsBatch :many
SELECT lesson_materials.lesson_id, lesson_materials.material_id, materials.attachment_id
FROM lesson_materials
JOIN materials ON materials.id = lesson_materials.material_id AND materials.tutor_id = lesson_materials.tutor_id
WHERE lesson_materials.tutor_id = @tutor_id
ORDER BY lesson_materials.lesson_id, lesson_materials.material_id
LIMIT sqlc.arg(batch_size);

-- name: DeleteLessonMaterialPairs :execrows
DELETE FROM lesson_materials
USING (SELECT unnest(sqlc.arg(lesson_ids)::uuid[]) AS lesson_id, unnest(sqlc.arg(material_ids)::uuid[]) AS material_id) AS pair
WHERE lesson_materials.lesson_id = pair.lesson_id AND lesson_materials.material_id = pair.material_id
    AND lesson_materials.tutor_id = sqlc.arg(tutor_id)::uuid;

-- name: ListTutorSeriesMaterialsBatch :many
SELECT series_materials.series_id, series_materials.material_id, materials.attachment_id
FROM series_materials
JOIN materials ON materials.id = series_materials.material_id AND materials.tutor_id = series_materials.tutor_id
WHERE series_materials.tutor_id = @tutor_id
ORDER BY series_materials.series_id, series_materials.material_id
LIMIT sqlc.arg(batch_size);

-- name: DeleteSeriesMaterialPairs :execrows
DELETE FROM series_materials
USING (SELECT unnest(sqlc.arg(series_ids)::uuid[]) AS series_id, unnest(sqlc.arg(material_ids)::uuid[]) AS material_id) AS pair
WHERE series_materials.series_id = pair.series_id AND series_materials.material_id = pair.material_id
    AND series_materials.tutor_id = sqlc.arg(tutor_id)::uuid;

-- name: RemoveReferencePairs :many
DELETE FROM attachment_references
USING (SELECT unnest(sqlc.arg(attachment_ids)::uuid[]) AS attachment_id, unnest(sqlc.arg(ref_ids)::uuid[]) AS ref_id) AS pair
WHERE attachment_references.ref_kind = sqlc.arg(ref_kind)::text AND attachment_references.attachment_id = pair.attachment_id
    AND attachment_references.ref_id = pair.ref_id
RETURNING attachment_references.attachment_id;

-- name: ListRetiredSeriesWithMaterials :many
SELECT DISTINCT lesson_series.id, lesson_series.tutor_id
FROM series_materials
JOIN lesson_series ON lesson_series.id = series_materials.series_id
WHERE (
    lesson_series.ended_at IS NOT NULL OR lesson_series.end_date IS NULL
    OR lesson_series.end_date < (sqlc.arg(now)::timestamptz AT TIME ZONE lesson_series.timezone)::date OR NOT EXISTS (
        SELECT 1 FROM generate_series(0, 6) AS step (days)
        WHERE extract(isodow FROM greatest(lesson_series.materialised_until + 1, lesson_series.first_date) + step.days)::smallint
                = ANY (lesson_series.weekdays)
            AND greatest(lesson_series.materialised_until + 1, lesson_series.first_date) + step.days <= lesson_series.end_date
    )
)
ORDER BY lesson_series.id
LIMIT sqlc.arg(row_limit)::int;

-- name: LockRetiredSeries :one
SELECT lesson_series.id FROM lesson_series
WHERE lesson_series.id = @id AND (
    lesson_series.ended_at IS NOT NULL OR lesson_series.end_date IS NULL
    OR lesson_series.end_date < (sqlc.arg(now)::timestamptz AT TIME ZONE lesson_series.timezone)::date OR NOT EXISTS (
        SELECT 1 FROM generate_series(0, 6) AS step (days)
        WHERE extract(isodow FROM greatest(lesson_series.materialised_until + 1, lesson_series.first_date) + step.days)::smallint
                = ANY (lesson_series.weekdays)
            AND greatest(lesson_series.materialised_until + 1, lesson_series.first_date) + step.days <= lesson_series.end_date
    )
)
FOR UPDATE;

-- name: FirstLessonPresentation :one
SELECT attachments.id, attachments.storage_key
FROM lesson_materials
JOIN materials ON materials.id = lesson_materials.material_id AND materials.tutor_id = lesson_materials.tutor_id
JOIN attachments ON attachments.id = materials.attachment_id AND attachments.state = 'bound'
WHERE lesson_materials.lesson_id = @lesson_id AND attachments.media_type = 'application/pdf'
ORDER BY lesson_materials.position
LIMIT 1;
