package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	TutorSubmissionParties  = dbgen.GetTutorSubmissionPartiesRow
	LockedReviewDraft       = dbgen.LockReviewDraftRow
	PublishedRevision       = dbgen.GetLatestPublishedRevisionRow
	PublishedRevisionBody   = dbgen.GetPublishedRevisionRow
	StoredRevision          = dbgen.ListSubmissionRevisionsRow
	ReviewItemFile          = dbgen.ListReviewItemFilesRow
	ReviewItemReference     = dbgen.ListReviewItemReferencesRow
	BindableCorrectedUpload = dbgen.LockBindableCorrectedUploadsRow
	ReviewQueueRow          = dbgen.ListReviewQueueRow
	PublishedReviewSummary  = dbgen.ListPublishedReviewSummariesRow
	VisibleReviewItem       = dbgen.VisibleReviewItemReferencesRow
	TutorReviewItemRef      = dbgen.ListTutorReviewItemReferencesBatchRow
)

type NewReviewDraft struct {
	ID         domain.ReviewRevisionID
	Submission domain.SubmissionID
	Tutor      domain.UserID
	Revision   int32
	Feedback   string
	Items      []byte
	SaveID     domain.EntityID
	Now        time.Time
}

type ReviewDraftChange struct {
	ID       domain.ReviewRevisionID
	Feedback string
	Items    []byte
	SaveID   domain.EntityID
	Now      time.Time
}

type CorrectedFile struct {
	Attachment domain.AttachmentID
	Question   domain.QuestionID
}

type ReviewPublication struct {
	Submission  domain.SubmissionID
	Status      domain.SubmissionStatus
	Checked     bool
	FinalPoints int32
	Revision    domain.ReviewRevisionID
	Visible     bool
	Now         time.Time
}

type ReviewQueueQuery struct {
	Tutor                 domain.UserID
	Issue                 *domain.IssueID
	Group                 *domain.GroupID
	Relationship          *domain.RelationshipID
	AfterFirstSubmittedAt *time.Time
	AfterID               domain.SubmissionID
	PageSize              int32
}

func (r Homework) TutorSubmissionParties(ctx context.Context, tutor domain.UserID, id domain.SubmissionID) (TutorSubmissionParties, error) {
	row, err := r.queries.GetTutorSubmissionParties(ctx, dbgen.GetTutorSubmissionPartiesParams{ID: id, TutorID: tutor})
	return one(row, err, "the parties of a tutor's submission")
}

func (r Homework) LockReviewDraft(ctx context.Context, submission domain.SubmissionID) (LockedReviewDraft, error) {
	row, err := r.queries.LockReviewDraft(ctx, submission)
	return one(row, err, "lock a review draft")
}

func (r Homework) LatestPublishedRevision(ctx context.Context, submission domain.SubmissionID) (PublishedRevision, error) {
	row, err := r.queries.GetLatestPublishedRevision(ctx, submission)
	return one(row, err, "the latest published review revision")
}

func (r Homework) PublishedRevision(ctx context.Context, id domain.ReviewRevisionID) (PublishedRevisionBody, error) {
	row, err := r.queries.GetPublishedRevision(ctx, id)
	return one(row, err, "a published review revision")
}

func (r Homework) RevisionCount(ctx context.Context, submission domain.SubmissionID) (int64, error) {
	count, err := r.queries.CountSubmissionRevisions(ctx, submission)
	return one(count, err, "count the review revisions of a submission")
}

func (r Homework) InsertReviewDraft(ctx context.Context, draft NewReviewDraft) error {
	return done(r.queries.InsertReviewDraft(ctx, dbgen.InsertReviewDraftParams{
		ID: draft.ID, SubmissionID: draft.Submission, TutorID: draft.Tutor, Revision: draft.Revision, Feedback: draft.Feedback,
		Items: draft.Items, SaveID: pgtype.UUID{Bytes: draft.SaveID, Valid: true}, Now: draft.Now,
	}), "insert a review draft")
}

func (r Homework) UpdateReviewDraft(ctx context.Context, change ReviewDraftChange) error {
	return done(r.queries.UpdateReviewDraft(ctx, dbgen.UpdateReviewDraftParams{
		Feedback: change.Feedback, Items: change.Items, SaveID: pgtype.UUID{Bytes: change.SaveID, Valid: true}, Now: change.Now, ID: change.ID,
	}), "update a review draft")
}

