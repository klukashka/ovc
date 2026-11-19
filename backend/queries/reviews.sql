-- name: GetTutorSubmissionParties :one
SELECT submissions.id, submissions.issue_id, submissions.relationship_id, submissions.student_id
FROM submissions
JOIN issues ON issues.id = submissions.issue_id AND issues.erase_pending_since IS NULL
WHERE submissions.id = @id AND submissions.tutor_id = @tutor_id;

-- name: LockReviewDraft :one
SELECT id, revision, feedback, items, version, last_save_id
FROM review_revisions WHERE submission_id = @submission_id AND state = 'draft'
FOR UPDATE;

-- name: GetLatestPublishedRevision :one
SELECT id, revision, feedback, items, action, published_at
FROM review_revisions WHERE submission_id = @submission_id AND state = 'published'
ORDER BY revision DESC
LIMIT 1;

-- name: GetPublishedRevision :one
SELECT id, revision, feedback, items FROM review_revisions WHERE id = @id AND state = 'published';

-- name: CountSubmissionRevisions :one
SELECT count(*)::bigint FROM review_revisions WHERE submission_id = @submission_id;

-- name: InsertReviewDraft :exec
INSERT INTO review_revisions (id, submission_id, tutor_id, revision, state, feedback, items, version, last_save_id, created_at, updated_at)
VALUES (@id, @submission_id, @tutor_id, @revision, 'draft', @feedback, @items, 1, @save_id, @now, @now);

-- name: UpdateReviewDraft :exec
UPDATE review_revisions SET feedback = @feedback, items = @items, version = version + 1, last_save_id = @save_id, updated_at = @now
WHERE id = @id AND state = 'draft';

-- name: PublishReviewRevision :execrows
UPDATE review_revisions SET state = 'published', action = @action, published_at = sqlc.arg(now)::timestamptz,
    updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id AND state = 'draft';

-- name: ListSubmissionRevisions :many
SELECT id, revision, state, action, feedback, items, version, published_at
FROM review_revisions WHERE submission_id = @submission_id
ORDER BY revision;

-- name: ListReviewItemFiles :many
SELECT attachment_references.ref_id, attachment_references.question_id, attachments.id, attachments.filename,
    attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes, attachments.width, attachments.height
FROM attachment_references
JOIN attachments ON attachments.id = attachment_references.attachment_id AND attachments.state = 'bound'
WHERE attachment_references.ref_kind = 'review_item' AND attachment_references.ref_id = ANY (sqlc.arg(revision_ids)::uuid[])
ORDER BY attachment_references.ref_id, attachment_references.created_at, attachments.id;

-- name: ListReviewItemReferences :many
SELECT attachment_id, question_id FROM attachment_references
WHERE ref_kind = 'review_item' AND ref_id = sqlc.arg(revision_id)::uuid
ORDER BY attachment_id;

-- name: CopyReviewItemReferences :exec
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, question_id, created_at)
SELECT attachment_id, 'review_item', sqlc.arg(to_revision)::uuid, question_id, created_at
FROM attachment_references
WHERE ref_kind = 'review_item' AND ref_id = sqlc.arg(from_revision)::uuid;

-- name: InsertReviewItemReferences :exec
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, question_id, created_at)
SELECT bound.attachment_id, 'review_item', sqlc.arg(revision_id)::uuid, bound.question_id, sqlc.arg(now)::timestamptz
FROM (
    SELECT unnest(sqlc.arg(attachment_ids)::uuid[]) AS attachment_id, unnest(sqlc.arg(question_ids)::uuid[]) AS question_id
) AS bound;

-- name: RemoveReviewItemReferences :many
DELETE FROM attachment_references
WHERE ref_kind = 'review_item' AND ref_id = sqlc.arg(revision_id)::uuid AND attachment_id = ANY (sqlc.arg(attachment_ids)::uuid[])
RETURNING attachment_id;

-- name: LockBindableCorrectedUploads :many
SELECT id, scope_question_id FROM attachments
WHERE id = ANY (sqlc.arg(ids)::uuid[]) AND uploader_id = @uploader_id AND context = 'corrected' AND state = 'pending'
    AND scope_kind = 'submission' AND scope_id = sqlc.arg(submission_id)::uuid AND expires_at > sqlc.arg(now)::timestamptz
    AND size_bytes IS NOT NULL
