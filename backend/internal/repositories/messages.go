package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

var ErrDuplicateClientKey = errors.New("the sender already used this idempotency key in the channel")

const (
	uniqueViolation       = "23505"
	messageClientKeyIndex = "messages_client_key_idx"
)

type (
	ActiveMembership   = dbgen.GetActiveMembershipRow
	MessageRow         = dbgen.GetMessageViewRow
	MessageAttachment  = dbgen.ListMessageAttachmentsRow
	MessageScope       = dbgen.GetMessageScopeRow
	LockedMessage      = dbgen.LockMessageRow
	BindableUpload     = dbgen.LockBindableUploadsRow
	SenderMessage      = dbgen.ListSenderMessagesBatchRow
	ReleasedCharge     = dbgen.ReleaseUnreferencedChargesRow
	ReplayedMessage    = dbgen.FindMessageByClientKeyRow
	UnreferencedUpload = DiscardedUpload
)

type NewMessage struct {
	ID        domain.MessageID
	Channel   domain.ChannelID
	Seq       int64
	Sender    domain.UserID
	ClientKey *string
	Body      string
	ReplyTo   *domain.MessageID
	Now       time.Time
}

type HistoryQuery struct {
	Channel  domain.ChannelID
	Before   *int64
	After    *int64
	RowLimit int32
}

type Messages struct {
	queries *dbgen.Queries
}

func (r Messages) ChannelTutor(ctx context.Context, channel domain.ChannelID) (domain.UserID, error) {
	tutor, err := r.queries.GetChannelTutor(ctx, channel)
	return one(tutor, err, "channel tutor")
}

func (r Messages) LockChannel(ctx context.Context, channel domain.ChannelID) error {
	locked, err := r.queries.LockChannelForUpdate(ctx, channel)
	_, err = one(locked, err, "lock channel")
	return err
}

func (r Messages) ShareChannel(ctx context.Context, channel domain.ChannelID) error {
	locked, err := r.queries.LockChannelForShare(ctx, channel)
	_, err = one(locked, err, "share-lock channel")
	return err
}

func (r Messages) LockChannels(ctx context.Context, channels []domain.ChannelID) error {
	if len(channels) == 0 {
		return nil
	}
	_, err := r.queries.LockChannelsForUpdate(ctx, uuids(channels))
	return done(err, "lock channels")
}

func (r Messages) Membership(ctx context.Context, channel domain.ChannelID, user domain.UserID) (ActiveMembership, error) {
	membership, err := r.queries.GetActiveMembership(ctx, dbgen.GetActiveMembershipParams{ChannelID: channel, UserID: user})
	return one(membership, err, "active membership")
}

func (r Messages) ByClientKey(ctx context.Context, sender domain.UserID, channel domain.ChannelID, key string) (ReplayedMessage, error) {
	found, err := r.queries.FindMessageByClientKey(ctx, dbgen.FindMessageByClientKeyParams{SenderID: &sender, ChannelID: channel, ClientKey: key})
	return one(found, err, "message by client key")
}

func (r Messages) AdvanceSeq(ctx context.Context, channel domain.ChannelID, now time.Time) (int64, error) {
	seq, err := r.queries.AdvanceChannelSeq(ctx, dbgen.AdvanceChannelSeqParams{Now: now, ID: channel})
	return one(seq, err, "advance channel sequence")
}

func (r Messages) Insert(ctx context.Context, message NewMessage) error {
	sender := message.Sender
	err := r.queries.InsertMessage(ctx, dbgen.InsertMessageParams{
		ID: message.ID, ChannelID: message.Channel, Seq: message.Seq, SenderID: &sender, ClientKey: message.ClientKey, Body: message.Body, Now: message.Now,
		ReplyToMessageID: message.ReplyTo,
	})
	var violation *pgconn.PgError
	if errors.As(err, &violation) && violation.Code == uniqueViolation && violation.ConstraintName == messageClientKeyIndex {
		return fmt.Errorf("insert message: %w", ErrDuplicateClientKey)
	}
	return done(err, "insert message")
}

func (r Messages) MoveSenderMarker(ctx context.Context, channel domain.ChannelID, user domain.UserID, seq int64, now time.Time) error {
	_, err := r.queries.MoveSenderReadMarker(ctx, dbgen.MoveSenderReadMarkerParams{
		Seq: seq, Now: now, ChannelID: pgtype.UUID{Bytes: channel, Valid: true}, UserID: pgtype.UUID{Bytes: user, Valid: true},
	})
	return done(err, "move the sender's read marker")
}

func (r Messages) FlagDigest(ctx context.Context, channel domain.ChannelID, sender domain.UserID, now time.Time) (int64, error) {
	flagged, err := r.queries.FlagDigestPending(ctx, dbgen.FlagDigestPendingParams{
		Now: now, ChannelID: channel, SenderID: pgtype.UUID{Bytes: sender, Valid: true},
	})
	return counted(flagged, err, "flag digest pending")
}

