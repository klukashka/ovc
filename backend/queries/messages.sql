-- name: GetChannelTutor :one
SELECT tutor_id FROM channels WHERE id = @id;

-- name: LockChannelForUpdate :one
SELECT id FROM channels WHERE id = @id FOR UPDATE;

-- name: LockChannelsForUpdate :many
SELECT id FROM channels WHERE id = ANY (sqlc.arg(ids)::uuid[]) ORDER BY id FOR UPDATE;

-- name: GetActiveMembership :one
SELECT channel_id, kind, tutor_id, read_only_since, last_seq, last_read_seq FROM active_channel_members
WHERE channel_id = @channel_id AND user_id = @user_id;

-- name: FindMessageByClientKey :one
SELECT id, channel_id FROM messages
WHERE sender_id = @sender_id AND channel_id = @channel_id AND client_key = sqlc.arg(client_key)::text;

-- name: AdvanceChannelSeq :one
UPDATE channels SET last_seq = last_seq + 1, last_activity_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id
RETURNING last_seq;

-- name: InsertMessage :exec
INSERT INTO messages (id, channel_id, seq, sender_id, client_key, body, version, created_at, updated_at)
VALUES (@id, @channel_id, @seq, @sender_id, sqlc.narg(client_key)::text, sqlc.arg(body)::text, 1, sqlc.arg(now)::timestamptz,
    sqlc.arg(now)::timestamptz);

-- name: MoveSenderReadMarker :execrows
UPDATE channel_members
SET last_read_seq = sqlc.arg(seq)::bigint, last_read_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
FROM active_channel_members AS active
WHERE channel_members.channel_id = active.channel_id AND channel_members.user_id = active.user_id
    AND active.channel_id = sqlc.arg(channel_id)::uuid AND active.user_id = sqlc.arg(user_id)::uuid
    AND channel_members.last_read_seq < sqlc.arg(seq)::bigint;

-- name: FlagDigestPending :execrows
INSERT INTO digest_pending (user_id, pending_since)
SELECT members.user_id, sqlc.arg(now)::timestamptz FROM active_channel_members AS members
WHERE members.channel_id = @channel_id AND members.user_id <> sqlc.arg(sender_id)::uuid
ORDER BY members.user_id
ON CONFLICT (user_id) DO NOTHING;

-- name: LockBindableUploads :many
SELECT id, size_bytes::bigint AS size_bytes FROM attachments
WHERE id = ANY (sqlc.arg(ids)::uuid[]) AND uploader_id = @uploader_id AND context = 'chat' AND state = 'pending' AND scope_kind = 'channel'
    AND scope_id = sqlc.arg(channel_id)::uuid AND expires_at > sqlc.arg(now)::timestamptz AND size_bytes IS NOT NULL
ORDER BY id
FOR UPDATE;

-- name: BindUploads :execrows
UPDATE attachments SET state = 'bound', expires_at = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = ANY (sqlc.arg(ids)::uuid[]) AND state = 'pending';

-- name: InsertMessageReferences :execrows
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, created_at)
SELECT bound.id, 'message', sqlc.arg(message_id)::uuid, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(attachment_ids)::uuid[]) AS bound (id);

-- name: ChargeAttachments :one
WITH charged AS (
    INSERT INTO quota_charges (tutor_id, attachment_id, size_bytes, created_at)
    SELECT sqlc.arg(tutor_id)::uuid, attachments.id, attachments.size_bytes, sqlc.arg(now)::timestamptz FROM attachments
    WHERE attachments.id = ANY (sqlc.arg(attachment_ids)::uuid[]) AND attachments.size_bytes IS NOT NULL
    ON CONFLICT (tutor_id, attachment_id) DO NOTHING
    RETURNING quota_charges.size_bytes
), added AS (
    SELECT COALESCE(sum(charged.size_bytes), 0)::bigint AS bytes FROM charged
)
UPDATE tutor_profiles SET file_usage_bytes = tutor_profiles.file_usage_bytes + added.bytes, updated_at = sqlc.arg(now)::timestamptz
FROM added
WHERE tutor_profiles.user_id = sqlc.arg(tutor_id)::uuid AND tutor_profiles.file_usage_bytes + added.bytes <= tutor_profiles.file_quota_bytes
RETURNING tutor_profiles.file_usage_bytes;