ORDER BY id
FOR UPDATE;

-- name: PublishSubmissionReview :exec
UPDATE submissions
SET status = @status,
    checked_at = CASE WHEN sqlc.arg(checked)::boolean THEN COALESCE(checked_at, sqlc.arg(now)::timestamptz) ELSE checked_at END,
    final_points = sqlc.arg(final_points)::integer, published_revision_id = sqlc.arg(revision_id)::uuid,
    version = version + CASE WHEN sqlc.arg(visible)::boolean THEN 1 ELSE 0 END,
    feedback_version = CASE WHEN sqlc.arg(visible)::boolean THEN version + 1 ELSE feedback_version END, updated_at = sqlc.arg(now)::timestamptz
WHERE id = @id;

-- name: ListReviewQueue :many
SELECT submissions.id, submissions.issue_id, issues.title, submissions.relationship_id, card.display_name, submissions.submitted_at,
    submissions.first_submitted_at, submissions.late, submissions.issued_before_reclaim,
    EXISTS (SELECT 1 FROM review_revisions WHERE review_revisions.submission_id = submissions.id AND review_revisions.state = 'draft')::boolean
        AS has_draft
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
JOIN relationships AS card ON card.id = submissions.relationship_id
WHERE submissions.tutor_id = @tutor_id AND submissions.status = 'submitted' AND issues.has_manual_questions
    AND issues.erase_pending_since IS NULL
    AND (sqlc.narg(issue_id)::uuid IS NULL OR submissions.issue_id = sqlc.narg(issue_id)::uuid)
    AND (sqlc.narg(group_id)::uuid IS NULL OR issues.group_id = sqlc.narg(group_id)::uuid)
    AND (sqlc.narg(relationship_id)::uuid IS NULL OR submissions.relationship_id = sqlc.narg(relationship_id)::uuid)
    AND (
        sqlc.narg(after_first_submitted_at)::timestamptz IS NULL
        OR (submissions.first_submitted_at, submissions.id) > (sqlc.narg(after_first_submitted_at)::timestamptz, sqlc.arg(after_id)::uuid)
    )
ORDER BY submissions.first_submitted_at, submissions.id
LIMIT sqlc.arg(page_size);

-- name: CountReviewQueue :one
SELECT count(*)::bigint FROM (
    SELECT 1 FROM submissions
    JOIN issues ON issues.id = submissions.issue_id
    WHERE submissions.tutor_id = @tutor_id AND submissions.status = 'submitted' AND issues.has_manual_questions
        AND issues.erase_pending_since IS NULL
        AND (sqlc.narg(issue_id)::uuid IS NULL OR submissions.issue_id = sqlc.narg(issue_id)::uuid)
        AND (sqlc.narg(group_id)::uuid IS NULL OR issues.group_id = sqlc.narg(group_id)::uuid)
        AND (sqlc.narg(relationship_id)::uuid IS NULL OR submissions.relationship_id = sqlc.narg(relationship_id)::uuid)
    LIMIT sqlc.arg(count_cap)
) AS queued;

-- name: GetTutorReviewIssueScope :one
SELECT id FROM issues WHERE id = @id AND tutor_id = @tutor_id AND erase_pending_since IS NULL;

-- name: ListDraftSubmissions :many
SELECT submission_id FROM review_revisions WHERE state = 'draft' AND submission_id = ANY (sqlc.arg(submission_ids)::uuid[]);

-- name: ListPublishedReviewSummaries :many
SELECT review_revisions.id, review_revisions.feedback, review_revisions.items,
    EXISTS (
        SELECT 1 FROM attachment_references
        JOIN attachments ON attachments.id = attachment_references.attachment_id AND attachments.state = 'bound'
        WHERE attachment_references.ref_kind = 'review_item' AND attachment_references.ref_id = review_revisions.id
    )::boolean AS has_corrected_files
