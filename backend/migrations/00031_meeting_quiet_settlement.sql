-- +goose Up
ALTER TABLE meeting_recovery
    ADD COLUMN dispatched_at timestamptz CHECK (isfinite(dispatched_at)),
    ADD COLUMN settlement text CHECK (settlement IN ('revoked', 'confirmed_end', 'quiet_absence')),
    ADD COLUMN reconcile_cycle_used boolean NOT NULL DEFAULT false;
UPDATE meeting_recovery SET dispatched_at = updated_at WHERE create_state <> 'prepared';
UPDATE meeting_recovery SET settlement = CASE create_state WHEN 'prepared' THEN 'revoked' ELSE 'confirmed_end' END
WHERE settled_at IS NOT NULL;
ALTER TABLE meeting_recovery
    DROP CONSTRAINT meeting_recovery_settlement,
    ADD CONSTRAINT meeting_recovery_dispatch CHECK (
        (create_state = 'prepared') = (dispatched_at IS NULL) AND (dispatched_at IS NULL OR dispatched_at >= created_at)
    ),
    ADD CONSTRAINT meeting_recovery_settlement CHECK (
        (settled_at IS NULL) = (settlement IS NULL)
        AND (settled_at IS NULL OR (settled_at >= created_at AND cleanup_requested AND (
            (settlement = 'revoked' AND create_state = 'prepared')
            OR (settlement = 'confirmed_end' AND create_state = 'returned')
            OR (settlement = 'quiet_absence' AND create_state IN ('started', 'uncertain') AND create_time IS NULL
                AND settled_at >= dispatched_at + interval '1 hour')
        )))
    );

-- +goose Down
LOCK TABLE meeting_recovery IN ACCESS EXCLUSIVE MODE;
CREATE TEMPORARY TABLE meeting_quiet_settlement_downgrade_guard (empty boolean NOT NULL CHECK (empty)) ON COMMIT DROP;
INSERT INTO meeting_quiet_settlement_downgrade_guard SELECT NOT EXISTS (SELECT 1 FROM meeting_recovery);
ALTER TABLE meeting_recovery
    DROP CONSTRAINT meeting_recovery_settlement,
    DROP CONSTRAINT meeting_recovery_dispatch,
    DROP COLUMN reconcile_cycle_used,
    DROP COLUMN settlement,
    DROP COLUMN dispatched_at,
    ADD CONSTRAINT meeting_recovery_settlement CHECK (
        settled_at IS NULL OR (settled_at >= created_at AND cleanup_requested AND create_state IN ('prepared', 'returned'))
    );
