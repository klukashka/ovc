package homework

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/library"
)

type Purge struct {
	Random core.RandomSource
	Logger *slog.Logger
}

func (p Purge) TutorSteps() []library.PurgeStep {
	return []library.PurgeStep{
		p.reviewItemReferencesBatch, p.answerReferencesBatch, reviewRevisionsBatch, answersBatch, submissionsBatch, p.snapshotReferencesBatch,
		issuesBatch, dailyAnswerBytesBatch,
	}
}

func (p Purge) reviewItemReferencesBatch(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, now time.Time,
) (int64, error) {
	references, err := repos.Homework.ReviewItemReferencesBatchOfTutor(ctx, tutor, batch)
	if err != nil || len(references) == 0 {
		return 0, err
	}
	if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
		return 0, err
	}
	revisions := make([]domain.EntityID, 0, len(references))
	attachments := make([]domain.AttachmentID, 0, len(references))
	for _, reference := range references {
		revisions = append(revisions, reference.RefID)
		attachments = append(attachments, reference.AttachmentID)
	}
	removed, err := repos.Homework.RemoveReviewItemReferencesOfTutor(ctx, tutor, revisions, attachments)
	if err != nil {
		return 0, err
	}
	if err := library.ReleaseReferences(ctx, repos, p.Random, p.Logger, domain.ReferenceReviewItem, distinctAttachments(removed), now); err != nil {
		return 0, err
	}
	return int64(len(references)), nil
}

func reviewRevisionsBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, _ time.Time) (int64, error) {
	return repos.Homework.DeleteReviewRevisionsBatchOfTutor(ctx, tutor, batch)
}

func (p Purge) answerReferencesBatch(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, now time.Time,
) (int64, error) {
	references, err := repos.Homework.AnswerReferencesBatchOfTutor(ctx, tutor, batch)
	if err != nil || len(references) == 0 {
		return 0, err
	}
	if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
		return 0, err
	}
	submissions := make([]domain.EntityID, 0, len(references))
	attachments := make([]domain.AttachmentID, 0, len(references))
	for _, reference := range references {
		submissions = append(submissions, reference.RefID)
		attachments = append(attachments, reference.AttachmentID)
	}
	removed, err := repos.Homework.RemoveAnswerReferencesOfTutor(ctx, tutor, submissions, attachments)
	if err != nil {
		return 0, err
	}
	if err := library.ReleaseReferences(ctx, repos, p.Random, p.Logger, domain.ReferenceAnswer, distinctAttachments(removed), now); err != nil {
		return 0, err
	}
	return int64(len(references)), nil
}

func answersBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, _ time.Time) (int64, error) {
	return repos.Homework.DeleteAnswersBatchOfTutor(ctx, tutor, batch)
}

func distinctAttachments(attachments []domain.AttachmentID) []domain.AttachmentID {
	slices.SortFunc(attachments, func(a, b domain.AttachmentID) int { return bytes.Compare(a[:], b[:]) })
	return slices.Compact(attachments)
}

func dailyAnswerBytesBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, _ time.Time) (int64, error) {
	return repos.DailyUploads.DeleteAnswerBatchOfTutor(ctx, tutor, batch)
}

func submissionsBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, _ time.Time) (int64, error) {
	return repos.Homework.DeleteSubmissionsBatchOfTutor(ctx, tutor, batch)
}

func (p Purge) snapshotReferencesBatch(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, now time.Time,
) (int64, error) {
	references, err := repos.Homework.SnapshotReferencesBatchOfTutor(ctx, tutor, batch)
	if err != nil || len(references) == 0 {
		return 0, err
	}
	if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
		return 0, err
	}
	issues := make([]domain.EntityID, 0, len(references))
	attachments := make([]domain.AttachmentID, 0, len(references))
	for _, reference := range references {
		issues = append(issues, reference.RefID)
		attachments = append(attachments, reference.AttachmentID)
	}
	removed, err := repos.Homework.RemoveSnapshotReferencesOfTutor(ctx, tutor, issues, attachments)
	if err != nil {
		return 0, err
	}
	if err := library.ReleaseReferences(ctx, repos, p.Random, p.Logger, domain.ReferenceIssueSnapshot, distinctAttachments(removed), now); err != nil {
		return 0, err
	}
	return int64(len(references)), nil
}

func issuesBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, _ time.Time) (int64, error) {
	sizes, err := repos.Homework.DeleteIssuesBatchOfTutor(ctx, tutor, batch)
	if err != nil || len(sizes) == 0 {
		return 0, err
	}
	var total int64
	for _, size := range sizes {
		total += size
	}
	if err := repos.Homework.SubtractSnapshotBytes(ctx, tutor, total); err != nil {
		return 0, err
	}
	return int64(len(sizes)), nil
}

func (p Purge) StudentBatch(ctx context.Context, repos repositories.Repositories, student domain.UserID, batch int32, now time.Time) (int64, error) {
	rows, err := repos.Homework.StudentSubmissionsBatch(ctx, student, batch)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	tutors := make([]domain.UserID, 0, len(rows))
	ids := make([]domain.SubmissionID, 0, len(rows))
	for _, row := range rows {
		tutors = append(tutors, row.TutorID)
		ids = append(ids, row.ID)
	}
	slices.SortFunc(tutors, func(a, b domain.UserID) int { return bytes.Compare(a[:], b[:]) })
	for _, tutor := range slices.Compact(tutors) {
		if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
			return 0, err
		}
	}
	corrected, err := repos.Homework.RemoveSubmissionsReviewItemReferences(ctx, ids)
	if err != nil {
		return 0, err
	}
	if err := library.ReleaseReferences(ctx, repos, p.Random, p.Logger, domain.ReferenceReviewItem, distinctAttachments(corrected), now); err != nil {
		return 0, err
	}
	if _, err := repos.Homework.DeleteSubmissionsReviewRevisions(ctx, ids); err != nil {
		return 0, err
	}
	removed, err := repos.Homework.RemoveSubmissionsAnswerReferences(ctx, ids)
	if err != nil {
		return 0, err
	}
	if err := library.ReleaseReferences(ctx, repos, p.Random, p.Logger, domain.ReferenceAnswer, distinctAttachments(removed), now); err != nil {
		return 0, err
	}
	if _, err := repos.Homework.DeleteSubmissionsAnswers(ctx, ids); err != nil {
		return 0, err
	}
	return repos.Homework.DeleteSubmissions(ctx, ids)
}

func FlagReclaimedWork(ctx context.Context, repos repositories.Repositories, relationships []domain.RelationshipID, now time.Time) error {
	if len(relationships) == 0 {
		return nil
	}
	flagged, err := repos.Homework.FlagReclaimed(ctx, relationships, now)
	if err != nil || len(flagged) == 0 {
		return err
	}
	signals := make([]domain.EntitySignal, 0, len(flagged))
	for _, submission := range flagged {
		signals = append(signals, domain.EntitySignal{
			Users: []domain.UserID{submission.TutorID}, Entity: domain.EntitySubmission, ID: domain.EntityID(submission.ID),
		})
	}
	return repos.Signals.Entities(ctx, signals)
}
