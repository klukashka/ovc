-- +goose Up
ALTER TABLE submissions
    ADD COLUMN feedback_version bigint CHECK (feedback_version >= 1),
    ADD COLUMN feedback_seen_version bigint,
    ADD CONSTRAINT submissions_feedback_seen_within_feedback CHECK (
        feedback_seen_version IS NULL OR (feedback_version IS NOT NULL AND feedback_seen_version BETWEEN 1 AND feedback_version)
    );
UPDATE submissions SET feedback_version = version, feedback_seen_version = version WHERE published_revision_id IS NOT NULL;

-- +goose Down
ALTER TABLE submissions DROP CONSTRAINT submissions_feedback_seen_within_feedback;
ALTER TABLE submissions DROP COLUMN feedback_seen_version, DROP COLUMN feedback_version;
