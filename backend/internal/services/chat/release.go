package chat

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type Author struct {
	UserID    domain.UserID
	SessionID domain.SessionID
	Role      domain.Role
}

func (s *Service) lockAuthor(ctx context.Context, repos repositories.Repositories, author Author, now time.Time) error {
	if author.Role == domain.RoleStudent {
		return lockStudent(ctx, repos, author, now)
	}
	err := repos.TutorProfiles.Lock(ctx, author.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrUnauthenticated
	}
	if err != nil {
		return err
	}

	checkedAt, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return err
	}
	session, err := repos.Sessions.ByID(ctx, author.SessionID)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	if session.UserID != author.UserID || session.RevokedAt != nil || !checkedAt.Before(session.ExpiresAt) {
		return ErrUnauthenticated
	}
	account, err := repos.Users.ByID(ctx, author.UserID)
	if err != nil {
		return err
	}
	if account.State != domain.AccountActive {
		return ErrUnauthenticated
	}
	return nil
}

func lockStudent(ctx context.Context, repos repositories.Repositories, author Author, now time.Time) error {
	locked, err := repos.Users.LockCredentials(ctx, author.UserID)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	session, err := repos.Sessions.ByID(ctx, author.SessionID)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	usable := locked.State == domain.AccountActive || locked.State == domain.AccountUnverified
	if session.UserID != author.UserID || session.RevokedAt != nil || !now.Before(session.ExpiresAt) || !usable {
		return ErrUnauthenticated
	}
	return nil
}

type Release struct {
	Random core.RandomSource
	Logger *slog.Logger
}

func (r Release) attachments(ctx context.Context, repos repositories.Repositories, attachments []domain.AttachmentID, now time.Time) error {
	if len(attachments) == 0 {
		return nil
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		for _, attachment := range attachments {
			r.Logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.removed", slog.String("attachment_id", attachment.String()),
				slog.String("ref_kind", string(domain.ReferenceMessage)))
		}
	})
	if _, err := repos.Messages.ReleaseCharges(ctx, attachments, now); err != nil {
		return err
	}
	deleted, err := repos.Messages.MarkUnreferencedDeleted(ctx, attachments, now)
	if err != nil {
		return err
	}
	return queueStorageDeletions(ctx, repos, r.Random, deleted, now)
}

func (r Release) messages(ctx context.Context, repos repositories.Repositories, messages []domain.MessageID, now time.Time) error {
	removed, err := repos.Messages.RemoveReferencesOfMessages(ctx, messages)
	if err != nil {
		return err
	}
	return r.attachments(ctx, repos, removed, now)
}

func sortedIDs[ID ~[16]byte](ids []ID) []ID {
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, func(a, b ID) int { return bytes.Compare(a[:], b[:]) })
	return slices.Compact(sorted)
}

func (r Release) PurgeSenderBatch(
	ctx context.Context, repos repositories.Repositories, sender domain.UserID, batch int32, now time.Time,
) (int64, error) {
	messages, err := repos.Messages.BatchOfSender(ctx, sender, batch)
	if err != nil || len(messages) == 0 {
		return 0, err
	}
	var tutors []domain.UserID
	channels := make([]domain.ChannelID, 0, len(messages))
	ids := make([]domain.MessageID, 0, len(messages))
	for _, message := range messages {
		if message.HasAttachments {
			tutors = append(tutors, message.TutorID)
		}
		channels = append(channels, message.ChannelID)
		ids = append(ids, message.ID)
	}
	if tutors = sortedIDs(tutors); len(tutors) > 0 {
		if err := repos.TutorProfiles.LockAll(ctx, tutors); err != nil {
			return 0, err
		}
	}
	if err := repos.Messages.LockChannels(ctx, sortedIDs(channels)); err != nil {
		return 0, err
	}
	locked, err := repos.Messages.LockOfSender(ctx, sender, ids)
	if err != nil {
		return 0, err
	}
	if len(locked) == 0 {
		return int64(len(messages)), nil
	}
	if err := r.messages(ctx, repos, locked, now); err != nil {
		return 0, err
	}
	if _, err := repos.Messages.EraseOfSender(ctx, sender, locked, now); err != nil {
		return 0, err
	}
	if err := repos.Messages.RemoveReactionsOf(ctx, locked); err != nil {
		return 0, err
	}
	if err := r.clearPinsOfErased(ctx, repos, locked, now); err != nil {
		return 0, err
	}
	return int64(len(messages)), nil
}

func (r Release) clearPinsOfErased(ctx context.Context, repos repositories.Repositories, messages []domain.MessageID, now time.Time) error {
	cleared, err := repos.Channels.ClearPinsOfMessages(ctx, messages, now)
	if err != nil || len(cleared) == 0 {
		return err
	}
	channels := make([]domain.ChannelID, 0, len(cleared))
	for _, pin := range cleared {
		channels = append(channels, pin.Channel)
	}
	if err := signalPinChanged(ctx, repos, channels); err != nil {
		return err
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		for _, pin := range cleared {
			r.Logger.LogAttrs(ctx, slog.LevelInfo, "chat.message.unpinned", channelAttribute(pin.Channel), messageAttribute(pin.Message),
				userAttribute(pin.Tutor), slog.String("cause", string(domain.UnpinPurged)))
		}
	})
	return nil
}

func (r Release) PurgeTutorReferencesBatch(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, now time.Time,
) (int64, error) {
	references, err := repos.Messages.ReferencesBatchOfTutor(ctx, tutor, batch)
	if err != nil || len(references) == 0 {
		return 0, err
	}
	if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
		return 0, err
	}
	if _, err := repos.Channels.LockOfTutor(ctx, tutor); err != nil {
		return 0, err
	}
	removed, err := repos.Messages.RemoveReferencesOfAttachments(ctx, references)
	if err != nil {
		return 0, err
	}
	if err := r.attachments(ctx, repos, removed, now); err != nil {
		return 0, err
	}
	return int64(len(removed)), nil
}

func PurgeTutorMessagesBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, batch int32, now time.Time) (int64, error) {
	if _, err := repos.Channels.LockOfTutor(ctx, tutor); err != nil {
		return 0, err
	}
	if _, err := repos.Channels.ClearPinsOfTutor(ctx, tutor, now); err != nil {
		return 0, err
	}
	return repos.Messages.DeleteBatchOfTutor(ctx, tutor, batch)
}
