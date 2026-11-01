package chat

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	fieldBody     = "body.body"
	fieldAfterSeq = "query.after_seq"
)

type Draft struct {
	Body        string
	Attachments []domain.AttachmentID
	ClientKey   *string
}

type HistoryQuery struct {
	Cursor   string
	AfterSeq *int64
	Limit    int32
}

type History struct {
	Items      []Message
	NextCursor *string
}

type ReadState struct {
	UnreadCount    int64
	UnreadChannels int64
}

func validBody(body string, attachments int) *ValidationError {
	for _, character := range body {
		if character != '\n' && character != '\t' && unicode.IsControl(character) {
			return &ValidationError{Path: fieldBody, Code: domain.FieldInvalid}
		}
	}
	if utf8.RuneCountInString(body) > domain.MessageBodyMaxCodePoints {
		return &ValidationError{Path: fieldBody, Code: domain.FieldTooLong}
	}
	if attachments == 0 && strings.TrimSpace(body) == "" {
		return &ValidationError{Path: fieldBody, Code: domain.FieldRequired}
	}
	return nil
}

func (s *Service) release() Release {
	return Release{Random: s.deps.Random, Logger: s.deps.Logger}
}

func (s *Service) now(ctx context.Context) (time.Time, error) {
	return s.deps.Clock.Instant(ctx)
}

func (s *Service) takeWrite(ctx context.Context, user domain.UserID, now time.Time) (func(), error) {
	release, retryAfter, allowed := s.writes.take(user, now)
	if !allowed {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.rate_limited", userAttribute(user), slog.String("limit", string(domain.LimitWrites)))
		return nil, &ThrottledError{RetryAfter: retryAfter}
	}
	return release, nil
}

func (s *Service) Send(ctx context.Context, author Author, channel domain.ChannelID, draft Draft) (Message, error) {
	now, err := s.now(ctx)
	if err != nil {
		return Message{}, err
	}
	if draft.ClientKey != nil {
		replayed, found, err := s.replay(ctx, author, channel, *draft.ClientKey, now)
		if err != nil || found {
			return replayed, err
		}
	}
	release, err := s.takeWrite(ctx, author.UserID, now)
	if err != nil {
		return Message{}, err
	}
	message, err := s.send(ctx, author, channel, draft, now)
	if errors.Is(err, repositories.ErrDuplicateClientKey) && draft.ClientKey != nil {
		release()
		replayed, found, err := s.replay(ctx, author, channel, *draft.ClientKey, now)
		if err == nil && !found {
			return Message{}, ErrNotFound
		}
		return replayed, err
	}
	if err != nil {
		release()
		return Message{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.message.persisted", channelAttribute(channel), messageAttribute(message.ID),
		userAttribute(author.UserID), slog.Int("attachment_count", len(message.Attachments)))
	for _, attachment := range message.Attachments {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.added", slog.String("attachment_id", attachment.ID.String()),
			slog.String("ref_kind", string(domain.ReferenceMessage)), messageAttribute(message.ID))
	}
	return message, nil
}

func (s *Service) send(ctx context.Context, author Author, channel domain.ChannelID, draft Draft, now time.Time) (Message, error) {
	attachments := sortedIDs(draft.Attachments)
	id, err := core.NewID[domain.MessageID](s.deps.Random)
	if err != nil {
		return Message{}, err
	}
	var message Message
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockAuthor(ctx, repos, author, now); err != nil {
			return err
		}
		if len(attachments) > 0 && author.Role == domain.RoleStudent {
			if err := lockChannelTutor(ctx, repos, channel); err != nil {
				return err
			}
		}
		membership, err := lockMembership(ctx, repos, channel, author.UserID, true)
		if err != nil {
			return err
		}
		if draft.ClientKey != nil {
			if _, err := repos.Messages.ByClientKey(ctx, author.UserID, channel, *draft.ClientKey); err == nil {
				return repositories.ErrDuplicateClientKey
			} else if !errors.Is(err, repositories.ErrNotFound) {
				return err
			}
		}
		uploads, err := repos.Messages.LockBindable(ctx, author.UserID, channel, attachments, now)
		if err != nil {
			return err
		}
		if len(uploads) != len(attachments) {
			return ErrNotFound
		}
		if membership.ReadOnlySince != nil {
			return ErrReadOnly
		}
		if invalid := validBody(draft.Body, len(draft.Attachments)); invalid != nil {
			return invalid
		}
		seq, err := repos.Messages.AdvanceSeq(ctx, channel, now)
		if err != nil {
			return err
		}
		err = repos.Messages.Insert(ctx, repositories.NewMessage{
			ID: id, Channel: channel, Seq: seq, Sender: author.UserID, ClientKey: draft.ClientKey, Body: draft.Body, Now: now,
		})
		if err != nil {
			return err
		}
		if len(attachments) > 0 {
			if err := repos.Messages.Bind(ctx, id, attachments, now); err != nil {
				return err
			}
			charged, err := repos.Messages.Charge(ctx, membership.TutorID, attachments, now)
			if err != nil {
				return err
			}
			if !charged {
				s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "quota.exceeded", userAttribute(author.UserID), tutorAttribute(membership.TutorID),
					channelAttribute(channel))
				return ErrQuotaExceeded
			}
		}
		if err := repos.Messages.MoveSenderMarker(ctx, channel, author.UserID, seq, now); err != nil {
			return err
		}
		if _, err := repos.Messages.FlagDigest(ctx, channel, author.UserID, now); err != nil {
			return err
		}
		if err := signalMessage(ctx, repos, channel, domain.FrameMessageNew, &id, &seq); err != nil {
			return err
		}
		message, err = s.viewOf(ctx, repos, author.UserID, id, now)
		return err
	})
	return message, err
}

