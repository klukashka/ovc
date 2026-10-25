-- +goose Up
CREATE TABLE email_preferences (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email_group text NOT NULL CHECK (email_group IN ('lesson_reminders', 'lesson_changes', 'homework', 'results', 'submissions', 'message_digest')),
    enabled boolean NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, email_group)
);

-- +goose Down
DROP TABLE email_preferences;
