package chat

import (
	"context"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func (s *Service) ExpirePresence(ctx context.Context, now time.Time) (int64, error) {
	settings := s.deps.PresenceCleanup
	deadline := settings.Wall().Add(settings.Budget)
	bounded, cancel := context.WithTimeout(ctx, settings.Budget)
	defer cancel()
	var deleted int64
	for settings.Wall().Before(deadline) {
		candidates, err := s.deps.Store.Repositories().Presence.Expired(bounded, now, settings.Batch)
		if err != nil {
			return deleted, err
		}
		if len(candidates) == 0 {
			return deleted, nil
		}
		count, err := s.expirePresenceBatch(bounded, candidates, now)
		if err != nil {
			return deleted, err
		}
		deleted += count
	}
	return deleted, nil
}

func (s *Service) expirePresenceBatch(ctx context.Context, leases []repositories.ExpiredPresence, now time.Time) (int64, error) {
	var deleted int64
	users := make([]domain.UserID, 0, len(leases))
	for _, lease := range leases {
		users = append(users, lease.UserID)
	}
	users = sortedIDs(users)
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		deleted = 0
		found, err := repos.Presence.LockUsers(ctx, users)
		if err != nil {
			return err
		}
		if _, err := repos.Presence.LockSummaries(ctx, found, now); err != nil {
			return err
		}
		retained := make(map[domain.UserID]bool, len(found))
		for _, user := range found {
			retained[user] = true
		}
		for _, lease := range leases {
			if !retained[lease.UserID] {
				continue
			}
			count, err := repos.Presence.DeleteExpired(ctx, lease, now)
			if err != nil {
				return err
			}
			deleted += count
		}
		return repos.Presence.RecomputeExpired(ctx, found)
	})
	return deleted, err
}
