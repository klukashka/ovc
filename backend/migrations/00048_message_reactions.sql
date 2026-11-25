-- +goose Up
CREATE TABLE message_reactions (
    message_id uuid NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    user_id uuid NOT NULL,
    reaction text NOT NULL CHECK (reaction IN ('thumbs_up', 'heart', 'joy', 'wow', 'sad', 'check')),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (message_id, user_id)
);

CREATE INDEX message_reactions_user_idx ON message_reactions (user_id);

-- +goose Down
DROP TABLE message_reactions;
