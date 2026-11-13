-- +goose Up
ALTER TABLE submissions ADD CONSTRAINT submissions_id_tutor_key UNIQUE (id, tutor_id);

CREATE TABLE review_revisions (
    id uuid PRIMARY KEY,
    submission_id uuid NOT NULL,
    tutor_id uuid NOT NULL,
    revision integer NOT NULL CHECK (revision BETWEEN 1 AND 100),
    state text NOT NULL CHECK (state IN ('draft', 'published')),
    action text CHECK (action IN ('check', 'return')),
    feedback text NOT NULL CHECK (char_length(feedback) <= 10000),
    items jsonb NOT NULL CHECK (jsonb_typeof(items) = 'array'),
    version bigint NOT NULL CHECK (version >= 1),
    last_save_id uuid,
    published_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT review_revisions_submission_revision_key UNIQUE (submission_id, revision),
    CONSTRAINT review_revisions_submission_fkey FOREIGN KEY (submission_id, tutor_id) REFERENCES submissions (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT review_revisions_published_together CHECK (
        (state = 'published') = (published_at IS NOT NULL) AND (state = 'published') = (action IS NOT NULL)
    )
);
CREATE UNIQUE INDEX review_revisions_one_draft_idx ON review_revisions (submission_id) WHERE state = 'draft';
CREATE INDEX review_revisions_tutor_idx ON review_revisions (tutor_id, id);

ALTER TABLE submissions ADD COLUMN published_revision_id uuid REFERENCES review_revisions (id) ON DELETE SET NULL;
CREATE INDEX submissions_published_revision_idx ON submissions (published_revision_id);

-- +goose Down
ALTER TABLE review_revisions ADD CONSTRAINT review_revisions_empty_before_rollback CHECK (false);
ALTER TABLE review_revisions DROP CONSTRAINT review_revisions_empty_before_rollback;
ALTER TABLE attachment_references ADD CONSTRAINT attachment_references_no_review_item_kind CHECK (ref_kind <> 'review_item');
ALTER TABLE attachment_references DROP CONSTRAINT attachment_references_no_review_item_kind;
ALTER TABLE submissions DROP COLUMN published_revision_id;
DROP TABLE review_revisions;
ALTER TABLE submissions DROP CONSTRAINT submissions_id_tutor_key;
