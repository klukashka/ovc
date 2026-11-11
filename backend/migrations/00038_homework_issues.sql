-- +goose Up
ALTER TABLE tutor_profiles ADD COLUMN snapshot_bytes_total bigint NOT NULL DEFAULT 0 CHECK (snapshot_bytes_total >= 0);

CREATE TABLE issues (
    id uuid PRIMARY KEY,
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE RESTRICT,
    template_id uuid NOT NULL,
    relationship_id uuid,
    group_id uuid,
    lesson_id uuid,
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot) = 'array'),
    snapshot_bytes bigint NOT NULL CHECK (snapshot_bytes >= 0),
    max_points integer NOT NULL CHECK (max_points BETWEEN 0 AND 10000),
    has_manual_questions boolean NOT NULL,
    deadline_at timestamptz,
    answer_policy text NOT NULL CHECK (answer_policy IN ('never', 'after_submission', 'after_check')),
    status text NOT NULL CHECK (status IN ('active', 'withdrawn')),
    create_key text CHECK (create_key ~ '^[A-Za-z0-9._-]{1,64}$'),
    version bigint NOT NULL CHECK (version >= 1),
    erase_pending_since timestamptz,
    issued_at timestamptz NOT NULL,
    withdrawn_at timestamptz,
    updated_at timestamptz NOT NULL,
    CONSTRAINT issues_tutor_id_key UNIQUE (tutor_id, id),
    CONSTRAINT issues_audience_xor CHECK (num_nonnulls(relationship_id, group_id) = 1),
    CONSTRAINT issues_withdrawn_at_when_withdrawn CHECK ((status = 'withdrawn') = (withdrawn_at IS NOT NULL)),
    CONSTRAINT issues_template_fkey FOREIGN KEY (template_id, tutor_id) REFERENCES templates (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT issues_relationship_fkey FOREIGN KEY (relationship_id, tutor_id) REFERENCES relationships (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT issues_group_fkey FOREIGN KEY (group_id, tutor_id) REFERENCES groups (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT issues_lesson_fkey FOREIGN KEY (lesson_id, tutor_id) REFERENCES lessons (id, tutor_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX issues_create_key_idx ON issues (tutor_id, create_key) WHERE create_key IS NOT NULL;
CREATE INDEX issues_tutor_issued_idx ON issues (tutor_id, issued_at, id);
CREATE INDEX issues_tutor_deadline_idx ON issues (tutor_id, deadline_at, id) WHERE status = 'active';
CREATE INDEX issues_lesson_idx ON issues (lesson_id);
CREATE INDEX issues_relationship_idx ON issues (relationship_id);
CREATE INDEX issues_group_idx ON issues (group_id);
CREATE INDEX issues_template_idx ON issues (template_id);
CREATE INDEX issues_erase_pending_idx ON issues (erase_pending_since) WHERE erase_pending_since IS NOT NULL;

CREATE TABLE submissions (
    id uuid PRIMARY KEY,
    issue_id uuid NOT NULL,
    tutor_id uuid NOT NULL,
    relationship_id uuid NOT NULL,
    student_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('not_started', 'draft', 'submitted', 'returned', 'checked', 'closed')),
    deadline_at timestamptz,
    issued_at timestamptz NOT NULL,
    version bigint NOT NULL CHECK (version >= 1),
    last_save_id uuid,
    file_bytes bigint NOT NULL DEFAULT 0 CHECK (file_bytes BETWEEN 0 AND 250000000),
    first_submitted_at timestamptz,
    submitted_at timestamptz,
    checked_at timestamptz,
    late boolean NOT NULL DEFAULT false,
    auto_points integer CHECK (auto_points BETWEEN 0 AND 10000),
    final_points integer CHECK (final_points BETWEEN 0 AND 10000),
    release_pending_since timestamptz,
    issued_before_reclaim boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT submissions_issue_student_key UNIQUE (issue_id, student_id),
    CONSTRAINT submissions_submitted_together CHECK ((first_submitted_at IS NULL) = (submitted_at IS NULL)),
    CONSTRAINT submissions_issue_fkey FOREIGN KEY (tutor_id, issue_id) REFERENCES issues (tutor_id, id) ON DELETE RESTRICT,
    CONSTRAINT submissions_relationship_fkey FOREIGN KEY (relationship_id, tutor_id) REFERENCES relationships (id, tutor_id) ON DELETE RESTRICT
);

CREATE INDEX submissions_tutor_issue_idx ON submissions (tutor_id, issue_id);
CREATE INDEX submissions_student_status_idx ON submissions (student_id, status);
CREATE INDEX submissions_relationship_idx ON submissions (relationship_id);
CREATE INDEX submissions_review_queue_idx ON submissions (tutor_id, first_submitted_at, id) WHERE status = 'submitted';
CREATE INDEX submissions_todo_idx ON submissions (student_id, deadline_at, issued_at, id)
    WHERE status IN ('not_started', 'draft', 'returned');
CREATE INDEX submissions_release_pending_idx ON submissions (release_pending_since) WHERE release_pending_since IS NOT NULL;

ALTER TABLE notifications DROP CONSTRAINT notifications_event_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_check CHECK (
    event IN (
        'invitation.accepted', 'join_request.created', 'join_request.accepted', 'join_request.declined', 'relationship.ended',
        'lesson.reminder_1h', 'lesson.moved', 'lesson.cancelled', 'lesson.series_changed', 'group.archived',
        'issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned',
        'submission.review_updated'
    )
);

CREATE INDEX notifications_homework_subject ON notifications (subject_id) WHERE event IN (
    'issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned',
    'submission.review_updated'
);

-- +goose Down
DROP INDEX notifications_homework_subject;
DELETE FROM notifications WHERE event IN (
    'issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned',
    'submission.review_updated'
);
ALTER TABLE notifications DROP CONSTRAINT notifications_event_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_check CHECK (
    event IN (
        'invitation.accepted', 'join_request.created', 'join_request.accepted', 'join_request.declined', 'relationship.ended',
        'lesson.reminder_1h', 'lesson.moved', 'lesson.cancelled', 'lesson.series_changed', 'group.archived'
    )
);
ALTER TABLE submissions ADD CONSTRAINT submissions_empty_before_rollback CHECK (false);
ALTER TABLE submissions DROP CONSTRAINT submissions_empty_before_rollback;
ALTER TABLE issues ADD CONSTRAINT issues_empty_before_rollback CHECK (false);
ALTER TABLE issues DROP CONSTRAINT issues_empty_before_rollback;
ALTER TABLE attachment_references ADD CONSTRAINT attachment_references_no_issue_snapshot_kind CHECK (ref_kind <> 'issue_snapshot');
ALTER TABLE attachment_references DROP CONSTRAINT attachment_references_no_issue_snapshot_kind;
DROP TABLE submissions;
DROP TABLE issues;
ALTER TABLE tutor_profiles DROP COLUMN snapshot_bytes_total;
