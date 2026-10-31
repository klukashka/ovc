package chat

import (
	"context"
	"log/slog"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type typingEffect bool

const (
	typingRelayed typingEffect = true
	typingChecked typingEffect = false
)

func (s *Service) PersistPresence(ctx context.Context, instance domain.PresenceInstanceID, users []domain.UserID, online bool) ([]domain.UserID, error) {
	now, err := s.now(ctx)
	if err != nil {
		return nil, err
	}
	until := now
	if online {
		until = now.Add(domain.PresenceWindow)
	}
	requested := sortedIDs(users)
	var found []domain.UserID
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := repos.Presence.LockUsers(ctx, requested)
		if err != nil {
			return err
		}
		found = locked
		if _, err := repos.Presence.LockSummaries(ctx, found, now); err != nil {
			return err
		}
		if online {
			if err := repos.Presence.Extend(ctx, instance, found, until, now); err != nil {
				return err
			}
		} else if err := repos.Presence.Release(ctx, instance, found); err != nil {
			return err
		}
		return repos.Presence.Recompute(ctx, found, now)
	})
	if err != nil {
		return nil, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "presence.batch", slog.Bool("online", online), slog.Int("accounts", len(found)),
		slog.Int("skipped", len(requested)-len(found)))
	return found, nil
}

func (s *Service) AnnouncePresence(ctx context.Context, account domain.UserID, _ bool) error {
	var recipients []domain.UserID
	var online bool
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		users, err := repos.Presence.LockUsers(ctx, []domain.UserID{account})
		if err != nil || len(users) == 0 {
			return err
		}
		summaries, err := repos.Presence.ExistingSummaries(ctx, users)
		if err != nil || len(summaries) == 0 {
			return err
		}
		now, err := s.now(ctx)
		if err != nil {
			return err
		}
		online = summaries[0].OnlineUntil.After(now)
		recipients, err = repos.Presence.Recipients(ctx, account)
		if err != nil {
			return err
		}
		return repos.Signals.Presence(ctx, recipients, account, online)
	})
	if err != nil {
		return err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "presence.announced", userAttribute(account), slog.Bool("online", online),
		slog.Int("recipients", len(recipients)))
	return nil
}

func (s *Service) Typing(ctx context.Context, typist domain.UserID, channel domain.ChannelID) error {
	return s.typing(ctx, typist, channel, typingRelayed)
}

func (s *Service) CheckTyping(ctx context.Context, typist domain.UserID, channel domain.ChannelID) error {
	return s.typing(ctx, typist, channel, typingChecked)
}

func (s *Service) typing(ctx context.Context, typist domain.UserID, channel domain.ChannelID, effect typingEffect) error {
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		membership, err := lockMembership(ctx, repos, channel, typist, false)
		if err != nil {
			return err
		}
		if membership.ReadOnlySince != nil {
			return ErrReadOnly
		}
		if effect == typingChecked {
			return nil
		}
		members, err := repos.Channels.ActiveMembers(ctx, []domain.ChannelID{channel})
		if err != nil {
			return err
		}
		signal := domain.TypingSignal{Channel: channel, Account: typist}
		for _, member := range members {
			switch {
			case member.UserID == typist:
			case typist == membership.TutorID || member.UserID == member.TutorID:
				signal.Shown = append(signal.Shown, member.UserID)
			default:
				signal.Anonymous = append(signal.Anonymous, member.UserID)
			}
		}
		if len(signal.Shown)+len(signal.Anonymous) == 0 {
			return nil
		}
		return repos.Signals.Typing(ctx, signal)
	})
}
