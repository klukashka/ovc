package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	IssueByKey               = dbgen.GetIssueByCreateKeyRow
	IssueRelationship        = dbgen.GetTutorIssueRelationshipRow
	LockedIssueRelationship  = dbgen.LockIssueRelationshipsRow
	IssueGroup               = dbgen.GetTutorIssueGroupRow
	LockedIssueGroup         = dbgen.LockIssueGroupRow
	TutorIssue               = dbgen.GetTutorIssueRow
	LockedIssue              = dbgen.LockTutorIssueRow
	IssueSubmissionState     = dbgen.ListIssueSubmissionStatesRow
	IssueListRow             = dbgen.ListTutorIssuesRow
	IssueProgress            = dbgen.IssueProgressRow
	RelationshipSubmission   = dbgen.ListRelationshipIssueSubmissionsRow
	IssueSubmission          = dbgen.ListTutorIssueSubmissionsRow
	VisibleIssueSnapshot     = dbgen.VisibleIssueSnapshotReferencesRow
	SubmissionTitle          = dbgen.ReadableStudentSubmissionTitlesRow
	LessonIssue              = dbgen.ListLessonIssuesRow
	LessonSubmission         = dbgen.ListStudentLessonSubmissionsRow
	ReviewSubmission         = dbgen.TutorReviewSubmissionsRow
	SubmissionForEmail       = dbgen.GetSubmissionForEmailRow
	IssueSnapshotReference   = dbgen.ListTutorIssueSnapshotReferencesBatchRow
	StudentSubmissionOfTutor = dbgen.ListStudentSubmissionsBatchRow
)

type Homework struct {
	queries *dbgen.Queries
}

type NewIssue = dbgen.InsertIssueParams

type NewSubmission struct {
	ID           domain.SubmissionID
	Relationship domain.RelationshipID
	Student      domain.UserID
}

type IssueListQuery struct {
	Tutor          domain.UserID
	Status         domain.IssueStatus
	Group          *domain.GroupID
	Relationship   *domain.RelationshipID
	NeedsChecking  bool
	DeadlineBefore *time.Time
	Now            time.Time
	AfterInstant   *time.Time
	AfterID        domain.IssueID
	PageSize       int32
}

func (r Homework) IssueByCreateKey(ctx context.Context, tutor domain.UserID, key string) (IssueByKey, error) {
	row, err := r.queries.GetIssueByCreateKey(ctx, dbgen.GetIssueByCreateKeyParams{TutorID: tutor, CreateKey: &key})
	return one(row, err, "issue by its create key")
}

func (r Homework) Relationship(ctx context.Context, tutor domain.UserID, id domain.RelationshipID) (IssueRelationship, error) {
	row, err := r.queries.GetTutorIssueRelationship(ctx, dbgen.GetTutorIssueRelationshipParams{ID: id, TutorID: tutor})
	return one(row, err, "the tutor's relationship for an issue")
}

func (r Homework) LockRelationships(ctx context.Context, tutor domain.UserID, ids []domain.RelationshipID) ([]LockedIssueRelationship, error) {
	rows, err := r.queries.LockIssueRelationships(ctx, dbgen.LockIssueRelationshipsParams{TutorID: tutor, Ids: uuids(ids)})
	return one(rows, err, "lock the relationships of an issue")
}

func (r Homework) Group(ctx context.Context, tutor domain.UserID, id domain.GroupID) (IssueGroup, error) {
	row, err := r.queries.GetTutorIssueGroup(ctx, dbgen.GetTutorIssueGroupParams{ID: id, TutorID: tutor})
	return one(row, err, "the tutor's group for an issue")
}

func (r Homework) GroupMembers(ctx context.Context, tutor domain.UserID, group domain.GroupID) ([]domain.RelationshipID, error) {
	rows, err := r.queries.ListIssueGroupMembers(ctx, dbgen.ListIssueGroupMembersParams{GroupID: group, TutorID: tutor})
	return one(rows, err, "the members of an issue's group")
}

