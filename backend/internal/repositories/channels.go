package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	LockedChannel    = dbgen.LockChannelsOfAudiencesRow
	ActiveMember     = dbgen.ListActiveMembersOfChannelsRow
	ReadOnlyChange   = dbgen.SyncChannelReadOnlyRow
	ChannelRow       = dbgen.ListChannelsOfMemberRow
	MemberLabel      = dbgen.ListChannelMemberLabelsRow
	PendingErasure   = dbgen.ListChannelsPendingErasureRow
	DiscardedChannel = dbgen.DiscardChannelsOfDiscardedLinksRow
	DiscardedUpload  = dbgen.MarkPendingUploadsOfChannelsRow
	ChannelReference = dbgen.ListRelationshipChannelsOfRow
)

var errSuspendedWithoutKey = errors.New("a suspended membership was returned without its key")

type Membership struct {
	Channel domain.ChannelID
	User    domain.UserID
}

type ChannelCursor struct {
	ActivityAt time.Time
	ID         domain.ChannelID
}

type ChannelQuery struct {
	Now     time.Time
	Member  domain.UserID
	Channel *domain.ChannelID
	Search  *string
	After   *ChannelCursor
	Limit   int32
}

type Channels struct {
	queries *dbgen.Queries
}

func (r Channels) EnsureDirect(ctx context.Context, ids []domain.ChannelID, relationships []domain.RelationshipID, now time.Time) error {
	if len(relationships) == 0 {
		return nil
	}
	_, err := r.queries.EnsureDirectChannels(ctx, dbgen.EnsureDirectChannelsParams{Now: now, Ids: uuids(ids), RelationshipIds: uuids(relationships)})
	return done(err, "ensure direct channels")
}

func (r Channels) EnsureGroup(ctx context.Context, ids []domain.ChannelID, groups []domain.GroupID, now time.Time) error {
	if len(groups) == 0 {
		return nil
	}
	_, err := r.queries.EnsureGroupChannels(ctx, dbgen.EnsureGroupChannelsParams{Now: now, Ids: uuids(ids), GroupIds: uuids(groups)})
	return done(err, "ensure group channels")
}

func (r Channels) LockOfAudiences(ctx context.Context, relationships []domain.RelationshipID, groups []domain.GroupID) ([]LockedChannel, error) {
	locked, err := r.queries.LockChannelsOfAudiences(ctx, dbgen.LockChannelsOfAudiencesParams{
		RelationshipIds: uuids(relationships), GroupIds: uuids(groups),
	})
	return one(locked, err, "lock channels of audiences")
}

func (r Channels) LockOfTutor(ctx context.Context, tutor domain.UserID) ([]LockedChannel, error) {
	rows, err := r.queries.LockChannelsOfTutor(ctx, tutor)
	if err != nil {
		return nil, done(err, "lock channels of tutor")
	}
	locked := make([]LockedChannel, 0, len(rows))
	for _, row := range rows {
		locked = append(locked, LockedChannel(row))
	}
	return locked, nil
}

func (r Channels) LockOfTutorForShare(ctx context.Context, tutor domain.UserID) ([]domain.ChannelID, error) {
	locked, err := r.queries.LockChannelsOfTutorForShare(ctx, tutor)
	return one(locked, err, "share-lock channels of tutor")
}

func (r Channels) LockOfRelationshipForShare(ctx context.Context, relationship domain.RelationshipID) (domain.ChannelID, error) {
	locked, err := r.queries.LockChannelOfRelationshipForShare(ctx, &relationship)
	return one(locked, err, "share-lock channel of relationship")
}

func (r Channels) LockOfMember(ctx context.Context, user domain.UserID) ([]domain.ChannelID, error) {
	locked, err := r.queries.LockChannelsOfMember(ctx, user)
	return one(locked, err, "lock channels of member")
}

func (r Channels) ActiveMembers(ctx context.Context, channels []domain.ChannelID) ([]ActiveMember, error) {
	if len(channels) == 0 {
		return nil, nil
	}
	members, err := r.queries.ListActiveMembersOfChannels(ctx, uuids(channels))
	return one(members, err, "active members of channels")
}

func (r Channels) SyncReadOnly(ctx context.Context, channels []domain.ChannelID, now time.Time) ([]ReadOnlyChange, error) {
	changed, err := r.queries.SyncChannelReadOnly(ctx, dbgen.SyncChannelReadOnlyParams{Now: now, ChannelIds: uuids(channels)})
	return one(changed, err, "sync channel writability")
}

