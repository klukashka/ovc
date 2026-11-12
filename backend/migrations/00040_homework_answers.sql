-- +goose Up
CREATE TABLE answers (
    submission_id uuid NOT NULL REFERENCES submissions (id) ON DELETE RESTRICT,
    question_id uuid NOT NULL,
    value jsonb NOT NULL CHECK (jsonb_typeof(value) = 'object'),
    normalised text,
    normaliser_version integer CHECK (normaliser_version = 1),
    auto_points integer CHECK (auto_points BETWEEN 0 AND 100),
    auto_correct boolean,
    frozen_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (submission_id, question_id),
    CONSTRAINT answers_scored_when_frozen CHECK ((frozen_at IS NULL) = (auto_points IS NULL) AND (frozen_at IS NULL) = (auto_correct IS NULL)),
    CONSTRAINT answers_normalised_with_version CHECK (normalised IS NULL OR normaliser_version IS NOT NULL)
);

-- +goose Down
ALTER TABLE answers ADD CONSTRAINT answers_empty_before_rollback CHECK (false);
ALTER TABLE answers DROP CONSTRAINT answers_empty_before_rollback;
ALTER TABLE attachment_references ADD CONSTRAINT attachment_references_no_answer_kind CHECK (ref_kind <> 'answer');
ALTER TABLE attachment_references DROP CONSTRAINT attachment_references_no_answer_kind;
DROP TABLE answers;
