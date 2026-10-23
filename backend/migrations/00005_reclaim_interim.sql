-- +goose Up
WITH reclaimed AS (
    UPDATE relationships
    SET state = 'reclaim_pending', updated_at = now()
    FROM users
    WHERE users.id = relationships.student_id
        AND relationships.state = 'provisional'
        AND users.email_verified_at IS NOT NULL
    RETURNING relationships.student_id
)
UPDATE users
SET reclaim_until = now() + interval '7 days', updated_at = now()
WHERE users.id IN (SELECT reclaimed.student_id FROM reclaimed) AND users.reclaim_until IS NULL;

-- +goose Down
