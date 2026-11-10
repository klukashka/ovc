package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

var (
	ErrUndescribedAttachment = errors.New("a stored attachment has no recorded media type or size")
	ErrNoCountedContexts     = errors.New("an open upload count names no context")
)

type ServableAvatar struct {
	StorageKey string
	MediaType  string
	SizeBytes  int64
}

type (
	ProfileUpdate      = dbgen.UpdateUserProfileParams
	TutorText          = dbgen.UpdateTutorProfileTextParams
	NewUploading       = dbgen.CreateUploadingAttachmentParams
	CompletedUpload    = dbgen.CompleteUploadParams
	AgedUpload         = dbgen.ClaimAgedUploadsRow
	DownloadableFile   = dbgen.GetDownloadableChatAttachmentRow
	ServableFile       = dbgen.GetServableChatAttachmentRow
	QuotaUse           = dbgen.GetQuotaUseRow
	MemberChannelShare = dbgen.GetChannelOfActiveMemberRow
)

func (r Attachments) CreateUploading(ctx context.Context, upload NewUploading) error {
	return done(r.queries.CreateUploadingAttachment(ctx, upload), "create uploading attachment")
}

func (r Attachments) RecordMultipartUpload(ctx context.Context, id domain.AttachmentID, upload string, now time.Time) (bool, error) {
	rows, err := r.queries.RecordMultipartUpload(ctx, dbgen.RecordMultipartUploadParams{MultipartUploadID: upload, Now: now, ID: id})
	return applied(rows, err, "record multipart upload")
}

func (r Attachments) CompleteUpload(ctx context.Context, upload CompletedUpload) (bool, error) {
	rows, err := r.queries.CompleteUpload(ctx, upload)
	return applied(rows, err, "complete upload")
}

func (r Attachments) CountOpenUploads(
	ctx context.Context, uploader domain.UserID, contexts []domain.AttachmentContext, now time.Time, uploadingSince time.Time,
) (int64, error) {
	if len(contexts) == 0 {
		return 0, ErrNoCountedContexts
	}
	names := make([]string, 0, len(contexts))
	for _, counted := range contexts {
		names = append(names, string(counted))
	}
	count, err := r.queries.CountOpenUploads(ctx, dbgen.CountOpenUploadsParams{UploaderID: uploader, Contexts: names, Now: now, UploadingSince: uploadingSince})
	return one(count, err, "count open uploads")
}

func (r Attachments) QuotaUse(
	ctx context.Context, tutor domain.UserID, excluded *domain.AttachmentID, now time.Time, uploadingSince time.Time,
) (QuotaUse, error) {
	params := dbgen.GetQuotaUseParams{TutorID: tutor, Now: now, UploadingSince: uploadingSince}
	if excluded != nil {
		params.Excluded = pgtype.UUID{Bytes: *excluded, Valid: true}
	}
	use, err := r.queries.GetQuotaUse(ctx, params)
	return one(use, err, "quota use")
}

func (r Attachments) ClaimAged(ctx context.Context, cutoff time.Time, batch int32) ([]AgedUpload, error) {
	rows, err := r.queries.ClaimAgedUploads(ctx, dbgen.ClaimAgedUploadsParams{Cutoff: cutoff, BatchSize: batch})
	return one(rows, err, "claim aged uploads")
}

func (r Attachments) ClaimUnbindableChat(ctx context.Context, batch int32) ([]domain.AttachmentID, error) {
	ids, err := r.queries.ClaimUnbindableChatUploads(ctx, batch)
	return one(ids, err, "claim unbindable chat uploads")
}

func (r Attachments) ClaimUnbindableLibrary(ctx context.Context, batch int32) ([]domain.AttachmentID, error) {
	ids, err := r.queries.ClaimUnbindableLibraryUploads(ctx, batch)
	return one(ids, err, "claim unbindable library uploads")
}

func (r Attachments) DownloadableChat(ctx context.Context, viewer domain.UserID, id domain.AttachmentID) (DownloadableFile, error) {
	file, err := r.queries.GetDownloadableChatAttachment(ctx, dbgen.GetDownloadableChatAttachmentParams{
		Viewer: pgtype.UUID{Bytes: viewer, Valid: true}, ID: id,
	})
	return one(file, err, "downloadable chat attachment")
}

func (r Attachments) ServableChat(ctx context.Context, id domain.AttachmentID, message domain.EntityID) (ServableFile, error) {
	file, err := r.queries.GetServableChatAttachment(ctx, dbgen.GetServableChatAttachmentParams{MessageID: message, ID: id})
	return one(file, err, "servable chat attachment")
}

func (r Channels) LockOfMemberForShare(ctx context.Context, id domain.ChannelID, member domain.UserID) (MemberChannelShare, error) {
	locked, err := r.queries.LockChannelForShare(ctx, id)
	if _, err := one(locked, err, "lock channel for share"); err != nil {
		return MemberChannelShare{}, err
	}
	channel, err := r.queries.GetChannelOfActiveMember(ctx, dbgen.GetChannelOfActiveMemberParams{ChannelID: id, UserID: member})
	return one(channel, err, "channel of active member")
}

