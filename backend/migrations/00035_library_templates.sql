-- +goose Up
CREATE TABLE templates (
    id uuid PRIMARY KEY,
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE RESTRICT,
    folder_id uuid,
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    normalised_title text COLLATE "C" NOT NULL CHECK (octet_length(normalised_title) BETWEEN 1 AND 800),
    blocks jsonb NOT NULL CHECK (jsonb_typeof(blocks) = 'array'),
    schema_version integer NOT NULL CHECK (schema_version = 1),
    version bigint NOT NULL CHECK (version >= 1),
    last_save_id uuid,
    create_key text CHECK (create_key ~ '^[A-Za-z0-9._-]{1,64}$'),
    question_count integer NOT NULL CHECK (question_count BETWEEN 0 AND 100),
    max_points integer NOT NULL CHECK (max_points BETWEEN 0 AND 10000),
    ready boolean NOT NULL,
    blocks_bytes bigint NOT NULL CHECK (blocks_bytes >= 0),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT templates_id_tutor_key UNIQUE (id, tutor_id),
    CONSTRAINT templates_folder_fkey FOREIGN KEY (folder_id, tutor_id) REFERENCES folders (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT templates_deleted_at_root CHECK (deleted_at IS NULL OR (folder_id IS NULL AND blocks_bytes = 0 AND blocks = '[]'::jsonb))
);

CREATE UNIQUE INDEX templates_create_key_idx ON templates (tutor_id, create_key) WHERE create_key IS NOT NULL;
CREATE INDEX templates_folder_listing_idx ON templates (tutor_id, folder_id, normalised_title, id) WHERE deleted_at IS NULL;
CREATE INDEX templates_search_idx ON templates (tutor_id, normalised_title, id) WHERE deleted_at IS NULL;
CREATE INDEX templates_tutor_idx ON templates (tutor_id, id);
CREATE INDEX templates_folder_idx ON templates (folder_id);
CREATE INDEX templates_budget_idx ON templates (tutor_id) INCLUDE (blocks_bytes) WHERE deleted_at IS NULL;

-- +goose Down
ALTER TABLE templates ADD CONSTRAINT templates_empty_before_rollback CHECK (false);
ALTER TABLE templates DROP CONSTRAINT templates_empty_before_rollback;
ALTER TABLE attachment_references ADD CONSTRAINT attachment_references_no_template_kind CHECK (ref_kind <> 'template');
ALTER TABLE attachment_references DROP CONSTRAINT attachment_references_no_template_kind;
DROP TABLE templates;
