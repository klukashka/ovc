-- name: CountTemplatesOfTutor :one
SELECT count(id) FILTER (WHERE deleted_at IS NULL) AS live, count(id) AS total FROM templates WHERE tutor_id = @tutor_id;

-- name: TemplateBlockBytesOfTutor :one
SELECT COALESCE(sum(blocks_bytes), 0)::bigint AS used FROM templates
WHERE tutor_id = @tutor_id AND deleted_at IS NULL AND id <> sqlc.arg(excluded_id)::uuid;

-- name: GetTemplateByCreateKey :one
SELECT id, deleted_at FROM templates WHERE tutor_id = @tutor_id AND create_key = sqlc.arg(create_key)::text;

-- name: CreateTemplate :exec
INSERT INTO templates (
    id, tutor_id, folder_id, title, normalised_title, blocks, schema_version, version, last_save_id, create_key, question_count, max_points,
    ready, blocks_bytes, created_at, updated_at
)
VALUES (
    @id, @tutor_id, sqlc.narg(folder_id)::uuid, @title, sqlc.arg(normalised_title)::text, @blocks, @schema_version, 1, NULL,
    sqlc.narg(create_key)::text, @question_count, @max_points, @ready, @blocks_bytes, @now, @now
);

-- name: GetLiveTemplate :one
SELECT id, folder_id, title, normalised_title, blocks, version, last_save_id, question_count, max_points, ready, blocks_bytes, created_at,
    updated_at
FROM templates
WHERE id = @id AND tutor_id = @tutor_id AND deleted_at IS NULL;

-- name: LockTemplateState :one
SELECT id, folder_id, version, last_save_id, deleted_at FROM templates WHERE id = @id AND tutor_id = @tutor_id FOR UPDATE;

-- name: GetTemplateLastSave :one
SELECT last_save_id FROM templates WHERE id = @id AND tutor_id = @tutor_id AND deleted_at IS NULL;

-- name: SaveTemplate :execrows
UPDATE templates
SET title = @title, normalised_title = sqlc.arg(normalised_title)::text, blocks = @blocks, version = version + 1,
    last_save_id = sqlc.arg(save_id)::uuid, question_count = @question_count, max_points = @max_points, ready = @ready,
    blocks_bytes = @blocks_bytes, updated_at = @now
WHERE id = @id AND tutor_id = @tutor_id AND deleted_at IS NULL;

-- name: MoveTemplate :execrows
UPDATE templates SET folder_id = sqlc.narg(folder_id)::uuid, version = version + 1, updated_at = @now
WHERE id = @id AND tutor_id = @tutor_id AND deleted_at IS NULL;

-- name: SoftDeleteTemplate :execrows
UPDATE templates
SET deleted_at = sqlc.arg(now)::timestamptz, folder_id = NULL, blocks = '[]'::jsonb, blocks_bytes = 0, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND tutor_id = @tutor_id AND deleted_at IS NULL;

-- name: ListTemplateReferences :many
SELECT attachment_id FROM attachment_references WHERE ref_kind = 'template' AND ref_id = sqlc.arg(template_id)::uuid ORDER BY attachment_id;

-- name: AddTemplateReferences :exec
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, question_id, created_at)
SELECT attachment_id, 'template', sqlc.arg(template_id)::uuid, NULL, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(attachment_ids)::uuid[]) AS attachment_id;

-- name: RemoveTemplateReferences :many
DELETE FROM attachment_references
WHERE ref_kind = 'template' AND ref_id = sqlc.arg(template_id)::uuid AND attachment_id = ANY (sqlc.arg(attachment_ids)::uuid[])
RETURNING attachment_id;

-- name: RemoveAllTemplateReferences :many
DELETE FROM attachment_references WHERE ref_kind = 'template' AND ref_id = sqlc.arg(template_id)::uuid RETURNING attachment_id;

-- name: LockBindablePromptImages :many
SELECT id FROM attachments
WHERE id = ANY (sqlc.arg(ids)::uuid[]) AND uploader_id = @uploader_id AND context = 'prompt_image' AND state = 'pending'
    AND expires_at > sqlc.arg(now)::timestamptz AND size_bytes IS NOT NULL AND width IS NOT NULL
ORDER BY id
FOR UPDATE;

-- name: ListPromptPictures :many
SELECT id FROM attachments
WHERE id = ANY (sqlc.arg(ids)::uuid[]) AND context = 'prompt_image' AND width IS NOT NULL AND state IN ('pending', 'bound')
ORDER BY id;

-- name: ListTemplateBlockMaterials :many
SELECT id, attachment_id, (deleted_at IS NOT NULL)::boolean AS deleted FROM materials
WHERE tutor_id = @tutor_id AND id = ANY (sqlc.arg(ids)::uuid[])
ORDER BY id;

