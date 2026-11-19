package homework

import (
	"context"
	"log/slog"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

func (s *Service) MarkFeedbackSeen(ctx context.Context, principal accounts.Principal, id domain.SubmissionID, version int64) error {
	if err := s.reserveFeedbackMark(ctx, principal, id); err != nil {
		return err
	}
	changed := false
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := s.lockStudentWork(ctx, repos, principal, id, false)
		if err != nil {
			return err
		}
		if locked.submission.Version != version {
			return ErrConflict
		}
		changed, err = repos.Homework.MarkFeedbackSeen(ctx, id, version)
		return err
	})
	if err != nil {
		return err
	}
	if changed {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "feedback.seen", slog.String("user_id", principal.UserID.String()),
			slog.String(recordSubmission, id.String()))
	}
	return nil
}

func (s *Service) reserveFeedbackMark(ctx context.Context, principal accounts.Principal, id domain.SubmissionID) error {
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockStudent(ctx, repos, principal)
		if err != nil {
			return err
		}
		student := principal.UserID
		hour, retryAfter := domain.FixedWindow(now, domain.FeedbackMarkHourWindow)
		count, err := repos.RateLimits.Reserve(ctx, repositories.RateLimitSlot{
			Purpose:     domain.PurposeFeedbackMarksHour,
			Subject:     s.deps.Keys.ThrottleSubject(string(domain.PurposeFeedbackMarksHour), student.String()),
			WindowStart: hour,
		})
		if err != nil {
			return err
		}
		if count <= domain.FeedbackMarksPerHour {
			return nil
		}
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "homework.rate_limited", slog.String("user_id", student.String()),
			slog.String(recordSubmission, id.String()), slog.String("limit", string(domain.PurposeFeedbackMarksHour)))
		return &ThrottledError{RetryAfter: retryAfter}
	})
}