FROM review_revisions
WHERE review_revisions.id = ANY (sqlc.arg(revision_ids)::uuid[]) AND review_revisions.state = 'published';

-- name: VisibleReviewItemReferences :many
SELECT review_revisions.id, attachments.width
FROM attachment_references
JOIN review_revisions ON review_revisions.id = attachment_references.ref_id
JOIN submissions ON submissions.id = review_revisions.submission_id
JOIN issues ON issues.id = submissions.issue_id AND issues.erase_pending_since IS NULL
JOIN relationships ON relationships.id = submissions.relationship_id
JOIN users AS tutor ON tutor.id = submissions.tutor_id
JOIN attachments ON attachments.id = attachment_references.attachment_id AND attachments.state = 'bound'
WHERE attachment_references.attachment_id = @attachment_id AND attachment_references.ref_kind = 'review_item'
    AND (
        submissions.tutor_id = sqlc.arg(viewer)::uuid
        OR (submissions.student_id = sqlc.arg(viewer)::uuid AND issues.status = 'active'
            AND relationships.state IN ('active', 'provisional', 'ended') AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz)
            AND review_revisions.state = 'published' AND submissions.published_revision_id = review_revisions.id)
    )
ORDER BY review_revisions.id;

-- name: GetServableReviewItemFile :one
SELECT attachments.storage_key, attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes,
    attachments.filename, attachments.width, attachments.context
FROM attachments
JOIN attachment_references
    ON attachment_references.attachment_id = attachments.id AND attachment_references.ref_kind = 'review_item'
        AND attachment_references.ref_id = sqlc.arg(revision_id)::uuid
JOIN review_revisions ON review_revisions.id = attachment_references.ref_id
JOIN submissions ON submissions.id = review_revisions.submission_id
JOIN issues ON issues.id = submissions.issue_id AND issues.erase_pending_since IS NULL
JOIN users AS tutor ON tutor.id = submissions.tutor_id
WHERE attachments.id = @id AND attachments.state = 'bound' AND (tutor.state <> 'deleted' OR tutor.deleted_at > sqlc.arg(tutor_deleted_after)::timestamptz)
LIMIT 1;

-- name: ListTutorReviewItemReferencesBatch :many
SELECT attachment_references.attachment_id, attachment_references.ref_id
FROM attachment_references
JOIN review_revisions ON review_revisions.id = attachment_references.ref_id
WHERE attachment_references.ref_kind = 'review_item' AND review_revisions.tutor_id = @tutor_id
ORDER BY attachment_references.ref_id, attachment_references.attachment_id
LIMIT sqlc.arg(batch_size);

-- name: RemoveTutorReviewItemReferences :many
DELETE FROM attachment_references
USING review_revisions
WHERE attachment_references.ref_kind = 'review_item' AND attachment_references.ref_id = review_revisions.id
    AND review_revisions.tutor_id = @tutor_id
    AND attachment_references.ref_id = ANY (sqlc.arg(revision_ids)::uuid[])
    AND attachment_references.attachment_id = ANY (sqlc.arg(attachment_ids)::uuid[])
RETURNING attachment_references.attachment_id;

-- name: DeleteTutorReviewRevisionsBatch :execrows
DELETE FROM review_revisions
WHERE id IN (
    SELECT candidate.id FROM review_revisions AS candidate
    WHERE candidate.tutor_id = @tutor_id
        AND NOT EXISTS (
            SELECT 1 FROM attachment_references
            WHERE attachment_references.ref_kind = 'review_item' AND attachment_references.ref_id = candidate.id
        )
    ORDER BY candidate.id
    LIMIT sqlc.arg(batch_size)
);

-- name: RemoveSubmissionsReviewItemReferences :many
DELETE FROM attachment_references
USING review_revisions
WHERE attachment_references.ref_kind = 'review_item' AND attachment_references.ref_id = review_revisions.id
    AND review_revisions.submission_id = ANY (sqlc.arg(submission_ids)::uuid[])
RETURNING attachment_references.attachment_id;

-- name: DeleteSubmissionsReviewRevisions :execrows
DELETE FROM review_revisions WHERE submission_id = ANY (sqlc.arg(submission_ids)::uuid[]);
