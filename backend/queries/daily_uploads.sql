-- name: GetChatUploadAccountingActivation :many
SELECT singleton, active_at FROM chat_upload_accounting_activation LIMIT 2;

-- name: GetDailyChatBytes :one
SELECT COALESCE((SELECT completed_bytes FROM daily_chat_upload_bytes
WHERE student_id = @student_id AND tutor_id = @tutor_id AND upload_day = @upload_day), 0)::bigint;

-- name: AddDailyChatBytes :exec
INSERT INTO daily_chat_upload_bytes (student_id, tutor_id, upload_day, completed_bytes)
VALUES (@student_id, @tutor_id, @upload_day, @completed_bytes)
ON CONFLICT (student_id, tutor_id, upload_day)
DO UPDATE SET completed_bytes = daily_chat_upload_bytes.completed_bytes + EXCLUDED.completed_bytes;

-- name: LockDailyUploadCleanupStudents :many
SELECT users.id FROM users
WHERE EXISTS (
    SELECT 1 FROM daily_chat_upload_bytes AS daily
    WHERE daily.student_id = users.id AND daily.upload_day < @before_day
        AND NOT EXISTS (SELECT 1 FROM attachments
            WHERE attachments.uploader_id = daily.student_id AND attachments.quota_tutor_id = daily.tutor_id
                AND attachments.context = 'chat' AND attachments.state = 'uploading'
                AND attachments.created_at >= daily.upload_day AND attachments.created_at < daily.upload_day + interval '24 hours')
)
ORDER BY users.id LIMIT 100 FOR UPDATE OF users SKIP LOCKED;

-- name: DeletePastDailyChatBytes :execrows
DELETE FROM daily_chat_upload_bytes
WHERE (student_id, tutor_id, upload_day) IN (
    SELECT daily.student_id, daily.tutor_id, daily.upload_day FROM daily_chat_upload_bytes AS daily
    WHERE daily.student_id = ANY(sqlc.arg(students)::uuid[]) AND daily.upload_day < @before_day
        AND NOT EXISTS (SELECT 1 FROM attachments
            WHERE attachments.uploader_id = daily.student_id AND attachments.quota_tutor_id = daily.tutor_id
                AND attachments.context = 'chat' AND attachments.state = 'uploading'
                AND attachments.created_at >= daily.upload_day AND attachments.created_at < daily.upload_day + interval '24 hours')
    ORDER BY daily.student_id, daily.tutor_id, daily.upload_day LIMIT 100 FOR UPDATE OF daily SKIP LOCKED
);

-- name: DeleteTutorDailyChatBytesBatch :execrows
DELETE FROM daily_chat_upload_bytes
WHERE (student_id, tutor_id, upload_day) IN (
    SELECT daily.student_id, daily.tutor_id, daily.upload_day FROM daily_chat_upload_bytes AS daily
    WHERE daily.tutor_id = @tutor_id ORDER BY daily.student_id, daily.upload_day LIMIT @batch_size FOR UPDATE OF daily
);
