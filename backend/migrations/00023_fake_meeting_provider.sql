-- +goose Up
CREATE TABLE fake_meeting_state (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    namespace bytea NOT NULL CHECK (octet_length(namespace) = 32),
    max_rooms bigint NOT NULL CHECK (max_rooms > 0),
    max_links bigint NOT NULL CHECK (max_links > 0),
    max_participants bigint NOT NULL CHECK (max_participants > 0),
    max_generations bigint NOT NULL CHECK (max_generations > 0),
    generations bigint NOT NULL DEFAULT 0 CHECK (generations >= 0 AND generations <= max_generations),
    last_create_time bigint NOT NULL DEFAULT 0 CHECK (last_create_time >= 0),
    link_sequence bigint NOT NULL DEFAULT 0 CHECK (link_sequence >= 0),
    unavailable boolean NOT NULL DEFAULT false
);

CREATE TABLE fake_meeting_rooms (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    create_time bigint NOT NULL CHECK (create_time > 0),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    UNIQUE (id, create_time)
);

CREATE TABLE fake_meeting_participants (
    room_id uuid NOT NULL,
    create_time bigint NOT NULL,
    participant_id uuid NOT NULL CHECK (participant_id <> '00000000-0000-0000-0000-000000000000'),
    role text NOT NULL CHECK (role IN ('moderator', 'viewer')),
    present boolean NOT NULL DEFAULT false,
    PRIMARY KEY (room_id, participant_id),
    UNIQUE (room_id, create_time, participant_id, role),
    FOREIGN KEY (room_id, create_time) REFERENCES fake_meeting_rooms (id, create_time) ON DELETE CASCADE
);

CREATE TABLE fake_meeting_links (
    digest bytea PRIMARY KEY CHECK (octet_length(digest) = 32),
    room_id uuid NOT NULL,
    create_time bigint NOT NULL,
    participant_id uuid NOT NULL,
    role text NOT NULL,
    FOREIGN KEY (room_id, create_time, participant_id, role)
        REFERENCES fake_meeting_participants (room_id, create_time, participant_id, role) ON DELETE CASCADE
);
CREATE INDEX fake_meeting_links_room_idx ON fake_meeting_links (room_id);

-- +goose Down
DROP TABLE fake_meeting_links;
DROP TABLE fake_meeting_participants;
DROP TABLE fake_meeting_rooms;
DROP TABLE fake_meeting_state;
