package schedule

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func (s *Service) CancelLesson(ctx context.Context, actor domain.TutorActor, id domain.LessonID, cancellation LessonCancellation) (Created, error) {
	tutor := actor.UserID
	if err := s.admitLessonWrite(ctx, actor, id); err != nil {
		return Created{}, err
	}
	var outcome writeOutcome
	err := s.writeWithAudienceRetry(ctx, func(repos repositories.Repositories) error {
		var err error
		outcome, err = s.cancel(ctx, repos, actor, id, cancellation)
		return err
	})
	if err != nil {
		return Created{}, err
	}
	if outcome.changed {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "lesson.cancelled", tutorAttribute(tutor), lessonAttributes(id),
			slog.String("scope", string(cancellation.Scope)), slog.String("cause", string(domain.CancelByTutor)), slog.Int("count", outcome.count))
		s.logRows(ctx, outcome.rows)
	}
	return outcome.result, nil
}

func (s *Service) currentState(ctx context.Context, repos repositories.Repositories, write lessonWrite, scope domain.EditScope) (writeOutcome, error) {
	ids := []domain.LessonID{write.selected.ID}
	var series *SeriesView
	if scope == domain.ScopeFollowing {
		ids = write.coveredIDs()
		series = seriesView(write.series.Series)
	}
	lessons, err := s.tutorViews(ctx, repos, write.account, ids)
	return writeOutcome{result: Created{Series: series, Lessons: lessons}}, err
}

func endsBefore(series *repositories.LockedSeriesForEdit, occurrence domain.LocalDate) bool {
	return series != nil && series.EndDate != nil && series.EndDate.Before(occurrence)
}

func (s *Service) cancel(
	ctx context.Context, repos repositories.Repositories, actor domain.TutorActor, id domain.LessonID, cancellation LessonCancellation,
) (writeOutcome, error) {
	write, err := s.lockWrite(ctx, repos, actor, id, cancellation.Scope)
	if err != nil {
		return writeOutcome{}, err
	}
	now, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return writeOutcome{}, err
	}
	if write.selected.Status == domain.LessonCancelled || cancellation.Scope == domain.ScopeFollowing && endsBefore(write.series, write.occurrenceDate()) {
		return s.currentState(ctx, repos, write, cancellation.Scope)
	}
	if err := s.checkState(write, cancellation.Version, now); err != nil {
		return writeOutcome{}, err
	}
	if write.membersChanged {
		return writeOutcome{}, errMembersChanged
	}
	targets := []domain.LessonID{write.selected.ID}
	if cancellation.Scope == domain.ScopeFollowing {
		targets = nil
		for _, lesson := range write.covered {
			if editable(lesson, now) {
				targets = append(targets, lesson.ID)
			}
		}
	}
	cancelled, err := repos.Lessons.CancelByTutor(ctx, targets, now)
	if err != nil {
		return writeOutcome{}, err
	}
	students := write.audience.students()
	for _, lesson := range cancelled {
		if err := repos.Lessons.FreezeRoster(ctx, lesson, students, now); err != nil {
			return writeOutcome{}, err
		}
	}
	if err := signalLessons(ctx, repos, write.account.ID, students, cancelled); err != nil {
		return writeOutcome{}, err
	}
	outcome := writeOutcome{changed: true, count: len(cancelled)}
	if cancellation.Scope == domain.ScopeFollowing {
		if err := repos.Lessons.EndSeriesBefore(ctx, write.series.ID, write.occurrenceDate(), now); err != nil {
			return writeOutcome{}, err
		}
		outcome.rows, err = s.queueSeriesChanged(ctx, repos, write, write.audience, orderedCancelled(write, cancelled), now)
	} else {
		outcome.rows, err = s.queueCancelled(ctx, repos, write, now)
	}
	if err != nil {
		return writeOutcome{}, err
	}
	state, err := s.cancelledState(ctx, repos, write, cancellation.Scope)
	outcome.result = state
	return outcome, err
}

func orderedCancelled(write lessonWrite, cancelled []domain.LessonID) []domain.LessonID {
	done := map[domain.LessonID]bool{}
	for _, lesson := range cancelled {
		done[lesson] = true
	}
	ordered := make([]domain.LessonID, 0, len(cancelled))
	for _, lesson := range write.covered {
		if done[lesson.ID] {
			ordered = append(ordered, lesson.ID)
		}
	}
	return ordered
}

func (s *Service) cancelledState(ctx context.Context, repos repositories.Repositories, write lessonWrite, scope domain.EditScope) (Created, error) {
	if scope != domain.ScopeFollowing {
		lessons, err := s.tutorViews(ctx, repos, write.account, []domain.LessonID{write.selected.ID})
		return Created{Lessons: lessons}, err
	}
	series, err := repos.Lessons.SeriesViews(ctx, []domain.SeriesID{write.series.ID})
	if err != nil {
		return Created{}, err
	}
	lessons, err := s.tutorViews(ctx, repos, write.account, write.coveredIDs())
	if err != nil || len(series) == 0 {
		return Created{Lessons: lessons}, err
	}
	return Created{Series: seriesView(series[0]), Lessons: lessons}, nil
}

func (s *Service) queueCancelled(ctx context.Context, repos repositories.Repositories, write lessonWrite, now time.Time) ([]queuedNotification, error) {
	var rows []queuedNotification
	for _, target := range studentTargets(write.audience) {
		key := domain.LessonCancelledKey(write.selected.ID, target.user)
		payload, err := encodeJSON(domain.LessonCancelledPayload{Lesson: hexID(write.selected.ID), Recipient: hexID(target.user)})
		if err != nil {
			return nil, err
		}
		if err := s.queueLessonEmail(ctx, repos, lessonEmail{
			kind: domain.EventLessonCancelled, priority: domain.PriorityNormal, payload: payload, key: key, now: now,
		}); err != nil {
			return nil, err
		}
		row, err := s.insertLessonRow(ctx, repos, lessonRow{
			event: domain.NotificationLessonCancelled, tutor: write.account.ID, subject: write.selected.ID, target: target, key: key, now: now,
		})
		if err != nil {
			return nil, err
		}
		if row != nil {
			rows = append(rows, *row)
		}
	}
	return rows, nil
}