func (r Attachments) CountPending(ctx context.Context, uploader domain.UserID, context domain.AttachmentContext, now time.Time) (int64, error) {
	count, err := r.queries.CountPendingUploads(ctx, dbgen.CountPendingUploadsParams{UploaderID: uploader, Context: context, Now: now})
	return one(count, err, "count pending uploads")
}

func (r Attachments) Lock(ctx context.Context, id domain.AttachmentID) (Attachment, error) {
	attachment, err := r.queries.LockAttachment(ctx, id)
	return one(attachment, err, "lock attachment")
}

func (r Attachments) Bind(ctx context.Context, id domain.AttachmentID, now time.Time) (bool, error) {
	rows, err := r.queries.BindAttachment(ctx, dbgen.BindAttachmentParams{Now: now, ID: id})
	return applied(rows, err, "bind attachment")
}

func (r Attachments) RemoveAvatarOf(ctx context.Context, user domain.UserID) ([]domain.AttachmentID, error) {
	removed, err := r.queries.RemoveAvatarReference(ctx, domain.EntityID(user))
	return one(removed, err, "remove avatar reference")
}

func (r Attachments) AvatarOf(ctx context.Context, user domain.UserID) (domain.AttachmentID, error) {
	id, err := r.queries.GetAvatarOf(ctx, domain.EntityID(user))
	return one(id, err, "avatar of user")
}

func (r Attachments) ServableAvatar(ctx context.Context, id domain.AttachmentID) (ServableAvatar, error) {
	avatar, err := r.queries.GetServableAvatar(ctx, id)
	if _, err := one(avatar, err, "servable avatar"); err != nil {
		return ServableAvatar{}, err
	}
	if avatar.MediaType == nil || avatar.SizeBytes == nil {
		return ServableAvatar{}, fmt.Errorf("servable avatar: %w", ErrUndescribedAttachment)
	}
	return ServableAvatar{StorageKey: avatar.StorageKey, MediaType: *avatar.MediaType, SizeBytes: *avatar.SizeBytes}, nil
}

func (r Attachments) MarkDeleted(ctx context.Context, id domain.AttachmentID, now time.Time) (bool, error) {
	rows, err := r.queries.MarkAttachmentDeleted(ctx, dbgen.MarkAttachmentDeletedParams{Now: now, ID: id})
	return applied(rows, err, "mark attachment deleted")
}

func (r Attachments) ClaimExpired(ctx context.Context, now time.Time, batch int32) ([]domain.AttachmentID, error) {
	ids, err := r.queries.ClaimExpiredUploads(ctx, dbgen.ClaimExpiredUploadsParams{Now: now, BatchSize: batch})
	return one(ids, err, "claim expired uploads")
}

func (r Attachments) DeleteRow(ctx context.Context, id domain.AttachmentID) (bool, error) {
	rows, err := r.queries.DeleteAttachmentRow(ctx, id)
	return applied(rows, err, "delete attachment row")
}

func (r Users) UpdateProfile(ctx context.Context, update ProfileUpdate) (bool, error) {
	rows, err := r.queries.UpdateUserProfile(ctx, update)
	return applied(rows, err, "update user profile")
}

func (r TutorProfiles) UpdateText(ctx context.Context, text TutorText) (bool, error) {
	rows, err := r.queries.UpdateTutorProfileText(ctx, text)
	return applied(rows, err, "update tutor profile text")
}

type (
	Upload     = dbgen.ListUploadsOfRow
	PurgeState = dbgen.LockUserForPurgeRow
)

func (r Attachments) UploadsOf(ctx context.Context, uploader domain.UserID) ([]Upload, error) {
	uploads, err := r.queries.ListUploadsOf(ctx, uploader)
	return one(uploads, err, "uploads of user")
}

func (r Attachments) DeleteUploadsOf(ctx context.Context, uploader domain.UserID) (int64, error) {
	rows, err := r.queries.DeleteUploadsOf(ctx, uploader)
	return counted(rows, err, "delete uploads of user")
}

func (r Users) DeletedBefore(ctx context.Context, cutoff time.Time, skip int32, batch int32) ([]domain.UserID, error) {
	ids, err := r.queries.ListDeletedBefore(ctx, dbgen.ListDeletedBeforeParams{Cutoff: cutoff, Skip: skip, BatchSize: batch})
	return one(ids, err, "users deleted before")
}

func (r Users) UnverifiedSince(ctx context.Context, cutoff time.Time, skip int32, batch int32) ([]domain.UserID, error) {
	ids, err := r.queries.ListUnverifiedSince(ctx, dbgen.ListUnverifiedSinceParams{Cutoff: cutoff, Skip: skip, BatchSize: batch})
	return one(ids, err, "users unverified since")
}

func (r Users) LockForPurge(ctx context.Context, id domain.UserID) (PurgeState, error) {
	state, err := r.queries.LockUserForPurge(ctx, id)
	return one(state, err, "lock user for purge")
}

func (r Users) Delete(ctx context.Context, id domain.UserID) (bool, error) {
	rows, err := r.queries.DeleteUser(ctx, id)
	return applied(rows, err, "delete user")
}

func (r Attachments) NameUploading(ctx context.Context, id domain.AttachmentID, name string, now time.Time) error {
	rows, err := r.queries.NameUploadingAttachment(ctx, dbgen.NameUploadingAttachmentParams{ID: id, Filename: name, Now: now})
	return intentApplied(rows, err)
}
