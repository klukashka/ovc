-- name: ListFoldersOfTutor :many
SELECT id, parent_id, name, normalised_name FROM folders WHERE tutor_id = @tutor_id ORDER BY normalised_name, id;

-- name: CountFoldersOfTutor :one
SELECT count(id) FROM folders WHERE tutor_id = @tutor_id;

-- name: FolderNameTaken :one
SELECT EXISTS (
    SELECT 1 FROM folders
    WHERE tutor_id = @tutor_id AND parent_id IS NOT DISTINCT FROM sqlc.narg(parent_id)::uuid AND normalised_name = sqlc.arg(normalised_name)::text
        AND id <> @excluded_id
);

-- name: CreateFolder :exec
INSERT INTO folders (id, tutor_id, parent_id, name, normalised_name, created_at, updated_at)
VALUES (@id, @tutor_id, sqlc.narg(parent_id)::uuid, @name, sqlc.arg(normalised_name)::text, @now, @now);

-- name: UpdateFolder :execrows
UPDATE folders SET parent_id = sqlc.narg(parent_id)::uuid, name = @name, normalised_name = sqlc.arg(normalised_name)::text, updated_at = @now
WHERE id = @id AND tutor_id = @tutor_id;

-- name: FolderHasContents :one
SELECT EXISTS (
    SELECT 1 FROM folders AS children WHERE children.parent_id = sqlc.arg(folder_id)::uuid AND children.tutor_id = sqlc.arg(tutor_id)::uuid
    UNION ALL
    SELECT 1 FROM materials
    WHERE materials.folder_id = sqlc.arg(folder_id)::uuid AND materials.tutor_id = sqlc.arg(tutor_id)::uuid AND materials.deleted_at IS NULL
    UNION ALL
    SELECT 1 FROM templates
    WHERE templates.folder_id = sqlc.arg(folder_id)::uuid AND templates.tutor_id = sqlc.arg(tutor_id)::uuid AND templates.deleted_at IS NULL
);

-- name: DeleteFolder :execrows
DELETE FROM folders WHERE id = @id AND tutor_id = @tutor_id;

-- name: CountMaterialsOfTutor :one
SELECT count(id) FILTER (WHERE deleted_at IS NULL) AS live, count(id) AS total FROM materials WHERE tutor_id = @tutor_id;

-- name: LockBindableLibraryUpload :one
SELECT id, size_bytes::bigint AS size_bytes FROM attachments
WHERE id = @id AND uploader_id = @uploader_id AND context = 'library' AND state = 'pending' AND expires_at > sqlc.arg(now)::timestamptz
    AND size_bytes IS NOT NULL
FOR UPDATE;

-- name: CreateMaterial :exec
INSERT INTO materials (id, tutor_id, folder_id, title, normalised_title, attachment_id, created_at, updated_at)
VALUES (@id, @tutor_id, sqlc.narg(folder_id)::uuid, @title, sqlc.arg(normalised_title)::text, @attachment_id, @now, @now);

-- name: GetMaterialState :one
SELECT id, folder_id, title, deleted_at FROM materials WHERE id = @id AND tutor_id = @tutor_id;

-- name: GetLiveMaterial :one
SELECT materials.id, materials.folder_id, materials.title, materials.normalised_title, materials.attachment_id, materials.updated_at,
    attachments.filename, attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes, attachments.width,
    attachments.height
FROM materials
JOIN attachments ON attachments.id = materials.attachment_id
WHERE materials.id = @id AND materials.tutor_id = @tutor_id AND materials.deleted_at IS NULL;

-- name: UpdateMaterial :execrows
UPDATE materials SET folder_id = sqlc.narg(folder_id)::uuid, title = @title, normalised_title = sqlc.arg(normalised_title)::text, updated_at = @now
WHERE id = @id AND tutor_id = @tutor_id AND deleted_at IS NULL;

-- name: SoftDeleteMaterial :execrows
UPDATE materials SET deleted_at = sqlc.arg(now)::timestamptz, folder_id = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND tutor_id = @tutor_id AND deleted_at IS NULL;

-- name: RemoveMaterialReference :many
DELETE FROM attachment_references WHERE ref_kind = 'material' AND ref_id = sqlc.arg(material_id)::uuid RETURNING attachment_id;

-- name: ListRootMaterials :many
SELECT materials.id, materials.folder_id, materials.title, materials.normalised_title, materials.attachment_id, materials.updated_at,
    attachments.filename, attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes, attachments.width,
    attachments.height
FROM materials
JOIN attachments ON attachments.id = materials.attachment_id
WHERE materials.tutor_id = @tutor_id AND materials.folder_id IS NULL AND materials.deleted_at IS NULL
    AND materials.normalised_title >= sqlc.arg(after_title)::text
    AND (
        materials.normalised_title > sqlc.arg(after_title)::text
        OR 'material' > sqlc.arg(after_kind)::text COLLATE "C"
        OR ('material' = sqlc.arg(after_kind)::text AND materials.id > sqlc.arg(after_id)::uuid)
    )
