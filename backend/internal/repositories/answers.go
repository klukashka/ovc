package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	AssignmentRow            = dbgen.ListStudentTodoAssignmentsRow
	AssignmentCounts         = dbgen.CountStudentAssignmentsRow
	StudentSubmissionRow     = dbgen.GetStudentSubmissionRow
	TutorSubmissionRow       = dbgen.GetTutorSubmissionRow
	StoredAnswer             = dbgen.ListSubmissionAnswersRow
	AnswerFile               = dbgen.ListSubmissionAnswerFilesRow
	SubmissionParties        = dbgen.GetStudentSubmissionPartiesRow
	LockedSubmissionIssue    = dbgen.LockSubmissionIssueRow
	LockedSubmission         = dbgen.LockSubmissionRow
	AnswerReference          = dbgen.ListAnswerReferencesRow
	BindableAnswerUpload     = dbgen.LockBindableAnswerUploadsRow
	ReleasePendingSubmission = dbgen.ListReleasePendingSubmissionsRow
	LockedReleasePending     = dbgen.LockReleasePendingSubmissionsRow
	TutorAnswerReference     = dbgen.ListTutorAnswerReferencesBatchRow
	VisibleAnswer            = dbgen.VisibleAnswerReferencesRow
	SubmissionForTutorEmail  = dbgen.GetSubmissionForTutorEmailRow
	SubmitResult             = dbgen.SubmitSubmissionRow
)

type AssignmentQuery struct {
	Student         domain.UserID
	Group           domain.AssignmentGroup
	Relationship    *domain.RelationshipID
	After           bool
	AfterDeadlineAt *time.Time
	AfterInstant    *time.Time
	AfterID         domain.SubmissionID
	PageSize        int32
}

type ClosedSubmission struct {
	ID               domain.SubmissionID
	Student          domain.UserID
	IssueStatus      domain.IssueStatus
	MarkedForErasure bool
}

type FrozenAnswer struct {
	Submission domain.SubmissionID
	Question   domain.QuestionID
	Value      []byte
	Normalised *string
	Version    *int32
	Points     int32
	Correct    bool
	Now        time.Time
}

type SubmitChange struct {
	ID          domain.SubmissionID
	Status      domain.SubmissionStatus
	Checked     bool
	AutoPoints  int32
	FinalPoints int32
	Now         time.Time
}

type BoundAnswerFile struct {
	Attachment domain.AttachmentID
	Question   domain.QuestionID
}

func (r Homework) Assignments(ctx context.Context, query AssignmentQuery) ([]AssignmentRow, error) {
	relationship := optionalUUID(query.Relationship)
	afterID := pgtype.UUID{Bytes: query.AfterID, Valid: true}
	switch query.Group {
	case domain.AssignmentsTodo:
		issuedAt := time.Time{}
		if query.AfterInstant != nil {
			issuedAt = *query.AfterInstant
		}
		rows, err := r.queries.ListStudentTodoAssignments(ctx, dbgen.ListStudentTodoAssignmentsParams{
			StudentID: query.Student, RelationshipID: relationship, After: query.After, AfterDeadlineAt: query.AfterDeadlineAt,
			AfterIssuedAt: issuedAt, AfterID: afterID, PageSize: query.PageSize,
		})
		return one(rows, err, "the student's work to do")
	case domain.AssignmentsSubmitted:
		rows, err := r.queries.ListStudentSubmittedAssignments(ctx, dbgen.ListStudentSubmittedAssignmentsParams{
			StudentID: query.Student, RelationshipID: relationship, AfterInstant: query.AfterInstant, AfterID: afterID, PageSize: query.PageSize,
		})
		return one(assignmentRows(rows), err, "the student's submitted work")
	case domain.AssignmentsChecked:
		rows, err := r.queries.ListStudentCheckedAssignments(ctx, dbgen.ListStudentCheckedAssignmentsParams{
			StudentID: query.Student, RelationshipID: relationship, AfterInstant: query.AfterInstant, AfterID: afterID, PageSize: query.PageSize,
		})
		return one(assignmentRows(rows), err, "the student's checked work")
	case domain.AssignmentsClosed:
		rows, err := r.queries.ListStudentClosedAssignments(ctx, dbgen.ListStudentClosedAssignmentsParams{
			StudentID: query.Student, RelationshipID: relationship, AfterInstant: query.AfterInstant, AfterID: afterID, PageSize: query.PageSize,
		})
		return one(assignmentRows(rows), err, "the student's closed work")
	}
	return nil, nil
}

