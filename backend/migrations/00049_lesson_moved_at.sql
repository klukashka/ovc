-- +goose Up
ALTER TABLE lessons ADD COLUMN moved_at timestamptz NULL;

-- +goose Down
ALTER TABLE lessons DROP COLUMN moved_at;