func (r Homework) LockGroup(ctx context.Context, tutor domain.UserID, id domain.GroupID) (LockedIssueGroup, error) {
	row, err := r.queries.LockIssueGroup(ctx, dbgen.LockIssueGroupParams{ID: id, TutorID: tutor})
	return one(row, err, "lock an issue's group")
}

func (r Homework) LockLesson(
	ctx context.Context, tutor domain.UserID, lesson domain.LessonID, relationship *domain.RelationshipID, group *domain.GroupID,
) error {
	_, err := r.queries.LockIssueLesson(ctx, dbgen.LockIssueLessonParams{
		ID: lesson, TutorID: tutor, RelationshipID: optionalUUID(relationship), GroupID: optionalUUID(group),
	})
	_, err = one(lesson, err, "lock an issue's lesson")
	return err
}

func (r Homework) LessonIssueCount(ctx context.Context, tutor domain.UserID, lesson domain.LessonID) (int64, error) {
	lessonID := lesson
	count, err := r.queries.CountLessonIssues(ctx, dbgen.CountLessonIssuesParams{TutorID: tutor, LessonID: &lessonID})
	return one(count, err, "count the issues of a lesson")
}

func (r Homework) SnapshotBytesTotal(ctx context.Context, tutor domain.UserID) (int64, error) {
	total, err := r.queries.GetSnapshotBytesTotal(ctx, tutor)
	return one(total, err, "the tutor's snapshot bytes")
}

func (r Homework) AddSnapshotBytes(ctx context.Context, tutor domain.UserID, bytes int64) error {
	return done(r.queries.AddSnapshotBytes(ctx, dbgen.AddSnapshotBytesParams{TutorID: tutor, Bytes: bytes}), "add snapshot bytes")
}

func (r Homework) SubtractSnapshotBytes(ctx context.Context, tutor domain.UserID, bytes int64) error {
	return done(r.queries.SubtractSnapshotBytes(ctx, dbgen.SubtractSnapshotBytesParams{TutorID: tutor, Bytes: bytes}), "subtract snapshot bytes")
}

func (r Homework) InsertIssue(ctx context.Context, issue NewIssue) error {
	return done(r.queries.InsertIssue(ctx, issue), "insert an issue")
}

func (r Homework) InsertSubmissions(
	ctx context.Context, issue domain.IssueID, tutor domain.UserID, deadline *time.Time, submissions []NewSubmission, now time.Time,
) error {
	ids := make([]pgtype.UUID, 0, len(submissions))
	relationships := make([]pgtype.UUID, 0, len(submissions))
	students := make([]pgtype.UUID, 0, len(submissions))
	for _, submission := range submissions {
		ids = append(ids, pgtype.UUID{Bytes: submission.ID, Valid: true})
		relationships = append(relationships, pgtype.UUID{Bytes: submission.Relationship, Valid: true})
		students = append(students, pgtype.UUID{Bytes: submission.Student, Valid: true})
	}
	return done(r.queries.InsertSubmissions(ctx, dbgen.InsertSubmissionsParams{
		IssueID: issue, TutorID: tutor, DeadlineAt: deadline, Now: now, Ids: ids, RelationshipIds: relationships, StudentIds: students,
	}), "insert the submissions of an issue")
}

func (r Homework) AddSnapshotReferences(ctx context.Context, issue domain.IssueID, attachments []domain.AttachmentID, now time.Time) error {
	if len(attachments) == 0 {
		return nil
	}
	return done(r.queries.AddIssueSnapshotReferences(ctx, dbgen.AddIssueSnapshotReferencesParams{
		IssueID: pgtype.UUID{Bytes: issue, Valid: true}, Now: now, AttachmentIds: uuids(attachments),
	}), "add issue snapshot references")
}

