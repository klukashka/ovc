-- +goose Up
CREATE TABLE test_clock (
    id smallint PRIMARY KEY CHECK (id = 1),
    instant timestamptz NOT NULL
);

-- +goose Down
DROP TABLE test_clock;
