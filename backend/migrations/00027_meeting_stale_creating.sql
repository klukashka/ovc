-- +goose Up
CREATE INDEX meetings_creating_idx ON meetings (created_at, id) WHERE state = 'creating';

-- +goose Down
DROP INDEX meetings_creating_idx;