func lockChannelTutor(ctx context.Context, repos repositories.Repositories, channel domain.ChannelID) error {
	tutor, err := repos.Messages.ChannelTutor(ctx, channel)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	err = repos.TutorProfiles.Lock(ctx, tutor)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func lockMembership(
	ctx context.Context, repos repositories.Repositories, channel domain.ChannelID, user domain.UserID, exclusive bool,
) (repositories.ActiveMembership, error) {
	lock := repos.Messages.ShareChannel
	if exclusive {
		lock = repos.Messages.LockChannel
	}
	if err := lock(ctx, channel); errors.Is(err, repositories.ErrNotFound) {
		return repositories.ActiveMembership{}, ErrNotFound
	} else if err != nil {
		return repositories.ActiveMembership{}, err
	}
	membership, err := repos.Messages.Membership(ctx, channel, user)
	if errors.Is(err, repositories.ErrNotFound) {
		return repositories.ActiveMembership{}, ErrNotFound
	}
	return membership, err
}

func signalMessage(
	ctx context.Context, repos repositories.Repositories, channel domain.ChannelID, event domain.FrameType, message *domain.MessageID, seq *int64,
) error {
	members, err := repos.Channels.ActiveMembers(ctx, []domain.ChannelID{channel})
	if err != nil {
		return err
	}
	users := make([]domain.UserID, 0, len(members))
	for _, member := range members {
		users = append(users, member.UserID)
	}
	if len(users) == 0 {
		return nil
	}
	return repos.Signals.Chat(ctx, domain.ChatSignal{Users: users, Event: event, Channel: channel, Message: message, Seq: seq})
}

func (s *Service) replay(ctx context.Context, author Author, channel domain.ChannelID, key string, now time.Time) (Message, bool, error) {
	var message Message
	found := false
	transaction := s.deps.Store.WithReadTx
	if author.Role == domain.RoleTutor {
		transaction = s.deps.Store.WithTx
	}
	err := transaction(ctx, func(repos repositories.Repositories) error {
		if author.Role == domain.RoleTutor {
			if err := s.lockAuthor(ctx, repos, author, now); err != nil {
				return err
			}
		}
		replayed, err := repos.Messages.ByClientKey(ctx, author.UserID, channel, key)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		if _, err := repos.Messages.Membership(ctx, channel, author.UserID); errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		message, err = s.viewOf(ctx, repos, author.UserID, replayed.ID, now)
		return err
	})
	return message, found, err
}

func (s *Service) Edit(ctx context.Context, author Author, id domain.MessageID, body string, version int32) (Message, error) {
	now, err := s.now(ctx)
	if err != nil {
		return Message{}, err
	}
	release, err := s.takeWrite(ctx, author.UserID, now)
	if err != nil {
		return Message{}, err
	}
	var message Message
	var channel domain.ChannelID
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
		if locked.SenderID == nil || *locked.SenderID != author.UserID || locked.DeletedAt != nil || locked.ErasedAt != nil {
			return ErrNotFound
		}
		switch {
		case membership.ReadOnlySince != nil:
			return ErrReadOnly
		case !now.Before(locked.CreatedAt.Add(domain.MessageEditWindow)):
			return ErrEditWindowPassed
		case locked.Version != version:
			return ErrConflict
		}
		attachmentCount := 0
		if scope.HasAttachments {
			attachmentCount = 1
		}
		if invalid := validBody(body, attachmentCount); invalid != nil {
			return invalid
		}
		if _, err := repos.Messages.Edit(ctx, id, body, now); err != nil {
			return err
		}
		if err := signalMessage(ctx, repos, channel, domain.FrameMessageUpdated, &id, nil); err != nil {
			return err
		}
		message, err = s.viewOf(ctx, repos, author.UserID, id, now)
		return err
	})
	if err != nil {
		release()
		return Message{}, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.message.edited", channelAttribute(channel), messageAttribute(id), userAttribute(author.UserID))
	return message, nil
}

