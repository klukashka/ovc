-- +goose Up
ALTER TABLE notifications DROP CONSTRAINT notifications_event_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_check CHECK (
    event IN (
        'invitation.accepted', 'join_request.created', 'join_request.accepted', 'join_request.declined', 'relationship.ended',
        'lesson.reminder_1h', 'lesson.moved', 'lesson.cancelled', 'lesson.series_changed', 'group.archived'
    )
);

-- +goose Down
DELETE FROM notifications WHERE event = 'group.archived';
ALTER TABLE notifications DROP CONSTRAINT notifications_event_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_check CHECK (
    event IN (
        'invitation.accepted', 'join_request.created', 'join_request.accepted', 'join_request.declined', 'relationship.ended',
        'lesson.reminder_1h', 'lesson.moved', 'lesson.cancelled', 'lesson.series_changed'
    )
);