func (r Messages) LockBindable(
	ctx context.Context, uploader domain.UserID, channel domain.ChannelID, ids []domain.AttachmentID, now time.Time,
) ([]BindableUpload, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.LockBindableUploads(ctx, dbgen.LockBindableUploadsParams{
		Ids: uuids(ids), UploaderID: &uploader, ChannelID: pgtype.UUID{Bytes: channel, Valid: true}, Now: now,
	})
	return one(rows, err, "lock bindable uploads")
}

func (r Messages) Bind(ctx context.Context, message domain.MessageID, ids []domain.AttachmentID, now time.Time) error {
	bound, err := r.queries.BindUploads(ctx, dbgen.BindUploadsParams{Now: now, Ids: uuids(ids)})
	if err != nil {
		return done(err, "bind uploads")
	}
	if bound != int64(len(ids)) {
		return fmt.Errorf("bind uploads: %d of %d were still pending", bound, len(ids))
	}
	_, err = r.queries.InsertMessageReferences(ctx, dbgen.InsertMessageReferencesParams{
		MessageID: pgtype.UUID{Bytes: message, Valid: true}, Now: now, AttachmentIds: uuids(ids),
	})
	return done(err, "insert message references")
}

func (r Messages) Charge(ctx context.Context, tutor domain.UserID, ids []domain.AttachmentID, now time.Time) (bool, error) {
	_, err := r.queries.ChargeAttachments(ctx, dbgen.ChargeAttachmentsParams{
		Now: now, TutorID: pgtype.UUID{Bytes: tutor, Valid: true}, AttachmentIds: uuids(ids),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("charge attachments: %w", err)
	}
	return true, nil
}

func (r Messages) View(ctx context.Context, id domain.MessageID) (MessageRow, error) {
	row, err := r.queries.GetMessageView(ctx, id)
	return one(row, err, "message view")
}

func (r Messages) Views(ctx context.Context, ids []domain.MessageID) ([]MessageRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListMessageViews(ctx, uuids(ids))
	if err != nil {
		return nil, done(err, "message views")
	}
	converted := make([]MessageRow, 0, len(rows))
	for _, row := range rows {
		converted = append(converted, MessageRow(row))
	}
	return converted, nil
}

func (r Messages) LiveOfChannel(ctx context.Context, channel domain.ChannelID, id domain.MessageID) (domain.MessageID, error) {
	found, err := r.queries.ResolveLiveMessageOfChannel(ctx, dbgen.ResolveLiveMessageOfChannelParams{ID: id, ChannelID: channel})
	return one(found, err, "live message of a channel")
}

func (r Messages) History(ctx context.Context, query HistoryQuery) ([]MessageRow, error) {
	if query.After != nil {
		rows, err := r.queries.ListMessagesAfter(ctx, dbgen.ListMessagesAfterParams{ChannelID: query.Channel, AfterSeq: *query.After, RowLimit: query.RowLimit})
		if err != nil {
			return nil, done(err, "messages after a sequence")
		}
		converted := make([]MessageRow, 0, len(rows))
		for _, row := range rows {
			converted = append(converted, MessageRow(row))
		}
		return converted, nil
	}
	rows, err := r.queries.ListMessagesBefore(ctx, dbgen.ListMessagesBeforeParams{ChannelID: query.Channel, BeforeSeq: query.Before, RowLimit: query.RowLimit})
	if err != nil {
		return nil, done(err, "messages before a sequence")
	}
	converted := make([]MessageRow, 0, len(rows))
	for _, row := range rows {
		converted = append(converted, MessageRow(row))
	}
	return converted, nil
}

func (r Messages) LastOf(ctx context.Context, channels []domain.ChannelID) ([]MessageRow, error) {
	if len(channels) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListLastMessages(ctx, uuids(channels))
	if err != nil {
		return nil, done(err, "last messages of channels")
	}
	converted := make([]MessageRow, 0, len(rows))
	for _, row := range rows {
		converted = append(converted, MessageRow(row))
	}
	return converted, nil
}

func (r Messages) AttachmentsOf(ctx context.Context, messages []domain.MessageID) ([]MessageAttachment, error) {
	if len(messages) == 0 {
		return nil, nil
	}
	rows, err := r.queries.ListMessageAttachments(ctx, uuids(messages))
	return one(rows, err, "attachments of messages")
}

func (r Messages) Scope(ctx context.Context, id domain.MessageID) (MessageScope, error) {
	scope, err := r.queries.GetMessageScope(ctx, id)
	return one(scope, err, "message scope")
}

func (r Messages) Lock(ctx context.Context, id domain.MessageID) (LockedMessage, error) {
	locked, err := r.queries.LockMessage(ctx, id)
	return one(locked, err, "lock message")
}

func (r Messages) Edit(ctx context.Context, id domain.MessageID, body string, now time.Time) (bool, error) {
	rows, err := r.queries.EditMessage(ctx, dbgen.EditMessageParams{Body: body, Now: now, ID: id})
	return applied(rows, err, "edit message")
}

func (r Messages) MarkDeleted(ctx context.Context, id domain.MessageID, now time.Time) (bool, error) {
	rows, err := r.queries.MarkMessageDeleted(ctx, dbgen.MarkMessageDeletedParams{Now: now, ID: id})
	return applied(rows, err, "mark message deleted")
}

func (r Messages) RemoveReferencesOfMessages(ctx context.Context, messages []domain.MessageID) ([]domain.AttachmentID, error) {
	if len(messages) == 0 {
		return nil, nil
	}
	removed, err := r.queries.RemoveMessageReferences(ctx, uuids(messages))
	return one(removed, err, "remove message references")
}

func (r Messages) RemoveReferencesOfAttachments(ctx context.Context, attachments []domain.AttachmentID) ([]domain.AttachmentID, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	removed, err := r.queries.RemoveReferencesOfAttachments(ctx, uuids(attachments))
	return one(removed, err, "remove references of attachments")
}

func (r Messages) ReleaseCharges(ctx context.Context, attachments []domain.AttachmentID, now time.Time) ([]ReleasedCharge, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	released, err := r.queries.ReleaseUnreferencedCharges(ctx, dbgen.ReleaseUnreferencedChargesParams{Now: now, AttachmentIds: uuids(attachments)})
	return one(released, err, "release unreferenced charges")
}

func (r Messages) MarkUnreferencedDeleted(ctx context.Context, attachments []domain.AttachmentID, now time.Time) ([]UnreferencedUpload, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	rows, err := r.queries.MarkUnreferencedAttachmentsDeleted(ctx, dbgen.MarkUnreferencedAttachmentsDeletedParams{Now: now, AttachmentIds: uuids(attachments)})
	if err != nil {
		return nil, done(err, "mark unreferenced attachments deleted")
	}
	deleted := make([]UnreferencedUpload, 0, len(rows))
	for _, row := range rows {
		deleted = append(deleted, UnreferencedUpload(row))
	}
	return deleted, nil
}

func (r Messages) MoveReadMarker(ctx context.Context, channel domain.ChannelID, user domain.UserID, seq int64, now time.Time) (bool, error) {
	rows, err := r.queries.MoveReadMarker(ctx, dbgen.MoveReadMarkerParams{
		Seq: seq, Now: now, ChannelID: pgtype.UUID{Bytes: channel, Valid: true}, UserID: pgtype.UUID{Bytes: user, Valid: true},
	})
	return applied(rows, err, "move read marker")
}

func (r Messages) LockBatchOfChannel(ctx context.Context, channel domain.ChannelID, batch int32) ([]domain.MessageID, error) {
	ids, err := r.queries.LockChannelMessagesBatch(ctx, dbgen.LockChannelMessagesBatchParams{ChannelID: channel, BatchSize: batch})
	return one(ids, err, "lock a batch of channel messages")
}

func (r Messages) Delete(ctx context.Context, ids []domain.MessageID) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	deleted, err := r.queries.DeleteMessagesByID(ctx, uuids(ids))
	return counted(deleted, err, "delete messages")
}

