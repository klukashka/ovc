-- +goose Up
CREATE TABLE user_presence (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    online_until timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL
);

CREATE TABLE user_presence_instances (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    instance_id uuid NOT NULL,
    online_until timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, instance_id)
);

CREATE INDEX user_presence_instances_expiry ON user_presence_instances (online_until);

-- +goose Down
DROP TABLE user_presence_instances;
DROP TABLE user_presence;