func (r Channels) DiscardOfDiscardedLinks(ctx context.Context, channels []domain.ChannelID, now time.Time) ([]DiscardedChannel, error) {
	discarded, err := r.queries.DiscardChannelsOfDiscardedLinks(ctx, dbgen.DiscardChannelsOfDiscardedLinksParams{Now: now, ChannelIds: uuids(channels)})
	return one(discarded, err, "discard channels of discarded links")
}

func (r Channels) DeleteMembers(ctx context.Context, channels []domain.ChannelID) (int64, error) {
	if len(channels) == 0 {
		return 0, nil
	}
	deleted, err := r.queries.DeleteMembersOfChannels(ctx, uuids(channels))
	return counted(deleted, err, "delete members of channels")
}

func (r Channels) AddDirectMembers(ctx context.Context, channels []domain.ChannelID, now time.Time) ([]Membership, error) {
	rows, err := r.queries.AddDirectChannelMembers(ctx, dbgen.AddDirectChannelMembersParams{Now: now, ChannelIds: uuids(channels)})
	if err != nil {
		return nil, done(err, "add direct channel members")
	}
	memberships := make([]Membership, 0, len(rows))
	for _, row := range rows {
		memberships = append(memberships, Membership{Channel: row.ChannelID, User: row.UserID})
	}
	return memberships, nil
}

func (r Channels) SuspendReclaimed(ctx context.Context, channels []domain.ChannelID, now time.Time) ([]Membership, error) {
	rows, err := r.queries.SuspendReclaimedMembers(ctx, dbgen.SuspendReclaimedMembersParams{Now: now, ChannelIds: uuids(channels)})
	if err != nil {
		return nil, done(err, "suspend reclaimed members")
	}
	memberships := make([]Membership, 0, len(rows))
	for _, row := range rows {
		if !row.ChannelID.Valid || !row.UserID.Valid {
			return nil, errSuspendedWithoutKey
		}
		memberships = append(memberships, Membership{Channel: row.ChannelID.Bytes, User: row.UserID.Bytes})
	}
	return memberships, nil
}

func (r Channels) RestoreKept(ctx context.Context, channels []domain.ChannelID, now time.Time) ([]Membership, error) {
	rows, err := r.queries.RestoreKeptMembers(ctx, dbgen.RestoreKeptMembersParams{Now: now, ChannelIds: uuids(channels)})
	if err != nil {
		return nil, done(err, "restore kept members")
	}
	memberships := make([]Membership, 0, len(rows))
	for _, row := range rows {
		memberships = append(memberships, Membership{Channel: row.ChannelID, User: row.UserID})
	}
	return memberships, nil
}

func (r Channels) RemoveFormerGroupMembers(ctx context.Context, channels []domain.ChannelID) ([]Membership, error) {
	rows, err := r.queries.RemoveFormerGroupMembers(ctx, uuids(channels))
	if err != nil {
		return nil, done(err, "remove former group members")
	}
	memberships := make([]Membership, 0, len(rows))
	for _, row := range rows {
		memberships = append(memberships, Membership{Channel: row.ChannelID, User: row.UserID})
	}
	return memberships, nil
}

func (r Channels) AddGroupMembers(ctx context.Context, channels []domain.ChannelID, now time.Time) ([]Membership, error) {
	rows, err := r.queries.AddGroupChannelMembers(ctx, dbgen.AddGroupChannelMembersParams{Now: now, ChannelIds: uuids(channels)})
	if err != nil {
		return nil, done(err, "add group channel members")
	}
	memberships := make([]Membership, 0, len(rows))
	for _, row := range rows {
		memberships = append(memberships, Membership{Channel: row.ChannelID, User: row.UserID})
	}
	return memberships, nil
}

func (r Channels) DiscardPendingUploadsOf(ctx context.Context, channels []domain.ChannelID, now time.Time) ([]DiscardedUpload, error) {
	if len(channels) == 0 {
		return nil, nil
	}
	discarded, err := r.queries.MarkPendingUploadsOfChannels(ctx, dbgen.MarkPendingUploadsOfChannelsParams{Now: now, ChannelIds: uuids(channels)})
	return one(discarded, err, "discard pending uploads of channels")
}

