-- +goose Up
CREATE INDEX lessons_tutor_scheduled_idx ON lessons (tutor_id, starts_at) WHERE status = 'scheduled';
CREATE INDEX lesson_series_tutor_open_idx ON lesson_series (tutor_id, id) WHERE ended_at IS NULL;

-- +goose Down
DROP INDEX lesson_series_tutor_open_idx;
DROP INDEX lessons_tutor_scheduled_idx;
