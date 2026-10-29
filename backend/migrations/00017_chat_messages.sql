-- +goose Up
CREATE TABLE messages (
    id uuid PRIMARY KEY,
    channel_id uuid NOT NULL REFERENCES channels (id) ON DELETE RESTRICT,
    seq bigint NOT NULL CHECK (seq > 0),
    sender_id uuid,
    client_key text CHECK (client_key ~ '^[A-Za-z0-9._-]{1,64}$'),
    body text CHECK (char_length(body) <= 4000),
    version integer NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL,
    edited_at timestamptz,
    deleted_at timestamptz,
    erased_at timestamptz,
    updated_at timestamptz NOT NULL,
    CONSTRAINT messages_channel_seq_key UNIQUE (channel_id, seq),
    CONSTRAINT messages_body_kept_until_removed CHECK ((body IS NULL) = (deleted_at IS NOT NULL OR erased_at IS NOT NULL))
);

CREATE UNIQUE INDEX messages_client_key_idx ON messages (sender_id, channel_id, client_key)
    WHERE sender_id IS NOT NULL AND client_key IS NOT NULL;
CREATE INDEX messages_sender_id_idx ON messages (sender_id, id) WHERE sender_id IS NOT NULL;

CREATE TABLE quota_charges (
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE RESTRICT,
    attachment_id uuid NOT NULL REFERENCES attachments (id) ON DELETE RESTRICT,
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (tutor_id, attachment_id)
);

CREATE INDEX quota_charges_attachment_id_idx ON quota_charges (attachment_id);

CREATE TABLE digest_pending (
    user_id uuid PRIMARY KEY,
    pending_since timestamptz NOT NULL
);

CREATE INDEX digest_pending_since_idx ON digest_pending (pending_since);

-- +goose Down
DROP TABLE digest_pending;
DROP TABLE quota_charges;
DROP TABLE messages;
