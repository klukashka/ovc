-- name: GetIssueByCreateKey :one
SELECT id, erase_pending_since FROM issues WHERE tutor_id = @tutor_id AND create_key = @create_key;

-- name: GetTutorIssueRelationship :one
SELECT id, state, student_id, display_name FROM relationships WHERE id = @id AND tutor_id = @tutor_id AND state <> 'discarded';

-- name: LockIssueRelationships :many
SELECT id, state, student_id FROM relationships
WHERE tutor_id = @tutor_id AND id = ANY (sqlc.arg(ids)::uuid[])
ORDER BY id
FOR SHARE;

-- name: GetTutorIssueGroup :one
SELECT id, name, archived_at FROM groups WHERE id = @id AND tutor_id = @tutor_id;

-- name: ListIssueGroupMembers :many
SELECT relationship_id FROM group_members WHERE group_id = @group_id AND tutor_id = @tutor_id ORDER BY relationship_id;

-- name: LockIssueGroup :one
SELECT id, archived_at FROM groups WHERE id = @id AND tutor_id = @tutor_id FOR SHARE;

-- name: LockIssueLesson :one
SELECT id FROM lessons
WHERE id = @id AND tutor_id = @tutor_id
    AND (relationship_id = sqlc.narg(relationship_id)::uuid OR group_id = sqlc.narg(group_id)::uuid)
FOR SHARE;

-- name: CountLessonIssues :one
SELECT count(id) FROM issues
WHERE tutor_id = @tutor_id AND lesson_id = @lesson_id AND status = 'active' AND erase_pending_since IS NULL;

-- name: GetSnapshotBytesTotal :one
SELECT snapshot_bytes_total FROM tutor_profiles WHERE user_id = @tutor_id;

-- name: AddSnapshotBytes :exec
UPDATE tutor_profiles SET snapshot_bytes_total = snapshot_bytes_total + sqlc.arg(bytes)::bigint WHERE user_id = @tutor_id;

-- name: SubtractSnapshotBytes :exec
UPDATE tutor_profiles SET snapshot_bytes_total = greatest(snapshot_bytes_total - sqlc.arg(bytes)::bigint, 0) WHERE user_id = @tutor_id;

-- name: InsertIssue :exec
INSERT INTO issues (
    id, tutor_id, template_id, relationship_id, group_id, lesson_id, title, snapshot, snapshot_bytes, max_points, has_manual_questions,
    deadline_at, answer_policy, status, create_key, version, issued_at, updated_at
) VALUES (
    @id, @tutor_id, @template_id, sqlc.narg(relationship_id), sqlc.narg(group_id), sqlc.narg(lesson_id), @title, @snapshot, @snapshot_bytes,
    @max_points, @has_manual_questions, sqlc.narg(deadline_at), @answer_policy, 'active', sqlc.narg(create_key), 1, @now, @now
);

-- name: InsertSubmissions :exec
INSERT INTO submissions (id, issue_id, tutor_id, relationship_id, student_id, status, deadline_at, issued_at, version, created_at, updated_at)
SELECT submission.id, @issue_id, @tutor_id, submission.relationship_id, submission.student_id, 'not_started', sqlc.narg(deadline_at),
    @now, 1, @now, @now
FROM (
    SELECT unnest(sqlc.arg(ids)::uuid[]) AS id, unnest(sqlc.arg(relationship_ids)::uuid[]) AS relationship_id,
        unnest(sqlc.arg(student_ids)::uuid[]) AS student_id
) AS submission;

-- name: AddIssueSnapshotReferences :exec
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, question_id, created_at)
SELECT attachment_id, 'issue_snapshot', sqlc.arg(issue_id)::uuid, NULL, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(attachment_ids)::uuid[]) AS attachment_id;

-- name: GetTutorIssue :one
SELECT issues.id, issues.template_id, issues.relationship_id, issues.group_id, issues.lesson_id, issues.title, issues.snapshot,
    issues.max_points, issues.has_manual_questions, issues.deadline_at, issues.answer_policy, issues.status, issues.version,
    issues.issued_at, issues.withdrawn_at, card.display_name AS card_name, issue_group.name AS group_name
FROM issues
LEFT JOIN relationships AS card ON card.id = issues.relationship_id
LEFT JOIN groups AS issue_group ON issue_group.id = issues.group_id
WHERE issues.id = @id AND issues.tutor_id = @tutor_id AND issues.erase_pending_since IS NULL;

-- name: LockTutorIssue :one
SELECT id, relationship_id, group_id, lesson_id, deadline_at, status, version FROM issues
WHERE id = @id AND tutor_id = @tutor_id AND erase_pending_since IS NULL
FOR UPDATE;

