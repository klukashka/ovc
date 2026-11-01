package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type Audiences struct {
	Relationships []domain.RelationshipID
	Groups        []domain.GroupID
	Relabelled    []domain.GroupID
	Confirmed     []domain.RelationshipID
}

type Mirror struct {
	Random core.RandomSource
	Logger *slog.Logger
}

type memberChange struct {
	channel domain.ChannelID
	tutor   domain.UserID
	user    *domain.UserID
	change  domain.MembershipChange
}

type queuedErasure struct {
	channel  domain.ChannelID
	tutor    domain.UserID
	cause    domain.DiscardReason
	messages int64
}

type reconciled struct {
	path      domain.MembershipPath
	changes   []memberChange
	erasures  []queuedErasure
	suspended []repositories.Membership
}

func (m Mirror) Apply(ctx context.Context, repos repositories.Repositories, path domain.MembershipPath, audiences Audiences, now time.Time) error {
	if err := m.ensure(ctx, repos, audiences, now); err != nil {
		return err
	}
	locked, err := repos.Channels.LockOfAudiences(ctx, audiences.Relationships, append(slices.Clone(audiences.Groups), audiences.Relabelled...))
	if err != nil {
		return err
	}
	return m.reconcile(ctx, repos, path, locked, audiences, now)
}

func (m Mirror) ApplyTutorDeletion(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, now time.Time) error {
	locked, err := repos.Channels.LockOfTutor(ctx, tutor)
	if err != nil {
		return err
	}
	return m.reconcile(ctx, repos, domain.PathTutorDeletion, locked, Audiences{}, now)
}

func (m Mirror) ensure(ctx context.Context, repos repositories.Repositories, audiences Audiences, now time.Time) error {
	direct := make([]domain.ChannelID, 0, len(audiences.Relationships))
	for range audiences.Relationships {
		id, err := core.NewID[domain.ChannelID](m.Random)
		if err != nil {
			return err
		}
		direct = append(direct, id)
	}
	if err := repos.Channels.EnsureDirect(ctx, direct, audiences.Relationships, now); err != nil {
		return err
	}
	group := make([]domain.ChannelID, 0, len(audiences.Groups))
	for range audiences.Groups {
		id, err := core.NewID[domain.ChannelID](m.Random)
		if err != nil {
			return err
		}
		group = append(group, id)
	}
	return repos.Channels.EnsureGroup(ctx, group, audiences.Groups, now)
}

func (m Mirror) reconcile(
	ctx context.Context, repos repositories.Repositories, path domain.MembershipPath, locked []repositories.LockedChannel,
	audiences Audiences, now time.Time,
) error {
	if len(locked) == 0 {
		return nil
	}
	ids := make([]domain.ChannelID, 0, len(locked))
	for _, channel := range locked {
		ids = append(ids, channel.ID)
	}
	before, err := activeMembers(ctx, repos, ids)
	if err != nil {
		return err
	}
	outcome := reconciled{path: path}
	readOnly, err := repos.Channels.SyncReadOnly(ctx, ids, now)
	if err != nil {
		return err
	}
	discarded, err := m.discard(ctx, repos, locked, ids, now, &outcome)
	if err != nil {
		return err
	}
	if err := m.syncMembers(ctx, repos, ids, now, &outcome); err != nil {
		return err
	}
	after, err := activeMembers(ctx, repos, ids)
	if err != nil {
		return err
	}
	changedWritability := map[domain.ChannelID]*time.Time{}
	for _, change := range readOnly {
		changedWritability[change.ID] = change.ReadOnlySince
	}
	signals := make([]domain.EntitySignal, 0, len(locked))
	for _, channel := range locked {
		recipients, changes := channelEffects(channel, before[channel.ID], after[channel.ID], discarded[channel.ID], changedWritability, audiences)
		recipients = slices.DeleteFunc(recipients, func(user domain.UserID) bool {
			deletedTutor := path == domain.PathTutorDeletion && user == channel.TutorID
			return deletedTutor || slices.Contains(outcome.suspended, repositories.Membership{Channel: channel.ID, User: user})
		})
		outcome.changes = append(outcome.changes, changes...)
		if len(recipients) > 0 {
			signals = append(signals, domain.EntitySignal{Users: recipients, Entity: domain.EntityChannel, ID: domain.EntityID(channel.ID)})
		}
	}
	if err := repos.Signals.Entities(ctx, signals); err != nil {
		return err
	}
	repos.AfterCommit(ctx, func(ctx context.Context) { m.log(ctx, outcome) })
	return nil
}

