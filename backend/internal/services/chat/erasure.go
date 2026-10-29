package chat

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func (s *Service) EraseChannels(ctx context.Context, now time.Time) (int64, error) {
	pending, err := s.deps.Store.Repositories().Channels.PendingErasure(ctx, erasureBatch)
	if err != nil {
		return 0, err
	}
	var erased int64
	var failures []error
	for _, channel := range pending {
		done, err := s.erase(ctx, channel, now)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if done {
			erased++
		}
	}
	return erased, errors.Join(failures...)
}

func (s *Service) erase(ctx context.Context, channel repositories.PendingErasure, now time.Time) (bool, error) {
	cleared := false
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		err := repos.TutorProfiles.Lock(ctx, channel.TutorID)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		err = repos.Channels.LockPendingErasure(ctx, channel.ID)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		cleared, err = repos.Channels.ClearErasure(ctx, channel.ID, now)
		return err
	})
	if err != nil || !cleared {
		return false, err
	}
	cause := domain.DiscardRemoved
	if channel.DiscardReason != nil {
		cause = *channel.DiscardReason
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "channel.erased", slog.String("channel_id", channel.ID.String()),
		slog.String("tutor_id", channel.TutorID.String()), slog.String("cause", string(cause)), slog.Int("messages", 0))
	return true, nil
}
