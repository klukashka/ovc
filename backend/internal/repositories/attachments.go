package repositories

import (
	"context"
	"time"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	ServableAvatar = dbgen.GetServableAvatarRow
	ProfileUpdate  = dbgen.UpdateUserProfileParams
	TutorText      = dbgen.UpdateTutorProfileTextParams
)

func (r Attachments) CountPending(ctx context.Context, uploader domain.UserID, context domain.AttachmentContext) (int64, error) {
	count, err := r.queries.CountPendingUploads(ctx, dbgen.CountPendingUploadsParams{UploaderID: uploader, Context: context})
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
	return one(avatar, err, "servable avatar")
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
