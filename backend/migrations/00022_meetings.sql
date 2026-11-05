-- +goose Up
CREATE TABLE meetings (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    lesson_id uuid NOT NULL REFERENCES lessons (id) ON DELETE CASCADE
        CHECK (lesson_id <> '00000000-0000-0000-0000-000000000000'),
    provider text NOT NULL CHECK (provider IN ('fake', 'bbb')),
    provider_meeting_id uuid NOT NULL UNIQUE CHECK (provider_meeting_id <> '00000000-0000-0000-0000-000000000000'),
    routing_key uuid NOT NULL UNIQUE CHECK (routing_key <> '00000000-0000-0000-0000-000000000000'),
    state text NOT NULL CHECK (state IN ('creating', 'open', 'closed')),
    create_time bigint CHECK (create_time IS NULL OR create_time > 0),
    created_at timestamptz NOT NULL CHECK (isfinite(created_at)),
    updated_at timestamptz NOT NULL CHECK (isfinite(updated_at)),
    first_join_at timestamptz CHECK (first_join_at IS NULL OR isfinite(first_join_at)),
    ended_at timestamptz CHECK (ended_at IS NULL OR isfinite(ended_at)),
    close_reason text CHECK (close_reason IN ('tutor_end', 'tutor_restart', 'status_job', 'provider_not_found', 'provider_error')),
    CONSTRAINT meetings_state_fields CHECK (
        (state = 'creating' AND create_time IS NULL AND first_join_at IS NULL AND ended_at IS NULL AND close_reason IS NULL)
        OR (state = 'open' AND create_time IS NOT NULL AND ended_at IS NULL AND close_reason IS NULL)
        OR (state = 'closed' AND ended_at IS NOT NULL AND close_reason IS NOT NULL)
    ),
    CONSTRAINT meetings_first_join_generation CHECK (first_join_at IS NULL OR create_time IS NOT NULL),
    CONSTRAINT meetings_time_order CHECK (
        updated_at >= created_at
        AND (first_join_at IS NULL OR (first_join_at >= created_at AND first_join_at <= updated_at))
        AND (ended_at IS NULL OR (ended_at >= created_at AND ended_at <= updated_at))
        AND (first_join_at IS NULL OR ended_at IS NULL OR first_join_at <= ended_at)
    )
);

CREATE UNIQUE INDEX meetings_current_lesson_idx ON meetings (lesson_id) WHERE state <> 'closed';
CREATE INDEX meetings_lesson_id_idx ON meetings (lesson_id);

-- +goose Down
DROP TABLE meetings;