func (r Homework) Issue(ctx context.Context, tutor domain.UserID, id domain.IssueID) (TutorIssue, error) {
	row, err := r.queries.GetTutorIssue(ctx, dbgen.GetTutorIssueParams{ID: id, TutorID: tutor})
	return one(row, err, "the tutor's issue")
}

func (r Homework) LockIssue(ctx context.Context, tutor domain.UserID, id domain.IssueID) (LockedIssue, error) {
	row, err := r.queries.LockTutorIssue(ctx, dbgen.LockTutorIssueParams{ID: id, TutorID: tutor})
	return one(row, err, "lock the tutor's issue")
}

func (r Homework) SubmissionRelationships(ctx context.Context, issue domain.IssueID) ([]domain.RelationshipID, error) {
	rows, err := r.queries.ListIssueSubmissionRelationships(ctx, issue)
	return one(rows, err, "the relationships of an issue's submissions")
}

func (r Homework) SubmissionStates(ctx context.Context, issue domain.IssueID) ([]IssueSubmissionState, error) {
	rows, err := r.queries.ListIssueSubmissionStates(ctx, issue)
	return one(rows, err, "the submission states of an issue")
}

func (r Homework) UpdateIssue(
	ctx context.Context, tutor domain.UserID, id domain.IssueID, deadline *time.Time, lesson *domain.LessonID, now time.Time,
) error {
	return done(r.queries.UpdateIssue(ctx, dbgen.UpdateIssueParams{DeadlineAt: deadline, LessonID: lesson, Now: now, ID: id, TutorID: tutor}),
		"update an issue")
}

func (r Homework) SetSubmissionDeadlines(ctx context.Context, issue domain.IssueID, deadline *time.Time, now time.Time) error {
	return done(r.queries.SetSubmissionDeadlines(ctx, dbgen.SetSubmissionDeadlinesParams{DeadlineAt: deadline, Now: now, IssueID: issue}),
		"set the deadlines of an issue's submissions")
}

func (r Homework) WithdrawIssue(ctx context.Context, tutor domain.UserID, id domain.IssueID, now time.Time) error {
	return done(r.queries.WithdrawIssue(ctx, dbgen.WithdrawIssueParams{Now: now, ID: id, TutorID: tutor}), "withdraw an issue")
}

func (r Homework) Issues(ctx context.Context, query IssueListQuery) ([]IssueListRow, error) {
	afterID := pgtype.UUID{Bytes: query.AfterID, Valid: true}
	if query.DeadlineBefore != nil {
		rows, err := r.queries.ListTutorIssuesByDeadline(ctx, dbgen.ListTutorIssuesByDeadlineParams{
			TutorID: query.Tutor, Now: query.Now, DeadlineBefore: *query.DeadlineBefore, GroupID: optionalUUID(query.Group),
			RelationshipID: optionalUUID(query.Relationship), NeedsChecking: query.NeedsChecking, AfterDeadlineAt: query.AfterInstant,
			AfterID: afterID, PageSize: query.PageSize,
		})
		return convertRows(rows, err, "the tutor's issues by deadline", func(row dbgen.ListTutorIssuesByDeadlineRow) IssueListRow {
			return IssueListRow(row)
		})
	}
	rows, err := r.queries.ListTutorIssues(ctx, dbgen.ListTutorIssuesParams{
		TutorID: query.Tutor, Status: query.Status, GroupID: optionalUUID(query.Group), RelationshipID: optionalUUID(query.Relationship),
		NeedsChecking: query.NeedsChecking, AfterIssuedAt: query.AfterInstant, AfterID: afterID, PageSize: query.PageSize,
	})
	return one(rows, err, "the tutor's issues")
}

func (r Homework) Progress(ctx context.Context, tutor domain.UserID, issues []domain.IssueID) ([]IssueProgress, error) {
	rows, err := r.queries.IssueProgress(ctx, dbgen.IssueProgressParams{TutorID: tutor, IssueIds: uuids(issues)})
	return one(rows, err, "the progress of issues")
}

