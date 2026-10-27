-- +goose Up
ALTER TABLE notifications DROP CONSTRAINT notifications_event_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_check CHECK (
    event IN (
        'invitation.accepted', 'join_request.created', 'join_request.accepted', 'join_request.declined', 'relationship.ended',
        'lesson.reminder_1h'
    )
);

ALTER TABLE mail_sink ADD COLUMN headers jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE mail_sink DROP COLUMN headers;

DELETE FROM notifications WHERE event = 'lesson.reminder_1h';
ALTER TABLE notifications DROP CONSTRAINT notifications_event_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_check CHECK (
    event IN ('invitation.accepted', 'join_request.created', 'join_request.accepted', 'join_request.declined', 'relationship.ended')
);
