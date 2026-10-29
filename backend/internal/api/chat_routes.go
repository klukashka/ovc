package api

import (
	"context"
	"errors"
	"net/http"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/chat"
)

const channelsPath = "/api/v1/channels"

type channelListInput struct {
	Q      string `query:"q" maxLength:"254"`
	Limit  int32  `query:"limit" default:"30" minimum:"1" maximum:"100"`
	Cursor string `query:"cursor" maxLength:"128"`
}

type channelListOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.ChannelPage
}

type unreadChannelsOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.UnreadChannels
}

type channelInput struct {
	ID string `path:"id"`
}

type channelOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.ChannelDetail
}

func (r *router) registerChat() {
	list := operation("channels-list", http.MethodGet, channelsPath)
	list.Errors = errorsOf(http.StatusUnauthorized)
	register(r, list, AudienceUnverifiedOK, r.listChannels)

	unread := operation("channels-unread", http.MethodGet, channelsPath+"/unread")
	unread.Errors = errorsOf(http.StatusUnauthorized)
	register(r, unread, AudienceUnverifiedOK, r.unreadChannels)

	get := operation("channels-get", http.MethodGet, channelsPath+"/{id}")
	get.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, get, AudienceUnverifiedOK, r.channel)
}

func (r *router) listChannels(ctx context.Context, input *channelListInput) (*channelListOutput, error) {
	viewer := principalFrom(ctx).UserID
	page, err := r.chat.List(ctx, viewer, chat.Query{Search: input.Q, Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return nil, r.chatError(ctx, "channels.list.failed", err)
	}
	body := schemas.ChannelPage{Items: make([]schemas.ChannelItem, 0, len(page.Items)), NextCursor: page.NextCursor, UnreadChannels: page.UnreadChannels}
	for _, item := range page.Items {
		body.Items = append(body.Items, channelItemBody(item))
	}
	return &channelListOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) unreadChannels(ctx context.Context, _ *struct{}) (*unreadChannelsOutput, error) {
	count, err := r.chat.UnreadChannels(ctx, principalFrom(ctx).UserID)
	if err != nil {
		return nil, r.chatError(ctx, "channels.unread.failed", err)
	}
	return &unreadChannelsOutput{CacheControl: noStore, Body: schemas.UnreadChannels{UnreadChannels: count}}, nil
}

func (r *router) channel(ctx context.Context, input *channelInput) (*channelOutput, error) {
	id, valid := domain.ParseUUID[domain.ChannelID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	channel, err := r.chat.Channel(ctx, principalFrom(ctx).UserID, id)
	if err != nil {
		return nil, r.chatError(ctx, "channels.get.failed", err)
	}
	body := schemas.ChannelDetail{ChannelItem: channelItemBody(channel)}
	if channel.TutorView {
		if channel.RelationshipID != nil {
			relationship := channel.RelationshipID.String()
			body.RelationshipID = &relationship
		}
		if channel.GroupID != nil {
			group := channel.GroupID.String()
			body.GroupID = &group
		}
		members := make([]schemas.ChannelMember, 0, len(channel.Members))
		for _, member := range channel.Members {
			members = append(members, schemas.ChannelMember{UserID: member.UserID.String(), Label: member.Label, Online: member.Online})
		}
		body.Members = &members
	}
	return &channelOutput{CacheControl: noStore, Body: body}, nil
}

func channelItemBody(channel chat.Channel) schemas.ChannelItem {
	body := schemas.ChannelItem{
		ID: channel.ID.String(), Kind: channel.Kind, Label: channel.Label, LabelKind: channel.LabelKind, Colour: channel.Colour,
		UnreadCount: channel.UnreadCount, Writable: channel.Writable, Online: channel.Online, Unconfirmed: channel.Unconfirmed,
	}
	if channel.Tutor != nil {
		body.Tutor = &schemas.ChannelTutor{Name: channel.Tutor.Name, Colour: channel.Tutor.Colour}
	}
	if channel.UserID != nil {
		user := channel.UserID.String()
		body.UserID = &user
	}
	return body
}

func (r *router) chatError(ctx context.Context, event string, err error) error {
	var invalid *chat.ValidationError
	switch {
	case errors.Is(err, chat.ErrNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.As(err, &invalid):
		fields := []schemas.FieldError{{Path: invalid.Path, Code: string(invalid.Code)}}
		return &Error{status: http.StatusUnprocessableEntity, ErrorEnvelope: schemas.NewErrorEnvelope(domain.ErrorValidationFailed, core.RequestID(ctx), fields)}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return NewError(ctx, domain.ErrorServiceUnavailable)
	}
	return r.internal(ctx, event, err)
}
