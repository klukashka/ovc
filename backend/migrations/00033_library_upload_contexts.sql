-- +goose Up
ALTER TABLE upload_intent_capacity ADD COLUMN capacity_class text NOT NULL DEFAULT 'chat';
ALTER TABLE upload_intent_capacity DROP COLUMN singleton;
ALTER TABLE upload_intent_capacity ALTER COLUMN capacity_class DROP DEFAULT;
ALTER TABLE upload_intent_capacity ADD PRIMARY KEY (capacity_class);
ALTER TABLE upload_intent_capacity DROP CONSTRAINT upload_intent_capacity_used_check;
ALTER TABLE upload_intent_capacity ADD CONSTRAINT upload_intent_capacity_class_bound CHECK (
    (capacity_class = 'chat' AND used BETWEEN 0 AND 4096) OR (capacity_class = 'library' AND used BETWEEN 0 AND 1024)
);
INSERT INTO upload_intent_capacity (capacity_class, used) VALUES ('library', 0);

ALTER TABLE upload_intents ADD COLUMN capacity_class text NOT NULL DEFAULT 'chat' CHECK (capacity_class IN ('chat', 'library'));
ALTER TABLE upload_intents ALTER COLUMN capacity_class DROP DEFAULT;

CREATE INDEX attachments_pending_library_idx ON attachments (uploader_id) WHERE state = 'pending' AND context IN ('library', 'prompt_image');

-- +goose Down
DROP INDEX attachments_pending_library_idx;

ALTER TABLE attachments ADD CONSTRAINT attachments_no_library_contexts CHECK (context NOT IN ('library', 'prompt_image'));
ALTER TABLE attachments DROP CONSTRAINT attachments_no_library_contexts;
ALTER TABLE upload_intents ADD CONSTRAINT upload_intents_chat_class_only CHECK (capacity_class = 'chat');
ALTER TABLE upload_intents DROP CONSTRAINT upload_intents_chat_class_only;
ALTER TABLE upload_intents DROP COLUMN capacity_class;

DELETE FROM upload_intent_capacity WHERE capacity_class = 'library';
ALTER TABLE upload_intent_capacity DROP CONSTRAINT upload_intent_capacity_class_bound;
ALTER TABLE upload_intent_capacity DROP CONSTRAINT upload_intent_capacity_pkey;
ALTER TABLE upload_intent_capacity ADD COLUMN singleton boolean NOT NULL DEFAULT true CHECK (singleton);
ALTER TABLE upload_intent_capacity ADD PRIMARY KEY (singleton);
ALTER TABLE upload_intent_capacity ADD CONSTRAINT upload_intent_capacity_used_check CHECK (used BETWEEN 0 AND 4096);
ALTER TABLE upload_intent_capacity DROP COLUMN capacity_class;
