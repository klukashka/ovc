package chat

import (
	"context"
	"log/slog"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type Pinned struct {
	MessageID         domain.MessageID
	Seq               int64
	SenderLabel       *string
	SenderLabelKind   *domain.LabelKind
	Own               bool
	Preview           *string
	SentBeforeReclaim bool
}

func everyMember(repositories.ActiveMember) bool {
	return true
}

func (s *Service) Pin(ctx context.Context, author Author, channel domain.ChannelID, message domain.MessageID) error {
	now, err := s.now(ctx)
	if err != nil {
		return err
	}
	release, err := s.takeWrite(ctx, author.UserID, now)
	if err != nil {
		return err
	}
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		membership, err := s.lockChannelAsTutor(ctx, repos, author, channel, now)
		if err != nil {
			return err
		}
		if err := liveMessageOf(ctx, repos, channel, message, fieldPinnedMessage); err != nil {
			return err
		}
		if membership.ReadOnlySince != nil {
			return ErrReadOnly
		}
		if err := repos.Channels.SetPin(ctx, channel, message, now); err != nil {
			return err
		}
		return signalMembers(ctx, repos, []domain.ChannelID{channel}, everyMember)
	})
	if err != nil {
		release()
		return err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.message.pinned", channelAttribute(channel), messageAttribute(message), userAttribute(author.UserID))
	return nil
}

func (s *Service) Unpin(ctx context.Context, author Author, channel domain.ChannelID) error {
	now, err := s.now(ctx)
	if err != nil {
		return err
	}
	release, err := s.takeWrite(ctx, author.UserID, now)
	if err != nil {
		return err
	}
	var unpinned *domain.MessageID
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if _, err := s.lockChannelAsTutor(ctx, repos, author, channel, now); err != nil {
			return err
		}
		pinned, err := repos.Channels.Pin(ctx, channel)
		if err != nil || pinned == nil {
			return err
		}
		cleared, err := repos.Channels.ClearPin(ctx, channel, now)
		if err != nil || !cleared {
			return err
		}
		unpinned = pinned
		return signalMembers(ctx, repos, []domain.ChannelID{channel}, everyMember)
	})
	if err != nil {
		release()
		return err
	}
	if unpinned != nil {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.message.unpinned", channelAttribute(channel), messageAttribute(*unpinned),
			userAttribute(author.UserID))
	}
	return nil
}

func (s *Service) lockChannelAsTutor(
	ctx context.Context, repos repositories.Repositories, author Author, channel domain.ChannelID, now time.Time,
) (repositories.ActiveMembership, error) {
	if author.Role != domain.RoleTutor {
		return repositories.ActiveMembership{}, ErrNotFound
	}
	if err := s.lockAuthor(ctx, repos, author, now); err != nil {
		return repositories.ActiveMembership{}, err
	}
	membership, err := lockMembership(ctx, repos, channel, author.UserID, true)
	if err != nil {
		return repositories.ActiveMembership{}, err
	}
	if membership.TutorID != author.UserID {
		return repositories.ActiveMembership{}, ErrNotFound
	}
	return membership, nil
}

func (s *Service) logUnpinned(ctx context.Context, channel domain.ChannelID, message domain.MessageID, tutor domain.UserID, cause domain.UnpinCause) {
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.message.unpinned", channelAttribute(channel), messageAttribute(message), userAttribute(tutor),
		slog.String("cause", string(cause)))
}

func pinnedView(ctx context.Context, repos repositories.Repositories, viewer domain.UserID, channel domain.ChannelID) (*Pinned, error) {
	id, err := repos.Channels.Pin(ctx, channel)
	if err != nil || id == nil {
		return nil, err
	}
	rows, err := repos.Messages.Views(ctx, []domain.MessageID{*id})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	row := rows[0]
	if row.DeletedAt != nil || row.ErasedAt != nil {
		return nil, nil
	}
	from := senderOf(viewer, row)
	return &Pinned{
		MessageID: row.ID, Seq: row.Seq, SenderLabel: from.label, SenderLabelKind: from.kind, Own: from.own, Preview: previewOf(row),
		SentBeforeReclaim: row.SentBeforeReclaim,
	}, nil
}