-- name: ListMessagesBefore :many
SELECT messages.id, messages.channel_id, messages.seq, messages.sender_id, messages.body, messages.version, messages.created_at,
    messages.edited_at, messages.deleted_at, messages.erased_at, channels.tutor_id, COALESCE(senders.state, '')::text AS sender_state, senders.name AS sender_name,
    COALESCE(cards.display_name, '')::text AS card_name,
    COALESCE(messages.seq <= channels.provisional_through_seq AND messages.sender_id = direct.student_id, false)::boolean AS sent_before_reclaim
FROM messages
JOIN channels ON channels.id = messages.channel_id
LEFT JOIN relationships AS direct ON direct.id = channels.relationship_id
LEFT JOIN users AS senders ON senders.id = messages.sender_id
LEFT JOIN LATERAL (
    SELECT relationships.display_name FROM relationships
    WHERE relationships.tutor_id = channels.tutor_id AND relationships.student_id = messages.sender_id
    ORDER BY relationships.state = 'discarded', relationships.created_at DESC
    LIMIT 1
) AS cards ON true
WHERE messages.channel_id = @channel_id AND (sqlc.narg(before_seq)::bigint IS NULL OR messages.seq < sqlc.narg(before_seq)::bigint)
ORDER BY messages.seq DESC
LIMIT sqlc.arg(row_limit);

-- name: ListMessagesAfter :many
SELECT messages.id, messages.channel_id, messages.seq, messages.sender_id, messages.body, messages.version, messages.created_at,
    messages.edited_at, messages.deleted_at, messages.erased_at, channels.tutor_id, COALESCE(senders.state, '')::text AS sender_state, senders.name AS sender_name,
    COALESCE(cards.display_name, '')::text AS card_name,
    COALESCE(messages.seq <= channels.provisional_through_seq AND messages.sender_id = direct.student_id, false)::boolean AS sent_before_reclaim
FROM messages
JOIN channels ON channels.id = messages.channel_id
LEFT JOIN relationships AS direct ON direct.id = channels.relationship_id
LEFT JOIN users AS senders ON senders.id = messages.sender_id
LEFT JOIN LATERAL (
    SELECT relationships.display_name FROM relationships
    WHERE relationships.tutor_id = channels.tutor_id AND relationships.student_id = messages.sender_id
    ORDER BY relationships.state = 'discarded', relationships.created_at DESC
    LIMIT 1
) AS cards ON true
WHERE messages.channel_id = @channel_id AND messages.seq > sqlc.arg(after_seq)::bigint
ORDER BY messages.seq
LIMIT sqlc.arg(row_limit);

-- name: GetMessageView :one
SELECT messages.id, messages.channel_id, messages.seq, messages.sender_id, messages.body, messages.version, messages.created_at,
    messages.edited_at, messages.deleted_at, messages.erased_at, channels.tutor_id, COALESCE(senders.state, '')::text AS sender_state, senders.name AS sender_name,
    COALESCE(cards.display_name, '')::text AS card_name,
    COALESCE(messages.seq <= channels.provisional_through_seq AND messages.sender_id = direct.student_id, false)::boolean AS sent_before_reclaim
FROM messages
JOIN channels ON channels.id = messages.channel_id
LEFT JOIN relationships AS direct ON direct.id = channels.relationship_id
LEFT JOIN users AS senders ON senders.id = messages.sender_id
LEFT JOIN LATERAL (
    SELECT relationships.display_name FROM relationships
    WHERE relationships.tutor_id = channels.tutor_id AND relationships.student_id = messages.sender_id
    ORDER BY relationships.state = 'discarded', relationships.created_at DESC
    LIMIT 1
) AS cards ON true
WHERE messages.id = @id;

