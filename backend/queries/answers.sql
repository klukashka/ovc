-- name: ListStudentTodoAssignments :many
SELECT submissions.id, issues.title, submissions.relationship_id, tutor.name AS tutor_name, relationships.colour, submissions.status,
    submissions.late, submissions.issued_before_reclaim, submissions.deadline_at, submissions.issued_at, submissions.submitted_at,
    submissions.checked_at, submissions.published_revision_id, submissions.feedback_version, submissions.feedback_seen_version
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
JOIN relationships ON relationships.id = submissions.relationship_id
JOIN users AS tutor ON tutor.id = submissions.tutor_id
WHERE submissions.student_id = @student_id AND submissions.status IN ('not_started', 'draft', 'returned')
    AND issues.status = 'active' AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
    AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz)
    AND (sqlc.narg(relationship_id)::uuid IS NULL OR submissions.relationship_id = sqlc.narg(relationship_id)::uuid)
    AND (
        NOT sqlc.arg(after)::boolean
        OR (
            sqlc.narg(after_deadline_at)::timestamptz IS NOT NULL
            AND (
                submissions.deadline_at IS NULL
                OR submissions.deadline_at > sqlc.narg(after_deadline_at)::timestamptz
                OR (submissions.deadline_at = sqlc.narg(after_deadline_at)::timestamptz
                    AND (submissions.issued_at, submissions.id) > (sqlc.arg(after_issued_at)::timestamptz, sqlc.arg(after_id)::uuid))
            )
        )
        OR (
            sqlc.narg(after_deadline_at)::timestamptz IS NULL AND submissions.deadline_at IS NULL
            AND (submissions.issued_at, submissions.id) > (sqlc.arg(after_issued_at)::timestamptz, sqlc.arg(after_id)::uuid)
        )
    )
ORDER BY submissions.deadline_at NULLS LAST, submissions.issued_at, submissions.id
LIMIT sqlc.arg(page_size);

-- name: ListStudentSubmittedAssignments :many
SELECT submissions.id, issues.title, submissions.relationship_id, tutor.name AS tutor_name, relationships.colour, submissions.status,
    submissions.late, submissions.issued_before_reclaim, submissions.deadline_at, submissions.issued_at, submissions.submitted_at,
    submissions.checked_at, submissions.published_revision_id, submissions.feedback_version, submissions.feedback_seen_version
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
JOIN relationships ON relationships.id = submissions.relationship_id
JOIN users AS tutor ON tutor.id = submissions.tutor_id
WHERE submissions.student_id = @student_id AND submissions.status = 'submitted'
    AND issues.status = 'active' AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
    AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz)
    AND (sqlc.narg(relationship_id)::uuid IS NULL OR submissions.relationship_id = sqlc.narg(relationship_id)::uuid)
    AND (
        sqlc.narg(after_instant)::timestamptz IS NULL
        OR (submissions.submitted_at, submissions.id) < (sqlc.narg(after_instant)::timestamptz, sqlc.arg(after_id)::uuid)
    )
ORDER BY submissions.submitted_at DESC, submissions.id DESC
LIMIT sqlc.arg(page_size);

-- name: ListStudentCheckedAssignments :many
SELECT submissions.id, issues.title, submissions.relationship_id, tutor.name AS tutor_name, relationships.colour, submissions.status,
    submissions.late, submissions.issued_before_reclaim, submissions.deadline_at, submissions.issued_at, submissions.submitted_at,
    submissions.checked_at, submissions.published_revision_id, submissions.feedback_version, submissions.feedback_seen_version
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
JOIN relationships ON relationships.id = submissions.relationship_id
JOIN users AS tutor ON tutor.id = submissions.tutor_id
WHERE submissions.student_id = @student_id AND submissions.status = 'checked'
    AND issues.status = 'active' AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
    AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz)
    AND (sqlc.narg(relationship_id)::uuid IS NULL OR submissions.relationship_id = sqlc.narg(relationship_id)::uuid)
    AND (
        sqlc.narg(after_instant)::timestamptz IS NULL
        OR (submissions.checked_at, submissions.id) < (sqlc.narg(after_instant)::timestamptz, sqlc.arg(after_id)::uuid)
    )