func (m Mirror) discard(
	ctx context.Context, repos repositories.Repositories, locked []repositories.LockedChannel, ids []domain.ChannelID, now time.Time,
	outcome *reconciled,
) (map[domain.ChannelID]bool, error) {
	rows, err := repos.Channels.DiscardOfDiscardedLinks(ctx, ids, now)
	if err != nil || len(rows) == 0 {
		return map[domain.ChannelID]bool{}, err
	}
	discarded := make(map[domain.ChannelID]bool, len(rows))
	channels := make([]domain.ChannelID, 0, len(rows))
	for _, row := range rows {
		discarded[row.ID] = true
		channels = append(channels, row.ID)
		cause := domain.DiscardRemoved
		if row.DiscardReason != nil {
			cause = *row.DiscardReason
		}
		for _, channel := range locked {
			if channel.ID == row.ID {
				outcome.erasures = append(outcome.erasures, queuedErasure{channel: row.ID, tutor: channel.TutorID, cause: cause, messages: channel.LastSeq})
			}
		}
	}
	if _, err := repos.Channels.DeleteMembers(ctx, channels); err != nil {
		return nil, err
	}
	uploads, err := repos.Channels.DiscardPendingUploadsOf(ctx, channels, now)
	if err != nil {
		return nil, err
	}
	return discarded, queueStorageDeletions(ctx, repos, m.Random, uploads, now)
}

func (m Mirror) syncMembers(ctx context.Context, repos repositories.Repositories, ids []domain.ChannelID, now time.Time, outcome *reconciled) error {
	if _, err := repos.Channels.AddDirectMembers(ctx, ids, now); err != nil {
		return err
	}
	suspended, err := repos.Channels.SuspendReclaimed(ctx, ids, now)
	if err != nil {
		return err
	}
	outcome.suspended = suspended
	if _, err := repos.Channels.RestoreKept(ctx, ids, now); err != nil {
		return err
	}
	removed, err := repos.Channels.RemoveFormerGroupMembers(ctx, ids)
	if err != nil {
		return err
	}
	uploads, err := repos.Channels.DiscardPendingUploadsOfMembers(ctx, removed, now)
	if err != nil {
		return err
	}
	if err := queueStorageDeletions(ctx, repos, m.Random, uploads, now); err != nil {
		return err
	}
	_, err = repos.Channels.AddGroupMembers(ctx, ids, now)
	return err
}

func activeMembers(ctx context.Context, repos repositories.Repositories, ids []domain.ChannelID) (map[domain.ChannelID][]domain.UserID, error) {
	rows, err := repos.Channels.ActiveMembers(ctx, ids)
	if err != nil {
		return nil, err
	}
	members := make(map[domain.ChannelID][]domain.UserID, len(ids))
	for _, row := range rows {
		members[row.ChannelID] = append(members[row.ChannelID], row.UserID)
	}
	return members, nil
}

