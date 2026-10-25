-- +goose Up
CREATE TABLE notifications (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    tutor_id uuid NOT NULL,
    student_id uuid,
    event text NOT NULL CHECK (
        event IN ('invitation.accepted', 'join_request.created', 'join_request.accepted', 'join_request.declined', 'relationship.ended')
    ),
    subject_id uuid NOT NULL,
    relationship_id uuid,
    idempotency_key text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    read_at timestamptz
);

CREATE INDEX notifications_user_list ON notifications (user_id, created_at DESC, id DESC);
CREATE INDEX notifications_user_unread ON notifications (user_id) WHERE read_at IS NULL;
CREATE INDEX notifications_created ON notifications (created_at);
CREATE INDEX notifications_tutor ON notifications (tutor_id);
CREATE INDEX notifications_student ON notifications (student_id) WHERE student_id IS NOT NULL;

-- +goose Down
DROP TABLE notifications;