ORDER BY submissions.checked_at DESC, submissions.id DESC
LIMIT sqlc.arg(page_size);

-- name: ListStudentClosedAssignments :many
SELECT submissions.id, issues.title, submissions.relationship_id, tutor.name AS tutor_name, relationships.colour, submissions.status,
    submissions.late, submissions.issued_before_reclaim, submissions.deadline_at, submissions.issued_at, submissions.submitted_at,
    submissions.checked_at, submissions.published_revision_id, submissions.feedback_version, submissions.feedback_seen_version
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
JOIN relationships ON relationships.id = submissions.relationship_id
JOIN users AS tutor ON tutor.id = submissions.tutor_id
WHERE submissions.student_id = @student_id AND submissions.status = 'closed'
    AND issues.status = 'active' AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
    AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz)
    AND (sqlc.narg(relationship_id)::uuid IS NULL OR submissions.relationship_id = sqlc.narg(relationship_id)::uuid)
    AND (
        sqlc.narg(after_instant)::timestamptz IS NULL
        OR (submissions.issued_at, submissions.id) < (sqlc.narg(after_instant)::timestamptz, sqlc.arg(after_id)::uuid)
    )
ORDER BY submissions.issued_at DESC, submissions.id DESC
LIMIT sqlc.arg(page_size);

-- name: CountStudentAssignments :one
SELECT
    (SELECT count(*) FROM (
        SELECT 1 FROM submissions
        JOIN issues ON issues.id = submissions.issue_id
        JOIN relationships ON relationships.id = submissions.relationship_id
        JOIN users AS tutor ON tutor.id = submissions.tutor_id
        WHERE submissions.student_id = @student_id AND submissions.status IN ('not_started', 'draft', 'returned')
            AND issues.status = 'active' AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
            AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz)
        LIMIT sqlc.arg(count_cap)
    ) AS todo)::bigint AS todo,
    (SELECT count(*) FROM (
        SELECT 1 FROM submissions
        JOIN issues ON issues.id = submissions.issue_id
        JOIN relationships ON relationships.id = submissions.relationship_id
        JOIN users AS tutor ON tutor.id = submissions.tutor_id
        WHERE submissions.student_id = @student_id AND submissions.status = 'closed'
            AND issues.status = 'active' AND issues.erase_pending_since IS NULL AND relationships.state IN ('active', 'provisional', 'ended')
            AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz)
        LIMIT sqlc.arg(count_cap)
    ) AS closed)::bigint AS closed;

-- name: GetStudentAssignmentTutor :one
SELECT relationships.id FROM relationships
JOIN users AS tutor ON tutor.id = relationships.tutor_id
WHERE relationships.id = @id AND relationships.student_id = @student_id AND relationships.state IN ('active', 'provisional', 'ended')
    AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz);

-- name: GetStudentSubmission :one
SELECT submissions.id, submissions.issue_id, submissions.tutor_id, submissions.relationship_id, submissions.status, submissions.late,
    submissions.issued_before_reclaim, submissions.deadline_at, submissions.version, issues.title, issues.snapshot, issues.answer_policy,
    issues.status AS issue_status, (issues.erase_pending_since IS NOT NULL)::boolean AS marked_for_erasure,
    relationships.state AS relationship_state, relationships.colour, tutor.name AS tutor_name, submissions.published_revision_id,
    tutor.state AS tutor_state, tutor.deleted_at AS tutor_deleted_at
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
JOIN relationships ON relationships.id = submissions.relationship_id
JOIN users AS tutor ON tutor.id = submissions.tutor_id
WHERE submissions.id = @id AND submissions.student_id = @student_id;

-- name: GetTutorSubmission :one
SELECT submissions.id, submissions.issue_id, submissions.relationship_id, submissions.status, submissions.late,
    submissions.issued_before_reclaim, submissions.deadline_at, submissions.version, submissions.first_submitted_at,
    submissions.submitted_at, submissions.checked_at, submissions.auto_points, submissions.final_points, issues.title, issues.snapshot,
    issues.max_points, issues.answer_policy, card.display_name
FROM submissions
JOIN issues ON issues.id = submissions.issue_id AND issues.erase_pending_since IS NULL
JOIN relationships AS card ON card.id = submissions.relationship_id
WHERE submissions.id = @id AND submissions.tutor_id = @tutor_id;

