-- +goose Up
CREATE INDEX meeting_recovery_unsettled_meeting_idx ON meeting_recovery (meeting_id) WHERE settled_at IS NULL;

-- +goose Down
DROP INDEX meeting_recovery_unsettled_meeting_idx;
