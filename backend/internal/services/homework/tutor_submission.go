package homework

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type TutorAnswer struct {
	Question          string
	Value             domain.AnswerValue
	Normalised        *string
	NormaliserVersion *int32
	AutoPoints        *int32
	AutoCorrect       *bool
	FrozenAt          *time.Time
	Files             []AnswerFileView
}

type TutorReviewItem struct {
	Question       string
	Points         *int32
	OverridePoints *int32
	OverrideReason *string
	Comment        string
	CorrectedFiles []AnswerFileView
}

type TutorRevision struct {
	Revision    int32
	State       domain.ReviewState
	Action      *domain.ReviewAction
	Feedback    string
	Items       []TutorReviewItem
	Version     int64
	PublishedAt *time.Time
}

type TutorWork struct {
	ID                  domain.SubmissionID
	Issue               domain.IssueID
	Relationship        domain.RelationshipID
	DisplayName         string
	Title               string
	Status              domain.SubmissionStatus
	Late                bool
	IssuedBeforeReclaim bool
	Deadline            *time.Time
	FirstSubmittedAt    *time.Time
	SubmittedAt         *time.Time
	CheckedAt           *time.Time
	Version             int64
	Policy              domain.AnswerPolicy
	Blocks              []IssueBlock
	Answers             []TutorAnswer
	Revisions           []TutorRevision
	AutoPoints          *int32
	FinalPoints         *int32
	MaxPoints           int32
	Percentage          *int64
}

func (s *Service) TutorSubmission(ctx context.Context, tutor domain.UserID, id domain.SubmissionID) (TutorWork, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return TutorWork{}, err
	}
	var work TutorWork
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		work, err = s.tutorWork(ctx, repos, tutor, id, now)
		return err
	})
	return work, err
}

func (s *Service) tutorWork(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.SubmissionID, now time.Time) (TutorWork, error) {
	row, err := repos.Homework.TutorSubmission(ctx, tutor, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return TutorWork{}, ErrNotFound
	}
	if err != nil {
		return TutorWork{}, err
	}
	blocks, err := s.describeBlocks(ctx, repos, row.IssueID, row.Snapshot, now)
	if err != nil {
		return TutorWork{}, err
	}
	answers, err := s.answerViews(ctx, repos, id, now)
	if err != nil {
		return TutorWork{}, err
	}
	revisions, err := s.tutorRevisions(ctx, repos, id, now)
	if err != nil {
		return TutorWork{}, err
	}
	work := TutorWork{
		ID: row.ID, Issue: row.IssueID, Relationship: row.RelationshipID, DisplayName: row.DisplayName, Title: row.Title,
		Status: row.Status, Late: row.Late, IssuedBeforeReclaim: row.IssuedBeforeReclaim, Deadline: row.DeadlineAt,
		FirstSubmittedAt: row.FirstSubmittedAt, SubmittedAt: row.SubmittedAt, CheckedAt: row.CheckedAt, Version: row.Version,
		Policy: row.AnswerPolicy, Blocks: blocks, Answers: make([]TutorAnswer, 0, len(answers)), Revisions: revisions,
		AutoPoints: row.AutoPoints, FinalPoints: row.FinalPoints, MaxPoints: row.MaxPoints,
	}
	for _, answer := range answers {
		work.Answers = append(work.Answers, TutorAnswer{
			Question: answer.question, Value: answer.value, Normalised: answer.stored.Normalised,
			NormaliserVersion: answer.stored.NormaliserVersion, AutoPoints: answer.stored.AutoPoints, AutoCorrect: answer.stored.AutoCorrect,
			FrozenAt: answer.stored.FrozenAt, Files: answer.files,
		})
	}
	if row.FinalPoints != nil {
		if percentage, scored := domain.ScorePercentage(int64(*row.FinalPoints), int64(row.MaxPoints)); scored {
			work.Percentage = &percentage
		}
	}
	return work, nil
}

func (s *Service) tutorRevisions(ctx context.Context, repos repositories.Repositories, id domain.SubmissionID, now time.Time) ([]TutorRevision, error) {
	rows, err := repos.Homework.Revisions(ctx, id)
	if err != nil {
		return nil, err
	}
	ids := make([]domain.ReviewRevisionID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	files, err := s.correctedFiles(ctx, repos, ids, now)
	if err != nil {
		return nil, err
	}
	revisions := make([]TutorRevision, 0, len(rows))
	for _, row := range rows {
		items, err := decodeReviewItems(row.Items)
		if err != nil {
			return nil, err
		}
		revision := TutorRevision{
			Revision: row.Revision, State: row.State, Action: row.Action, Feedback: row.Feedback, Version: row.Version,
			PublishedAt: row.PublishedAt, Items: make([]TutorReviewItem, 0, len(items)),
		}
		for _, item := range items {
			revision.Items = append(revision.Items, TutorReviewItem{
				Question: item.QuestionID, Points: item.Points, OverridePoints: item.OverridePoints, OverrideReason: item.OverrideReason,
				Comment: item.Comment, CorrectedFiles: files.of(row.ID, item.QuestionID),
			})
		}
		revisions = append(revisions, revision)
	}
	return revisions, nil
}

type revisionFiles map[domain.ReviewRevisionID]map[string][]AnswerFileView

func (f revisionFiles) of(revision domain.ReviewRevisionID, question string) []AnswerFileView {
	files := f[revision][question]
	if files == nil {
		return []AnswerFileView{}
	}
	return files
}

func (s *Service) correctedFiles(
	ctx context.Context, repos repositories.Repositories, revisions []domain.ReviewRevisionID, now time.Time,
) (revisionFiles, error) {
	rows, err := repos.Homework.ReviewItemFiles(ctx, revisions)
	if err != nil {
		return nil, err
	}
	files := revisionFiles{}
	for _, row := range rows {
		revision := domain.ReviewRevisionID(row.RefID)
		if row.QuestionID == nil {
			return nil, fmt.Errorf("revision %s: %w", revision, errStoredReviewInvalid)
		}
		question := domain.QuestionID(*row.QuestionID).String()
		url, expiresAt := s.deps.Addresses.ReviewItemAddress(row.ID, revision, dispositionOf(row.Width), now)
		if files[revision] == nil {
			files[revision] = map[string][]AnswerFileView{}
		}
		files[revision][question] = append(files[revision][question], AnswerFileView{
			Attachment: row.ID, Filename: row.Filename, MediaType: row.MediaType, SizeBytes: row.SizeBytes, Width: row.Width,
			Height: row.Height, URL: url, ExpiresAt: expiresAt,
		})
	}
	return files, nil
}