ORDER BY materials.normalised_title, materials.id
LIMIT sqlc.arg(page_size);

-- name: ListFolderMaterials :many
SELECT materials.id, materials.folder_id, materials.title, materials.normalised_title, materials.attachment_id, materials.updated_at,
    attachments.filename, attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes, attachments.width,
    attachments.height
FROM materials
JOIN attachments ON attachments.id = materials.attachment_id
WHERE materials.tutor_id = @tutor_id AND materials.folder_id = sqlc.arg(folder_id)::uuid AND materials.deleted_at IS NULL
    AND materials.normalised_title >= sqlc.arg(after_title)::text
    AND (
        materials.normalised_title > sqlc.arg(after_title)::text
        OR 'material' > sqlc.arg(after_kind)::text COLLATE "C"
        OR ('material' = sqlc.arg(after_kind)::text AND materials.id > sqlc.arg(after_id)::uuid)
    )
ORDER BY materials.normalised_title, materials.id
LIMIT sqlc.arg(page_size);

-- name: SearchMaterials :many
SELECT materials.id, materials.folder_id, materials.title, materials.normalised_title, materials.attachment_id, materials.updated_at,
    attachments.filename, attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes, attachments.width,
    attachments.height
FROM materials
JOIN attachments ON attachments.id = materials.attachment_id
WHERE materials.tutor_id = @tutor_id AND materials.deleted_at IS NULL
    AND materials.normalised_title LIKE '%' || sqlc.arg(pattern)::text || '%'
    AND materials.normalised_title >= sqlc.arg(after_title)::text
    AND (
        materials.normalised_title > sqlc.arg(after_title)::text
        OR 'material' > sqlc.arg(after_kind)::text COLLATE "C"
        OR ('material' = sqlc.arg(after_kind)::text AND materials.id > sqlc.arg(after_id)::uuid)
    )
ORDER BY materials.normalised_title, materials.id
LIMIT sqlc.arg(page_size);

-- name: VisibleMaterialReferences :many
SELECT materials.id, attachments.width
FROM attachment_references
JOIN materials
    ON materials.id = attachment_references.ref_id AND materials.attachment_id = attachment_references.attachment_id
        AND materials.tutor_id = sqlc.arg(viewer)::uuid AND materials.deleted_at IS NULL
JOIN attachments ON attachments.id = attachment_references.attachment_id AND attachments.state = 'bound'
WHERE attachment_references.attachment_id = @attachment_id AND attachment_references.ref_kind = 'material'
ORDER BY materials.id;

-- name: GetServableMaterialFile :one
SELECT attachments.storage_key, attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes,
    attachments.filename, attachments.width
FROM attachments
JOIN attachment_references
    ON attachment_references.attachment_id = attachments.id AND attachment_references.ref_kind = 'material'
        AND attachment_references.ref_id = sqlc.arg(material_id)::uuid
JOIN materials ON materials.id = attachment_references.ref_id AND materials.attachment_id = attachments.id AND materials.deleted_at IS NULL
WHERE attachments.id = @id AND attachments.state = 'bound';

-- name: ListTutorMaterialReferencesBatch :many
SELECT attachment_references.attachment_id
FROM attachment_references
JOIN materials ON materials.id = attachment_references.ref_id
WHERE attachment_references.ref_kind = 'material' AND materials.tutor_id = @tutor_id
ORDER BY attachment_references.attachment_id
LIMIT sqlc.arg(batch_size);

-- name: RemoveTutorMaterialReferences :many
DELETE FROM attachment_references
USING materials
WHERE attachment_references.ref_kind = 'material' AND attachment_references.ref_id = materials.id AND materials.tutor_id = @tutor_id
    AND attachment_references.attachment_id = ANY (sqlc.arg(attachment_ids)::uuid[])
RETURNING attachment_references.attachment_id;

-- name: DeleteTutorMaterialsBatch :execrows
DELETE FROM materials
WHERE id IN (SELECT candidate.id FROM materials AS candidate WHERE candidate.tutor_id = @tutor_id ORDER BY candidate.id LIMIT sqlc.arg(batch_size));

-- name: DeleteTutorLeafFoldersBatch :execrows
DELETE FROM folders
WHERE id IN (
    SELECT candidate.id FROM folders AS candidate
    WHERE candidate.tutor_id = @tutor_id
        AND NOT EXISTS (SELECT 1 FROM folders AS children WHERE children.parent_id = candidate.id AND children.tutor_id = candidate.tutor_id)
    ORDER BY candidate.id
    LIMIT sqlc.arg(batch_size)
);
