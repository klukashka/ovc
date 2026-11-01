-- +goose Up
CREATE TABLE upload_intent_capacity (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    used integer NOT NULL CHECK (used BETWEEN 0 AND 4096),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO upload_intent_capacity (used) VALUES (0);

CREATE TABLE upload_intents (
    attachment_id uuid PRIMARY KEY,
    storage_key text UNIQUE NOT NULL,
    multipart_upload_id text,
    phase text NOT NULL CHECK (phase IN ('opening', 'streaming', 'completing', 'cleanup', 'uncertain')),
    completion_started boolean NOT NULL DEFAULT false,
    completion_confirmed boolean NOT NULL DEFAULT false,
    cleanup_requested boolean NOT NULL DEFAULT false,
    final_cleanup_required boolean NOT NULL DEFAULT false,
    cleanup_generation smallint CHECK (cleanup_generation IN (0, 1)),
    cleanup_outbox_id uuid,
    cleanup_result text NOT NULL DEFAULT 'none' CHECK (cleanup_result IN ('none', 'queued', 'deleted', 'failed')),
    error_code text CHECK (error_code IN ('permanent', 'lease_expired', 'effect_failed', 'outbox_missing')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK (NOT completion_confirmed OR completion_started),
    CHECK (NOT completion_started OR multipart_upload_id IS NOT NULL),
    CHECK ((cleanup_generation IS NULL) = (cleanup_outbox_id IS NULL)),
    CHECK ((cleanup_generation IS NULL) = (cleanup_result = 'none')),
    CHECK (cleanup_generation IS NULL OR cleanup_requested),
    CHECK (NOT final_cleanup_required OR (completion_confirmed AND cleanup_requested)),
    CHECK (phase NOT IN ('cleanup', 'uncertain') OR cleanup_requested),
    CHECK (phase <> 'uncertain' OR (completion_started AND NOT completion_confirmed)),
    CHECK (NOT cleanup_requested OR phase IN ('cleanup', 'uncertain')),
    CHECK (phase <> 'opening' OR (multipart_upload_id IS NULL AND NOT completion_started)),
    CHECK (phase <> 'streaming' OR (multipart_upload_id IS NOT NULL AND NOT completion_started)),
    CHECK (phase <> 'completing' OR completion_started)
);
CREATE INDEX upload_intents_outbox ON upload_intents (cleanup_outbox_id) WHERE cleanup_outbox_id IS NOT NULL;
CREATE INDEX upload_intents_recovery ON upload_intents (attachment_id, updated_at);

CREATE TABLE upload_cleanup_receipts (
    attachment_id uuid NOT NULL REFERENCES upload_intents (attachment_id) ON DELETE CASCADE,
    attempt_id uuid NOT NULL,
    outbox_id uuid UNIQUE NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'in_progress', 'done', 'dead')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (attachment_id, attempt_id)
);
CREATE UNIQUE INDEX upload_cleanup_receipts_active ON upload_cleanup_receipts (attachment_id) WHERE status IN ('pending', 'in_progress');

INSERT INTO upload_intents (
    attachment_id, storage_key, multipart_upload_id, phase, completion_started, cleanup_requested, created_at, updated_at
)
SELECT id, storage_key, multipart_upload_id,
    CASE WHEN multipart_upload_id IS NULL THEN 'cleanup' ELSE 'uncertain' END,
    multipart_upload_id IS NOT NULL, true, created_at, updated_at
FROM attachments WHERE context = 'chat' AND (state = 'uploading' OR (state = 'deleted' AND multipart_upload_id IS NOT NULL));
UPDATE upload_intent_capacity SET used = (SELECT count(*) FROM upload_intents);

-- +goose Down
DROP TABLE upload_cleanup_receipts;
DROP TABLE upload_intents;
DROP TABLE upload_intent_capacity;
