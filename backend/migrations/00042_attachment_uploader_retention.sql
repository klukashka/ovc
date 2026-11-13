-- +goose Up
ALTER TABLE attachments
    DROP CONSTRAINT attachments_uploader_id_fkey,
    ALTER COLUMN uploader_id DROP NOT NULL,
    ADD CONSTRAINT attachments_retained_uploader CHECK (uploader_id IS NOT NULL OR (context = 'answer' AND state IN ('bound', 'deleted'))),
    ADD CONSTRAINT attachments_uploader_id_fkey FOREIGN KEY (uploader_id) REFERENCES users (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE attachments ALTER COLUMN uploader_id SET NOT NULL;
ALTER TABLE attachments DROP CONSTRAINT attachments_retained_uploader;
ALTER TABLE attachments
    DROP CONSTRAINT attachments_uploader_id_fkey,
    ADD CONSTRAINT attachments_uploader_id_fkey FOREIGN KEY (uploader_id) REFERENCES users (id) ON DELETE RESTRICT;