-- name: ListIssueSubmissionRelationships :many
SELECT DISTINCT relationship_id FROM submissions WHERE issue_id = @issue_id ORDER BY relationship_id;

-- name: ListIssueSubmissionStates :many
SELECT submissions.id, submissions.student_id, submissions.relationship_id, submissions.first_submitted_at, relationships.state AS relationship_state
FROM submissions
JOIN relationships ON relationships.id = submissions.relationship_id
WHERE submissions.issue_id = @issue_id
ORDER BY submissions.id;

-- name: UpdateIssue :exec
UPDATE issues SET deadline_at = sqlc.narg(deadline_at), lesson_id = sqlc.narg(lesson_id), version = version + 1, updated_at = @now
WHERE id = @id AND tutor_id = @tutor_id;

-- name: SetSubmissionDeadlines :exec
UPDATE submissions
SET deadline_at = sqlc.narg(deadline_at),
    late = (first_submitted_at IS NOT NULL AND sqlc.narg(deadline_at)::timestamptz IS NOT NULL AND first_submitted_at > sqlc.narg(deadline_at)::timestamptz),
    updated_at = @now
WHERE issue_id = @issue_id;

-- name: WithdrawIssue :exec
UPDATE issues SET status = 'withdrawn', withdrawn_at = sqlc.arg(now)::timestamptz, version = version + 1, updated_at = sqlc.arg(now)::timestamptz WHERE id = @id AND tutor_id = @tutor_id;

-- name: ListTutorIssues :many
SELECT issues.id, issues.relationship_id, issues.group_id, issues.lesson_id, issues.title, issues.max_points, issues.has_manual_questions,
    issues.deadline_at, issues.answer_policy, issues.status, issues.version, issues.issued_at, issues.withdrawn_at,
    card.display_name AS card_name, issue_group.name AS group_name
FROM issues
LEFT JOIN relationships AS card ON card.id = issues.relationship_id
LEFT JOIN groups AS issue_group ON issue_group.id = issues.group_id
WHERE issues.tutor_id = @tutor_id AND issues.erase_pending_since IS NULL AND issues.status = @status
    AND (sqlc.narg(group_id)::uuid IS NULL OR issues.group_id = sqlc.narg(group_id)::uuid)
    AND (
        sqlc.narg(relationship_id)::uuid IS NULL
        OR EXISTS (SELECT 1 FROM submissions WHERE submissions.issue_id = issues.id AND submissions.relationship_id = sqlc.narg(relationship_id)::uuid)
    )
    AND (
        NOT sqlc.arg(needs_checking)::boolean
        OR (issues.has_manual_questions AND EXISTS (SELECT 1 FROM submissions WHERE submissions.issue_id = issues.id AND submissions.status = 'submitted'))
    )
    AND (
        sqlc.narg(after_issued_at)::timestamptz IS NULL
        OR (issues.issued_at, issues.id) < (sqlc.narg(after_issued_at)::timestamptz, sqlc.arg(after_id)::uuid)
    )
ORDER BY issues.issued_at DESC, issues.id DESC
LIMIT sqlc.arg(page_size);

-- name: ListTutorIssuesByDeadline :many
SELECT issues.id, issues.relationship_id, issues.group_id, issues.lesson_id, issues.title, issues.max_points, issues.has_manual_questions,
    issues.deadline_at, issues.answer_policy, issues.status, issues.version, issues.issued_at, issues.withdrawn_at,
    card.display_name AS card_name, issue_group.name AS group_name
FROM issues
LEFT JOIN relationships AS card ON card.id = issues.relationship_id
LEFT JOIN groups AS issue_group ON issue_group.id = issues.group_id
WHERE issues.tutor_id = @tutor_id AND issues.erase_pending_since IS NULL AND issues.status = 'active'
    AND issues.deadline_at > sqlc.arg(now)::timestamptz AND issues.deadline_at < sqlc.arg(deadline_before)::timestamptz
    AND (sqlc.narg(group_id)::uuid IS NULL OR issues.group_id = sqlc.narg(group_id)::uuid)
    AND (
        sqlc.narg(relationship_id)::uuid IS NULL
        OR EXISTS (SELECT 1 FROM submissions WHERE submissions.issue_id = issues.id AND submissions.relationship_id = sqlc.narg(relationship_id)::uuid)
    )
    AND (
        NOT sqlc.arg(needs_checking)::boolean
        OR (issues.has_manual_questions AND EXISTS (SELECT 1 FROM submissions WHERE submissions.issue_id = issues.id AND submissions.status = 'submitted'))
    )
    AND (
        sqlc.narg(after_deadline_at)::timestamptz IS NULL
        OR (issues.deadline_at, issues.id) > (sqlc.narg(after_deadline_at)::timestamptz, sqlc.arg(after_id)::uuid)
    )
