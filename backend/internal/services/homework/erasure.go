package homework

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/library"
)

type Cleanup struct {
	Store  *repositories.Store
	Random core.RandomSource
	Logger *slog.Logger
	Budget time.Duration
	Batch  int32
}

func (c Cleanup) Run(ctx context.Context, now time.Time) (domain.HomeworkCleanupCounts, error) {
	budget := c.Budget
	if budget <= 0 {
		budget = domain.HomeworkCleanupBudget
	}
	if c.Batch <= 0 || c.Batch > domain.HomeworkCleanupBatch {
		c.Batch = domain.HomeworkCleanupBatch
	}
	bounded, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var total domain.HomeworkCleanupCounts
	var failedIssues []domain.IssueID
	var failedSubmissions []domain.SubmissionID
	var failures []error
	for {
		issues, issueErr := c.Store.Repositories().Homework.EraseCandidates(bounded, failedIssues)
		releases, releaseErr := c.Store.Repositories().Homework.ReleaseCandidates(bounded, failedSubmissions)
		if issueErr != nil || releaseErr != nil {
			return total, errors.Join(append(failures, issueErr, releaseErr)...)
		}
		if len(issues) == 0 && len(releases) == 0 {
			return total, errors.Join(failures...)
		}
		for _, issue := range issues {
			count, err := c.erase(bounded, issue, now)
			addCleanupCounts(&total, count)
			if err != nil {
				failures = append(failures, err)
				failedIssues = append(failedIssues, issue.ID)
			}
		}
		for _, release := range releases {
			count, err := c.release(bounded, release, now)
			addCleanupCounts(&total, count)
			if err != nil {
				failures = append(failures, err)
				failedSubmissions = append(failedSubmissions, release.ID)
			}
		}
		if bounded.Err() != nil {
			return total, errors.Join(append(failures, bounded.Err())...)
		}
	}
}

func addCleanupCounts(total *domain.HomeworkCleanupCounts, count domain.HomeworkCleanupCounts) {
	total.IssuesErased += count.IssuesErased
	total.SubmissionsReleased += count.SubmissionsReleased
	total.ReferencesRemoved += count.ReferencesRemoved
}

func (c Cleanup) erase(ctx context.Context, candidate repositories.EraseCandidate, now time.Time) (domain.HomeworkCleanupCounts, error) {
	var count domain.HomeworkCleanupCounts
	err := c.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := repos.TutorProfiles.Lock(ctx, candidate.TutorID); err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				return nil
			}
			return err
		}
		issue, err := repos.Homework.LockCleanupIssue(ctx, candidate.TutorID, candidate.ID)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if issue.ErasePendingSince == nil {
			return nil
		}
		submission, err := repos.Homework.LockCleanupSubmission(ctx, candidate.ID, nil)
		if err == nil {
			return c.eraseSubmission(ctx, repos, submission.ID, now, &count)
		}
		if !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
		removed, err := c.removeReferences(ctx, repos, domain.ReferenceIssueSnapshot, domain.EntityID(candidate.ID), nil, now)
		if err != nil {
			return err
		}
		count.ReferencesRemoved += removed
		if removed > 0 {
			return nil
		}
		deleted, err := repos.Homework.DeleteCleanupIssue(ctx, candidate.ID)
		if err != nil {
			return err
		}
		if deleted == 0 {
			return nil
		}
		if err := repos.Homework.SubtractSnapshotBytes(ctx, candidate.TutorID, issue.SnapshotBytes); err != nil {
			return err
		}
		count.IssuesErased = deleted
		return nil
	})
	if err != nil {
		return domain.HomeworkCleanupCounts{}, err
	}
	return count, nil
}

func (c Cleanup) eraseSubmission(ctx context.Context, repos repositories.Repositories, id domain.SubmissionID, now time.Time, count *domain.HomeworkCleanupCounts) error {
	revision, err := repos.Homework.LockCleanupRevision(ctx, id)
	if err == nil {
		removed, err := c.removeReferences(ctx, repos, domain.ReferenceReviewItem, domain.EntityID(revision), nil, now)
		if err != nil {
			return err
		}
		count.ReferencesRemoved += removed
		if removed > 0 {
			return nil
		}
		_, err = repos.Homework.DeleteCleanupRevision(ctx, revision)
		return err
	}
	if !errors.Is(err, repositories.ErrNotFound) {
		return err
	}
	removed, err := c.removeReferences(ctx, repos, domain.ReferenceAnswer, domain.EntityID(id), nil, now)
	if err != nil {
		return err
	}
	count.ReferencesRemoved += removed
	if removed > 0 {
		_, err := repos.Homework.RefreshFileBytes(ctx, id)
		return err
	}
	deleted, err := repos.Homework.DeleteCleanupAnswers(ctx, id, c.Batch)
	if err != nil || deleted > 0 {
		return err
	}
	deleted, err = repos.Homework.DeleteCleanupNotifications(ctx, id, c.Batch)
	if err != nil || deleted > 0 {
		return err
	}
	_, err = repos.Homework.DeleteCleanupSubmission(ctx, id)
	return err
}

func (c Cleanup) release(ctx context.Context, candidate repositories.ReleaseCandidate, now time.Time) (domain.HomeworkCleanupCounts, error) {
	var count domain.HomeworkCleanupCounts
	err := c.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := repos.TutorProfiles.Lock(ctx, candidate.TutorID); err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				return nil
			}
			return err
		}
		_, err := repos.Homework.LockCleanupIssue(ctx, candidate.TutorID, candidate.IssueID)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		submission, err := repos.Homework.LockCleanupSubmission(ctx, candidate.IssueID, &candidate.ID)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if submission.ReleasePendingSince == nil {
			return nil
		}
		removed, err := c.removeReferences(ctx, repos, domain.ReferenceAnswer, domain.EntityID(candidate.ID), submission.SubmittedAt, now)
		if err != nil {
			return err
		}
		if _, err := repos.Homework.RefreshFileBytes(ctx, candidate.ID); err != nil {
			return err
		}
		count.ReferencesRemoved = removed
		if removed == 0 {
			if err := repos.Homework.ClearReleasePending(ctx, candidate.ID); err != nil {
				return err
			}
			count.SubmissionsReleased = 1
		}
		return nil
	})
	if err != nil {
		return domain.HomeworkCleanupCounts{}, err
	}
	return count, nil
}

func (c Cleanup) removeReferences(ctx context.Context, repos repositories.Repositories, kind domain.ReferenceKind, id domain.EntityID, after *time.Time, now time.Time) (int64, error) {
	removed, err := repos.Homework.RemoveCleanupReferences(ctx, kind, id, after, c.Batch)
	if err != nil {
		return 0, err
	}
	count := int64(len(removed))
	attachments := distinctAttachments(removed)
	for _, attachment := range attachments {
		if _, err := repos.Attachments.Lock(ctx, attachment); err != nil {
			return 0, err
		}
	}
	if err := library.ReleaseReferences(ctx, repos, c.Random, c.Logger, kind, attachments, now); err != nil {
		return 0, err
	}
	return count, nil
}
