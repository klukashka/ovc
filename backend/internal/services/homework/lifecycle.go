package homework

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func ApplyLifecycle(ctx context.Context, repos repositories.Repositories, logger *slog.Logger, removed, ended []repositories.Relationship, purged bool, cause domain.SubmissionCloseCause, now time.Time) error {
	relationships := make([]domain.RelationshipID, 0, len(removed)+len(ended))
	dropped := make([]domain.RelationshipID, 0, len(removed))
	closing := make([]domain.RelationshipID, 0, len(ended))
	previous := make(map[domain.RelationshipID]domain.RelationshipState, len(removed))
	for _, link := range removed {
		relationships = append(relationships, link.ID)
		dropped = append(dropped, link.ID)
		previous[link.ID] = link.State
	}
	for _, link := range ended {
		relationships = append(relationships, link.ID)
		closing = append(closing, link.ID)
	}
	if len(relationships) == 0 {
		return nil
	}
	if err := repos.Homework.LockLifecycle(ctx, relationships); err != nil {
		return err
	}
	marked, err := repos.Homework.MarkLifecycle(ctx, dropped, now)
	if err != nil {
		return err
	}
	closed, err := repos.Homework.CloseLifecycle(ctx, closing, now)
	if err != nil {
		return err
	}
	issues := make([]domain.IssueID, 0, len(marked)+len(closed))
	markedCounts := map[domain.RelationshipID]int{}
	for _, issue := range marked {
		issues = append(issues, issue.ID)
		if issue.RelationshipID != nil {
			markedCounts[*issue.RelationshipID]++
		}
	}
	closedIDs := map[domain.SubmissionID]bool{}
	closedCounts := map[domain.RelationshipID]int{}
	for _, submission := range closed {
		issues = append(issues, submission.IssueID)
		closedIDs[submission.ID] = true
		closedCounts[submission.RelationshipID]++
	}
	rows, err := repos.Homework.LifecycleSubmissions(ctx, issues)
	if err != nil {
		return err
	}
	signals := make([]domain.EntitySignal, 0, len(rows))
	for _, submission := range rows {
		state, removing := previous[submission.RelationshipID]
		if !removing && !closedIDs[submission.ID] {
			continue
		}
		if !removing {
			state = domain.RelationshipEnded
		}
		users := []domain.UserID{submission.TutorID}
		if !purged && domain.ReadableStudentSubmission(submission.IssueStatus, false, state) {
			users = append(users, submission.StudentID)
		}
		signals = append(signals, domain.EntitySignal{Users: users, Entity: domain.EntitySubmission, ID: domain.EntityID(submission.ID)})
	}
	if err := repos.Signals.Entities(ctx, signals); err != nil {
		return err
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		for _, link := range removed {
			if count := markedCounts[link.ID]; count > 0 {
				logger.LogAttrs(ctx, slog.LevelInfo, "homework.issues.marked_for_erasure", tutorAttribute(link.TutorID), slog.String("relationship_id", link.ID.String()), slog.Int("count", count))
			}
		}
		for _, link := range ended {
			LogClosedWork(ctx, logger, link.TutorID, ClosedWork{Relationship: link.ID, Count: closedCounts[link.ID]}, cause)
		}
	})
	return nil
}