ORDER BY issues.deadline_at, issues.id
LIMIT sqlc.arg(page_size);

-- name: IssueProgress :many
SELECT submissions.issue_id, count(submissions.id) AS students, count(submissions.first_submitted_at) AS submitted,
    count(submissions.id) FILTER (WHERE submissions.status = 'checked') AS checked,
    count(submissions.id) FILTER (WHERE submissions.status = 'returned') AS returned,
    count(submissions.id) FILTER (WHERE submissions.late) AS late,
    count(submissions.id) FILTER (WHERE submissions.status = 'submitted' AND issues.has_manual_questions) AS needs_checking
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
WHERE submissions.tutor_id = @tutor_id AND submissions.issue_id = ANY (sqlc.arg(issue_ids)::uuid[])
GROUP BY submissions.issue_id;

-- name: ListRelationshipIssueSubmissions :many
SELECT id, issue_id, status, late, final_points, issued_before_reclaim FROM submissions
WHERE tutor_id = @tutor_id AND relationship_id = @relationship_id AND issue_id = ANY (sqlc.arg(issue_ids)::uuid[]);

-- name: ListTutorIssueSubmissions :many
SELECT submissions.id, submissions.relationship_id, card.display_name, submissions.status, submissions.late, submissions.first_submitted_at,
    submissions.submitted_at, submissions.checked_at, submissions.final_points, submissions.issued_before_reclaim,
    EXISTS (SELECT 1 FROM review_revisions WHERE review_revisions.submission_id = submissions.id AND review_revisions.state = 'draft')::boolean
        AS has_draft
FROM submissions
JOIN relationships AS card ON card.id = submissions.relationship_id
WHERE submissions.issue_id = @issue_id AND submissions.tutor_id = @tutor_id
ORDER BY card.display_name, submissions.id;

-- name: GetTutorIssueGroupScope :one
SELECT id FROM groups WHERE id = @id AND tutor_id = @tutor_id;

-- name: GetTutorIssueStudentScope :one
SELECT id FROM relationships WHERE id = @id AND tutor_id = @tutor_id AND state <> 'discarded';

-- name: VisibleIssueSnapshotReferences :many
SELECT issues.id, attachments.width
FROM attachment_references
JOIN issues ON issues.id = attachment_references.ref_id AND issues.erase_pending_since IS NULL
JOIN attachments ON attachments.id = attachment_references.attachment_id AND attachments.state = 'bound'
WHERE attachment_references.attachment_id = @attachment_id AND attachment_references.ref_kind = 'issue_snapshot'
    AND (
        issues.tutor_id = sqlc.arg(viewer)::uuid
        OR (
            issues.status = 'active'
            AND EXISTS (
                SELECT 1 FROM submissions
                JOIN relationships ON relationships.id = submissions.relationship_id
                WHERE submissions.issue_id = issues.id AND submissions.student_id = sqlc.arg(viewer)::uuid
                    AND relationships.state IN ('active', 'provisional', 'ended')
            )
        )
    )
ORDER BY issues.id;

-- name: GetServableIssueSnapshotFile :one
SELECT attachments.storage_key, attachments.media_type::text AS media_type, attachments.size_bytes::bigint AS size_bytes,
    attachments.filename, attachments.width, attachments.context
FROM attachments
JOIN attachment_references
    ON attachment_references.attachment_id = attachments.id AND attachment_references.ref_kind = 'issue_snapshot'
        AND attachment_references.ref_id = sqlc.arg(issue_id)::uuid
JOIN issues ON issues.id = attachment_references.ref_id AND issues.erase_pending_since IS NULL
WHERE attachments.id = @id AND attachments.state = 'bound';

-- name: ReadableStudentSubmissionTitles :many
SELECT submissions.id, issues.title
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
JOIN relationships ON relationships.id = submissions.relationship_id
WHERE submissions.student_id = @student_id AND submissions.id = ANY (sqlc.arg(ids)::uuid[])
    AND issues.status = 'active' AND issues.erase_pending_since IS NULL
    AND relationships.state IN ('active', 'provisional', 'ended');

-- name: ListLessonIssues :many
SELECT id, title, deadline_at FROM issues
WHERE tutor_id = @tutor_id AND lesson_id = @lesson_id AND status = 'active' AND erase_pending_since IS NULL
ORDER BY issued_at, id
LIMIT sqlc.arg(page_size);

-- name: ListStudentLessonSubmissions :many
SELECT submissions.id, issues.title, submissions.deadline_at, submissions.status, submissions.late
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
JOIN relationships ON relationships.id = submissions.relationship_id
WHERE submissions.student_id = @student_id AND issues.lesson_id = @lesson_id
    AND issues.status = 'active' AND issues.erase_pending_since IS NULL
    AND relationships.state IN ('active', 'provisional', 'ended')