func assignmentRows[Row dbgen.ListStudentSubmittedAssignmentsRow | dbgen.ListStudentCheckedAssignmentsRow | dbgen.ListStudentClosedAssignmentsRow](
	rows []Row,
) []AssignmentRow {
	converted := make([]AssignmentRow, 0, len(rows))
	for _, row := range rows {
		converted = append(converted, AssignmentRow(row))
	}
	return converted
}

func (r Homework) AssignmentCounts(ctx context.Context, student domain.UserID, countCap int32) (AssignmentCounts, error) {
	row, err := r.queries.CountStudentAssignments(ctx, dbgen.CountStudentAssignmentsParams{StudentID: student, CountCap: countCap})
	return one(row, err, "count the student's work")
}

func (r Homework) StudentTutorInScope(ctx context.Context, student domain.UserID, relationship domain.RelationshipID) error {
	_, err := r.queries.GetStudentAssignmentTutor(ctx, dbgen.GetStudentAssignmentTutorParams{ID: relationship, StudentID: &student})
	_, err = one(relationship, err, "the student's tutor for work")
	return err
}

func (r Homework) StudentSubmission(ctx context.Context, student domain.UserID, id domain.SubmissionID) (StudentSubmissionRow, error) {
	row, err := r.queries.GetStudentSubmission(ctx, dbgen.GetStudentSubmissionParams{ID: id, StudentID: student})
	return one(row, err, "the student's submission")
}

func (r Homework) TutorSubmission(ctx context.Context, tutor domain.UserID, id domain.SubmissionID) (TutorSubmissionRow, error) {
	row, err := r.queries.GetTutorSubmission(ctx, dbgen.GetTutorSubmissionParams{ID: id, TutorID: tutor})
	return one(row, err, "the tutor's submission")
}

func (r Homework) Answers(ctx context.Context, submission domain.SubmissionID) ([]StoredAnswer, error) {
	rows, err := r.queries.ListSubmissionAnswers(ctx, submission)
	return one(rows, err, "the answers of a submission")
}

func (r Homework) AnswerFiles(ctx context.Context, submission domain.SubmissionID) ([]AnswerFile, error) {
	rows, err := r.queries.ListSubmissionAnswerFiles(ctx, pgtype.UUID{Bytes: submission, Valid: true})
	return one(rows, err, "the answer files of a submission")
}

func (r Homework) SubmissionParties(ctx context.Context, student domain.UserID, id domain.SubmissionID) (SubmissionParties, error) {
	row, err := r.queries.GetStudentSubmissionParties(ctx, dbgen.GetStudentSubmissionPartiesParams{ID: id, StudentID: student})
	return one(row, err, "the parties of a submission")
}

func (r Homework) LockSubmissionRelationship(ctx context.Context, tutor domain.UserID, id domain.RelationshipID) (domain.RelationshipState, error) {
	state, err := r.queries.LockSubmissionRelationship(ctx, dbgen.LockSubmissionRelationshipParams{ID: id, TutorID: tutor})
	return one(state, err, "lock the relationship of a submission")
}

func (r Homework) LockSubmissionIssue(ctx context.Context, tutor domain.UserID, id domain.IssueID) (LockedSubmissionIssue, error) {
	row, err := r.queries.LockSubmissionIssue(ctx, dbgen.LockSubmissionIssueParams{ID: id, TutorID: tutor})
	return one(row, err, "lock the issue of a submission")
}

func (r Homework) LockSubmission(ctx context.Context, issue domain.IssueID, id domain.SubmissionID) (LockedSubmission, error) {
	row, err := r.queries.LockSubmission(ctx, dbgen.LockSubmissionParams{ID: id, IssueID: issue})
	return one(row, err, "lock a submission")
}

func (r Homework) UpsertAnswer(ctx context.Context, submission domain.SubmissionID, question domain.QuestionID, value []byte, now time.Time) (bool, error) {
	changed, err := r.queries.UpsertAnswer(ctx, dbgen.UpsertAnswerParams{SubmissionID: submission, QuestionID: question, Value: value, Now: now})
	return applied(changed, err, "save an answer")
}