-- name: DescribeTemplateMaterials :many
SELECT materials.id, materials.attachment_id, materials.title, (materials.deleted_at IS NOT NULL)::boolean AS deleted, attachments.filename,
    attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes, attachments.width, attachments.height
FROM materials
JOIN attachments ON attachments.id = materials.attachment_id
WHERE materials.tutor_id = @tutor_id AND materials.id = ANY (sqlc.arg(ids)::uuid[])
ORDER BY materials.id;

-- name: DescribeTemplateImages :many
SELECT id, filename, media_type::text AS media_type, size_bytes::bigint AS size_bytes, width, height FROM attachments
WHERE id = ANY (sqlc.arg(ids)::uuid[])
ORDER BY id;

-- name: CountLiveTemplatesOfTutor :one
SELECT count(id) FROM templates WHERE tutor_id = @tutor_id AND deleted_at IS NULL;

-- name: ListRootTemplates :many
SELECT id, folder_id, title, normalised_title, updated_at, question_count, max_points, ready, version FROM templates
WHERE tutor_id = @tutor_id AND folder_id IS NULL AND deleted_at IS NULL
    AND normalised_title >= sqlc.arg(after_title)::text
    AND (
        normalised_title > sqlc.arg(after_title)::text
        OR 'template' > sqlc.arg(after_kind)::text COLLATE "C"
        OR ('template' = sqlc.arg(after_kind)::text AND id > sqlc.arg(after_id)::uuid)
    )
ORDER BY normalised_title, id
LIMIT sqlc.arg(page_size);

-- name: ListFolderTemplates :many
SELECT id, folder_id, title, normalised_title, updated_at, question_count, max_points, ready, version FROM templates
WHERE tutor_id = @tutor_id AND folder_id = sqlc.arg(folder_id)::uuid AND deleted_at IS NULL
    AND normalised_title >= sqlc.arg(after_title)::text
    AND (
        normalised_title > sqlc.arg(after_title)::text
        OR 'template' > sqlc.arg(after_kind)::text COLLATE "C"
        OR ('template' = sqlc.arg(after_kind)::text AND id > sqlc.arg(after_id)::uuid)
    )
ORDER BY normalised_title, id
LIMIT sqlc.arg(page_size);

-- name: SearchTemplates :many
SELECT id, folder_id, title, normalised_title, updated_at, question_count, max_points, ready, version FROM templates
WHERE tutor_id = @tutor_id AND deleted_at IS NULL
    AND normalised_title LIKE '%' || sqlc.arg(pattern)::text || '%'
    AND normalised_title >= sqlc.arg(after_title)::text
    AND (
        normalised_title > sqlc.arg(after_title)::text
        OR 'template' > sqlc.arg(after_kind)::text COLLATE "C"
        OR ('template' = sqlc.arg(after_kind)::text AND id > sqlc.arg(after_id)::uuid)
    )
ORDER BY normalised_title, id
LIMIT sqlc.arg(page_size);

-- name: VisibleTemplateReferences :many
SELECT templates.id, attachments.width
FROM attachment_references
JOIN templates
    ON templates.id = attachment_references.ref_id AND templates.tutor_id = sqlc.arg(viewer)::uuid AND templates.deleted_at IS NULL
JOIN attachments ON attachments.id = attachment_references.attachment_id AND attachments.state = 'bound'
WHERE attachment_references.attachment_id = @attachment_id AND attachment_references.ref_kind = 'template'
ORDER BY templates.id;

-- name: GetServableTemplateFile :one
SELECT attachments.storage_key, attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes,
    attachments.filename, attachments.width, attachments.context
FROM attachments
JOIN attachment_references
    ON attachment_references.attachment_id = attachments.id AND attachment_references.ref_kind = 'template'
        AND attachment_references.ref_id = sqlc.arg(template_id)::uuid
JOIN templates ON templates.id = attachment_references.ref_id AND templates.deleted_at IS NULL
WHERE attachments.id = @id AND attachments.state = 'bound';

-- name: ListTutorTemplateReferencesBatch :many
SELECT attachment_references.attachment_id
FROM attachment_references
JOIN templates ON templates.id = attachment_references.ref_id
WHERE attachment_references.ref_kind = 'template' AND templates.tutor_id = @tutor_id
ORDER BY attachment_references.attachment_id
LIMIT sqlc.arg(batch_size);

-- name: RemoveTutorTemplateReferences :many
DELETE FROM attachment_references
USING templates
WHERE attachment_references.ref_kind = 'template' AND attachment_references.ref_id = templates.id AND templates.tutor_id = @tutor_id
    AND attachment_references.attachment_id = ANY (sqlc.arg(attachment_ids)::uuid[])
RETURNING attachment_references.attachment_id;

-- name: DeleteTutorTemplatesBatch :execrows
DELETE FROM templates
WHERE id IN (SELECT candidate.id FROM templates AS candidate WHERE candidate.tutor_id = @tutor_id ORDER BY candidate.id LIMIT sqlc.arg(batch_size));