-- name: ListSubmissionAnswers :many
SELECT question_id, value, normalised, normaliser_version, auto_points, auto_correct, frozen_at
FROM answers WHERE submission_id = @submission_id ORDER BY question_id;

-- name: ListSubmissionAnswerFiles :many
SELECT attachments.id, attachment_references.question_id, attachments.filename, attachments.media_type::text AS media_type,
    attachments.size_bytes::bigint AS size_bytes, attachments.width, attachments.height, attachment_references.created_at
FROM attachment_references
JOIN attachments ON attachments.id = attachment_references.attachment_id AND attachments.state = 'bound'
WHERE attachment_references.ref_kind = 'answer' AND attachment_references.ref_id = sqlc.arg(submission_id)::uuid
ORDER BY attachment_references.created_at, attachments.id;

-- name: GetStudentSubmissionParties :one
SELECT id, issue_id, tutor_id, relationship_id FROM submissions WHERE id = @id AND student_id = @student_id;

-- name: LockSubmissionRelationship :one
SELECT state FROM relationships WHERE id = @id AND tutor_id = @tutor_id FOR SHARE;

-- name: LockSubmissionIssue :one
SELECT status, (erase_pending_since IS NOT NULL)::boolean AS marked_for_erasure, snapshot, has_manual_questions, max_points, answer_policy, lesson_id
FROM issues WHERE id = @id AND tutor_id = @tutor_id FOR SHARE;

-- name: MarkFeedbackSeen :execrows
UPDATE submissions SET feedback_seen_version = feedback_version
WHERE id = @id AND version = @version AND feedback_seen_version IS DISTINCT FROM feedback_version;

-- name: LockSubmission :one
SELECT status, version, last_save_id, file_bytes, deadline_at, first_submitted_at, submitted_at
FROM submissions WHERE id = @id AND issue_id = @issue_id FOR UPDATE;

-- name: UpsertAnswer :execrows
INSERT INTO answers (submission_id, question_id, value, created_at, updated_at)
VALUES (@submission_id, @question_id, @value, @now, @now)
ON CONFLICT (submission_id, question_id) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
WHERE answers.frozen_at IS NULL;

-- name: FreezeAnswer :execrows
INSERT INTO answers (submission_id, question_id, value, normalised, normaliser_version, auto_points, auto_correct, frozen_at, created_at,
    updated_at)
VALUES (@submission_id, @question_id, @value, sqlc.narg(normalised), sqlc.narg(normaliser_version), sqlc.arg(auto_points)::integer,
    sqlc.arg(auto_correct)::boolean, sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz)
ON CONFLICT (submission_id, question_id) DO UPDATE SET value = excluded.value, normalised = excluded.normalised,
    normaliser_version = excluded.normaliser_version, auto_points = excluded.auto_points, auto_correct = excluded.auto_correct,
    frozen_at = excluded.frozen_at, updated_at = excluded.updated_at
WHERE answers.frozen_at IS NULL;

-- name: SaveSubmission :exec
UPDATE submissions SET status = @status, version = version + 1, last_save_id = @save_id, updated_at = @now WHERE id = @id;

-- name: SubmitSubmission :one
UPDATE submissions
SET status = @status, version = version + 1,
    first_submitted_at = COALESCE(first_submitted_at, sqlc.arg(now)::timestamptz), submitted_at = sqlc.arg(now)::timestamptz,
    late = (deadline_at IS NOT NULL AND COALESCE(first_submitted_at, sqlc.arg(now)::timestamptz) > deadline_at),
    checked_at = CASE WHEN sqlc.arg(checked)::boolean THEN COALESCE(checked_at, sqlc.arg(now)::timestamptz) ELSE checked_at END,
    auto_points = sqlc.arg(auto_points)::integer, final_points = sqlc.arg(final_points)::integer, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id
RETURNING late, first_submitted_at IS DISTINCT FROM submitted_at AS resubmission;

-- name: RefreshSubmissionFileBytes :one
UPDATE submissions SET file_bytes = (
    SELECT COALESCE(sum(attachments.size_bytes), 0)::bigint FROM attachment_references
    JOIN attachments ON attachments.id = attachment_references.attachment_id
    WHERE attachment_references.ref_kind = 'answer' AND attachment_references.ref_id = submissions.id
)
WHERE submissions.id = @id
RETURNING submissions.file_bytes;

