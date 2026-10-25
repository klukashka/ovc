package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ovc/backend/internal/dbgen"
	"ovc/backend/internal/domain"
)

type (
	NewNotification          = dbgen.InsertNotificationParams
	Notification             = dbgen.ListNotificationsRow
	NotificationRelationship = dbgen.ListNotificationRelationshipsRow
	NotificationJoinRequest  = dbgen.ListNotificationJoinRequestsRow
	NotificationPosition     = dbgen.GetOwnNotificationRow
)

type NotificationCursor struct {
	CreatedAt time.Time
	ID        domain.NotificationID
}

type Notifications struct {
	queries *dbgen.Queries
}

func (r Notifications) Insert(ctx context.Context, row NewNotification) (bool, error) {
	rows, err := r.queries.InsertNotification(ctx, row)
	return applied(rows, err, "insert notification")
}

func (r Notifications) Of(ctx context.Context, user domain.UserID, after *NotificationCursor, limit int32) ([]Notification, error) {
	params := dbgen.ListNotificationsParams{UserID: user, RowLimit: limit}
	if after != nil {
		params.AfterCreatedAt = &after.CreatedAt
		params.AfterID = pgtype.UUID{Bytes: after.ID, Valid: true}
	}
	rows, err := r.queries.ListNotifications(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	return rows, nil
}

func (r Notifications) UnreadCount(ctx context.Context, user domain.UserID) (int64, error) {
	count, err := r.queries.CountUnreadNotifications(ctx, dbgen.CountUnreadNotificationsParams{UserID: user, Cap: domain.NotificationUnreadCap})
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}

func (r Notifications) CountOwn(ctx context.Context, user domain.UserID, ids []domain.NotificationID) (int64, error) {
	count, err := r.queries.CountOwnNotifications(ctx, dbgen.CountOwnNotificationsParams{UserID: user, Ids: uuids(ids)})
	if err != nil {
		return 0, fmt.Errorf("count own notifications: %w", err)
	}
	return count, nil
}

func (r Notifications) Own(ctx context.Context, user domain.UserID, id domain.NotificationID) (NotificationPosition, error) {
	row, err := r.queries.GetOwnNotification(ctx, dbgen.GetOwnNotificationParams{UserID: user, ID: id})
	return one(row, err, "get own notification")
}

func (r Notifications) MarkRead(ctx context.Context, user domain.UserID, ids []domain.NotificationID, now time.Time) (int64, error) {
	rows, err := r.queries.MarkNotificationsRead(ctx, dbgen.MarkNotificationsReadParams{UserID: user, Ids: uuids(ids), Now: now})
	return counted(rows, err, "mark notifications read")
}

func (r Notifications) MarkReadThrough(ctx context.Context, user domain.UserID, through NotificationPosition, now time.Time) (int64, error) {
	rows, err := r.queries.MarkNotificationsReadThrough(ctx, dbgen.MarkNotificationsReadThroughParams{
		UserID: user, ThroughCreatedAt: through.CreatedAt, ThroughID: pgtype.UUID{Bytes: through.ID, Valid: true}, Now: now,
	})
	return counted(rows, err, "mark notifications read through a row")
}

func (r Notifications) DeleteNaming(ctx context.Context, account domain.UserID) (int64, error) {
	rows, err := r.queries.DeleteNotificationsNaming(ctx, account)
	return counted(rows, err, "delete notifications naming an account")
}

func (r Notifications) DeleteBatchNaming(ctx context.Context, account domain.UserID, batch int32) (int64, error) {
	rows, err := r.queries.DeleteNotificationsNamingBatch(ctx, dbgen.DeleteNotificationsNamingBatchParams{AccountID: account, BatchSize: batch})
	return counted(rows, err, "delete a batch of notifications naming an account")
}

func (r Notifications) DeleteBefore(ctx context.Context, before time.Time, batch int32) (int64, error) {
	ids, err := r.queries.DeleteNotificationsBefore(ctx, dbgen.DeleteNotificationsBeforeParams{Before: before, BatchSize: batch})
	if err != nil {
		return 0, fmt.Errorf("delete old notifications: %w", err)
	}
	return int64(len(ids)), nil
}

func (r Notifications) Relationships(ctx context.Context, ids []domain.RelationshipID) ([]NotificationRelationship, error) {
	rows, err := r.queries.ListNotificationRelationships(ctx, uuids(ids))
	if err != nil {
		return nil, fmt.Errorf("list notification relationships: %w", err)
	}
	return rows, nil
}

func (r Notifications) JoinRequests(ctx context.Context, ids []domain.JoinRequestID) ([]NotificationJoinRequest, error) {
	rows, err := r.queries.ListNotificationJoinRequests(ctx, uuids(ids))
	if err != nil {
		return nil, fmt.Errorf("list notification join requests: %w", err)
	}
	return rows, nil
}
