package tutoring

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type lessonAudience struct {
	links  map[domain.RelationshipID]domain.UserID
	groups map[domain.GroupID][]domain.UserID
}

func (a lessonAudience) of(lesson repositories.EffectLesson) []domain.UserID {
	if lesson.RelationshipID != nil {
		if student, found := a.links[*lesson.RelationshipID]; found {
			return []domain.UserID{student}
		}
		return nil
	}
	if lesson.GroupID != nil {
		return a.groups[*lesson.GroupID]
	}
	return nil
}

func linkAudience(link domain.RelationshipID, student domain.UserID) lessonAudience {
	return lessonAudience{links: map[domain.RelationshipID]domain.UserID{link: student}}
}

func endedLinkAudience(link domain.RelationshipID, student domain.UserID, groups []domain.GroupID) lessonAudience {
	audience := lessonAudience{links: map[domain.RelationshipID]domain.UserID{link: student}, groups: make(map[domain.GroupID][]domain.UserID, len(groups))}
	for _, group := range groups {
		audience.groups[group] = []domain.UserID{student}
	}
	return audience
}

type lessonCancellation struct {
	scope    repositories.EffectScope
	cause    domain.CancelCause
	at       time.Time
	audience lessonAudience
}

type lessonEffects struct {
	tutor     domain.UserID
	cause     domain.CancelCause
	cancelled int
	series    int
	underWay  int
	frozen    int64
}

func (s *Service) endSeries(ctx context.Context, repos repositories.Repositories, scope repositories.EffectScope, at time.Time) (int, error) {
	ended, err := repos.Lessons.EndSeriesForEffect(ctx, scope, at)
	return len(ended), err
}

func (s *Service) cancelLessons(ctx context.Context, repos repositories.Repositories, cancellation lessonCancellation) (lessonEffects, error) {
	effects := lessonEffects{tutor: cancellation.scope.Tutor, cause: cancellation.cause}
	cancelled, err := repos.Lessons.CancelForEffect(ctx, cancellation.scope, cancellation.cause, cancellation.at)
	if err != nil {
		return lessonEffects{}, err
	}
	var entries []repositories.RosterEntry
	signals := make([]domain.EntitySignal, 0, len(cancelled))
	for _, lesson := range cancelled {
		students := cancellation.audience.of(lesson)
		for _, student := range students {
			entries = append(entries, repositories.RosterEntry{Lesson: lesson.ID, Student: student})
		}
		recipients := append([]domain.UserID{cancellation.scope.Tutor}, students...)
		signals = append(signals, domain.EntitySignal{Users: recipients, Entity: domain.EntityLesson, ID: domain.EntityID(lesson.ID)})
	}
	if effects.frozen, err = repos.Lessons.FreezeRosters(ctx, entries, cancellation.at); err != nil {
		return lessonEffects{}, err
	}
	if err := repos.Signals.Entities(ctx, signals); err != nil {
		return lessonEffects{}, err
	}
	effects.cancelled = len(cancelled)
	return effects, nil
}

func (s *Service) freezeUnderWay(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, audience lessonAudience, at time.Time, effects *lessonEffects,
) error {
	lessons, err := repos.Lessons.UnderWayOfTutor(ctx, tutor, at)
	if err != nil {
		return err
	}
	var entries []repositories.RosterEntry
	for _, lesson := range lessons {
		students := audience.of(lesson)
		if len(students) == 0 {
			continue
		}
		effects.underWay++
		for _, student := range students {
			entries = append(entries, repositories.RosterEntry{Lesson: lesson.ID, Student: student})
		}
	}
	frozen, err := repos.Lessons.FreezeRosters(ctx, entries, at)
	effects.frozen += frozen
	return err
}

func (s *Service) endLinkLessons(
	ctx context.Context, repos repositories.Repositories, ended repositories.Relationship, student domain.UserID, groups []domain.GroupID,
) (lessonEffects, error) {
	if ended.EndedAt == nil {
		return lessonEffects{}, errEndedWithoutTime
	}
	scope := repositories.EffectScope{Tutor: ended.TutorID, Relationships: []domain.RelationshipID{ended.ID}}
	series, err := s.endSeries(ctx, repos, scope, *ended.EndedAt)
	if err != nil {
		return lessonEffects{}, err
	}
	effects, err := s.cancelLessons(ctx, repos, lessonCancellation{
		scope: scope, cause: domain.CancelRelationshipEnded, at: *ended.EndedAt, audience: linkAudience(ended.ID, student),
	})
	if err != nil {
		return lessonEffects{}, err
	}
	effects.series = series
	err = s.freezeUnderWay(ctx, repos, ended.TutorID, endedLinkAudience(ended.ID, student, groups), *ended.EndedAt, &effects)
	return effects, err
}

func (s *Service) discardLinkLessons(ctx context.Context, repos repositories.Repositories, links []repositories.LinkStudent, tutor domain.UserID, at time.Time) (lessonEffects, error) {
	if len(links) == 0 {
		return lessonEffects{tutor: tutor, cause: domain.CancelProvisionalRemoved}, nil
	}
	relationships := make([]domain.RelationshipID, 0, len(links))
	for _, link := range links {
		relationships = append(relationships, link.Relationship)
	}
	scope := repositories.EffectScope{Tutor: tutor, Relationships: relationships}
	series, err := s.endSeries(ctx, repos, scope, at)
	if err != nil {
		return lessonEffects{}, err
	}
	effects, err := s.cancelLessons(ctx, repos, lessonCancellation{scope: scope, cause: domain.CancelProvisionalRemoved, at: at})
	if err != nil {
		return lessonEffects{}, err
	}
	if _, err := repos.Lessons.DeleteRosterRowsOfLinks(ctx, links); err != nil {
		return lessonEffects{}, err
	}
	effects.series = series
	return effects, nil
}

func (s *Service) logLessonEffects(ctx context.Context, effects ...lessonEffects) {
	for _, effect := range effects {
		if effect.cancelled == 0 && effect.series == 0 && effect.underWay == 0 {
			continue
		}
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "lessons.cancelled_by_effect", slog.String("tutor_id", effect.tutor.String()),
			slog.String("cause", string(effect.cause)), slog.Int("count", effect.cancelled), slog.Int("series_ended", effect.series),
			slog.Int("under_way", effect.underWay), slog.Int64("roster_rows", effect.frozen))
	}
}
