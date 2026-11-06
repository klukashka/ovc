-- +goose Up
CREATE TABLE meeting_recovery_capacity (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    unsettled bigint NOT NULL CHECK (unsettled >= 0)
);
INSERT INTO meeting_recovery_capacity (unsettled) SELECT count(*) FROM meeting_recovery;
ALTER TABLE meeting_recovery
    ADD COLUMN settled_at timestamptz CHECK (isfinite(settled_at)),
    ADD COLUMN cycle_number bigint NOT NULL DEFAULT 0 CHECK (cycle_number >= 0),
    ADD COLUMN cycle_outbox_id uuid,
    ADD COLUMN cycle_result text NOT NULL DEFAULT 'none' CHECK (cycle_result IN ('none', 'pending', 'settled', 'unresolved', 'conflict', 'exhausted')),
    ADD COLUMN final_cycle_requested boolean NOT NULL DEFAULT false,
    ADD COLUMN final_cycle_used boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT meeting_recovery_cycle_identity CHECK (
        (cycle_number = 0 AND cycle_outbox_id IS NULL AND cycle_result = 'none')
        OR (cycle_number > 0 AND cycle_outbox_id IS NOT NULL AND cycle_result <> 'none')
    ),
    ADD CONSTRAINT meeting_recovery_settlement CHECK (
        settled_at IS NULL OR (settled_at >= created_at AND cleanup_requested AND create_state IN ('prepared', 'returned'))
    );
DROP INDEX meeting_recovery_cleanup_idx;
CREATE INDEX meeting_recovery_cleanup_idx ON meeting_recovery (provider_meeting_id)
    WHERE settled_at IS NULL AND (cleanup_requested OR meeting_id IS NULL);
CREATE INDEX meeting_recovery_receipt_idx ON meeting_recovery (settled_at, provider_meeting_id) WHERE settled_at IS NOT NULL;
CREATE TABLE meeting_recovery_cycles (
    outbox_id uuid PRIMARY KEY CHECK (outbox_id <> '00000000-0000-0000-0000-000000000000'),
    provider_meeting_id uuid NOT NULL REFERENCES meeting_recovery (provider_meeting_id) ON DELETE CASCADE,
    cycle_number bigint NOT NULL CHECK (cycle_number > 0),
    mode text NOT NULL CHECK (mode IN ('automatic', 'operator')),
    result text NOT NULL CHECK (result IN ('pending', 'settled', 'unresolved', 'conflict', 'exhausted')),
    created_at timestamptz NOT NULL CHECK (isfinite(created_at)),
    completed_at timestamptz CHECK (isfinite(completed_at) AND completed_at >= created_at),
    UNIQUE (provider_meeting_id, cycle_number),
    CHECK ((result = 'pending') = (completed_at IS NULL))
);
CREATE TABLE meeting_recovery_cursor (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    after_provider_meeting_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0)
);
INSERT INTO meeting_recovery_cursor DEFAULT VALUES;

-- +goose Down
LOCK TABLE meeting_recovery IN ACCESS EXCLUSIVE MODE;
CREATE TEMPORARY TABLE meeting_recovery_cycles_downgrade_guard (empty boolean NOT NULL CHECK (empty)) ON COMMIT DROP;
INSERT INTO meeting_recovery_cycles_downgrade_guard SELECT NOT EXISTS (SELECT 1 FROM meeting_recovery);
DROP TABLE meeting_recovery_cursor;
DROP TABLE meeting_recovery_cycles;
DROP INDEX meeting_recovery_receipt_idx;
DROP INDEX meeting_recovery_cleanup_idx;
ALTER TABLE meeting_recovery
    DROP CONSTRAINT meeting_recovery_settlement,
    DROP CONSTRAINT meeting_recovery_cycle_identity,
    DROP COLUMN settled_at,
    DROP COLUMN cycle_number,
    DROP COLUMN cycle_outbox_id,
    DROP COLUMN cycle_result,
    DROP COLUMN final_cycle_requested,
    DROP COLUMN final_cycle_used;
CREATE INDEX meeting_recovery_cleanup_idx ON meeting_recovery (provider_meeting_id) WHERE cleanup_requested OR meeting_id IS NULL;
DROP TABLE meeting_recovery_capacity;
