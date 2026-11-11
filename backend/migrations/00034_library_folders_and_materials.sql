-- +goose Up
CREATE TABLE folders (
    id uuid PRIMARY KEY,
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE RESTRICT,
    parent_id uuid,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    normalised_name text COLLATE "C" NOT NULL CHECK (octet_length(normalised_name) BETWEEN 1 AND 400),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT folders_id_tutor_key UNIQUE (id, tutor_id),
    CONSTRAINT folders_parent_fkey FOREIGN KEY (parent_id, tutor_id) REFERENCES folders (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT folders_sibling_name_key UNIQUE NULLS NOT DISTINCT (tutor_id, parent_id, normalised_name)
);

CREATE INDEX folders_parent_idx ON folders (parent_id, tutor_id);

CREATE TABLE materials (
    id uuid PRIMARY KEY,
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE RESTRICT,
    folder_id uuid,
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    normalised_title text COLLATE "C" NOT NULL CHECK (octet_length(normalised_title) BETWEEN 1 AND 800),
    attachment_id uuid NOT NULL,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT materials_id_tutor_key UNIQUE (id, tutor_id),
    CONSTRAINT materials_folder_fkey FOREIGN KEY (folder_id, tutor_id) REFERENCES folders (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT materials_deleted_at_root CHECK (deleted_at IS NULL OR folder_id IS NULL)
);

CREATE INDEX materials_folder_listing_idx ON materials (tutor_id, folder_id, normalised_title, id) WHERE deleted_at IS NULL;
CREATE INDEX materials_search_idx ON materials (tutor_id, normalised_title, id) WHERE deleted_at IS NULL;
CREATE INDEX materials_tutor_idx ON materials (tutor_id, id);
CREATE INDEX materials_folder_idx ON materials (folder_id);

-- +goose Down
ALTER TABLE materials ADD CONSTRAINT materials_empty_before_rollback CHECK (false);
ALTER TABLE materials DROP CONSTRAINT materials_empty_before_rollback;
ALTER TABLE folders ADD CONSTRAINT folders_empty_before_rollback CHECK (false);
ALTER TABLE folders DROP CONSTRAINT folders_empty_before_rollback;
ALTER TABLE attachment_references ADD CONSTRAINT attachment_references_no_material_kind CHECK (ref_kind <> 'material');
ALTER TABLE attachment_references DROP CONSTRAINT attachment_references_no_material_kind;
DROP TABLE materials;
DROP TABLE folders;
