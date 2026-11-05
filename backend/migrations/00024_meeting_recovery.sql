-- +goose Up
CREATE TABLE meeting_recovery (
    provider_meeting_id uuid PRIMARY KEY CHECK (provider_meeting_id <> '00000000-0000-0000-0000-000000000000'),
    provider text NOT NULL CHECK (provider IN ('fake', 'bbb')),
    meeting_id uuid REFERENCES meetings (id) ON DELETE SET NULL,
    origin text NOT NULL CHECK (origin IN ('attempt', 'legacy')),
    create_state text NOT NULL CHECK (create_state IN ('prepared', 'started', 'returned', 'uncertain')),
    attempt_id uuid CHECK (attempt_id <> '00000000-0000-0000-0000-000000000000'),
    create_time bigint CHECK (create_time > 0),
    cleanup_requested boolean NOT NULL DEFAULT false,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL CHECK (isfinite(created_at)),
    updated_at timestamptz NOT NULL CHECK (isfinite(updated_at) AND updated_at >= created_at),
    CONSTRAINT meeting_recovery_evidence CHECK (
        (origin = 'attempt' AND (
            (create_state = 'prepared' AND attempt_id IS NULL AND create_time IS NULL)
            OR (create_state IN ('started', 'uncertain') AND attempt_id IS NOT NULL AND create_time IS NULL)
            OR (create_state = 'returned' AND attempt_id IS NOT NULL AND create_time IS NOT NULL)
        )) OR (origin = 'legacy' AND attempt_id IS NULL AND (
            (create_state = 'uncertain' AND create_time IS NULL)
            OR (create_state = 'returned' AND create_time IS NOT NULL)
        ))
    )
);
CREATE UNIQUE INDEX meeting_recovery_meeting_id_idx ON meeting_recovery (meeting_id);
CREATE INDEX meeting_recovery_cleanup_idx ON meeting_recovery (provider_meeting_id)
    WHERE cleanup_requested OR meeting_id IS NULL;
INSERT INTO meeting_recovery (provider_meeting_id, provider, meeting_id, origin, create_state, create_time, cleanup_requested, created_at, updated_at)
SELECT provider_meeting_id, provider, id, 'legacy', CASE WHEN create_time IS NULL THEN 'uncertain' ELSE 'returned' END,
    create_time, state = 'closed', created_at, updated_at FROM meetings;

-- +goose Down
LOCK TABLE meeting_recovery IN ACCESS EXCLUSIVE MODE;
CREATE TEMPORARY TABLE meeting_recovery_downgrade_guard (empty boolean NOT NULL CHECK (empty)) ON COMMIT DROP;
INSERT INTO meeting_recovery_downgrade_guard SELECT NOT EXISTS (SELECT 1 FROM meeting_recovery);
DROP TABLE meeting_recovery;
