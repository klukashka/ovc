-- +goose Up
ALTER TABLE fake_meeting_rooms
    ADD COLUMN presentation_size bigint CHECK (presentation_size >= 0),
    ADD COLUMN presentation_sha256 bytea CHECK (octet_length(presentation_sha256) = 32),
    ADD CONSTRAINT fake_meeting_rooms_presentation_pair CHECK ((presentation_size IS NULL) = (presentation_sha256 IS NULL));

-- +goose Down
ALTER TABLE fake_meeting_rooms DROP COLUMN presentation_sha256, DROP COLUMN presentation_size;
