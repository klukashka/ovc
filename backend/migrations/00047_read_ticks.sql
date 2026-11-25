-- +goose Up
ALTER TABLE channel_members
    ADD COLUMN shared_read_seq bigint NOT NULL DEFAULT 0,
    ADD CONSTRAINT channel_members_shared_read_within_read CHECK (shared_read_seq <= last_read_seq);

-- +goose Down
ALTER TABLE channel_members DROP CONSTRAINT channel_members_shared_read_within_read, DROP COLUMN shared_read_seq;