func (r Homework) RelationshipSubmissions(
	ctx context.Context, tutor domain.UserID, relationship domain.RelationshipID, issues []domain.IssueID,
) ([]RelationshipSubmission, error) {
	rows, err := r.queries.ListRelationshipIssueSubmissions(ctx, dbgen.ListRelationshipIssueSubmissionsParams{
		TutorID: tutor, RelationshipID: relationship, IssueIds: uuids(issues),
	})
	return one(rows, err, "a student's submissions of issues")
}

func (r Homework) IssueSubmissions(ctx context.Context, tutor domain.UserID, issue domain.IssueID) ([]IssueSubmission, error) {
	rows, err := r.queries.ListTutorIssueSubmissions(ctx, dbgen.ListTutorIssueSubmissionsParams{IssueID: issue, TutorID: tutor})
	return one(rows, err, "the submissions of the tutor's issue")
}

func (r Homework) GroupInScope(ctx context.Context, tutor domain.UserID, id domain.GroupID) error {
	_, err := r.queries.GetTutorIssueGroupScope(ctx, dbgen.GetTutorIssueGroupScopeParams{ID: id, TutorID: tutor})
	_, err = one(id, err, "a group of the tutor")
	return err
}

func (r Homework) StudentInScope(ctx context.Context, tutor domain.UserID, id domain.RelationshipID) error {
	_, err := r.queries.GetTutorIssueStudentScope(ctx, dbgen.GetTutorIssueStudentScopeParams{ID: id, TutorID: tutor})
	_, err = one(id, err, "a card of the tutor")
	return err
}

func (r Homework) VisibleSnapshots(ctx context.Context, viewer domain.UserID, attachment domain.AttachmentID) ([]VisibleIssueSnapshot, error) {
	rows, err := r.queries.VisibleIssueSnapshotReferences(ctx, dbgen.VisibleIssueSnapshotReferencesParams{
		Viewer: pgtype.UUID{Bytes: viewer, Valid: true}, AttachmentID: attachment,
	})
	return one(rows, err, "visible issue snapshot references")
}

func (r Homework) ServableSnapshotFile(ctx context.Context, attachment domain.AttachmentID, issue domain.EntityID) (ServableFile, error) {
	row, err := r.queries.GetServableIssueSnapshotFile(ctx, dbgen.GetServableIssueSnapshotFileParams{
		IssueID: pgtype.UUID{Bytes: issue, Valid: true}, ID: attachment,
	})
	return one(ServableFile(row), err, "servable issue snapshot file")
}

func (r Homework) ReadableSubmissionTitles(ctx context.Context, student domain.UserID, ids []domain.SubmissionID) ([]SubmissionTitle, error) {
	rows, err := r.queries.ReadableStudentSubmissionTitles(ctx, dbgen.ReadableStudentSubmissionTitlesParams{StudentID: student, Ids: uuids(ids)})
	return one(rows, err, "readable submission titles")
}

func (r Homework) LessonIssues(ctx context.Context, tutor domain.UserID, lesson domain.LessonID) ([]LessonIssue, error) {
	rows, err := r.queries.ListLessonIssues(ctx, dbgen.ListLessonIssuesParams{TutorID: tutor, LessonID: &lesson, PageSize: domain.IssuesPerLesson})
	return one(rows, err, "the lesson's linked issues")
}

func (r Homework) StudentLessonSubmissions(ctx context.Context, student domain.UserID, lesson domain.LessonID) ([]LessonSubmission, error) {
	rows, err := r.queries.ListStudentLessonSubmissions(ctx, dbgen.ListStudentLessonSubmissionsParams{
		StudentID: student, LessonID: &lesson, PageSize: domain.IssuesPerLesson,
	})
	return one(rows, err, "the student's submissions linked to a lesson")
}