func (s *Service) Delete(ctx context.Context, author Author, id domain.MessageID) error {
	now, err := s.now(ctx)
	if err != nil {
		return err
	}
	release, err := s.takeWrite(ctx, author.UserID, now)
	if err != nil {
		return err
	}
	scope, err := s.deps.Store.Repositories().Messages.Scope(ctx, id)
	if errors.Is(err, repositories.ErrNotFound) {
		release()
		return ErrNotFound
	}
	if err != nil {
		release()
		return err
	}
	by := domain.DeletedByAuthor
	deleted := false
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockAuthor(ctx, repos, author, now); err != nil {
			return err
		}
		if scope.HasAttachments && author.Role == domain.RoleStudent {
			if err := lockChannelTutor(ctx, repos, scope.ChannelID); err != nil {
				return err
			}
		}
		membership, err := lockMembership(ctx, repos, scope.ChannelID, author.UserID, scope.HasAttachments)
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
		own := locked.SenderID != nil && *locked.SenderID == author.UserID
		if !own && membership.TutorID != author.UserID {
			return ErrNotFound
		}
		if !own {
			by = domain.DeletedByTutor
		}
		if deleted, err = repos.Messages.MarkDeleted(ctx, id, now); err != nil || !deleted {
			return err
		}
		if err := s.release().messages(ctx, repos, []domain.MessageID{id}, now); err != nil {
			return err
		}
		return signalMessage(ctx, repos, scope.ChannelID, domain.FrameMessageDeleted, &id, nil)
	})
	if err != nil {
		release()
		return err
	}
	if deleted {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.message.deleted", channelAttribute(scope.ChannelID), messageAttribute(id),
			userAttribute(author.UserID), slog.String("by", string(by)))
	}
	return nil
}

func (s *Service) MarkRead(ctx context.Context, author Author, channel domain.ChannelID, seq int64) (ReadState, error) {
	now, err := s.now(ctx)
	if err != nil {
		return ReadState{}, err
	}
	var state ReadState
	moved := false
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if author.Role == domain.RoleStudent {
			if err := lockStudent(ctx, repos, author, now); err != nil {
				return err
			}
		}
		if _, err := lockMembership(ctx, repos, channel, author.UserID, false); err != nil {
			return err
		}
		revision, err := repos.Digests.Revision(ctx, author.UserID)
		if err != nil {
			return err
		}
		moved, err = repos.Messages.MoveReadMarker(ctx, channel, author.UserID, seq, now)
		if err != nil {
			return err
		}
		if moved {
			signal := domain.ChatSignal{Users: []domain.UserID{author.UserID}, Event: domain.FrameReadUpdated, Channel: channel}
			if err := repos.Signals.Chat(ctx, signal); err != nil {
				return err
			}
		}
		membership, err := repos.Messages.Membership(ctx, channel, author.UserID)
		if err != nil {
			return err
		}
		state.UnreadCount = min(membership.LastSeq-membership.LastReadSeq, domain.UnreadCountCap)
		state.UnreadChannels, err = repos.Channels.CountUnread(ctx, author.UserID, domain.UnreadChannelsCap)
		if err != nil {
			return err
		}
		return repos.Digests.ClearRead(ctx, author.UserID, revision)
	})
	if err != nil {
		return ReadState{}, err
	}
	if moved {
		s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "chat.read_moved", channelAttribute(channel), userAttribute(author.UserID))
	}
	return state, nil
}

func (s *Service) History(ctx context.Context, viewer domain.UserID, channel domain.ChannelID, query HistoryQuery) (History, error) {
	listing := repositories.HistoryQuery{Channel: channel, RowLimit: query.Limit + 1}
	switch {
	case query.Cursor != "" && query.AfterSeq != nil:
		return History{}, &ValidationError{Path: fieldAfterSeq, Code: domain.FieldInvalid}
	case query.Cursor != "":
		before, valid := decodeSeqCursor(query.Cursor)
		if !valid {
			return History{}, &ValidationError{Path: fieldCursor, Code: domain.FieldInvalid}
		}
		listing.Before = &before
	case query.AfterSeq != nil:
		listing.After = query.AfterSeq
		listing.RowLimit = query.Limit
	}
	now, err := s.now(ctx)
	if err != nil {
		return History{}, err
	}
	var history History
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		if _, err := repos.Messages.Membership(ctx, channel, viewer); errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		rows, err := repos.Messages.History(ctx, listing)
		if err != nil {
			return err
		}
		if query.AfterSeq == nil && len(rows) > int(query.Limit) {
			rows = rows[:query.Limit]
			next := encodeSeqCursor(rows[len(rows)-1].Seq)
			history.NextCursor = &next
		}
		history.Items, err = s.messageViews(ctx, repos, viewer, rows, now)
		return err
	})
	return history, err
}

func encodeSeqCursor(seq int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(seq, 10)))
}

func decodeSeqCursor(text string) (int64, bool) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil {
		return 0, false
	}
	seq, err := strconv.ParseInt(string(decoded), 10, 64)
	if err != nil || seq <= 0 || strconv.FormatInt(seq, 10) != string(decoded) {
		return 0, false
	}
	return seq, true
}

func userAttribute(user domain.UserID) slog.Attr {
	return slog.String("user_id", user.String())
}

func tutorAttribute(tutor domain.UserID) slog.Attr {
	return slog.String("tutor_id", tutor.String())
}

func channelAttribute(channel domain.ChannelID) slog.Attr {
	return slog.String("channel_id", channel.String())
}

func messageAttribute(message domain.MessageID) slog.Attr {
	return slog.String("message_id", message.String())
}