-- name: ListAnswerReferences :many
SELECT attachment_references.attachment_id, attachment_references.question_id, attachments.size_bytes::bigint AS size_bytes
FROM attachment_references
JOIN attachments ON attachments.id = attachment_references.attachment_id
WHERE attachment_references.ref_kind = 'answer' AND attachment_references.ref_id = sqlc.arg(submission_id)::uuid
ORDER BY attachment_references.attachment_id;

-- name: LockBindableAnswerUploads :many
SELECT id, scope_question_id, size_bytes::bigint AS size_bytes FROM attachments
WHERE id = ANY (sqlc.arg(ids)::uuid[]) AND uploader_id = @uploader_id AND context = 'answer' AND state = 'pending' AND scope_kind = 'submission'
    AND scope_id = sqlc.arg(submission_id)::uuid AND expires_at > sqlc.arg(now)::timestamptz AND size_bytes IS NOT NULL
ORDER BY id
FOR UPDATE;

-- name: InsertAnswerReferences :exec
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, question_id, created_at)
SELECT bound.attachment_id, 'answer', sqlc.arg(submission_id)::uuid, bound.question_id, sqlc.arg(now)::timestamptz
FROM (
    SELECT unnest(sqlc.arg(attachment_ids)::uuid[]) AS attachment_id, unnest(sqlc.arg(question_ids)::uuid[]) AS question_id
) AS bound;

-- name: RemoveAnswerReferences :many
DELETE FROM attachment_references
WHERE ref_kind = 'answer' AND ref_id = sqlc.arg(submission_id)::uuid AND attachment_id = ANY (sqlc.arg(attachment_ids)::uuid[])
RETURNING attachment_id;

-- name: RemoveIssueAnswerReferences :many
DELETE FROM attachment_references
USING submissions
WHERE attachment_references.ref_kind = 'answer' AND attachment_references.ref_id = submissions.id AND submissions.issue_id = @issue_id
RETURNING attachment_references.attachment_id;

-- name: ClearIssueFileBytes :exec
UPDATE submissions SET file_bytes = 0 WHERE issue_id = @issue_id AND file_bytes <> 0;

-- name: CloseRelationshipSubmissions :many
WITH closed AS (
    UPDATE submissions
    SET status = 'closed', version = version + 1, updated_at = sqlc.arg(now)::timestamptz,
        release_pending_since = CASE WHEN EXISTS (
            SELECT 1 FROM attachment_references
            WHERE attachment_references.ref_kind = 'answer' AND attachment_references.ref_id = submissions.id
                AND (submissions.submitted_at IS NULL OR attachment_references.created_at > submissions.submitted_at)
        ) THEN sqlc.arg(now)::timestamptz ELSE release_pending_since END
    WHERE submissions.relationship_id = @relationship_id AND submissions.status IN ('not_started', 'draft', 'returned')
    RETURNING submissions.id, submissions.issue_id, submissions.student_id
)
SELECT closed.id, closed.student_id, closed.issue_id, issues.lesson_id, issues.status AS issue_status,
    (issues.erase_pending_since IS NOT NULL)::boolean AS marked_for_erasure
FROM closed
JOIN issues ON issues.id = closed.issue_id
ORDER BY closed.id;

-- name: ListReleasePendingSubmissions :many
SELECT id, tutor_id FROM submissions WHERE release_pending_since IS NOT NULL ORDER BY release_pending_since, id LIMIT sqlc.arg(batch_size);

-- name: LockReleasePendingSubmissions :many
SELECT id, submitted_at FROM submissions
WHERE id = ANY (sqlc.arg(ids)::uuid[]) AND tutor_id = @tutor_id AND release_pending_since IS NOT NULL
ORDER BY id
FOR UPDATE SKIP LOCKED;

-- name: RemoveUnsubmittedAnswerReferences :many
DELETE FROM attachment_references
WHERE ref_kind = 'answer' AND ref_id = sqlc.arg(submission_id)::uuid
    AND (sqlc.narg(submitted_at)::timestamptz IS NULL OR created_at > sqlc.narg(submitted_at)::timestamptz)
