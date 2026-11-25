-- +goose Up
ALTER TABLE messages ADD CONSTRAINT messages_channel_id_id_key UNIQUE (channel_id, id);

ALTER TABLE messages
    ADD COLUMN reply_to_message_id uuid,
    ADD CONSTRAINT messages_reply_to_fkey FOREIGN KEY (channel_id, reply_to_message_id)
        REFERENCES messages (channel_id, id) ON DELETE SET NULL (reply_to_message_id),
    ADD CONSTRAINT messages_reply_to_other_message CHECK (reply_to_message_id IS DISTINCT FROM id);
CREATE INDEX messages_reply_to_idx ON messages (reply_to_message_id) WHERE reply_to_message_id IS NOT NULL;

ALTER TABLE channels
    ADD COLUMN pinned_message_id uuid,
    ADD CONSTRAINT channels_pinned_message_fkey FOREIGN KEY (id, pinned_message_id) REFERENCES messages (channel_id, id);
CREATE INDEX channels_pinned_message_idx ON channels (pinned_message_id) WHERE pinned_message_id IS NOT NULL;

-- +goose Down
DROP INDEX channels_pinned_message_idx;
ALTER TABLE channels DROP CONSTRAINT channels_pinned_message_fkey, DROP COLUMN pinned_message_id;

DROP INDEX messages_reply_to_idx;
ALTER TABLE messages
    DROP CONSTRAINT messages_reply_to_other_message,
    DROP CONSTRAINT messages_reply_to_fkey,
    DROP COLUMN reply_to_message_id;

ALTER TABLE messages DROP CONSTRAINT messages_channel_id_id_key;
