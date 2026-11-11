-- name: GetUploadSubmission :one
SELECT id, issue_id, tutor_id, relationship_id, student_id FROM submissions
WHERE id = @id AND ((sqlc.arg(as_student)::boolean AND student_id = @caller) OR (NOT sqlc.arg(as_student)::boolean AND tutor_id = @caller));

-- name: LockUploadRelationship :one
SELECT state FROM relationships WHERE id = @id AND tutor_id = @tutor_id FOR SHARE;

-- name: LockUploadIssue :one
SELECT status, (erase_pending_since IS NOT NULL)::boolean AS marked_for_erasure,
    (SELECT block -> 'allowed_kinds' FROM jsonb_array_elements(issues.snapshot) AS block
        WHERE block ->> 'id' = sqlc.arg(question_id)::text AND block ->> 'type' = 'file' LIMIT 1)::jsonb AS allowed_kinds
FROM issues WHERE id = @id AND tutor_id = @tutor_id FOR SHARE;

-- name: LockUploadSubmission :one
SELECT status FROM submissions WHERE id = @id AND issue_id = @issue_id FOR SHARE;

-- name: SumOpenAnswerBytes :one
SELECT COALESCE(sum(CASE WHEN state = 'pending' THEN size_bytes ELSE reserved_bytes END), 0)::bigint FROM attachments
WHERE uploader_id = @student_id AND quota_tutor_id = @tutor_id AND context = 'answer'
    AND (sqlc.narg(excluded)::uuid IS NULL OR id <> sqlc.narg(excluded)::uuid)
    AND ((state = 'pending' AND expires_at > sqlc.arg(now)::timestamptz)
        OR (state = 'uploading' AND created_at > sqlc.arg(uploading_since)::timestamptz));

-- name: ClaimUnbindableHomeworkUploads :many
SELECT attachments.id FROM attachments
WHERE attachments.state = 'pending' AND attachments.context IN ('answer', 'corrected')
    AND NOT EXISTS (
        SELECT 1 FROM submissions
        JOIN issues ON issues.id = submissions.issue_id AND issues.tutor_id = submissions.tutor_id
        JOIN relationships ON relationships.id = submissions.relationship_id AND relationships.tutor_id = submissions.tutor_id
        JOIN users AS tutors ON tutors.id = submissions.tutor_id
        WHERE attachments.scope_kind = 'submission' AND submissions.id = attachments.scope_id
            AND issues.status = 'active' AND issues.erase_pending_since IS NULL
            AND (
                (attachments.context = 'answer' AND submissions.student_id = attachments.uploader_id
                    AND submissions.status IN ('not_started', 'draft', 'returned') AND relationships.state IN ('active', 'provisional'))
                OR (attachments.context = 'corrected' AND submissions.tutor_id = attachments.uploader_id
                    AND submissions.status IN ('submitted', 'checked') AND tutors.state NOT IN ('blocked', 'deleted'))
            )
    )
ORDER BY attachments.id
LIMIT sqlc.arg(batch_size)
FOR UPDATE OF attachments SKIP LOCKED;
