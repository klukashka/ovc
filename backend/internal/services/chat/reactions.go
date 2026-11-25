package chat

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const fieldReaction = "body.reaction"

type ReactionMember struct {
	Label     *string
	LabelKind *domain.LabelKind
}

type ReactionView struct {
	Reaction domain.Reaction
	Count    int64
	Mine     bool
	Members  *[]ReactionMember
}

func (s *Service) React(ctx context.Context, author Author, id domain.MessageID, value domain.Reaction) error {
	reaction, valid := domain.ParseReaction(string(value))
	if !valid {
		return &ValidationError{Path: fieldReaction, Code: domain.FieldInvalid}
	}
	return s.changeReaction(ctx, author, id, &reaction)
}

func (s *Service) Unreact(ctx context.Context, author Author, id domain.MessageID) error {
	return s.changeReaction(ctx, author, id, nil)
}

func (s *Service) changeReaction(ctx context.Context, author Author, id domain.MessageID, reaction *domain.Reaction) error {
	now, err := s.now(ctx)
	if err != nil {
		return err
	}
	release, err := s.takeWrite(ctx, author.UserID, now)
	if err != nil {
		return err
	}
	var channel domain.ChannelID
	changed := false
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockAuthor(ctx, repos, author, now); err != nil {
			return err
		}
		scope, err := repos.Messages.Scope(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		channel = scope.ChannelID
		membership, err := lockMembership(ctx, repos, channel, author.UserID, false)
		if err != nil {
			return err
		}
		locked, err := repos.Messages.Lock(ctx, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if locked.DeletedAt != nil || locked.ErasedAt != nil {
			return ErrNotFound
		}
		if reaction == nil {
			changed, err = repos.Messages.RemoveReaction(ctx, id, author.UserID)
		} else {
			if membership.ReadOnlySince != nil {
				return ErrReadOnly
			}
			changed, err = repos.Messages.SetReaction(ctx, id, author.UserID, *reaction, now)
		}
		if err != nil || !changed {
			return err
		}
		return signalMessage(ctx, repos, channel, domain.FrameMessageUpdated, &id, nil)
	})
	if err != nil {
		release()
		return err
	}
	if !changed {
		return nil
	}
	if reaction == nil {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.reaction.removed", channelAttribute(channel), messageAttribute(id), userAttribute(author.UserID))
		return nil
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.reaction.set", channelAttribute(channel), messageAttribute(id), userAttribute(author.UserID),
		slog.String("reaction", string(*reaction)))
	return nil
}

func reactionViews(viewer domain.UserID, rows []repositories.ReactionRow) map[domain.MessageID][]ReactionView {
	type tally struct {
		count   int64
		mine    bool
		members []ReactionMember
		tutor   bool
	}
	grouped := map[domain.MessageID]map[domain.Reaction]*tally{}
	for _, row := range rows {
		byReaction := grouped[row.MessageID]
		if byReaction == nil {
			byReaction = map[domain.Reaction]*tally{}
			grouped[row.MessageID] = byReaction
		}
		entry := byReaction[row.Reaction]
		if entry == nil {
			entry = &tally{tutor: viewer == row.TutorID}
			byReaction[row.Reaction] = entry
		}
		entry.count++
		if row.UserID == viewer {
			entry.mine = true
			continue
		}
		if entry.tutor {
			from := senderLabel(labelInput{viewer: viewer, tutor: row.TutorID, sender: &row.UserID, state: row.ReactorState, name: row.ReactorName,
				card: row.CardName})
			entry.members = append(entry.members, ReactionMember{Label: from.label, LabelKind: from.kind})
		}
	}
	views := make(map[domain.MessageID][]ReactionView, len(grouped))
	for message, byReaction := range grouped {
		list := make([]ReactionView, 0, len(byReaction))
		for reaction, entry := range byReaction {
			view := ReactionView{Reaction: reaction, Count: entry.count, Mine: entry.mine}
			if entry.tutor {
				members := entry.members
				slices.SortStableFunc(members, compareMembers)
				if members == nil {
					members = []ReactionMember{}
				}
				view.Members = &members
			}
			list = append(list, view)
		}
		slices.SortFunc(list, func(left, right ReactionView) int {
			return domain.ReactionRank(left.Reaction) - domain.ReactionRank(right.Reaction)
		})
		views[message] = list
	}
	return views
}

func compareMembers(left, right ReactionMember) int {
	switch {
	case left.Label == nil && right.Label == nil:
		return 0
	case left.Label == nil:
		return 1
	case right.Label == nil:
		return -1
	}
	return strings.Compare(*left.Label, *right.Label)
}