func channelEffects(
	channel repositories.LockedChannel, before []domain.UserID, after []domain.UserID, discarded bool,
	writability map[domain.ChannelID]*time.Time, audiences Audiences,
) ([]domain.UserID, []memberChange) {
	var changes []memberChange
	if discarded {
		changes = append(changes, memberChange{channel: channel.ID, tutor: channel.TutorID, change: domain.MembershipDiscarded})
		return sortedUsers(before), changes
	}
	recipients := map[domain.UserID]bool{}
	for _, user := range after {
		if !slices.Contains(before, user) {
			joined := user
			recipients[user] = true
			changes = append(changes, memberChange{channel: channel.ID, tutor: channel.TutorID, user: &joined, change: domain.MembershipJoined})
		}
	}
	for _, user := range before {
		if !slices.Contains(after, user) {
			left := user
			recipients[user] = true
			changes = append(changes, memberChange{channel: channel.ID, tutor: channel.TutorID, user: &left, change: domain.MembershipLeft})
		}
	}
	since, changed := writability[channel.ID]
	confirmed := channel.RelationshipID != nil && slices.Contains(audiences.Confirmed, *channel.RelationshipID)
	if (len(recipients) > 0 || changed || confirmed) && slices.Contains(after, channel.TutorID) {
		recipients[channel.TutorID] = true
	}
	relabel := channel.GroupID != nil && slices.Contains(audiences.Relabelled, *channel.GroupID)
	if changed || relabel {
		for _, user := range after {
			recipients[user] = true
		}
	}
	if changed {
		change := domain.MembershipWritable
		if since != nil {
			change = domain.MembershipReadOnly
		}
		changes = append(changes, memberChange{channel: channel.ID, tutor: channel.TutorID, change: change})
	}
	users := make([]domain.UserID, 0, len(recipients))
	for user := range recipients {
		users = append(users, user)
	}
	return sortedUsers(users), changes
}

func sortedUsers(users []domain.UserID) []domain.UserID {
	sorted := slices.Clone(users)
	slices.SortFunc(sorted, func(a, b domain.UserID) int { return bytes.Compare(a[:], b[:]) })
	return sorted
}

func queueStorageDeletions(
	ctx context.Context, repos repositories.Repositories, random core.RandomSource, uploads []repositories.DiscardedUpload, now time.Time,
) error {
	if len(uploads) == 0 {
		return nil
	}
	rows := make([]repositories.QueuedOutboxRow, 0, len(uploads))
	for _, upload := range uploads {
		payload, err := json.Marshal(domain.AttachmentPayload{Attachment: upload.ID.String(), Key: upload.StorageKey})
		if err != nil {
			return fmt.Errorf("encode the storage deletion payload: %w", err)
		}
		id, err := core.NewID[domain.OutboxID](random)
		if err != nil {
			return err
		}
		rows = append(rows, repositories.QueuedOutboxRow{
			ID: id, Payload: payload, IdempotencyKey: string(domain.KindStorageDelete) + ":" + upload.ID.String(),
		})
	}
	_, err := repos.Outbox.InsertAll(ctx, repositories.NewOutboxRows{
		Lane: domain.LaneStorageDelete, Priority: domain.PriorityNormal, Kind: domain.KindStorageDelete, NextAttemptAt: now, CreatedAt: now,
		Rows: rows,
	})
	return err
}

func (m Mirror) log(ctx context.Context, outcome reconciled) {
	for _, change := range outcome.changes {
		attributes := []slog.Attr{
			slog.String("channel_id", change.channel.String()), slog.String("tutor_id", change.tutor.String()),
			slog.String("change", string(change.change)), slog.String("path", string(outcome.path)),
		}
		if change.user != nil {
			attributes = append(attributes, slog.String("user_id", change.user.String()))
		}
		m.Logger.LogAttrs(ctx, slog.LevelDebug, "channel.membership.changed", attributes...)
	}
	for _, erasure := range outcome.erasures {
		m.Logger.LogAttrs(ctx, slog.LevelInfo, "channel.erase_queued", slog.String("channel_id", erasure.channel.String()),
			slog.String("tutor_id", erasure.tutor.String()), slog.String("cause", string(erasure.cause)), slog.Int64("messages", erasure.messages))
	}
}

func (m Mirror) LockForStudentDeletion(ctx context.Context, repos repositories.Repositories, student domain.UserID, audiences Audiences, now time.Time) error {
	if err := m.ensure(ctx, repos, audiences, now); err != nil {
		return err
	}
	return repos.Channels.LockForStudentDeletion(ctx, student, audiences.Relationships, audiences.Groups)
}