func (r Messages) BatchOfSender(ctx context.Context, sender domain.UserID, batch int32) ([]SenderMessage, error) {
	rows, err := r.queries.ListSenderMessagesBatch(ctx, dbgen.ListSenderMessagesBatchParams{SenderID: pgtype.UUID{Bytes: sender, Valid: true}, BatchSize: batch})
	return one(rows, err, "a batch of the sender's messages")
}

func (r Messages) LockOfSender(ctx context.Context, sender domain.UserID, ids []domain.MessageID) ([]domain.MessageID, error) {
	locked, err := r.queries.LockMessagesOfSender(ctx, dbgen.LockMessagesOfSenderParams{Ids: uuids(ids), SenderID: pgtype.UUID{Bytes: sender, Valid: true}})
	return one(locked, err, "lock messages of sender")
}

func (r Messages) EraseOfSender(ctx context.Context, sender domain.UserID, ids []domain.MessageID, now time.Time) (int64, error) {
	erased, err := r.queries.EraseMessagesOfSender(ctx, dbgen.EraseMessagesOfSenderParams{
		Now: now, Ids: uuids(ids), SenderID: pgtype.UUID{Bytes: sender, Valid: true},
	})
	return counted(erased, err, "erase messages of sender")
}

func (r Messages) ReferencesBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) ([]domain.AttachmentID, error) {
	ids, err := r.queries.ListTutorMessageReferencesBatch(ctx, dbgen.ListTutorMessageReferencesBatchParams{TutorID: tutor, BatchSize: batch})
	return one(ids, err, "a batch of the tutor's message references")
}

func (r Messages) DeleteBatchOfTutor(ctx context.Context, tutor domain.UserID, batch int32) (int64, error) {
	deleted, err := r.queries.DeleteTutorMessagesBatch(ctx, dbgen.DeleteTutorMessagesBatchParams{TutorID: tutor, BatchSize: batch})
	return counted(deleted, err, "delete a batch of the tutor's messages")
}

func (r Messages) ForgetDigestOf(ctx context.Context, user domain.UserID) error {
	_, err := r.queries.DeleteDigestPendingOf(ctx, user)
	if err != nil {
		return done(err, "forget the digest flag")
	}
	return done(r.queries.DeleteDigestStateOf(ctx, user), "forget digest state")
}