-- name: ListLastMessages :many
SELECT messages.id, messages.channel_id, messages.seq, messages.sender_id, messages.body, messages.version, messages.created_at,
    messages.edited_at, messages.deleted_at, messages.erased_at, channels.tutor_id, COALESCE(senders.state, '')::text AS sender_state, senders.name AS sender_name,
    COALESCE(cards.display_name, '')::text AS card_name,
    COALESCE(messages.seq <= channels.provisional_through_seq AND messages.sender_id = direct.student_id, false)::boolean AS sent_before_reclaim
FROM channels
JOIN messages ON messages.channel_id = channels.id AND messages.seq = channels.last_seq
LEFT JOIN relationships AS direct ON direct.id = channels.relationship_id
LEFT JOIN users AS senders ON senders.id = messages.sender_id
LEFT JOIN LATERAL (
    SELECT relationships.display_name FROM relationships
    WHERE relationships.tutor_id = channels.tutor_id AND relationships.student_id = messages.sender_id
    ORDER BY relationships.state = 'discarded', relationships.created_at DESC
    LIMIT 1
) AS cards ON true
WHERE channels.id = ANY (sqlc.arg(channel_ids)::uuid[]);

-- name: ListMessageAttachments :many
SELECT attachment_references.ref_id AS message_id, attachments.id, attachments.filename, attachments.media_type::text AS media_type,
    attachments.size_bytes::bigint AS size_bytes, attachments.width, attachments.height
FROM attachment_references
JOIN attachments ON attachments.id = attachment_references.attachment_id
WHERE attachment_references.ref_kind = 'message' AND attachment_references.ref_id = ANY (sqlc.arg(message_ids)::uuid[])
    AND attachments.state = 'bound' AND attachments.media_type IS NOT NULL AND attachments.size_bytes IS NOT NULL
ORDER BY attachment_references.ref_id, attachments.created_at, attachments.id;

-- name: GetMessageScope :one
SELECT messages.channel_id, channels.tutor_id,
    EXISTS (
        SELECT 1 FROM attachment_references WHERE attachment_references.ref_kind = 'message' AND attachment_references.ref_id = messages.id
    ) AS has_attachments
FROM messages
JOIN channels ON channels.id = messages.channel_id
WHERE messages.id = @id;

-- name: LockMessage :one
SELECT id, channel_id, sender_id, version, created_at, deleted_at, erased_at FROM messages WHERE id = @id FOR UPDATE;

-- name: EditMessage :execrows
UPDATE messages
SET body = sqlc.arg(body)::text, version = version + 1, edited_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND deleted_at IS NULL AND erased_at IS NULL;

-- name: MarkMessageDeleted :execrows
UPDATE messages SET body = NULL, deleted_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND deleted_at IS NULL;

-- name: RemoveMessageReferences :many
DELETE FROM attachment_references
WHERE ref_kind = 'message' AND ref_id = ANY (sqlc.arg(message_ids)::uuid[])
RETURNING attachment_id;

-- name: RemoveReferencesOfAttachments :many
DELETE FROM attachment_references
WHERE ref_kind = 'message' AND attachment_id = ANY (sqlc.arg(attachment_ids)::uuid[])
RETURNING attachment_id;

-- name: ReleaseUnreferencedCharges :many
WITH released AS (
    DELETE FROM quota_charges
    WHERE quota_charges.attachment_id = ANY (sqlc.arg(attachment_ids)::uuid[])
        AND NOT EXISTS (
            SELECT 1 FROM attachment_references WHERE attachment_references.attachment_id = quota_charges.attachment_id
        )
    RETURNING quota_charges.tutor_id, quota_charges.size_bytes
), totals AS (
    SELECT released.tutor_id, sum(released.size_bytes)::bigint AS bytes FROM released GROUP BY released.tutor_id
)
UPDATE tutor_profiles SET file_usage_bytes = tutor_profiles.file_usage_bytes - totals.bytes, updated_at = sqlc.arg(now)::timestamptz
FROM totals
WHERE tutor_profiles.user_id = totals.tutor_id
RETURNING tutor_profiles.user_id, totals.bytes;

