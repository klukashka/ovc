package chat

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type erasureStep struct {
	erased int64
	done   bool
	gone   bool
}

func (s *Service) EraseChannels(ctx context.Context, now time.Time) (int64, error) {
	settings := s.deps.Erasure
	deadline := settings.Wall().Add(settings.Budget)
	bounded, cancel := context.WithTimeout(ctx, settings.Budget)
	defer cancel()
	pending, err := s.deps.Store.Repositories().Channels.PendingErasure(bounded, erasureBatch)
	if err != nil {
		return 0, err
	}
	var erased int64
	var failures []error
	for _, channel := range pending {
		if !settings.Wall().Before(deadline) {
			break
		}
		var messages int64
		for settings.Wall().Before(deadline) {
			step, err := s.eraseBatch(bounded, channel, now)
			if err != nil {
				failures = append(failures, err)
				if bounded.Err() != nil {
					return erased, errors.Join(append(failures, bounded.Err())...)
				}
				break
			}
			messages += step.erased
			if step.gone {
				break
			}
			if step.done {
				erased++
				s.logErased(ctx, channel, messages)
				break
			}
		}
	}
	return erased, errors.Join(failures...)
}

func (s *Service) eraseBatch(ctx context.Context, channel repositories.PendingErasure, now time.Time) (erasureStep, error) {
	var step erasureStep
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		step = erasureStep{}
		err := repos.TutorProfiles.Lock(ctx, channel.TutorID)
		if errors.Is(err, repositories.ErrNotFound) {
			step.gone = true
			return nil
		}
		if err != nil {
			return err
		}
		err = repos.Channels.LockPendingErasure(ctx, channel.ID)
		if errors.Is(err, repositories.ErrNotFound) {
			step.gone = true
			return nil
		}
		if err != nil {
			return err
		}
		batch := s.deps.Erasure.MessageBatch
		messages, err := repos.Messages.LockBatchOfChannel(ctx, channel.ID, batch)
		if err != nil {
			return err
		}
		if err := s.release().messages(ctx, repos, messages, now); err != nil {
			return err
		}
		if step.erased, err = repos.Messages.Delete(ctx, messages); err != nil {
			return err
		}
		if len(messages) == int(batch) {
			return nil
		}
		step.done = true
		_, err = repos.Channels.ClearErasure(ctx, channel.ID, now)
		return err
	})
	return step, err
}

func (s *Service) logErased(ctx context.Context, channel repositories.PendingErasure, messages int64) {
	cause := domain.DiscardRemoved
	if channel.DiscardReason != nil {
		cause = *channel.DiscardReason
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "channel.erased", channelAttribute(channel.ID), tutorAttribute(channel.TutorID),
		slog.String("cause", string(cause)), slog.Int64("messages", messages))
}