func (r Homework) PublishRevision(ctx context.Context, id domain.ReviewRevisionID, action domain.ReviewAction, now time.Time) (bool, error) {
	rows, err := r.queries.PublishReviewRevision(ctx, dbgen.PublishReviewRevisionParams{Action: &action, Now: now, ID: id})
	return applied(rows, err, "publish a review revision")
}

func (r Homework) Revisions(ctx context.Context, submission domain.SubmissionID) ([]StoredRevision, error) {
	rows, err := r.queries.ListSubmissionRevisions(ctx, submission)
	return one(rows, err, "the review revisions of a submission")
}

func (r Homework) ReviewItemFiles(ctx context.Context, revisions []domain.ReviewRevisionID) ([]ReviewItemFile, error) {
	if len(revisions) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListReviewItemFiles(ctx, uuids(revisions))
	return one(rows, err, "the corrected files of review revisions")
}

func (r Homework) ReviewItemReferences(ctx context.Context, revision domain.ReviewRevisionID) ([]ReviewItemReference, error) {
	rows, err := r.queries.ListReviewItemReferences(ctx, pgtype.UUID{Bytes: revision, Valid: true})
	return one(rows, err, "the corrected file references of a revision")
}

func (r Homework) CopyReviewItemReferences(ctx context.Context, from domain.ReviewRevisionID, to domain.ReviewRevisionID) error {
	return done(r.queries.CopyReviewItemReferences(ctx, dbgen.CopyReviewItemReferencesParams{
		ToRevision: pgtype.UUID{Bytes: to, Valid: true}, FromRevision: pgtype.UUID{Bytes: from, Valid: true},
	}), "copy corrected file references")
}

func (r Homework) AddReviewItemReferences(ctx context.Context, revision domain.ReviewRevisionID, files []CorrectedFile, now time.Time) error {
	if len(files) == 0 {
		return nil
	}
	attachments := make([]pgtype.UUID, 0, len(files))
	questions := make([]pgtype.UUID, 0, len(files))
	for _, file := range files {
		attachments = append(attachments, pgtype.UUID{Bytes: file.Attachment, Valid: true})
		questions = append(questions, pgtype.UUID{Bytes: file.Question, Valid: true})
	}
	return done(r.queries.InsertReviewItemReferences(ctx, dbgen.InsertReviewItemReferencesParams{
		RevisionID: pgtype.UUID{Bytes: revision, Valid: true}, Now: now, AttachmentIds: attachments, QuestionIds: questions,
	}), "add corrected file references")
}

func (r Homework) RemoveReviewItemReferences(
	ctx context.Context, revision domain.ReviewRevisionID, attachments []domain.AttachmentID,
) ([]domain.AttachmentID, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	removed, err := r.queries.RemoveReviewItemReferences(ctx, dbgen.RemoveReviewItemReferencesParams{
		RevisionID: pgtype.UUID{Bytes: revision, Valid: true}, AttachmentIds: uuids(attachments),
	})
	return one(removed, err, "remove corrected file references")
}

func (r Homework) LockBindableCorrectedUploads(
	ctx context.Context, tutor domain.UserID, submission domain.SubmissionID, ids []domain.AttachmentID, now time.Time,
) ([]BindableCorrectedUpload, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.LockBindableCorrectedUploads(ctx, dbgen.LockBindableCorrectedUploadsParams{
		Ids: uuids(ids), UploaderID: &tutor, SubmissionID: pgtype.UUID{Bytes: submission, Valid: true}, Now: now,
	})
	return one(rows, err, "lock bindable corrected uploads")
}

func (r Homework) PublishSubmissionReview(ctx context.Context, publication ReviewPublication) error {
	return done(r.queries.PublishSubmissionReview(ctx, dbgen.PublishSubmissionReviewParams{
		Status: publication.Status, Checked: publication.Checked, Now: publication.Now, FinalPoints: publication.FinalPoints,
		RevisionID: pgtype.UUID{Bytes: publication.Revision, Valid: true}, Visible: publication.Visible, ID: publication.Submission,
	}), "publish a review on a submission")
}