func (r Channels) DiscardPendingUploadsOfMembers(ctx context.Context, memberships []Membership, now time.Time) ([]DiscardedUpload, error) {
	if len(memberships) == 0 {
		return nil, nil
	}
	channels := make([]domain.ChannelID, 0, len(memberships))
	users := make([]domain.UserID, 0, len(memberships))
	for _, membership := range memberships {
		channels = append(channels, membership.Channel)
		users = append(users, membership.User)
	}
	rows, err := r.queries.MarkPendingUploadsOfMembers(ctx, dbgen.MarkPendingUploadsOfMembersParams{
		Now: now, ChannelIds: uuids(channels), UserIds: uuids(users),
	})
	if err != nil {
		return nil, done(err, "discard pending uploads of former members")
	}
	discarded := make([]DiscardedUpload, 0, len(rows))
	for _, row := range rows {
		discarded = append(discarded, DiscardedUpload(row))
	}
	return discarded, nil
}

func (r Channels) DeleteMembershipsOf(ctx context.Context, user domain.UserID) (int64, error) {
	deleted, err := r.queries.DeleteMembershipsOfUser(ctx, user)
	return counted(deleted, err, "delete channel memberships of user")
}

func (r Channels) List(ctx context.Context, query ChannelQuery) ([]ChannelRow, error) {
	params := dbgen.ListChannelsOfMemberParams{Now: query.Now, UserID: query.Member, RowLimit: query.Limit}
	if query.Search != nil {
		pattern := "%" + escapeLikePattern(*query.Search) + "%"
		params.Pattern = &pattern
	}
	if query.Channel != nil {
		params.ChannelID = pgtype.UUID{Bytes: *query.Channel, Valid: true}
	}
	if query.After != nil {
		activity := query.After.ActivityAt
		params.AfterActivityAt = &activity
		params.AfterID = pgtype.UUID{Bytes: query.After.ID, Valid: true}
	}
	rows, err := r.queries.ListChannelsOfMember(ctx, params)
	return one(rows, err, "channels of member")
}

func (r Channels) CountUnread(ctx context.Context, user domain.UserID, limit int32) (int64, error) {
	count, err := r.queries.CountUnreadChannels(ctx, dbgen.CountUnreadChannelsParams{UserID: user, RowCap: limit})
	return one(count, err, "count unread channels")
}

func (r Channels) MemberLabels(ctx context.Context, channel domain.ChannelID, now time.Time) ([]MemberLabel, error) {
	labels, err := r.queries.ListChannelMemberLabels(ctx, dbgen.ListChannelMemberLabelsParams{Now: now, ChannelID: channel})
	return one(labels, err, "channel member labels")
}

func (r Channels) OfRelationshipsFor(ctx context.Context, user domain.UserID, relationships []domain.RelationshipID) (map[domain.RelationshipID]domain.ChannelID, error) {
	found := make(map[domain.RelationshipID]domain.ChannelID, len(relationships))
	if len(relationships) == 0 {
		return found, nil
	}
	rows, err := r.queries.ListRelationshipChannelsOf(ctx, dbgen.ListRelationshipChannelsOfParams{UserID: user, RelationshipIds: uuids(relationships)})
	if err != nil {
		return nil, done(err, "relationship channels of user")
	}
	for _, row := range rows {
		if row.RelationshipID != nil {
			found[*row.RelationshipID] = row.ChannelID
		}
	}
	return found, nil
}

