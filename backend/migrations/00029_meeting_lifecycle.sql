-- +goose Up
CREATE TABLE lesson_participants (
    lesson_id uuid NOT NULL REFERENCES lessons (id) ON DELETE CASCADE,
    account_id uuid NOT NULL CHECK (account_id <> '00000000-0000-0000-0000-000000000000'),
    participant_id uuid NOT NULL UNIQUE CHECK (participant_id <> '00000000-0000-0000-0000-000000000000'),
    created_at timestamptz NOT NULL CHECK (isfinite(created_at)),
    PRIMARY KEY (lesson_id, account_id)
);

CREATE INDEX lesson_participants_account_idx ON lesson_participants (account_id);

ALTER TABLE meetings
    ADD COLUMN revocation_confirmed_at timestamptz CHECK (revocation_confirmed_at IS NULL OR isfinite(revocation_confirmed_at)),
    ADD COLUMN ended_hint_at timestamptz CHECK (ended_hint_at IS NULL OR isfinite(ended_hint_at)),
    ADD COLUMN ended_hint_until timestamptz CHECK (ended_hint_until IS NULL OR isfinite(ended_hint_until)),
    ADD COLUMN ended_hint_checks smallint NOT NULL DEFAULT 0 CHECK (ended_hint_checks BETWEEN 0 AND 3);

CREATE INDEX lessons_live_ends_idx ON lessons ((timezone('UTC', starts_at) + make_interval(mins => duration_minutes::int)), id)
WHERE status = 'live';

-- +goose Down
DROP INDEX lessons_live_ends_idx;
ALTER TABLE meetings
    DROP COLUMN ended_hint_checks,
    DROP COLUMN ended_hint_until,
    DROP COLUMN ended_hint_at,
    DROP COLUMN revocation_confirmed_at;
DROP TABLE lesson_participants;
