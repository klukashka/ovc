-- +goose Up
ALTER TABLE upload_intent_capacity DROP CONSTRAINT upload_intent_capacity_class_bound;
ALTER TABLE upload_intent_capacity ADD CONSTRAINT upload_intent_capacity_class_bound CHECK (
    (capacity_class = 'chat' AND used BETWEEN 0 AND 4096) OR (capacity_class = 'library' AND used BETWEEN 0 AND 1024)
        OR (capacity_class = 'homework' AND used BETWEEN 0 AND 1024)
);
INSERT INTO upload_intent_capacity (capacity_class, used) VALUES ('homework', 0);

ALTER TABLE upload_intents DROP CONSTRAINT upload_intents_capacity_class_check;
ALTER TABLE upload_intents ADD CONSTRAINT upload_intents_capacity_class_check CHECK (capacity_class IN ('chat', 'library', 'homework'));

CREATE TABLE daily_answer_upload_bytes (
    student_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tutor_id uuid NOT NULL,
    upload_day timestamptz NOT NULL CHECK (isfinite(upload_day) AND upload_day = date_trunc('day', upload_day AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),
    completed_bytes bigint NOT NULL CHECK (completed_bytes >= 0),
    PRIMARY KEY (student_id, tutor_id, upload_day)
);

CREATE INDEX daily_answer_upload_bytes_day ON daily_answer_upload_bytes (upload_day, student_id);
CREATE INDEX daily_answer_upload_bytes_tutor ON daily_answer_upload_bytes (tutor_id, student_id, upload_day);

CREATE INDEX attachments_answer_open_idx ON attachments (uploader_id, quota_tutor_id, created_at)
    WHERE context = 'answer' AND state IN ('uploading', 'pending');
CREATE INDEX attachments_pending_homework_idx ON attachments (id) WHERE state = 'pending' AND context IN ('answer', 'corrected');

-- +goose Down
DROP INDEX attachments_pending_homework_idx;
DROP INDEX attachments_answer_open_idx;
DROP TABLE daily_answer_upload_bytes;

ALTER TABLE attachments ADD CONSTRAINT attachments_no_homework_contexts CHECK (context NOT IN ('answer', 'corrected'));
ALTER TABLE attachments DROP CONSTRAINT attachments_no_homework_contexts;
ALTER TABLE upload_intents DROP CONSTRAINT upload_intents_capacity_class_check;
ALTER TABLE upload_intents ADD CONSTRAINT upload_intents_capacity_class_check CHECK (capacity_class IN ('chat', 'library'));

DELETE FROM upload_intent_capacity WHERE capacity_class = 'homework';
ALTER TABLE upload_intent_capacity DROP CONSTRAINT upload_intent_capacity_class_bound;
ALTER TABLE upload_intent_capacity ADD CONSTRAINT upload_intent_capacity_class_bound CHECK (
    (capacity_class = 'chat' AND used BETWEEN 0 AND 4096) OR (capacity_class = 'library' AND used BETWEEN 0 AND 1024)
);