-- name: MarkUnreferencedAttachmentsDeleted :many
UPDATE attachments SET state = 'deleted', expires_at = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = ANY (sqlc.arg(attachment_ids)::uuid[]) AND state = 'bound'
    AND NOT EXISTS (SELECT 1 FROM attachment_references WHERE attachment_references.attachment_id = attachments.id)
RETURNING id, storage_key;

-- name: MoveReadMarker :execrows
UPDATE channel_members
SET last_read_seq = LEAST(sqlc.arg(seq)::bigint, active.last_seq), last_read_at = sqlc.arg(now)::timestamptz,
    updated_at = sqlc.arg(now)::timestamptz
FROM active_channel_members AS active
WHERE channel_members.channel_id = active.channel_id AND channel_members.user_id = active.user_id
    AND active.channel_id = sqlc.arg(channel_id)::uuid AND active.user_id = sqlc.arg(user_id)::uuid
    AND channel_members.last_read_seq < LEAST(sqlc.arg(seq)::bigint, active.last_seq);

-- name: LockChannelMessagesBatch :many
SELECT id FROM messages WHERE channel_id = @channel_id ORDER BY seq LIMIT sqlc.arg(batch_size) FOR UPDATE;

-- name: DeleteMessagesByID :execrows
DELETE FROM messages WHERE id = ANY (sqlc.arg(ids)::uuid[]);

-- name: ListSenderMessagesBatch :many
SELECT messages.id, messages.channel_id, channels.tutor_id,
    EXISTS (
        SELECT 1 FROM attachment_references WHERE attachment_references.ref_kind = 'message' AND attachment_references.ref_id = messages.id
    ) AS has_attachments
FROM messages
JOIN channels ON channels.id = messages.channel_id
WHERE messages.sender_id = sqlc.arg(sender_id)::uuid
ORDER BY messages.id
LIMIT sqlc.arg(batch_size);

-- name: LockMessagesOfSender :many
SELECT id FROM messages WHERE id = ANY (sqlc.arg(ids)::uuid[]) AND sender_id = sqlc.arg(sender_id)::uuid ORDER BY id FOR UPDATE;

-- name: EraseMessagesOfSender :execrows
UPDATE messages
SET body = NULL, erased_at = CASE WHEN deleted_at IS NULL THEN sqlc.arg(now)::timestamptz ELSE erased_at END, sender_id = NULL,
    client_key = NULL, updated_at = sqlc.arg(now)::timestamptz
WHERE id = ANY (sqlc.arg(ids)::uuid[]) AND sender_id = sqlc.arg(sender_id)::uuid;

-- name: ListTutorMessageReferencesBatch :many
SELECT attachment_references.attachment_id FROM attachment_references
JOIN messages ON messages.id = attachment_references.ref_id
JOIN channels ON channels.id = messages.channel_id
WHERE attachment_references.ref_kind = 'message' AND channels.tutor_id = @tutor_id
ORDER BY attachment_references.attachment_id
LIMIT sqlc.arg(batch_size);

-- name: DeleteTutorMessagesBatch :execrows
DELETE FROM messages
WHERE id IN (
    SELECT doomed.id FROM messages AS doomed
    WHERE doomed.channel_id = (
        SELECT channels.id FROM channels
        WHERE channels.tutor_id = @tutor_id AND EXISTS (SELECT 1 FROM messages AS kept WHERE kept.channel_id = channels.id)
        ORDER BY channels.id
        LIMIT 1
    )
    ORDER BY doomed.seq
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE OF doomed
);

-- name: DeleteDigestPendingOf :execrows
DELETE FROM digest_pending WHERE user_id = @user_id;
