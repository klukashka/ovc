package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/chat"
)

const (
	channelsPath = "/api/v1/channels"
	messagesPath = "/api/v1/messages"
)

type channelListInput struct {
	Filter domain.ChannelFilter `query:"filter" default:"all" enum:"all,direct,group,unread"`
	Q      string               `query:"q" maxLength:"254"`
	Limit  int32                `query:"limit" default:"30" minimum:"1" maximum:"100"`
	Cursor string               `query:"cursor" maxLength:"128"`
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

type messageListInput struct {
	ID        string `path:"id"`
	Cursor    string `query:"cursor" maxLength:"64"`
	AfterSeq  string `query:"after_seq" pattern:"^[0-9]{1,18}$"`
	AroundSeq string `query:"around_seq" pattern:"^[0-9]{1,18}$"`
	Limit     int32  `query:"limit" default:"50" minimum:"1" maximum:"100"`
}

type messageListOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.MessagePage
}

type sendMessageInput struct {
	ID             string `path:"id"`
	ContentType    string `header:"Content-Type"`
	IdempotencyKey string `header:"Idempotency-Key" maxLength:"64" pattern:"^[A-Za-z0-9._-]{1,64}$"`
	Body           schemas.NewMessage
}

type messageOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.Message
}

type editMessageInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.MessagePatch
}

type messageInput struct {
	ID string `path:"id"`
}

type deletedMessageOutput struct {
	CacheControl string `header:"Cache-Control"`
}

type pinInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.PinRequest
}

type readMarkerInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.ReadMarker
}

type searchInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.SearchRequest
}

type searchOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.SearchPage
}

type reactionInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.ReactionRequest
}

type readStateOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.ReadState
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

	messages := operation("channels-messages-list", http.MethodGet, channelsPath+"/{id}/messages")
	messages.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, messages, AudienceUnverifiedOK, r.listMessages)

	send := operation("channels-messages-create", http.MethodPost, channelsPath+"/{id}/messages")
	send.DefaultStatus = http.StatusCreated
	send.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests,
		http.StatusInsufficientStorage)
	register(r, send, AudienceUnverifiedOK, r.sendMessage)

	edit := operation("messages-update", http.MethodPatch, messagesPath+"/{id}")
	edit.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, edit, AudienceUnverifiedOK, r.editMessage)

	remove := operation("messages-delete", http.MethodDelete, messagesPath+"/{id}")
	remove.DefaultStatus = http.StatusNoContent
	remove.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests)
	register(r, remove, AudienceUnverifiedOK, r.deleteMessage)

	search := operation("channels-messages-search", http.MethodPost, channelsPath+"/{id}/messages/search")
	search.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests, http.StatusServiceUnavailable)
	register(r, search, AudienceUnverifiedOK, r.searchMessages)

	react := operation("messages-reaction-set", http.MethodPut, messagesPath+"/{id}/reaction")
	react.DefaultStatus = http.StatusNoContent
	react.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, react, AudienceUnverifiedOK, r.setReaction)

	unreact := operation("messages-reaction-delete", http.MethodDelete, messagesPath+"/{id}/reaction")
	unreact.DefaultStatus = http.StatusNoContent
	unreact.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests)
	register(r, unreact, AudienceUnverifiedOK, r.removeReaction)

	read := operation("channels-read", http.MethodPost, channelsPath+"/{id}/read")
	read.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, read, AudienceUnverifiedOK, r.markRead)

	pin := operation("channels-pin-set", http.MethodPut, channelsPath+"/{id}/pin")
	pin.DefaultStatus = http.StatusNoContent
	pin.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, pin, AudienceTutorUnverifiedOK, r.pinMessage)

	unpin := operation("channels-pin-delete", http.MethodDelete, channelsPath+"/{id}/pin")
	unpin.DefaultStatus = http.StatusNoContent
	unpin.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests)
	register(r, unpin, AudienceTutorUnverifiedOK, r.unpinMessage)
}

