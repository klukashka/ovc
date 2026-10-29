-- +goose Up
ALTER TABLE attachments
    ALTER COLUMN media_type DROP NOT NULL,
    ALTER COLUMN size_bytes DROP NOT NULL,
    ALTER COLUMN sha256 DROP NOT NULL,
    DROP CONSTRAINT attachments_state_check,
    ADD CONSTRAINT attachments_state_check CHECK (state IN ('uploading', 'pending', 'bound', 'deleted')),
    ADD COLUMN reserved_bytes bigint CHECK (reserved_bytes > 0),
    ADD COLUMN multipart_upload_id text CHECK (char_length(multipart_upload_id) BETWEEN 1 AND 1024),
    ADD COLUMN width integer CHECK (width > 0),
    ADD COLUMN height integer CHECK (height > 0),
    ADD CONSTRAINT attachments_stored_rows_described CHECK (
        state IN ('uploading', 'deleted') OR (media_type IS NOT NULL AND size_bytes IS NOT NULL AND sha256 IS NOT NULL)
    ),
    ADD CONSTRAINT attachments_uploading_rows_reserved CHECK (state <> 'uploading' OR reserved_bytes IS NOT NULL),
    ADD CONSTRAINT attachments_dimensions_paired CHECK ((width IS NULL) = (height IS NULL));

CREATE INDEX attachments_uploading_created_at_idx ON attachments (created_at) WHERE state = 'uploading';
CREATE INDEX attachments_chat_daily_idx ON attachments (uploader_id, quota_tutor_id, created_at) WHERE context = 'chat';
CREATE INDEX attachments_chat_open_quota_idx ON attachments (quota_tutor_id) WHERE context = 'chat' AND state IN ('uploading', 'pending');

-- +goose Down
DROP INDEX attachments_chat_open_quota_idx;
DROP INDEX attachments_chat_daily_idx;
DROP INDEX attachments_uploading_created_at_idx;
DELETE FROM attachments WHERE state = 'uploading' OR media_type IS NULL OR size_bytes IS NULL OR sha256 IS NULL;
ALTER TABLE attachments
    DROP CONSTRAINT attachments_dimensions_paired,
    DROP CONSTRAINT attachments_uploading_rows_reserved,
    DROP CONSTRAINT attachments_stored_rows_described,
    DROP COLUMN height,
    DROP COLUMN width,
    DROP COLUMN multipart_upload_id,
    DROP COLUMN reserved_bytes,
    DROP CONSTRAINT attachments_state_check,
    ADD CONSTRAINT attachments_state_check CHECK (state IN ('pending', 'bound', 'deleted')),
    ALTER COLUMN sha256 SET NOT NULL,
    ALTER COLUMN size_bytes SET NOT NULL,
    ALTER COLUMN media_type SET NOT NULL;