func (r Channels) OfGroupFor(ctx context.Context, user domain.UserID, group domain.GroupID) (*domain.ChannelID, error) {
	channel, err := r.queries.GetGroupChannelOf(ctx, dbgen.GetGroupChannelOfParams{UserID: user, GroupID: &group})
	channel, err = one(channel, err, "group channel of user")
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

func (r Channels) OfLessonsFor(ctx context.Context, user domain.UserID, lessons []domain.LessonID) (map[domain.LessonID]domain.ChannelID, error) {
	found := make(map[domain.LessonID]domain.ChannelID, len(lessons))
	if len(lessons) == 0 {
		return found, nil
	}
	rows, err := r.queries.ListLessonChannelsOf(ctx, dbgen.ListLessonChannelsOfParams{UserID: user, LessonIds: uuids(lessons)})
	if err != nil {
		return nil, done(err, "lesson channels of user")
	}
	for _, row := range rows {
		found[row.LessonID] = row.ChannelID
	}
	return found, nil
}

func (r Channels) PendingErasure(ctx context.Context, batch int32) ([]PendingErasure, error) {
	pending, err := r.queries.ListChannelsPendingErasure(ctx, batch)
	return one(pending, err, "channels pending erasure")
}

func (r Channels) LockPendingErasure(ctx context.Context, id domain.ChannelID) error {
	locked, err := r.queries.LockChannelPendingErasure(ctx, id)
	_, err = one(locked, err, "lock channel pending erasure")
	return err
}

func (r Channels) ClearErasure(ctx context.Context, id domain.ChannelID, now time.Time) (bool, error) {
	rows, err := r.queries.ClearChannelErasure(ctx, dbgen.ClearChannelErasureParams{Now: now, ID: id})
	return applied(rows, err, "clear channel erasure")
}

func (r Channels) DeleteMembersBatchOfTutor(ctx context.Context, tutor domain.UserID, batchSize int32) (int64, error) {
	deleted, err := r.queries.DeleteTutorChannelMembersBatch(ctx, dbgen.DeleteTutorChannelMembersBatchParams{TutorID: tutor, BatchSize: batchSize})
	return counted(deleted, err, "delete channel members batch of tutor")
}

func (r Channels) DeleteBatchOfTutor(ctx context.Context, tutor domain.UserID, batchSize int32) (int64, error) {
	deleted, err := r.queries.DeleteTutorChannelsBatch(ctx, dbgen.DeleteTutorChannelsBatchParams{TutorID: tutor, BatchSize: batchSize})
	return counted(deleted, err, "delete channels batch of tutor")
}

func (r Channels) StudentDeletionTutors(ctx context.Context, student domain.UserID) ([]domain.UserID, error) {
	tutors, err := r.queries.ListStudentDeletionTutors(ctx, &student)
	return one(tutors, err, "tutors for student deletion")
}

func (r Channels) LockForStudentDeletion(ctx context.Context, student domain.UserID, relationships []domain.RelationshipID, groups []domain.GroupID) error {
	_, err := r.queries.LockStudentDeletionChannels(ctx, dbgen.LockStudentDeletionChannelsParams{
		UserID: student, RelationshipIds: uuids(relationships), GroupIds: uuids(groups),
	})
	return done(err, "lock all channels for student deletion")
}

func (r Channels) Pin(ctx context.Context, channel domain.ChannelID) (*domain.MessageID, error) {
	pinned, err := r.queries.GetChannelPin(ctx, channel)
	return one(pinned, err, "channel pin")
}

func (r Channels) SetPin(ctx context.Context, channel domain.ChannelID, message domain.MessageID, now time.Time) error {
	return done(r.queries.SetChannelPin(ctx, dbgen.SetChannelPinParams{MessageID: &message, Now: now, ID: channel}), "set the channel pin")
}

func (r Channels) ClearPin(ctx context.Context, channel domain.ChannelID, now time.Time) (bool, error) {
	cleared, err := r.queries.ClearChannelPin(ctx, dbgen.ClearChannelPinParams{Now: now, ID: channel})
	return cleared > 0, done(err, "clear the channel pin")
}

func (r Channels) ClearPinOfMessage(ctx context.Context, channel domain.ChannelID, message domain.MessageID, now time.Time) (bool, error) {
	cleared, err := r.queries.ClearChannelPinOfMessage(ctx, dbgen.ClearChannelPinOfMessageParams{Now: now, ID: channel, MessageID: &message})
	return cleared > 0, done(err, "clear the pin of a message")
}

type ClearedPin struct {
	Channel domain.ChannelID
	Message domain.MessageID
	Tutor   domain.UserID
}

func (r Channels) ClearPinsOfMessages(ctx context.Context, messages []domain.MessageID, now time.Time) ([]ClearedPin, error) {
	if len(messages) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ClearPinsOfMessages(ctx, dbgen.ClearPinsOfMessagesParams{Now: now, MessageIds: uuids(messages)})
	if err != nil {
		return nil, done(err, "clear the pins of messages")
	}
	cleared := make([]ClearedPin, 0, len(rows))
	for _, row := range rows {
		cleared = append(cleared, ClearedPin{Channel: row.ID, Message: domain.MessageID(row.MessageID.Bytes), Tutor: domain.UserID(row.TutorID.Bytes)})
	}
	return cleared, nil
}

func (r Channels) ClearPinsOfTutor(ctx context.Context, tutor domain.UserID, now time.Time) (int64, error) {
	cleared, err := r.queries.ClearPinsOfTutorChannels(ctx, dbgen.ClearPinsOfTutorChannelsParams{Now: now, TutorID: tutor})
	return cleared, done(err, "clear the pins of a tutor's channels")
}