RETURNING attachment_id;

-- name: ClearReleasePending :exec
UPDATE submissions SET release_pending_since = NULL WHERE id = @id;

-- name: ListTutorAnswerReferencesBatch :many
SELECT attachment_references.attachment_id, attachment_references.ref_id
FROM attachment_references
JOIN submissions ON submissions.id = attachment_references.ref_id
WHERE attachment_references.ref_kind = 'answer' AND submissions.tutor_id = @tutor_id
ORDER BY attachment_references.ref_id, attachment_references.attachment_id
LIMIT sqlc.arg(batch_size);

-- name: RemoveTutorAnswerReferences :many
DELETE FROM attachment_references
USING submissions
WHERE attachment_references.ref_kind = 'answer' AND attachment_references.ref_id = submissions.id AND submissions.tutor_id = @tutor_id
    AND attachment_references.ref_id = ANY (sqlc.arg(submission_ids)::uuid[])
    AND attachment_references.attachment_id = ANY (sqlc.arg(attachment_ids)::uuid[])
RETURNING attachment_references.attachment_id;

-- name: DeleteTutorAnswersBatch :execrows
DELETE FROM answers
WHERE (submission_id, question_id) IN (
    SELECT candidate.submission_id, candidate.question_id FROM answers AS candidate
    JOIN submissions ON submissions.id = candidate.submission_id
    WHERE submissions.tutor_id = @tutor_id
    ORDER BY candidate.submission_id, candidate.question_id
    LIMIT sqlc.arg(batch_size)
);

-- name: RemoveSubmissionsAnswerReferences :many
DELETE FROM attachment_references
WHERE ref_kind = 'answer' AND ref_id = ANY (sqlc.arg(submission_ids)::uuid[])
RETURNING attachment_id;

-- name: DeleteSubmissionsAnswers :execrows
DELETE FROM answers WHERE submission_id = ANY (sqlc.arg(submission_ids)::uuid[]);

-- name: VisibleAnswerReferences :many
SELECT submissions.id, attachments.width
FROM attachment_references
JOIN submissions ON submissions.id = attachment_references.ref_id
JOIN issues ON issues.id = submissions.issue_id AND issues.erase_pending_since IS NULL
JOIN relationships ON relationships.id = submissions.relationship_id
JOIN users AS tutor ON tutor.id = submissions.tutor_id
JOIN attachments ON attachments.id = attachment_references.attachment_id AND attachments.state = 'bound'
WHERE attachment_references.attachment_id = @attachment_id AND attachment_references.ref_kind = 'answer'
    AND (
        (submissions.tutor_id = sqlc.arg(viewer)::uuid AND relationships.state NOT IN ('reclaim_pending', 'discarded'))
        OR (submissions.student_id = sqlc.arg(viewer)::uuid AND issues.status = 'active'
            AND relationships.state IN ('active', 'provisional', 'ended') AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz))
    )
ORDER BY submissions.id;

-- name: GetServableAnswerFile :one
SELECT attachments.storage_key, attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes,
    attachments.filename, attachments.width, attachments.context
FROM attachments
JOIN attachment_references
    ON attachment_references.attachment_id = attachments.id AND attachment_references.ref_kind = 'answer'
        AND attachment_references.ref_id = sqlc.arg(submission_id)::uuid
JOIN submissions ON submissions.id = attachment_references.ref_id
JOIN issues ON issues.id = submissions.issue_id AND issues.erase_pending_since IS NULL
JOIN relationships ON relationships.id = submissions.relationship_id AND relationships.state NOT IN ('reclaim_pending', 'discarded')
JOIN users AS tutor ON tutor.id = submissions.tutor_id
WHERE attachments.id = @id AND attachments.state = 'bound' AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz);

-- name: GetSubmissionForTutorEmail :one
SELECT submissions.id, submissions.tutor_id, submissions.status, submissions.submitted_at, submissions.deadline_at, issues.title,
    (issues.erase_pending_since IS NOT NULL)::boolean AS marked_for_erasure, card.display_name, card.state AS card_state
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
JOIN relationships AS card ON card.id = submissions.relationship_id
WHERE submissions.id = @id;
