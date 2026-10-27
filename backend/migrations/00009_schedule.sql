-- +goose Up
CREATE TABLE lesson_series (
    id uuid PRIMARY KEY,
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE RESTRICT,
    relationship_id uuid,
    group_id uuid,
    weekdays smallint[] NOT NULL CHECK (
        cardinality(weekdays) BETWEEN 1 AND 7 AND weekdays <@ ARRAY[1, 2, 3, 4, 5, 6, 7]::smallint[]
    ),
    local_start time NOT NULL,
    duration_minutes smallint NOT NULL CHECK (duration_minutes BETWEEN 15 AND 240 AND duration_minutes % 5 = 0),
    timezone text NOT NULL CHECK (char_length(timezone) BETWEEN 1 AND 64),
    first_date date NOT NULL,
    end_date date,
    count smallint CHECK (count BETWEEN 1 AND 100),
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    description text CHECK (char_length(description) <= 2000),
    materialised_until date NOT NULL,
    split_from_series_id uuid REFERENCES lesson_series (id) ON DELETE SET NULL,
    create_key text CHECK (char_length(create_key) BETWEEN 1 AND 200),
    ended_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT lesson_series_audience_xor CHECK (num_nonnulls(relationship_id, group_id) = 1),
    CONSTRAINT lesson_series_end_xor CHECK (num_nonnulls(end_date, count) = 1),
    CONSTRAINT lesson_series_end_after_first CHECK (end_date IS NULL OR end_date >= first_date),
    CONSTRAINT lesson_series_relationship_fkey FOREIGN KEY (relationship_id, tutor_id)
        REFERENCES relationships (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT lesson_series_group_fkey FOREIGN KEY (group_id, tutor_id) REFERENCES groups (id, tutor_id) ON DELETE RESTRICT
);

CREATE INDEX lesson_series_tutor_created_idx ON lesson_series (tutor_id, created_at DESC, id);
CREATE INDEX lesson_series_relationship_id_idx ON lesson_series (relationship_id);
CREATE INDEX lesson_series_group_id_idx ON lesson_series (group_id);
CREATE INDEX lesson_series_split_from_idx ON lesson_series (split_from_series_id);
CREATE INDEX lesson_series_materialised_until_idx ON lesson_series (materialised_until) WHERE ended_at IS NULL;
CREATE UNIQUE INDEX lesson_series_tutor_create_key_idx ON lesson_series (tutor_id, create_key) WHERE create_key IS NOT NULL;

CREATE TABLE lessons (
    id uuid PRIMARY KEY,
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE RESTRICT,
    series_id uuid REFERENCES lesson_series (id) ON DELETE RESTRICT,
    occurrence_date date,
    relationship_id uuid,
    group_id uuid,
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    description text CHECK (char_length(description) <= 2000),
    starts_at timestamptz NOT NULL,
    duration_minutes smallint NOT NULL CHECK (duration_minutes BETWEEN 15 AND 240 AND duration_minutes % 5 = 0),
    status text NOT NULL CHECK (status IN ('scheduled', 'live', 'ended', 'cancelled')),
    timezone text NOT NULL CHECK (char_length(timezone) BETWEEN 1 AND 64),
    schedule_revision integer NOT NULL DEFAULT 1 CHECK (schedule_revision >= 1),
    version integer NOT NULL DEFAULT 1 CHECK (version >= 1),
    reminded_24h_revision integer,
    reminded_1h_revision integer,
    overridden_fields text[] NOT NULL DEFAULT '{}' CHECK (
        overridden_fields <@ ARRAY['start', 'date', 'duration', 'title', 'description', 'materials']::text[]
    ),
    cancelled_at timestamptz,
    cancel_cause text CHECK (
        cancel_cause IN ('tutor', 'relationship_ended', 'provisional_removed', 'group_archived', 'tutor_deleted')
    ),
    create_key text CHECK (char_length(create_key) BETWEEN 1 AND 200),
    live_at timestamptz,
    ended_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT lessons_audience_xor CHECK (num_nonnulls(relationship_id, group_id) = 1),
    CONSTRAINT lessons_occurrence_with_series CHECK ((series_id IS NULL) = (occurrence_date IS NULL)),
    CONSTRAINT lessons_cancellation_when_cancelled CHECK (
        (status = 'cancelled') = (cancelled_at IS NOT NULL) AND (cancelled_at IS NULL) = (cancel_cause IS NULL)
    ),
    CONSTRAINT lessons_relationship_fkey FOREIGN KEY (relationship_id, tutor_id) REFERENCES relationships (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT lessons_group_fkey FOREIGN KEY (group_id, tutor_id) REFERENCES groups (id, tutor_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX lessons_series_occurrence_idx ON lessons (series_id, occurrence_date);
CREATE INDEX lessons_tutor_starts_idx ON lessons (tutor_id, starts_at, id);
CREATE INDEX lessons_relationship_starts_idx ON lessons (relationship_id, starts_at);
CREATE INDEX lessons_group_starts_idx ON lessons (group_id, starts_at);
CREATE INDEX lessons_group_scheduled_idx ON lessons (group_id, starts_at) WHERE status = 'scheduled';
CREATE INDEX lessons_scheduled_starts_idx ON lessons (starts_at) WHERE status = 'scheduled';
CREATE INDEX lessons_live_starts_idx ON lessons (starts_at) WHERE status = 'live';
CREATE INDEX lessons_cancellations_idx ON lessons (cancel_cause, cancelled_at) WHERE cancelled_at IS NOT NULL;
CREATE UNIQUE INDEX lessons_tutor_create_key_idx ON lessons (tutor_id, create_key) WHERE create_key IS NOT NULL;

CREATE TABLE lesson_roster (
    lesson_id uuid NOT NULL REFERENCES lessons (id) ON DELETE CASCADE,
    student_id uuid NOT NULL,
    lesson_starts_at timestamptz NOT NULL,
    added_at timestamptz NOT NULL,
    PRIMARY KEY (lesson_id, student_id)
);

CREATE INDEX lesson_roster_student_idx ON lesson_roster (student_id, lesson_starts_at);

-- +goose Down
DROP TABLE lesson_roster;
DROP TABLE lessons;
DROP TABLE lesson_series;
