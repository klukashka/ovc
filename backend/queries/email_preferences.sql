-- name: ListEmailPreferences :many
SELECT email_group, enabled FROM email_preferences WHERE user_id = @user_id ORDER BY email_group;

-- name: SaveEmailPreference :exec
INSERT INTO email_preferences (user_id, email_group, enabled, updated_at)
VALUES (@user_id, @email_group, @enabled, @now)
ON CONFLICT (user_id, email_group) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at;

-- name: DisableEmailPreferenceOfUsableAccount :execrows
INSERT INTO email_preferences (user_id, email_group, enabled, updated_at)
SELECT users.id, @email_group, false, @now
FROM users
WHERE users.id = @user_id AND users.state IN ('unverified', 'active') AND users.role = ANY (sqlc.arg(roles)::text[])
ON CONFLICT (user_id, email_group) DO UPDATE SET enabled = false, updated_at = excluded.updated_at;