func (r Homework) FreezeAnswer(ctx context.Context, answer FrozenAnswer) (bool, error) {
	changed, err := r.queries.FreezeAnswer(ctx, dbgen.FreezeAnswerParams{
		SubmissionID: answer.Submission, QuestionID: answer.Question, Value: answer.Value, Normalised: answer.Normalised,
		NormaliserVersion: answer.Version, AutoPoints: answer.Points, AutoCorrect: answer.Correct, Now: answer.Now,
	})
	return applied(changed, err, "freeze an automatic answer")
}

func (r Homework) SaveSubmission(
	ctx context.Context, id domain.SubmissionID, status domain.SubmissionStatus, saveID domain.EntityID, now time.Time,
) error {
	return done(r.queries.SaveSubmission(ctx, dbgen.SaveSubmissionParams{
		Status: status, SaveID: pgtype.UUID{Bytes: saveID, Valid: true}, Now: now, ID: id,
	}), "record a saved submission")
}

func (r Homework) Submit(ctx context.Context, change SubmitChange) (SubmitResult, error) {
	row, err := r.queries.SubmitSubmission(ctx, dbgen.SubmitSubmissionParams{
		Status: change.Status, Now: change.Now, Checked: change.Checked, AutoPoints: change.AutoPoints, FinalPoints: change.FinalPoints,
		ID: change.ID,
	})
	return one(row, err, "submit a submission")
}

func (r Homework) RefreshFileBytes(ctx context.Context, id domain.SubmissionID) (int64, error) {
	total, err := r.queries.RefreshSubmissionFileBytes(ctx, id)
	return one(total, err, "refresh the file bytes of a submission")
}

func (r Homework) AnswerReferences(ctx context.Context, submission domain.SubmissionID) ([]AnswerReference, error) {
	rows, err := r.queries.ListAnswerReferences(ctx, pgtype.UUID{Bytes: submission, Valid: true})
	return one(rows, err, "the answer references of a submission")
}

func (r Homework) LockBindableAnswerUploads(
	ctx context.Context, student domain.UserID, submission domain.SubmissionID, ids []domain.AttachmentID, now time.Time,
) ([]BindableAnswerUpload, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.LockBindableAnswerUploads(ctx, dbgen.LockBindableAnswerUploadsParams{
		Ids: uuids(ids), UploaderID: student, SubmissionID: pgtype.UUID{Bytes: submission, Valid: true}, Now: now,
	})
	return one(rows, err, "lock bindable answer uploads")
}

func (r Homework) AddAnswerReferences(ctx context.Context, submission domain.SubmissionID, files []BoundAnswerFile, now time.Time) error {
	if len(files) == 0 {
		return nil
	}
	attachments := make([]pgtype.UUID, 0, len(files))
	questions := make([]pgtype.UUID, 0, len(files))
	for _, file := range files {
		attachments = append(attachments, pgtype.UUID{Bytes: file.Attachment, Valid: true})
		questions = append(questions, pgtype.UUID{Bytes: file.Question, Valid: true})
	}
	return done(r.queries.InsertAnswerReferences(ctx, dbgen.InsertAnswerReferencesParams{
		SubmissionID: pgtype.UUID{Bytes: submission, Valid: true}, Now: now, AttachmentIds: attachments, QuestionIds: questions,
	}), "add answer references")
}

func (r Homework) RemoveAnswerReferences(ctx context.Context, submission domain.SubmissionID, attachments []domain.AttachmentID) ([]domain.AttachmentID, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	removed, err := r.queries.RemoveAnswerReferences(ctx, dbgen.RemoveAnswerReferencesParams{
		SubmissionID: pgtype.UUID{Bytes: submission, Valid: true}, AttachmentIds: uuids(attachments),
	})
	return one(removed, err, "remove answer references")
}

func (r Homework) RemoveIssueAnswerReferences(ctx context.Context, issue domain.IssueID) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveIssueAnswerReferences(ctx, issue)
	if err != nil {
		return nil, done(err, "remove the answer references of an issue")
	}
	return removed, done(r.queries.ClearIssueFileBytes(ctx, issue), "clear the file bytes of an issue")
}

func (r Homework) CloseRelationshipSubmissions(ctx context.Context, relationship domain.RelationshipID, now time.Time) ([]ClosedSubmission, error) {
	rows, err := r.queries.CloseRelationshipSubmissions(ctx, dbgen.CloseRelationshipSubmissionsParams{Now: now, RelationshipID: relationship})
	if err != nil {
		return nil, done(err, "close the open work of a relationship")
	}
	closed := make([]ClosedSubmission, 0, len(rows))
	for _, row := range rows {
		closed = append(closed, ClosedSubmission{
			ID: domain.SubmissionID(row.ID.Bytes), Student: domain.UserID(row.StudentID.Bytes), IssueStatus: row.IssueStatus,
			MarkedForErasure: row.MarkedForErasure,
		})
	}
	return closed, nil
}

