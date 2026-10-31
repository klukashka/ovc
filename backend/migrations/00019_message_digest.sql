-- +goose Up
CREATE SEQUENCE digest_pending_revision_seq AS bigint;
ALTER TABLE digest_pending ADD COLUMN revision bigint NOT NULL DEFAULT nextval('digest_pending_revision_seq');
ALTER SEQUENCE digest_pending_revision_seq OWNED BY digest_pending.revision;
CREATE TABLE digest_state (
    user_id uuid PRIMARY KEY,
    last_digest_at timestamptz
);

-- +goose Down
DROP TABLE digest_state;
ALTER TABLE digest_pending DROP COLUMN revision;
