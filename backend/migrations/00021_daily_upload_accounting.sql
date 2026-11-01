-- +goose Up
CREATE TABLE chat_upload_accounting_activation (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    active_at timestamptz NOT NULL CHECK (isfinite(active_at) AND active_at = date_trunc('day', active_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC')
);

-- +goose StatementBegin
CREATE FUNCTION preserve_chat_upload_accounting_activation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'chat upload accounting activation is immutable' USING ERRCODE = '23514';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER chat_upload_accounting_activation_immutable
BEFORE UPDATE OR DELETE ON chat_upload_accounting_activation
FOR EACH ROW EXECUTE FUNCTION preserve_chat_upload_accounting_activation();

CREATE TRIGGER chat_upload_accounting_activation_no_truncate
BEFORE TRUNCATE ON chat_upload_accounting_activation
FOR EACH STATEMENT EXECUTE FUNCTION preserve_chat_upload_accounting_activation();

CREATE TABLE daily_chat_upload_bytes (
    student_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tutor_id uuid NOT NULL,
    upload_day timestamptz NOT NULL CHECK (isfinite(upload_day) AND upload_day = date_trunc('day', upload_day AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),
    completed_bytes bigint NOT NULL CHECK (completed_bytes >= 0),
    PRIMARY KEY (student_id, tutor_id, upload_day)
);

CREATE INDEX daily_chat_upload_bytes_day ON daily_chat_upload_bytes (upload_day, student_id);
CREATE INDEX daily_chat_upload_bytes_tutor ON daily_chat_upload_bytes (tutor_id, student_id, upload_day);

INSERT INTO daily_chat_upload_bytes (student_id, tutor_id, upload_day, completed_bytes)
SELECT attachments.uploader_id, attachments.quota_tutor_id,
    date_trunc('day', attachments.created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC', sum(attachments.size_bytes)::bigint
FROM attachments
JOIN users ON users.id = attachments.uploader_id AND users.role = 'student'
WHERE attachments.context = 'chat' AND attachments.size_bytes IS NOT NULL AND attachments.quota_tutor_id IS NOT NULL
GROUP BY attachments.uploader_id, attachments.quota_tutor_id, date_trunc('day', attachments.created_at AT TIME ZONE 'UTC');

INSERT INTO chat_upload_accounting_activation (active_at)
VALUES ((date_trunc('day', clock_timestamp() AT TIME ZONE 'UTC') + interval '1 day') AT TIME ZONE 'UTC');

-- +goose Down
DROP TABLE daily_chat_upload_bytes;
DROP TABLE chat_upload_accounting_activation;
DROP FUNCTION preserve_chat_upload_accounting_activation();