func (r Homework) ReleasePending(ctx context.Context, batch int32) ([]ReleasePendingSubmission, error) {
	rows, err := r.queries.ListReleasePendingSubmissions(ctx, batch)
	return one(rows, err, "submissions whose release is pending")
}

func (r Homework) LockReleasePending(ctx context.Context, tutor domain.UserID, ids []domain.SubmissionID) ([]LockedReleasePending, error) {
	rows, err := r.queries.LockReleasePendingSubmissions(ctx, dbgen.LockReleasePendingSubmissionsParams{Ids: uuids(ids), TutorID: tutor})
	return one(rows, err, "lock submissions whose release is pending")
}

func (r Homework) RemoveUnsubmittedAnswerReferences(
	ctx context.Context, submission domain.SubmissionID, submittedAt *time.Time,
) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveUnsubmittedAnswerReferences(ctx, dbgen.RemoveUnsubmittedAnswerReferencesParams{
		SubmissionID: pgtype.UUID{Bytes: submission, Valid: true}, SubmittedAt: submittedAt,
	})
	return one(removed, err, "remove never-submitted answer references")
}

func (r Homework) ClearReleasePending(ctx context.Context, id domain.SubmissionID) error {
	return done(r.queries.ClearReleasePending(ctx, id), "clear a pending release")
}

func (r Homework) AnswerReferencesBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) ([]TutorAnswerReference, error) {
	rows, err := r.queries.ListTutorAnswerReferencesBatch(ctx, dbgen.ListTutorAnswerReferencesBatchParams{TutorID: tutor, BatchSize: batch})
	return one(rows, err, "a batch of the tutor's answer references")
}

func (r Homework) RemoveAnswerReferencesOfTutor(
	ctx context.Context, tutor domain.UserID, submissions []domain.EntityID, attachments []domain.AttachmentID,
) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveTutorAnswerReferences(ctx, dbgen.RemoveTutorAnswerReferencesParams{
		TutorID: tutor, SubmissionIds: uuids(submissions), AttachmentIds: uuids(attachments),
	})
	return one(removed, err, "remove a batch of the tutor's answer references")
}

func (r Homework) DeleteAnswersBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) (int64, error) {
	deleted, err := r.queries.DeleteTutorAnswersBatch(ctx, dbgen.DeleteTutorAnswersBatchParams{TutorID: tutor, BatchSize: batch})
	return counted(deleted, err, "delete a batch of the tutor's answers")
}

func (r Homework) RemoveSubmissionsAnswerReferences(ctx context.Context, submissions []domain.SubmissionID) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveSubmissionsAnswerReferences(ctx, uuids(submissions))
	return one(removed, err, "remove the answer references of submissions")
}

func (r Homework) DeleteSubmissionsAnswers(ctx context.Context, submissions []domain.SubmissionID) (int64, error) {
	deleted, err := r.queries.DeleteSubmissionsAnswers(ctx, uuids(submissions))
	return counted(deleted, err, "delete the answers of submissions")
}

func (r Homework) VisibleAnswers(ctx context.Context, viewer domain.UserID, attachment domain.AttachmentID) ([]VisibleAnswer, error) {
	rows, err := r.queries.VisibleAnswerReferences(ctx, dbgen.VisibleAnswerReferencesParams{
		AttachmentID: attachment, Viewer: pgtype.UUID{Bytes: viewer, Valid: true},
	})
	return one(rows, err, "visible answer references")
}

func (r Homework) ServableAnswerFile(ctx context.Context, attachment domain.AttachmentID, submission domain.EntityID) (ServableFile, error) {
	row, err := r.queries.GetServableAnswerFile(ctx, dbgen.GetServableAnswerFileParams{
		SubmissionID: pgtype.UUID{Bytes: submission, Valid: true}, ID: attachment,
	})
	return one(ServableFile(row), err, "a servable answer file")
}

func (r Homework) SubmissionForTutorEmail(ctx context.Context, id domain.SubmissionID) (SubmissionForTutorEmail, error) {
	row, err := r.queries.GetSubmissionForTutorEmail(ctx, id)
	return one(row, err, "a submission for a tutor email")
}
