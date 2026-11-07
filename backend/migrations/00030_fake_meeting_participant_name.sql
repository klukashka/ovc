-- +goose Up
ALTER TABLE fake_meeting_participants
    ADD COLUMN display_name text NOT NULL DEFAULT '' CHECK (char_length(display_name) <= 100);
ALTER TABLE fake_meeting_participants ALTER COLUMN display_name DROP DEFAULT;

-- +goose Down
ALTER TABLE fake_meeting_participants DROP COLUMN display_name;
