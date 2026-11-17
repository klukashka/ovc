package homework

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type ClosedWork struct {
	Relationship domain.RelationshipID
	Count        int
}

func CloseOnEnding(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, relationship domain.RelationshipID, now time.Time,
) (ClosedWork, error) {
	if err := repos.Homework.LockLifecycle(ctx, []domain.RelationshipID{relationship}); err != nil {
		return ClosedWork{}, err
	}
	closed, err := repos.Homework.CloseRelationshipSubmissions(ctx, relationship, now)
	if err != nil || len(closed) == 0 {
		return ClosedWork{Relationship: relationship}, err
	}
	var batch frameBatch
	for _, submission := range closed {
		lessons := lessonsOf(submission.Lesson)
		batch.tutorSubmissionOfIssue(tutor, submission.ID, submission.Issue, lessons)
		if domain.ReadableStudentSubmission(submission.IssueStatus, submission.MarkedForErasure, domain.RelationshipEnded) {
			batch.studentSubmission(submission.Student, submission.ID, lessons)
		}
	}
	return ClosedWork{Relationship: relationship, Count: len(closed)}, batch.send(ctx, repos)
}

func LogClosedWork(ctx context.Context, logger *slog.Logger, tutor domain.UserID, closed ClosedWork, cause domain.SubmissionCloseCause) {
	if closed.Count == 0 {
		return
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "homework.submissions.closed", tutorAttribute(tutor),
		slog.String("relationship_id", closed.Relationship.String()), slog.Int("count", closed.Count), slog.String("cause", string(cause)))
}
