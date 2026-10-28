-- +goose Up
ALTER TABLE notifications DROP CONSTRAINT notifications_event_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_check CHECK (
    event IN (
        'invitation.accepted', 'join_request.created', 'join_request.accepted', 'join_request.declined', 'relationship.ended',
        'lesson.reminder_1h', 'lesson.moved', 'lesson.cancelled', 'lesson.series_changed'
    )
);

ALTER TABLE lesson_series DROP CONSTRAINT lesson_series_end_after_first;
ALTER TABLE lesson_series ADD CONSTRAINT lesson_series_end_after_first CHECK (
    end_date IS NULL OR end_date >= first_date OR ended_at IS NOT NULL
);

-- +goose Down
UPDATE lesson_series SET end_date = first_date WHERE end_date < first_date;
ALTER TABLE lesson_series DROP CONSTRAINT lesson_series_end_after_first;
ALTER TABLE lesson_series ADD CONSTRAINT lesson_series_end_after_first CHECK (end_date IS NULL OR end_date >= first_date);

DELETE FROM notifications WHERE event IN ('lesson.moved', 'lesson.cancelled', 'lesson.series_changed');
ALTER TABLE notifications DROP CONSTRAINT notifications_event_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_check CHECK (
    event IN (
        'invitation.accepted', 'join_request.created', 'join_request.accepted', 'join_request.declined', 'relationship.ended',
        'lesson.reminder_1h'
    )
);