func (r Homework) ReviewSubmissions(ctx context.Context, tutor domain.UserID, ids []domain.SubmissionID) ([]ReviewSubmission, error) {
	rows, err := r.queries.TutorReviewSubmissions(ctx, dbgen.TutorReviewSubmissionsParams{TutorID: tutor, Ids: uuids(ids)})
	return one(rows, err, "the tutor's submissions for notification labels")
}

func (r Homework) SubmissionForEmail(ctx context.Context, id domain.SubmissionID) (SubmissionForEmail, error) {
	row, err := r.queries.GetSubmissionForEmail(ctx, id)
	return one(row, err, "submission for an email")
}

type ReclaimedSubmission struct {
	ID       domain.SubmissionID
	TutorID  domain.UserID
	IssueID  domain.IssueID
	LessonID *domain.LessonID
}

func (r Homework) FlagReclaimed(ctx context.Context, relationships []domain.RelationshipID, now time.Time) ([]ReclaimedSubmission, error) {
	rows, err := r.queries.FlagReclaimedSubmissions(ctx, dbgen.FlagReclaimedSubmissionsParams{Now: now, RelationshipIds: uuids(relationships)})
	if err != nil {
		return nil, done(err, "flag the submissions of reclaimed links")
	}
	flagged := make([]ReclaimedSubmission, 0, len(rows))
	for _, row := range rows {
		flagged = append(flagged, ReclaimedSubmission{
			ID: domain.SubmissionID(row.ID.Bytes), TutorID: domain.UserID(row.TutorID.Bytes), IssueID: domain.IssueID(row.IssueID.Bytes),
			LessonID: row.LessonID,
		})
	}
	return flagged, nil
}

func (r Homework) SnapshotReferencesBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) ([]IssueSnapshotReference, error) {
	rows, err := r.queries.ListTutorIssueSnapshotReferencesBatch(ctx, dbgen.ListTutorIssueSnapshotReferencesBatchParams{TutorID: tutor, BatchSize: batch})
	return one(rows, err, "a batch of the tutor's issue snapshot references")
}

func (r Homework) RemoveSnapshotReferencesOfTutor(
	ctx context.Context, tutor domain.UserID, issues []domain.EntityID, attachments []domain.AttachmentID,
) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveTutorIssueSnapshotReferences(ctx, dbgen.RemoveTutorIssueSnapshotReferencesParams{
		TutorID: tutor, IssueIds: uuids(issues), AttachmentIds: uuids(attachments),
	})
	return one(removed, err, "remove the tutor's issue snapshot references")
}

func (r Homework) DeleteSubmissionsBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) (int64, error) {
	deleted, err := r.queries.DeleteTutorSubmissionsBatch(ctx, dbgen.DeleteTutorSubmissionsBatchParams{TutorID: tutor, BatchSize: batch})
	return counted(deleted, err, "delete a batch of the tutor's submissions")
}

func (r Homework) DeleteIssuesBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) ([]int64, error) {
	sizes, err := r.queries.DeleteTutorIssuesBatch(ctx, dbgen.DeleteTutorIssuesBatchParams{TutorID: tutor, BatchSize: batch})
	return one(sizes, err, "delete a batch of the tutor's issues")
}

func (r Homework) StudentSubmissionsBatch(ctx context.Context, student domain.UserID, batch int32) ([]StudentSubmissionOfTutor, error) {
	rows, err := r.queries.ListStudentSubmissionsBatch(ctx, dbgen.ListStudentSubmissionsBatchParams{StudentID: student, BatchSize: batch})
	return one(rows, err, "a batch of the student's submissions")
}

func (r Homework) DeleteSubmissions(ctx context.Context, ids []domain.SubmissionID) (int64, error) {
	deleted, err := r.queries.DeleteSubmissions(ctx, uuids(ids))
	return counted(deleted, err, "delete submissions")
}
