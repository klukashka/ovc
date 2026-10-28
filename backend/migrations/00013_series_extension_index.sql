-- +goose Up
DROP INDEX lesson_series_materialised_until_idx;
CREATE INDEX lesson_series_materialised_until_id_idx ON lesson_series (materialised_until, id) WHERE ended_at IS NULL;
CREATE INDEX lessons_scheduled_ends_idx ON lessons ((timezone('UTC', starts_at) + make_interval(mins => duration_minutes::int)), id)
WHERE status = 'scheduled';

-- +goose Down
DROP INDEX lessons_scheduled_ends_idx;
DROP INDEX lesson_series_materialised_until_id_idx;
CREATE INDEX lesson_series_materialised_until_idx ON lesson_series (materialised_until) WHERE ended_at IS NULL;
