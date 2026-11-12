package homework

import (
	"context"
	"errors"
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
		row, err := repos.Homework.TutorSubmission(ctx, tutor, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		blocks, err := s.describeBlocks(ctx, repos, row.IssueID, row.Snapshot, now)
		if err != nil {
			return err
		}
		answers, err := s.answerViews(ctx, repos, id, now)
		if err != nil {
			return err
		}
		work = TutorWork{
			ID: row.ID, Issue: row.IssueID, Relationship: row.RelationshipID, DisplayName: row.DisplayName, Title: row.Title,
			Status: row.Status, Late: row.Late, IssuedBeforeReclaim: row.IssuedBeforeReclaim, Deadline: row.DeadlineAt,
			FirstSubmittedAt: row.FirstSubmittedAt, SubmittedAt: row.SubmittedAt, CheckedAt: row.CheckedAt, Version: row.Version,
			Policy: row.AnswerPolicy, Blocks: blocks, Answers: make([]TutorAnswer, 0, len(answers)), AutoPoints: row.AutoPoints,
			FinalPoints: row.FinalPoints, MaxPoints: row.MaxPoints,
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
		return nil
	})
	return work, err
}
