-- +goose Up
CREATE INDEX attachments_open_quota_idx ON attachments (quota_tutor_id) WHERE state IN ('uploading', 'pending');
DROP INDEX attachments_chat_open_quota_idx;

-- +goose Down
CREATE INDEX attachments_chat_open_quota_idx ON attachments (quota_tutor_id) WHERE context = 'chat' AND state IN ('uploading', 'pending');
DROP INDEX attachments_open_quota_idx;
