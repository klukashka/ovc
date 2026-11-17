-- name: ListErasePendingIssues :many
SELECT id, tutor_id FROM issues
WHERE erase_pending_since IS NOT NULL AND NOT (id = ANY(sqlc.arg(skipped)::uuid[]))
ORDER BY erase_pending_since, id LIMIT 1;

-- name: ListCleanupReleasePending :many
SELECT id, tutor_id, issue_id FROM submissions
WHERE release_pending_since IS NOT NULL AND NOT (id = ANY(sqlc.arg(skipped)::uuid[]))
ORDER BY release_pending_since, id LIMIT 1;

-- name: LockCleanupIssue :one
SELECT id, erase_pending_since, snapshot_bytes FROM issues
WHERE id = @id AND tutor_id = @tutor_id FOR UPDATE;

-- name: LockCleanupSubmission :one
SELECT id, submitted_at, release_pending_since FROM submissions
WHERE issue_id = @issue_id AND (sqlc.narg(submission_id)::uuid IS NULL OR id = sqlc.narg(submission_id)::uuid)
ORDER BY id LIMIT 1 FOR UPDATE;

-- name: LockCleanupRevision :one
SELECT id FROM review_revisions WHERE submission_id = @submission_id ORDER BY id LIMIT 1 FOR UPDATE;

-- name: RemoveCleanupReferences :many
WITH selected AS (
    SELECT candidate.attachment_id, candidate.ref_kind, candidate.ref_id, candidate.question_id FROM attachment_references AS candidate
    WHERE candidate.ref_kind = @ref_kind AND candidate.ref_id = @ref_id
        AND (sqlc.narg(after_time)::timestamptz IS NULL OR candidate.created_at > sqlc.narg(after_time)::timestamptz)
    ORDER BY candidate.attachment_id, candidate.question_id LIMIT sqlc.arg(batch_size)::int
)
DELETE FROM attachment_references USING selected
WHERE attachment_references.attachment_id = selected.attachment_id
    AND attachment_references.ref_kind = selected.ref_kind AND attachment_references.ref_id = selected.ref_id
    AND attachment_references.question_id IS NOT DISTINCT FROM selected.question_id
RETURNING attachment_references.attachment_id;

-- name: DeleteCleanupRevision :execrows
DELETE FROM review_revisions WHERE id = @id;

-- name: DeleteCleanupAnswers :execrows
DELETE FROM answers WHERE (submission_id, question_id) IN (
    SELECT candidate.submission_id, candidate.question_id FROM answers AS candidate WHERE candidate.submission_id = @submission_id
    ORDER BY question_id LIMIT sqlc.arg(batch_size)::int
);

-- name: DeleteCleanupNotifications :execrows
DELETE FROM notifications WHERE id IN (
    SELECT candidate.id FROM notifications AS candidate WHERE candidate.subject_id = @subject_id AND event IN (
        'issue.created', 'issue.withdrawn', 'submission.submitted', 'submission.checked', 'submission.returned', 'submission.review_updated'
    ) ORDER BY id LIMIT sqlc.arg(batch_size)::int
);

-- name: DeleteCleanupSubmission :execrows
DELETE FROM submissions WHERE id = @id;

-- name: DeleteCleanupIssue :execrows
DELETE FROM issues WHERE id = @id AND erase_pending_since IS NOT NULL;

-- name: LockLifecycleIssues :many
SELECT id FROM issues WHERE id IN (
    SELECT linked.id FROM issues AS linked WHERE linked.relationship_id = ANY(sqlc.arg(relationships)::uuid[])
    UNION
    SELECT submissions.issue_id FROM submissions WHERE submissions.relationship_id = ANY(sqlc.arg(relationships)::uuid[])
)
ORDER BY id FOR UPDATE;

-- name: LockLifecycleSubmissions :many
SELECT id FROM submissions WHERE relationship_id = ANY(sqlc.arg(relationships)::uuid[]) ORDER BY id FOR UPDATE;

-- name: MarkLifecycleIssues :many
UPDATE issues SET erase_pending_since = sqlc.arg(now)::timestamptz
WHERE relationship_id = ANY(sqlc.arg(relationships)::uuid[]) AND erase_pending_since IS NULL
RETURNING id, tutor_id, relationship_id;

-- name: ListLifecycleSubmissions :many
SELECT submissions.id, submissions.student_id, submissions.relationship_id, submissions.tutor_id, issues.status AS issue_status,
    submissions.issue_id, issues.lesson_id
FROM submissions JOIN issues ON issues.id = submissions.issue_id
WHERE submissions.issue_id = ANY(sqlc.arg(issues)::uuid[]) ORDER BY submissions.id;

-- name: CloseLifecycleSubmissions :many
UPDATE submissions SET status = 'closed', version = version + 1, updated_at = sqlc.arg(now)::timestamptz,
    release_pending_since = CASE WHEN EXISTS (
        SELECT 1 FROM attachment_references WHERE ref_kind = 'answer' AND ref_id = submissions.id
            AND (submissions.submitted_at IS NULL OR created_at > submissions.submitted_at)
    ) THEN sqlc.arg(now)::timestamptz ELSE release_pending_since END
WHERE relationship_id = ANY(sqlc.arg(relationships)::uuid[]) AND status IN ('not_started', 'draft', 'returned')
RETURNING id, issue_id, student_id, tutor_id, relationship_id;

-- name: GetCleanupSubmissionIssue :one
SELECT issue_id FROM submissions WHERE id = @id;

-- name: LockStudentPurgeIssues :many
SELECT id FROM issues WHERE id IN (
    SELECT issue_id FROM submissions WHERE submissions.id = ANY(sqlc.arg(submissions)::uuid[])
) ORDER BY id FOR UPDATE;

-- name: LockStudentPurgeSubmissions :many
SELECT id FROM submissions WHERE id = ANY(sqlc.arg(submissions)::uuid[]) ORDER BY id FOR UPDATE;

-- name: LockStudentPurgeRevisions :many
SELECT id, submission_id FROM review_revisions WHERE submission_id = ANY(sqlc.arg(submissions)::uuid[])
ORDER BY id FOR UPDATE;
