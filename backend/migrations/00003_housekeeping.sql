-- +goose Up
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
CREATE INDEX sessions_revoked_at_idx ON sessions (revoked_at) WHERE revoked_at IS NOT NULL;
CREATE INDEX email_requests_created_at_idx ON email_requests (created_at);

-- +goose Down
DROP INDEX email_requests_created_at_idx;
DROP INDEX sessions_revoked_at_idx;
DROP INDEX sessions_expires_at_idx;
