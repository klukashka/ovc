package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

var ErrMalformedAllowedKinds = errors.New("a file question's allowed kinds are malformed")

type UploadSubmission = dbgen.GetUploadSubmissionRow

type LockedUploadIssue struct {
	Status           domain.IssueStatus
	MarkedForErasure bool
	FileQuestion     bool
	AllowedKinds     []domain.AnswerFileKind
}

func (r Homework) UploadSubmission(ctx context.Context, id domain.SubmissionID, caller domain.UserID, asStudent bool) (UploadSubmission, error) {
	row, err := r.queries.GetUploadSubmission(ctx, dbgen.GetUploadSubmissionParams{ID: id, AsStudent: asStudent, Caller: caller})
	return one(row, err, "get upload submission")
}

func (r Homework) LockUploadRelationship(ctx context.Context, id domain.RelationshipID, tutor domain.UserID) (domain.RelationshipState, error) {
	state, err := r.queries.LockUploadRelationship(ctx, dbgen.LockUploadRelationshipParams{ID: id, TutorID: tutor})
	return one(state, err, "lock upload relationship")
}

func (r Homework) LockUploadIssue(ctx context.Context, id domain.IssueID, tutor domain.UserID, question domain.QuestionID) (LockedUploadIssue, error) {
	row, err := r.queries.LockUploadIssue(ctx, dbgen.LockUploadIssueParams{QuestionID: question.String(), ID: id, TutorID: tutor})
	if row, err = one(row, err, "lock upload issue"); err != nil {
		return LockedUploadIssue{}, err
	}
	locked := LockedUploadIssue{Status: row.Status, MarkedForErasure: row.MarkedForErasure}
	if row.AllowedKinds == nil {
		return locked, nil
	}
	var kinds []domain.AnswerFileKind
	if err := json.Unmarshal(row.AllowedKinds, &kinds); err != nil {
		return LockedUploadIssue{}, fmt.Errorf("%w: %w", ErrMalformedAllowedKinds, err)
	}
	if !domain.ValidAllowedKinds(kinds) {
		return LockedUploadIssue{}, ErrMalformedAllowedKinds
	}
	locked.FileQuestion, locked.AllowedKinds = true, kinds
	return locked, nil
}

func (r Homework) LockUploadSubmission(ctx context.Context, id domain.SubmissionID, issue domain.IssueID) (domain.SubmissionStatus, error) {
	status, err := r.queries.LockUploadSubmission(ctx, dbgen.LockUploadSubmissionParams{ID: id, IssueID: issue})
	return one(status, err, "lock upload submission")
}

func (r Attachments) OpenAnswerBytes(
	ctx context.Context, student domain.UserID, tutor domain.UserID, excluded *domain.AttachmentID, now time.Time, uploadingSince time.Time,
) (int64, error) {
	params := dbgen.SumOpenAnswerBytesParams{StudentID: student, TutorID: &tutor, Now: now, UploadingSince: uploadingSince}
	if excluded != nil {
		params.Excluded = pgtype.UUID{Bytes: *excluded, Valid: true}
	}
	total, err := r.queries.SumOpenAnswerBytes(ctx, params)
	return one(total, err, "sum open answer bytes")
}

func (r Attachments) ClaimUnbindableHomework(ctx context.Context, batch int32) ([]domain.AttachmentID, error) {
	ids, err := r.queries.ClaimUnbindableHomeworkUploads(ctx, batch)
	return one(ids, err, "claim unbindable homework uploads")
}

func (r DailyUploads) AnswerBytes(ctx context.Context, student domain.UserID, tutor domain.UserID, day time.Time) (int64, error) {
	total, err := r.queries.GetDailyAnswerBytes(ctx, dbgen.GetDailyAnswerBytesParams{StudentID: student, TutorID: tutor, UploadDay: day})
	return one(total, err, "read daily answer bytes")
}

func (r DailyUploads) AddAnswer(ctx context.Context, student domain.UserID, tutor domain.UserID, day time.Time, size int64) error {
	return done(r.queries.AddDailyAnswerBytes(ctx, dbgen.AddDailyAnswerBytesParams{
		StudentID: student, TutorID: tutor, UploadDay: day, CompletedBytes: size,
	}), "add daily answer bytes")
}

func (r DailyUploads) DeletePastAnswerBatch(ctx context.Context, students []domain.UserID, before time.Time) (int64, error) {
	deleted, err := r.queries.DeletePastDailyAnswerBytes(ctx, dbgen.DeletePastDailyAnswerBytesParams{Students: uuids(students), BeforeDay: before})
	return counted(deleted, err, "delete past daily answer bytes")
}

func (r DailyUploads) DeleteAnswerBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) (int64, error) {
	deleted, err := r.queries.DeleteTutorDailyAnswerBytesBatch(ctx, dbgen.DeleteTutorDailyAnswerBytesBatchParams{TutorID: tutor, BatchSize: batch})
	return counted(deleted, err, "delete daily answer bytes of purged tutor")
}
