-- +goose Up
CREATE INDEX outbox_meeting_cleanup_expired_idx ON outbox (lease_expires_at, id)
    WHERE kind = 'meeting_cleanup' AND status = 'in_progress';

-- +goose Down
DROP INDEX outbox_meeting_cleanup_expired_idx;
