package api

import (
	"context"
	"errors"
	"net/http"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/notifications"
)

type notificationListOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.NotificationPage
}

type notificationReadInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.NotificationRead
}

type notificationReadOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.NotificationUnread
}

func (r *router) registerNotificationRoutes() {
	list := operation("notifications-list", http.MethodGet, "/api/v1/notifications")
	list.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden)
	register(r, list, AudienceUnverifiedOK, r.listNotifications)

	read := operation("notifications-read", http.MethodPost, "/api/v1/notifications/read")
	read.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, read, AudienceUnverifiedOK, r.readNotifications)
}

func (r *router) listNotifications(ctx context.Context, input *pageInput) (*notificationListOutput, error) {
	page, err := r.inbox.List(ctx, principalFrom(ctx), notifications.PageQuery{Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return nil, r.flowError(ctx, "notifications.list_failed", err)
	}
	body := schemas.NotificationPage{Items: make([]schemas.Notification, 0, len(page.Items)), NextCursor: page.NextCursor, UnreadCount: page.UnreadCount}
	for _, item := range page.Items {
		shown := schemas.Notification{
			ID: item.ID.String(), Event: item.Event, CreatedAt: item.CreatedAt, ReadAt: item.ReadAt, Label: item.Label,
			LabelDeleted: item.LabelDeleted, LabelKind: item.LabelKind, Colour: item.Colour,
		}
		if item.Link != nil {
			shown.Link = &schemas.NotificationLink{Kind: item.Link.Kind, ID: item.Link.ID.String()}
		}
		body.Items = append(body.Items, shown)
	}
	return &notificationListOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) readNotifications(ctx context.Context, input *notificationReadInput) (*notificationReadOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	unread, err := r.inbox.MarkRead(ctx, principalFrom(ctx), notifications.ReadRequest{IDs: input.Body.IDs, Through: input.Body.Through})
	if errors.Is(err, notifications.ErrNotificationNotFound) {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err != nil {
		return nil, r.flowError(ctx, "notifications.read_failed", err)
	}
	return &notificationReadOutput{CacheControl: noStore, Body: schemas.NotificationUnread{UnreadCount: unread}}, nil
}
