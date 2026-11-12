package homework

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/library"
)

type ClosedWork struct {
	Relationship domain.RelationshipID
	Count        int
}

func CloseOnEnding(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, relationship domain.RelationshipID, now time.Time,
) (ClosedWork, error) {
	closed, err := repos.Homework.CloseRelationshipSubmissions(ctx, relationship, now)
	if err != nil || len(closed) == 0 {
		return ClosedWork{Relationship: relationship}, err
	}
	signals := make([]domain.EntitySignal, 0, len(closed))
	for _, submission := range closed {
		users := []domain.UserID{tutor}
		if domain.ReadableStudentSubmission(submission.IssueStatus, submission.MarkedForErasure, domain.RelationshipEnded) {
			users = append(users, submission.Student)
		}
		signals = append(signals, domain.EntitySignal{Users: users, Entity: domain.EntitySubmission, ID: domain.EntityID(submission.ID)})
	}
	return ClosedWork{Relationship: relationship, Count: len(closed)}, repos.Signals.Entities(ctx, signals)
}

func LogClosedWork(ctx context.Context, logger *slog.Logger, tutor domain.UserID, closed ClosedWork, cause string) {
	if closed.Count == 0 {
		return
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "homework.submissions.closed", tutorAttribute(tutor),
		slog.String("relationship_id", closed.Relationship.String()), slog.Int("count", closed.Count), slog.String("cause", cause))
}

type Cleanup struct {
	Store  *repositories.Store
	Random core.RandomSource
	Logger *slog.Logger
	Budget time.Duration
	Batch  int32
}

func (c Cleanup) Run(ctx context.Context, now time.Time) (int64, error) {
	budget, batch := c.Budget, c.Batch
	if budget <= 0 {
		budget = domain.HomeworkCleanupBudget
	}
	if batch <= 0 {
		batch = domain.HomeworkCleanupBatch
	}
	bounded, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	pending, err := c.Store.Repositories().Homework.ReleasePending(bounded, batch)
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	byTutor := map[domain.UserID][]domain.SubmissionID{}
	var tutors []domain.UserID
	for _, row := range pending {
		if _, seen := byTutor[row.TutorID]; !seen {
			tutors = append(tutors, row.TutorID)
		}
		byTutor[row.TutorID] = append(byTutor[row.TutorID], row.ID)
	}
	slices.SortFunc(tutors, func(a, b domain.UserID) int { return bytes.Compare(a[:], b[:]) })
	var released int64
	var failures []error
	for _, tutor := range tutors {
		count, err := c.releaseOfTutor(bounded, tutor, byTutor[tutor], now)
		released += count
		if err != nil {
			failures = append(failures, err)
			if bounded.Err() != nil {
				break
			}
		}
	}
	return released, errors.Join(failures...)
}

func (c Cleanup) releaseOfTutor(ctx context.Context, tutor domain.UserID, ids []domain.SubmissionID, now time.Time) (int64, error) {
	var released int64
	err := c.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		released = 0
		err := repos.TutorProfiles.Lock(ctx, tutor)
		if err != nil && !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
		locked, err := repos.Homework.LockReleasePending(ctx, tutor, ids)
		if err != nil {
			return err
		}
		for _, submission := range locked {
			removed, err := repos.Homework.RemoveUnsubmittedAnswerReferences(ctx, submission.ID, submission.SubmittedAt)
			if err != nil {
				return err
			}
			if err := library.ReleaseReferences(ctx, repos, c.Random, c.Logger, domain.ReferenceAnswer, removed, now); err != nil {
				return err
			}
			if _, err := repos.Homework.RefreshFileBytes(ctx, submission.ID); err != nil {
				return err
			}
			if err := repos.Homework.ClearReleasePending(ctx, submission.ID); err != nil {
				return err
			}
			released += int64(len(removed))
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return released, nil
}
