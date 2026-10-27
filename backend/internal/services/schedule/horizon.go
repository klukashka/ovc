package schedule

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/domain/schedule"
	"ovc/backend/internal/repositories"
)

var firstCandidateDate = domain.LocalDate{Year: 1, Month: time.January, Day: 1}

func (j *Jobs) ExtendHorizons(ctx context.Context, now time.Time) (int64, error) {
	deadline := j.settings.Wall().Add(j.settings.ExtensionBudget)
	after := repositories.SeriesCandidate{MaterialisedUntil: firstCandidateDate}
	var extended, created int64
	var failures []error
	for j.settings.Wall().Before(deadline) {
		batch, err := j.service.deps.Store.Repositories().Lessons.SeriesToExtend(ctx, repositories.SeriesCandidateQuery{
			Now: now, Horizon: now.Add(domain.SeriesHorizon), After: after, Limit: j.settings.ExtensionBatch,
		})
		if err != nil {
			failures = append(failures, err)
			break
		}
		for _, candidate := range batch {
			inserted, err := j.extend(ctx, candidate.ID, now)
			if err != nil {
				failures = append(failures, err)
				j.service.deps.Logger.LogAttrs(ctx, slog.LevelError, "series.extension_failed", slog.String("series_id", candidate.ID.String()),
					slog.String("kind", core.ErrorKind(err)))
				continue
			}
			if inserted > 0 {
				extended++
				created += inserted
			}
		}
		if len(batch) < int(j.settings.ExtensionBatch) {
			break
		}
		after = batch[len(batch)-1]
	}
	j.service.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "series.extended", slog.Int64("count", extended), slog.Int64("lessons", created))
	return extended, errors.Join(failures...)
}

func (j *Jobs) extend(ctx context.Context, id domain.SeriesID, now time.Time) (int64, error) {
	var inserted int64
	_, err := j.withAudienceRetry(ctx, func(repos repositories.Repositories) error {
		inserted = 0
		series, err := repos.Lessons.SeriesJobAudience(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		audience, err := lockJobAudience(ctx, repos, series.TutorID, series.RelationshipID, series.GroupID)
		if err != nil {
			return err
		}
		locked, err := repos.Lessons.LockSeriesForExtension(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if locked.Ended || locked.EndDate == nil || locked.TutorState != domain.AccountActive || !audience.active {
			return nil
		}
		occurrences, through, err := j.missingOccurrences(locked, now)
		if err != nil || !through.After(locked.MaterialisedUntil) {
			return err
		}
		ids, err := repos.Lessons.InsertOccurrences(ctx, id, occurrences, now)
		if err != nil {
			return err
		}
		if err := repos.Lessons.SetMaterialisedUntil(ctx, id, through, now); err != nil {
			return err
		}
		inserted = int64(len(ids))
		return signalLessons(ctx, repos, series.TutorID, audience.students(), ids)
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

func (j *Jobs) missingOccurrences(locked repositories.LockedSeries, now time.Time) ([]repositories.NewOccurrence, domain.LocalDate, error) {
	zone, err := loadZone(locked.Timezone)
	if err != nil {
		return nil, domain.LocalDate{}, err
	}
	series := schedule.Series{Weekdays: locked.Weekdays, First: locked.First, EndDate: locked.EndDate, Start: locked.Start, Zone: zone}
	through, _ := series.Horizon(now)
	if !through.After(locked.MaterialisedUntil) {
		return nil, through, nil
	}
	var occurrences []repositories.NewOccurrence
	for _, occurrence := range schedule.DatesBetween(series, locked.MaterialisedUntil, through) {
		if !occurrence.StartsAt.After(now) {
			continue
		}
		lesson, err := core.NewID[domain.LessonID](j.service.deps.Random)
		if err != nil {
			return nil, domain.LocalDate{}, err
		}
		occurrences = append(occurrences, repositories.NewOccurrence{ID: lesson, Date: occurrence.Date, StartsAt: occurrence.StartsAt})
	}
	return occurrences, through, nil
}

func encodeJSON(payload domain.LessonReminderPayload) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode a lesson email payload: %w", err)
	}
	return encoded, nil
}

func hexID[ID ~[16]byte](id ID) string {
	return hex.EncodeToString(id[:])
}
