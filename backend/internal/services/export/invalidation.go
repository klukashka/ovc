package export

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type invalidated struct {
	entry     repositories.ExportInvalidation
	row       repositories.Export
	found     bool
	withdrawn bool
}

func (i invalidated) holdsOpenIntent() bool {
	if !i.found || i.row.BuildID == nil {
		return false
	}
	if i.entry.Purged {
		return i.row.State == domain.ExportBuilding || i.row.State == domain.ExportFailed
	}
	return i.withdrawn && i.row.State == domain.ExportBuilding
}

type invalidationPass struct {
	users      int
	locked     int
	statements int
	causes     []string
	withdrawn  []invalidated
}

func (s *Service) Apply(ctx context.Context, repos repositories.Repositories, now time.Time) error {
	if err := repos.StartExportWithdrawal(); err != nil {
		return err
	}
	entries := repos.TakeExportInvalidations()
	if len(entries) == 0 {
		return nil
	}
	started := s.deps.Wall()
	pass := invalidationPass{users: len(entries)}
	if err := repos.Exports.LockUsersForChange(ctx, usersOf(entries)...); err != nil {
		return err
	}
	outcomes, err := s.lockInvalidated(ctx, repos, entries, &pass)
	if err != nil {
		return err
	}
	if err := s.settleRows(ctx, repos, outcomes, now); err != nil {
		return err
	}
	if err := releaseLimits(ctx, repos, outcomes); err != nil {
		return err
	}
	for _, outcome := range outcomes {
		if outcome.holdsOpenIntent() {
			if err := s.requestCleanupIfOpen(ctx, repos, *outcome.row.BuildID, now); err != nil {
				return err
			}
		}
	}
	for _, outcome := range outcomes {
		if !outcome.withdrawn {
			continue
		}
		pass.withdrawn = append(pass.withdrawn, outcome)
		if outcome.entry.Own {
			continue
		}
		if err := s.notifyOwner(ctx, repos, outcome.row, *outcome.row.BuildID, domain.NotificationExportFailed, now); err != nil {
			return err
		}
	}
	pass.causes = causesOf(entries)
	repos.AfterCommit(ctx, func(ctx context.Context) { s.recordInvalidation(ctx, pass, started) })
	return nil
}

func (s *Service) Purge(ctx context.Context, repos repositories.Repositories, user domain.UserID, now time.Time) error {
	if err := repos.PurgeExportOf(user); err != nil {
		return err
	}
	return s.Apply(ctx, repos, now)
}

func (s *Service) lockInvalidated(
	ctx context.Context, repos repositories.Repositories, entries []repositories.ExportInvalidation, pass *invalidationPass,
) ([]invalidated, error) {
	rows := make(map[domain.UserID]repositories.Export, len(entries))
	for chunk := range slices.Chunk(entries, domain.ExportInvalidationBatch) {
		users := make([]domain.UserID, 0, len(chunk))
		for _, entry := range chunk {
			users = append(users, entry.User)
		}
		locked, err := repos.Exports.LockOfUsers(ctx, users)
		if err != nil {
			return nil, err
		}
		pass.statements++
		pass.locked += len(locked)
		for _, row := range locked {
			rows[row.UserID] = row
		}
	}
	outcomes := make([]invalidated, 0, len(entries))
	for _, entry := range entries {
		row, found := rows[entry.User]
		outcomes = append(outcomes, invalidated{entry: entry, row: row, found: found})
	}
	return outcomes, nil
}

func (s *Service) settleRows(ctx context.Context, repos repositories.Repositories, outcomes []invalidated, now time.Time) error {
	for index := range outcomes {
		outcome := &outcomes[index]
		if !outcome.found {
			continue
		}
		if outcome.entry.Purged {
			if err := s.deleteForPurge(ctx, repos, outcome.row, now); err != nil {
				return err
			}
			continue
		}
		withdrawn, err := s.failWithdrawn(ctx, repos, outcome.row, now)
		if err != nil {
			return err
		}
		outcome.withdrawn = withdrawn
	}
	return nil
}

func (s *Service) failWithdrawn(ctx context.Context, repos repositories.Repositories, row repositories.Export, now time.Time) (bool, error) {
	switch row.State {
	case domain.ExportBuilding:
		_, err := repos.Exports.Fail(ctx, row.ID, domain.ExportFailureWithdrawn, now)
		return err == nil, err
	case domain.ExportReady:
		if _, err := repos.Exports.Fail(ctx, row.ID, domain.ExportFailureWithdrawn, now); err != nil {
			return false, err
		}
		if _, err := repos.Exports.ClearStorage(ctx, row.ID, now); err != nil {
			return false, err
		}
		return true, s.queueObjectDeletion(ctx, repos, row, now)
	case domain.ExportQueued, domain.ExportFailed, domain.ExportExpired:
	}
	return false, nil
}

func (s *Service) deleteForPurge(ctx context.Context, repos repositories.Repositories, row repositories.Export, now time.Time) error {
	if row.State == domain.ExportReady {
		if err := s.queueObjectDeletion(ctx, repos, row, now); err != nil {
			return err
		}
	}
	_, err := repos.Exports.Delete(ctx, row.ID)
	return err
}

func releaseLimits(ctx context.Context, repos repositories.Repositories, outcomes []invalidated) error {
	for _, outcome := range outcomes {
		switch {
		case outcome.entry.Purged:
			if _, err := repos.Exports.DeleteLimit(ctx, outcome.entry.User); err != nil {
				return err
			}
		case outcome.withdrawn && outcome.row.State == domain.ExportReady:
			if _, err := repos.Exports.DeleteLimitOf(ctx, outcome.row.UserID, outcome.row.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func causesOf(entries []repositories.ExportInvalidation) []string {
	var causes []string
	for _, entry := range entries {
		if entry.Cause != "" && !slices.Contains(causes, string(entry.Cause)) {
			causes = append(causes, string(entry.Cause))
		}
	}
	slices.Sort(causes)
	return causes
}

func (s *Service) recordInvalidation(ctx context.Context, pass invalidationPass, started time.Time) {
	for _, outcome := range pass.withdrawn {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "export.invalidated", exportAttribute(outcome.row.ID), buildAttribute(outcome.row.BuildID),
			userAttribute(outcome.row.UserID), slog.String("state", string(outcome.row.State)), slog.String("cause", string(outcome.entry.Cause)))
	}
	level := slog.LevelDebug
	if len(pass.withdrawn) > 0 {
		level = slog.LevelInfo
	}
	s.deps.Logger.LogAttrs(ctx, level, "export.invalidation.applied", slog.String("causes", strings.Join(pass.causes, ",")),
		slog.Int("users", pass.users), slog.Int("locked", pass.locked), slog.Int("withdrawn", len(pass.withdrawn)),
		slog.Int("statements", pass.statements), slog.Int64("duration_ms", s.deps.Wall().Sub(started).Milliseconds()))
}

func usersOf(entries []repositories.ExportInvalidation) []domain.UserID {
	users := make([]domain.UserID, 0, len(entries))
	for _, entry := range entries {
		users = append(users, entry.User)
	}
	return users
}
