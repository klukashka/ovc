package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type CleanupIssue = dbgen.LockCleanupIssueRow
type CleanupSubmission = dbgen.LockCleanupSubmissionRow
type EraseCandidate = dbgen.ListErasePendingIssuesRow
type ReleaseCandidate = dbgen.ListCleanupReleasePendingRow
type MarkedIssue = dbgen.MarkLifecycleIssuesRow
type LifecycleSubmission = dbgen.ListLifecycleSubmissionsRow
type LifecycleClosedSubmission = dbgen.CloseLifecycleSubmissionsRow

func (r Homework) EraseCandidates(ctx context.Context, skipped []domain.IssueID) ([]EraseCandidate, error) {
	rows, err := r.queries.ListErasePendingIssues(ctx, uuids(skipped))
	return one(rows, err, "probe marked issues")
}

func (r Homework) ReleaseCandidates(ctx context.Context, skipped []domain.SubmissionID) ([]ReleaseCandidate, error) {
	rows, err := r.queries.ListCleanupReleasePending(ctx, uuids(skipped))
	return one(rows, err, "probe pending releases")
}

func (r Homework) LockCleanupIssue(ctx context.Context, tutor domain.UserID, issue domain.IssueID) (CleanupIssue, error) {
	row, err := r.queries.LockCleanupIssue(ctx, dbgen.LockCleanupIssueParams{ID: issue, TutorID: tutor})
	return one(row, err, "lock cleanup issue")
}

func (r Homework) LockCleanupSubmission(ctx context.Context, issue domain.IssueID, submission *domain.SubmissionID) (CleanupSubmission, error) {
	id := pgtype.UUID{}
	if submission != nil {
		id = pgtype.UUID{Bytes: *submission, Valid: true}
	}
	row, err := r.queries.LockCleanupSubmission(ctx, dbgen.LockCleanupSubmissionParams{IssueID: issue, SubmissionID: id})
	return one(row, err, "lock cleanup submission")
}

func (r Homework) LockCleanupRevision(ctx context.Context, submission domain.SubmissionID) (domain.ReviewRevisionID, error) {
	row, err := r.queries.LockCleanupRevision(ctx, submission)
	return one(row, err, "lock cleanup revision")
}

func (r Homework) RemoveCleanupReferences(ctx context.Context, kind domain.ReferenceKind, id domain.EntityID, after *time.Time, batch int32) ([]domain.AttachmentID, error) {
	rows, err := r.queries.RemoveCleanupReferences(ctx, dbgen.RemoveCleanupReferencesParams{RefKind: kind, RefID: id, AfterTime: after, BatchSize: batch})
	return one(rows, err, "remove cleanup references")
}

func (r Homework) DeleteCleanupRevision(ctx context.Context, id domain.ReviewRevisionID) (int64, error) {
	count, err := r.queries.DeleteCleanupRevision(ctx, id)
	return one(count, err, "erase cleanup revision")
}

func (r Homework) DeleteCleanupAnswers(ctx context.Context, id domain.SubmissionID, batch int32) (int64, error) {
	count, err := r.queries.DeleteCleanupAnswers(ctx, dbgen.DeleteCleanupAnswersParams{SubmissionID: id, BatchSize: batch})
	return one(count, err, "erase cleanup answers")
}

func (r Homework) DeleteCleanupNotifications(ctx context.Context, id domain.SubmissionID, batch int32) (int64, error) {
	count, err := r.queries.DeleteCleanupNotifications(ctx, dbgen.DeleteCleanupNotificationsParams{SubjectID: domain.EntityID(id), BatchSize: batch})
	return one(count, err, "erase cleanup notification subjects")
}

func (r Homework) DeleteCleanupSubmission(ctx context.Context, id domain.SubmissionID) (int64, error) {
	count, err := r.queries.DeleteCleanupSubmission(ctx, id)
	return one(count, err, "erase cleanup submission")
}

func (r Homework) DeleteCleanupIssue(ctx context.Context, id domain.IssueID) (int64, error) {
	count, err := r.queries.DeleteCleanupIssue(ctx, id)
	return one(count, err, "erase cleanup issue")
}

func (r Homework) LockLifecycle(ctx context.Context, relationships []domain.RelationshipID) error {
	if _, err := r.queries.LockLifecycleIssues(ctx, uuids(relationships)); err != nil {
		return done(err, "lock lifecycle issues")
	}
	_, err := r.queries.LockLifecycleSubmissions(ctx, uuids(relationships))
	return done(err, "lock lifecycle submissions")
}

func (r Homework) MarkLifecycle(ctx context.Context, relationships []domain.RelationshipID, now time.Time) ([]MarkedIssue, error) {
	rows, err := r.queries.MarkLifecycleIssues(ctx, dbgen.MarkLifecycleIssuesParams{Relationships: uuids(relationships), Now: now})
	return one(rows, err, "mark lifecycle issues")
}

func (r Homework) LifecycleSubmissions(ctx context.Context, issues []domain.IssueID) ([]LifecycleSubmission, error) {
	rows, err := r.queries.ListLifecycleSubmissions(ctx, uuids(issues))
	return one(rows, err, "read lifecycle submissions")
}

func (r Homework) CloseLifecycle(ctx context.Context, relationships []domain.RelationshipID, now time.Time) ([]LifecycleClosedSubmission, error) {
	rows, err := r.queries.CloseLifecycleSubmissions(ctx, dbgen.CloseLifecycleSubmissionsParams{Relationships: uuids(relationships), Now: now})
	return one(rows, err, "close lifecycle submissions")
}

func (r Homework) IssueOfSubmission(ctx context.Context, id domain.SubmissionID) (domain.IssueID, error) {
	issue, err := r.queries.GetCleanupSubmissionIssue(ctx, id)
	return one(issue, err, "find cleanup submission issue")
}

func (r Homework) LockStudentPurge(ctx context.Context, submissions []domain.SubmissionID) ([]domain.SubmissionID, []dbgen.LockStudentPurgeRevisionsRow, error) {
	ids := uuids(submissions)
	if _, err := r.queries.LockStudentPurgeIssues(ctx, ids); err != nil {
		return nil, nil, done(err, "lock student purge issues")
	}
	locked, err := r.queries.LockStudentPurgeSubmissions(ctx, ids)
	if err != nil {
		return nil, nil, done(err, "lock student purge submissions")
	}
	revisions, err := r.queries.LockStudentPurgeRevisions(ctx, ids)
	return locked, revisions, done(err, "lock student purge revisions")
}
