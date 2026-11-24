-- +goose Up
CREATE TABLE exports (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL UNIQUE REFERENCES users (id) ON DELETE RESTRICT,
    state text NOT NULL CHECK (state IN ('queued', 'building', 'ready', 'failed', 'expired')),
    language text NOT NULL CHECK (language IN ('ru', 'en')),
    timezone text NOT NULL CHECK (char_length(timezone) BETWEEN 1 AND 64),
    requested_at timestamptz NOT NULL,
    build_id uuid UNIQUE,
    storage_key text UNIQUE CHECK (storage_key LIKE 'exports/%'),
    build_started_at timestamptz,
    progress_done integer NOT NULL DEFAULT 0 CHECK (progress_done >= 0),
    progress_total integer NOT NULL DEFAULT 0 CHECK (progress_total >= 0),
    built_at timestamptz,
    size_bytes bigint CHECK (size_bytes >= 0),
    expires_at timestamptz,
    failure text CHECK (failure IN ('error', 'too_long', 'too_large', 'withdrawn')),
    updated_at timestamptz NOT NULL,
    CHECK (state <> 'queued' OR (build_id IS NULL AND storage_key IS NULL AND build_started_at IS NULL)),
    CHECK (state <> 'building' OR (build_id IS NOT NULL AND storage_key IS NOT NULL AND build_started_at IS NOT NULL)),
    CHECK (state <> 'ready' OR (build_id IS NOT NULL AND storage_key IS NOT NULL AND built_at IS NOT NULL
        AND size_bytes IS NOT NULL AND expires_at IS NOT NULL)),
    CHECK ((state = 'failed') = (failure IS NOT NULL)),
    CHECK (state <> 'expired' OR storage_key IS NULL),
    CHECK (storage_key IS NULL OR build_id IS NOT NULL)
);
CREATE INDEX exports_queued ON exports (requested_at, id) WHERE state = 'queued';
CREATE INDEX exports_building ON exports (build_started_at, id) WHERE state = 'building';
CREATE INDEX exports_ready_expiry ON exports (expires_at, id) WHERE state = 'ready';
CREATE INDEX exports_finished ON exports (coalesce(CASE WHEN state = 'expired' THEN expires_at END, built_at, requested_at), id)
    WHERE state IN ('failed', 'expired');

CREATE TABLE export_limits (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    export_id uuid NOT NULL,
    allowed_from timestamptz NOT NULL
);

ALTER TABLE upload_intents ADD COLUMN subject_kind text NOT NULL DEFAULT 'attachment'
    CHECK (subject_kind IN ('attachment', 'export_build'));
ALTER TABLE upload_intents DROP CONSTRAINT upload_intents_capacity_class_check;
ALTER TABLE upload_intents ADD CONSTRAINT upload_intents_capacity_class_check
    CHECK (capacity_class IN ('chat', 'library', 'homework', 'export'));
ALTER TABLE upload_intents ADD CONSTRAINT upload_intents_export_subject
    CHECK ((subject_kind = 'export_build') = (capacity_class = 'export'));
ALTER TABLE upload_intents ADD CONSTRAINT upload_intents_export_key
    CHECK (subject_kind <> 'export_build' OR storage_key LIKE 'exports/%');
CREATE INDEX upload_intents_subject ON upload_intents (subject_kind, attachment_id);

ALTER TABLE upload_cleanup_receipts ADD COLUMN subject_kind text NOT NULL DEFAULT 'attachment'
    CHECK (subject_kind IN ('attachment', 'export_build'));

ALTER TABLE upload_intent_capacity DROP CONSTRAINT upload_intent_capacity_class_bound;
ALTER TABLE upload_intent_capacity ADD CONSTRAINT upload_intent_capacity_class_bound CHECK (
    (capacity_class = 'chat' AND used BETWEEN 0 AND 4096) OR (capacity_class = 'library' AND used BETWEEN 0 AND 1024)
        OR (capacity_class = 'homework' AND used BETWEEN 0 AND 1024) OR (capacity_class = 'export' AND used BETWEEN 0 AND 256)
);
INSERT INTO upload_intent_capacity (capacity_class, used) VALUES ('export', 0);

ALTER TABLE notifications ALTER COLUMN tutor_id DROP NOT NULL;
ALTER TABLE notifications DROP CONSTRAINT notifications_event_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_check CHECK (
    event IN (
        'invitation.accepted', 'join_request.created', 'join_request.accepted', 'join_request.declined', 'relationship.ended',
        'lesson.reminder_1h', 'lesson.moved', 'lesson.cancelled', 'lesson.series_changed', 'group.archived',
        'issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned',
        'submission.review_updated', 'export.ready', 'export.failed'
    )
);
ALTER TABLE notifications ADD CONSTRAINT notifications_tutor_unless_export
    CHECK ((tutor_id IS NULL) = (event IN ('export.ready', 'export.failed')));

-- +goose Down
-- +goose StatementBegin
CREATE FUNCTION refuse_export_rollback() RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM upload_intents WHERE subject_kind = 'export_build')
        OR EXISTS (SELECT 1 FROM exports WHERE storage_key IS NOT NULL)
        OR EXISTS (SELECT 1 FROM outbox WHERE kind IN ('export_intent', 'export_object') AND status IN ('pending', 'in_progress')) THEN
        RAISE EXCEPTION 'export archives still need cleanup; the earlier code cannot delete them';
    END IF;
END;
$$;
-- +goose StatementEnd
SELECT refuse_export_rollback();
DROP FUNCTION refuse_export_rollback();

DELETE FROM notifications WHERE event IN ('export.ready', 'export.failed');
ALTER TABLE notifications DROP CONSTRAINT notifications_tutor_unless_export;
ALTER TABLE notifications DROP CONSTRAINT notifications_event_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_check CHECK (
    event IN (
        'invitation.accepted', 'join_request.created', 'join_request.accepted', 'join_request.declined', 'relationship.ended',
        'lesson.reminder_1h', 'lesson.moved', 'lesson.cancelled', 'lesson.series_changed', 'group.archived',
        'issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned',
        'submission.review_updated'
    )
);
ALTER TABLE notifications ALTER COLUMN tutor_id SET NOT NULL;

DELETE FROM upload_intent_capacity WHERE capacity_class = 'export';
ALTER TABLE upload_intent_capacity DROP CONSTRAINT upload_intent_capacity_class_bound;
ALTER TABLE upload_intent_capacity ADD CONSTRAINT upload_intent_capacity_class_bound CHECK (
    (capacity_class = 'chat' AND used BETWEEN 0 AND 4096) OR (capacity_class = 'library' AND used BETWEEN 0 AND 1024)
        OR (capacity_class = 'homework' AND used BETWEEN 0 AND 1024)
);

ALTER TABLE upload_cleanup_receipts DROP COLUMN subject_kind;
DROP INDEX upload_intents_subject;
ALTER TABLE upload_intents DROP CONSTRAINT upload_intents_export_key;
ALTER TABLE upload_intents DROP CONSTRAINT upload_intents_export_subject;
ALTER TABLE upload_intents DROP CONSTRAINT upload_intents_capacity_class_check;
ALTER TABLE upload_intents ADD CONSTRAINT upload_intents_capacity_class_check CHECK (capacity_class IN ('chat', 'library', 'homework'));
ALTER TABLE upload_intents DROP COLUMN subject_kind;

DROP TABLE export_limits;
DROP TABLE exports;