ORDER BY issues.issued_at, issues.id
LIMIT sqlc.arg(page_size);

-- name: TutorReviewSubmissions :many
SELECT submissions.id, card.display_name, card.state AS card_state, (issues.erase_pending_since IS NULL)::boolean AS issue_readable
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
JOIN relationships AS card ON card.id = submissions.relationship_id
WHERE submissions.tutor_id = @tutor_id AND submissions.id = ANY (sqlc.arg(ids)::uuid[]);

-- name: GetSubmissionForEmail :one
SELECT submissions.id, submissions.student_id, submissions.status, submissions.deadline_at, issues.title, issues.status AS issue_status,
    (issues.erase_pending_since IS NOT NULL)::boolean AS marked_for_erasure, relationships.state AS relationship_state,
    tutor.name AS tutor_name, tutor.state AS tutor_state
FROM submissions
JOIN issues ON issues.id = submissions.issue_id
JOIN relationships ON relationships.id = submissions.relationship_id
JOIN users AS tutor ON tutor.id = submissions.tutor_id
WHERE submissions.id = @id;

-- name: FlagReclaimedSubmissions :many
WITH flagged AS (
    UPDATE submissions SET issued_before_reclaim = true, updated_at = @now
    WHERE relationship_id = ANY (sqlc.arg(relationship_ids)::uuid[]) AND NOT issued_before_reclaim
    RETURNING id, tutor_id, issue_id
)
SELECT flagged.id, flagged.tutor_id, flagged.issue_id, issues.lesson_id
FROM flagged JOIN issues ON issues.id = flagged.issue_id
ORDER BY flagged.id;

-- name: ListTutorIssueSnapshotReferencesBatch :many
SELECT attachment_references.attachment_id, attachment_references.ref_id
FROM attachment_references
JOIN issues ON issues.id = attachment_references.ref_id
WHERE attachment_references.ref_kind = 'issue_snapshot' AND issues.tutor_id = @tutor_id
ORDER BY attachment_references.ref_id, attachment_references.attachment_id
LIMIT sqlc.arg(batch_size);

-- name: RemoveTutorIssueSnapshotReferences :many
DELETE FROM attachment_references
USING issues
WHERE attachment_references.ref_kind = 'issue_snapshot' AND attachment_references.ref_id = issues.id AND issues.tutor_id = @tutor_id
    AND attachment_references.ref_id = ANY (sqlc.arg(issue_ids)::uuid[])
    AND attachment_references.attachment_id = ANY (sqlc.arg(attachment_ids)::uuid[])
RETURNING attachment_references.attachment_id;

-- name: DeleteTutorSubmissionsBatch :execrows
DELETE FROM submissions
WHERE id IN (SELECT candidate.id FROM submissions AS candidate WHERE candidate.tutor_id = @tutor_id ORDER BY candidate.id LIMIT sqlc.arg(batch_size));

-- name: DeleteTutorIssuesBatch :many
DELETE FROM issues
WHERE id IN (
    SELECT candidate.id FROM issues AS candidate
    WHERE candidate.tutor_id = @tutor_id
        AND NOT EXISTS (SELECT 1 FROM submissions WHERE submissions.issue_id = candidate.id)
        AND NOT EXISTS (
            SELECT 1 FROM attachment_references WHERE attachment_references.ref_kind = 'issue_snapshot' AND attachment_references.ref_id = candidate.id
        )
    ORDER BY candidate.id
    LIMIT sqlc.arg(batch_size)
)
RETURNING snapshot_bytes;

-- name: ListStudentSubmissionsBatch :many
SELECT submissions.id, submissions.tutor_id FROM submissions
JOIN relationships ON relationships.id = submissions.relationship_id
WHERE submissions.student_id = @student_id AND relationships.state NOT IN ('provisional', 'reclaim_pending')
ORDER BY submissions.id LIMIT sqlc.arg(batch_size);

-- name: DeleteSubmissions :execrows
DELETE FROM submissions WHERE id = ANY (sqlc.arg(ids)::uuid[]);

-- name: CountTutorIssuesByDeadline :one
SELECT count(*)::bigint FROM (
    SELECT 1 FROM issues
    WHERE issues.tutor_id = @tutor_id AND issues.erase_pending_since IS NULL AND issues.status = 'active'
        AND issues.deadline_at > sqlc.arg(now)::timestamptz AND issues.deadline_at < sqlc.arg(deadline_before)::timestamptz
    LIMIT sqlc.arg(count_cap)
) AS upcoming;