func (r Homework) ReviewQueue(ctx context.Context, query ReviewQueueQuery) ([]ReviewQueueRow, error) {
	rows, err := r.queries.ListReviewQueue(ctx, dbgen.ListReviewQueueParams{
		TutorID: query.Tutor, IssueID: optionalUUID(query.Issue), GroupID: optionalUUID(query.Group),
		RelationshipID: optionalUUID(query.Relationship), AfterFirstSubmittedAt: query.AfterFirstSubmittedAt,
		AfterID: pgtype.UUID{Bytes: query.AfterID, Valid: true}, PageSize: query.PageSize,
	})
	return one(rows, err, "the review queue")
}

func (r Homework) ReviewQueueCount(ctx context.Context, query ReviewQueueQuery, countCap int32) (int64, error) {
	count, err := r.queries.CountReviewQueue(ctx, dbgen.CountReviewQueueParams{
		TutorID: query.Tutor, IssueID: optionalUUID(query.Issue), GroupID: optionalUUID(query.Group),
		RelationshipID: optionalUUID(query.Relationship), CountCap: countCap,
	})
	return one(count, err, "count the review queue")
}

func (r Homework) ReviewIssueInScope(ctx context.Context, tutor domain.UserID, id domain.IssueID) error {
	_, err := r.queries.GetTutorReviewIssueScope(ctx, dbgen.GetTutorReviewIssueScopeParams{ID: id, TutorID: tutor})
	_, err = one(id, err, "an issue in the tutor's review scope")
	return err
}

func (r Homework) PublishedReviewSummaries(ctx context.Context, revisions []domain.ReviewRevisionID) ([]PublishedReviewSummary, error) {
	if len(revisions) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListPublishedReviewSummaries(ctx, uuids(revisions))
	return one(rows, err, "summaries of published review revisions")
}

func (r Homework) VisibleReviewItems(
	ctx context.Context, viewer domain.UserID, attachment domain.AttachmentID, now time.Time,
) ([]VisibleReviewItem, error) {
	rows, err := r.queries.VisibleReviewItemReferences(ctx, dbgen.VisibleReviewItemReferencesParams{
		AttachmentID: attachment, Viewer: pgtype.UUID{Bytes: viewer, Valid: true}, TutorDeletedAfter: domain.ReadableTutorWorkCutoff(now),
	})
	return one(rows, err, "visible corrected file references")
}

func (r Homework) ServableReviewItemFile(
	ctx context.Context, attachment domain.AttachmentID, revision domain.EntityID, now time.Time,
) (ServableFile, error) {
	row, err := r.queries.GetServableReviewItemFile(ctx, dbgen.GetServableReviewItemFileParams{
		RevisionID: pgtype.UUID{Bytes: revision, Valid: true}, ID: attachment, TutorDeletedAfter: domain.ReadableTutorWorkCutoff(now),
	})
	return one(ServableFile(row), err, "a servable corrected file")
}

func (r Homework) ReviewItemReferencesBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) ([]TutorReviewItemRef, error) {
	rows, err := r.queries.ListTutorReviewItemReferencesBatch(ctx, dbgen.ListTutorReviewItemReferencesBatchParams{
		TutorID: tutor, BatchSize: batch,
	})
	return one(rows, err, "a batch of the tutor's corrected file references")
}

func (r Homework) RemoveReviewItemReferencesOfTutor(
	ctx context.Context, tutor domain.UserID, revisions []domain.EntityID, attachments []domain.AttachmentID,
) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveTutorReviewItemReferences(ctx, dbgen.RemoveTutorReviewItemReferencesParams{
		TutorID: tutor, RevisionIds: uuids(revisions), AttachmentIds: uuids(attachments),
	})
	return one(removed, err, "remove a batch of the tutor's corrected file references")
}

func (r Homework) DeleteReviewRevisionsBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) (int64, error) {
	deleted, err := r.queries.DeleteTutorReviewRevisionsBatch(ctx, dbgen.DeleteTutorReviewRevisionsBatchParams{TutorID: tutor, BatchSize: batch})
	return counted(deleted, err, "delete a batch of the tutor's review revisions")
}

func (r Homework) RemoveSubmissionsReviewItemReferences(ctx context.Context, submissions []domain.SubmissionID) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveSubmissionsReviewItemReferences(ctx, uuids(submissions))
	return one(removed, err, "remove the corrected file references of submissions")
}

func (r Homework) DeleteSubmissionsReviewRevisions(ctx context.Context, submissions []domain.SubmissionID) (int64, error) {
	deleted, err := r.queries.DeleteSubmissionsReviewRevisions(ctx, uuids(submissions))
	return counted(deleted, err, "delete the review revisions of submissions")
}