func (r *router) pinMessage(ctx context.Context, input *pinInput) (*deletedMessageOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.ChannelID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	message, valid := domain.ParseUUID[domain.MessageID](input.Body.MessageID)
	if !valid {
		message = domain.MessageID{}
	}
	if err := r.chat.Pin(ctx, authorOf(ctx), id, message); err != nil {
		return nil, r.chatError(ctx, "channels.pin.set.failed", err)
	}
	return &deletedMessageOutput{CacheControl: noStore}, nil
}

func (r *router) unpinMessage(ctx context.Context, input *messageInput) (*deletedMessageOutput, error) {
	id, valid := domain.ParseUUID[domain.ChannelID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err := r.chat.Unpin(ctx, authorOf(ctx), id); err != nil {
		return nil, r.chatError(ctx, "channels.pin.delete.failed", err)
	}
	return &deletedMessageOutput{CacheControl: noStore}, nil
}

func authorOf(ctx context.Context) chat.Author {
	principal := principalFrom(ctx)
	return chat.Author{UserID: principal.UserID, SessionID: principal.SessionID, Role: principal.Role}
}

func (r *router) listMessages(ctx context.Context, input *messageListInput) (*messageListOutput, error) {
	id, valid := domain.ParseUUID[domain.ChannelID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	query := chat.HistoryQuery{Cursor: input.Cursor, Limit: input.Limit}
	if input.AfterSeq != "" {
		after, err := strconv.ParseInt(input.AfterSeq, 10, 64)
		if err != nil {
			return nil, r.chatError(ctx, "channels.messages.list.failed", &chat.ValidationError{Path: "query.after_seq", Code: domain.FieldInvalid})
		}
		query.AfterSeq = &after
	}
	if input.AroundSeq != "" {
		around, err := strconv.ParseInt(input.AroundSeq, 10, 64)
		if err != nil {
			return nil, r.chatError(ctx, "channels.messages.list.failed", &chat.ValidationError{Path: "query.around_seq", Code: domain.FieldInvalid})
		}
		query.AroundSeq = &around
	}
	page, err := r.chat.History(ctx, principalFrom(ctx).UserID, id, query)
	if err != nil {
		return nil, r.chatError(ctx, "channels.messages.list.failed", err)
	}
	body := schemas.MessagePage{
		Items: make([]schemas.Message, 0, len(page.Items)), NextCursor: page.NextCursor, HasNewer: page.HasNewer, PeerReadSeq: page.PeerReadSeq,
	}
	for _, message := range page.Items {
		body.Items = append(body.Items, messageBody(message))
	}
	return &messageListOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) setReaction(ctx context.Context, input *reactionInput) (*deletedMessageOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.MessageID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err := r.chat.React(ctx, authorOf(ctx), id, input.Body.Reaction); err != nil {
		return nil, r.chatError(ctx, "messages.reaction.set.failed", err)
	}
	return &deletedMessageOutput{CacheControl: noStore}, nil
}

func (r *router) removeReaction(ctx context.Context, input *messageInput) (*deletedMessageOutput, error) {
	id, valid := domain.ParseUUID[domain.MessageID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err := r.chat.Unreact(ctx, authorOf(ctx), id); err != nil {
		return nil, r.chatError(ctx, "messages.reaction.delete.failed", err)
	}
	return &deletedMessageOutput{CacheControl: noStore}, nil
}

func (r *router) searchMessages(ctx context.Context, input *searchInput) (*searchOutput, error) {
	recordAccessCount(ctx, accessQueryLength, chat.SearchTermLength(input.Body.Q))
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.ChannelID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	page, err := r.chat.Search(ctx, principalFrom(ctx).UserID, id, chat.SearchRequest{Term: input.Body.Q, Cursor: input.Body.Cursor})
	if err != nil {
		return nil, r.chatError(ctx, "channels.messages.search.failed", err)
	}
	body := schemas.SearchPage{Items: make([]schemas.Message, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, message := range page.Items {
		body.Items = append(body.Items, messageBody(message))
	}
	return &searchOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) sendMessage(ctx context.Context, input *sendMessageInput) (*messageOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.ChannelID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	draft := chat.Draft{Body: input.Body.Body, Attachments: make([]domain.AttachmentID, 0, len(input.Body.AttachmentIDs))}
	for _, text := range input.Body.AttachmentIDs {
		attachment, valid := domain.ParseUUID[domain.AttachmentID](text)
		if !valid {
			return nil, r.chatError(ctx, "channels.messages.create.failed", &chat.ValidationError{Path: "body.attachment_ids", Code: domain.FieldInvalid})
		}
		draft.Attachments = append(draft.Attachments, attachment)
	}
	if input.IdempotencyKey != "" {
		draft.ClientKey = &input.IdempotencyKey
	}
	if input.Body.ReplyToMessageID != nil {
		replyTo, valid := domain.ParseUUID[domain.MessageID](*input.Body.ReplyToMessageID)
		if !valid {
			replyTo = domain.MessageID{}
		}
		draft.ReplyTo = &replyTo
	}
	message, err := r.chat.Send(ctx, authorOf(ctx), id, draft)
	if err != nil {
		return nil, r.chatError(ctx, "channels.messages.create.failed", err)
	}
	return &messageOutput{CacheControl: noStore, Body: messageBody(message)}, nil
}

func (r *router) editMessage(ctx context.Context, input *editMessageInput) (*messageOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.MessageID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	message, err := r.chat.Edit(ctx, authorOf(ctx), id, input.Body.Body, input.Body.Version)
	if err != nil {
		return nil, r.chatError(ctx, "messages.update.failed", err)
	}
	return &messageOutput{CacheControl: noStore, Body: messageBody(message)}, nil
}

func (r *router) deleteMessage(ctx context.Context, input *messageInput) (*deletedMessageOutput, error) {
	id, valid := domain.ParseUUID[domain.MessageID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err := r.chat.Delete(ctx, authorOf(ctx), id); err != nil {
		return nil, r.chatError(ctx, "messages.delete.failed", err)
	}
	return &deletedMessageOutput{CacheControl: noStore}, nil
}

func (r *router) markRead(ctx context.Context, input *readMarkerInput) (*readStateOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.ChannelID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	state, err := r.chat.MarkRead(ctx, authorOf(ctx), id, input.Body.Seq)
	if err != nil {
		return nil, r.chatError(ctx, "channels.read.failed", err)
	}
	return &readStateOutput{CacheControl: noStore, Body: schemas.ReadState{UnreadCount: state.UnreadCount, UnreadChannels: state.UnreadChannels}}, nil
}

func messageBody(message chat.Message) schemas.Message {
	body := schemas.Message{
		ID: message.ID.String(), Seq: message.Seq, SenderLabel: message.SenderLabel, SenderLabelKind: message.SenderLabelKind, Own: message.Own,
		Body: message.Body, CreatedAt: message.CreatedAt.UTC(), EditedAt: utc(message.EditedAt), Deleted: message.Deleted, Erased: message.Erased,
		Version: message.Version, SentBeforeReclaim: message.SentBeforeReclaim,
		Attachments: make([]schemas.MessageAttachment, 0, len(message.Attachments)),
		Reactions:   make([]schemas.MessageReaction, 0, len(message.Reactions)),
	}
	for _, reaction := range message.Reactions {
		item := schemas.MessageReaction{Reaction: reaction.Reaction, Count: reaction.Count, Mine: reaction.Mine}
		if reaction.Members != nil {
			members := make([]schemas.ReactionMember, 0, len(*reaction.Members))
			for _, member := range *reaction.Members {
				members = append(members, schemas.ReactionMember{Label: member.Label, LabelKind: member.LabelKind})
			}
			item.Members = &members
		}
		body.Reactions = append(body.Reactions, item)
	}
	if quote := message.ReplyTo; quote != nil {
		body.ReplyTo = &schemas.MessageQuote{
			MessageID: quote.MessageID.String(), Seq: quote.Seq, SenderLabel: quote.SenderLabel, SenderLabelKind: quote.SenderLabelKind, Own: quote.Own,
			Preview: quote.Preview, State: quote.State, SentBeforeReclaim: quote.SentBeforeReclaim,
		}
	}
	for _, attachment := range message.Attachments {
		body.Attachments = append(body.Attachments, schemas.MessageAttachment{
			ID: attachment.ID.String(), Filename: attachment.Filename, MediaType: attachment.MediaType, SizeBytes: attachment.SizeBytes,
			Width: attachment.Width, Height: attachment.Height, URL: attachment.URL, ExpiresAt: attachment.ExpiresAt.UTC(),
		})
	}
	return body
}

func (r *router) listChannels(ctx context.Context, input *channelListInput) (*channelListOutput, error) {
	viewer := principalFrom(ctx).UserID
	page, err := r.chat.List(ctx, viewer, chat.Query{Filter: input.Filter, Search: input.Q, Cursor: input.Cursor, Limit: input.Limit})
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
	body := schemas.ChannelDetail{ChannelItem: channelItemBody(channel), PeerReadSeq: channel.PeerReadSeq}
	if pinned := channel.Pinned; pinned != nil {
		body.Pinned = &schemas.PinnedMessage{
			MessageID: pinned.MessageID.String(), Seq: pinned.Seq, SenderLabel: pinned.SenderLabel, SenderLabelKind: pinned.SenderLabelKind,
			Own: pinned.Own, Preview: pinned.Preview, SentBeforeReclaim: pinned.SentBeforeReclaim,
		}
	}
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
	if last := channel.LastMessage; last != nil {
		body.LastMessage = &schemas.ChannelLastMessage{
			Seq: last.Seq, CreatedAt: last.CreatedAt.UTC(), Own: last.Own, SenderLabel: last.SenderLabel, SenderLabelKind: last.SenderLabelKind,
			Preview: last.Preview, AttachmentCount: last.AttachmentCount, PeerRead: last.PeerRead,
		}
	}
	return body
}

func (r *router) chatError(ctx context.Context, event string, err error) error {
	var invalid *chat.ValidationError
	var throttled *chat.ThrottledError
	var missing *chat.NotFoundAtError
	switch {
	case errors.Is(err, chat.ErrNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, chat.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	case errors.Is(err, chat.ErrReadOnly):
		return NewError(ctx, domain.ErrorChannelReadOnly)
	case errors.Is(err, chat.ErrEditWindowPassed):
		return NewError(ctx, domain.ErrorEditWindowPassed)
	case errors.Is(err, chat.ErrConflict):
		return NewError(ctx, domain.ErrorConflict)
	case errors.Is(err, chat.ErrQuotaExceeded):
		return NewError(ctx, domain.ErrorQuotaExceeded)
	case errors.Is(err, chat.ErrSearchBusy), errors.Is(err, chat.ErrSearchTimedOut):
		retryAfter := strconv.FormatInt(int64(domain.MessageSearchRetryAfter.Seconds()), 10)
		return NewError(ctx, domain.ErrorServiceUnavailable).withHeader("Retry-After", retryAfter)
	case errors.As(err, &throttled):
		seconds := int64(math.Ceil(throttled.RetryAfter.Seconds()))
		return NewError(ctx, domain.ErrorRateLimited).withHeader("Retry-After", strconv.FormatInt(max(seconds, 1), 10))
	case errors.As(err, &missing):
		fields := []schemas.FieldError{{Path: missing.Path, Code: string(domain.FieldNotFound)}}
		return &Error{status: http.StatusNotFound, ErrorEnvelope: schemas.NewErrorEnvelope(domain.ErrorNotFound, core.RequestID(ctx), fields)}
	case errors.As(err, &invalid):
		fields := []schemas.FieldError{{Path: invalid.Path, Code: string(invalid.Code)}}
		return &Error{status: http.StatusUnprocessableEntity, ErrorEnvelope: schemas.NewErrorEnvelope(domain.ErrorValidationFailed, core.RequestID(ctx), fields)}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return NewError(ctx, domain.ErrorServiceUnavailable)
	}
	return r.internal(ctx, event, err)
}
