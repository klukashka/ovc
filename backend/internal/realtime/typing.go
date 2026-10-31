package realtime

import (
	"context"
	"errors"
	"log/slog"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/chat"
)

func (s *socket) typed(ctx context.Context, frame schemas.ClientFrame, ref *string) {
	var payload schemas.TypingRequestPayload
	if frame.Payload == nil || decodeStrict(frame.Payload, &payload) != nil {
		s.reject(ctx, "malformed", domain.ErrorBadRequest, ref)
		return
	}
	channel, valid := domain.ParseUUID[domain.ChannelID](payload.ChannelID)
	if !valid {
		s.reject(ctx, "malformed", domain.ErrorBadRequest, ref)
		return
	}
	if !s.openTypingWindow(channel) {
		return
	}
	checkContext, cancel := context.WithTimeout(ctx, s.g.deps.Timing.LookupTimeout)
	defer cancel()
	check := s.g.deps.Chat.CheckTyping
	if frame.Type == domain.FrameTypingStart {
		check = s.g.deps.Chat.Typing
	}
	err := check(checkContext, s.user, channel)
	switch {
	case err == nil:
	case errors.Is(err, chat.ErrNotFound):
		s.refuseTyping(ctx, "not_found", domain.ErrorNotFound, ref)
	case errors.Is(err, chat.ErrReadOnly):
		s.refuseTyping(ctx, "read_only", domain.ErrorChannelReadOnly, ref)
	default:
		s.log().LogAttrs(ctx, slog.LevelError, "typing.failed", slog.String("kind", core.ErrorKind(err)))
		s.sendError(ctx, domain.ErrorInternal, ref)
	}
}

func (s *socket) openTypingWindow(channel domain.ChannelID) bool {
	s.g.mu.Lock()
	defer s.g.mu.Unlock()
	if _, open := s.typing[channel]; open {
		return false
	}
	if s.typing == nil {
		s.typing = map[domain.ChannelID]Timer{}
	}
	s.typing[channel] = s.g.deps.Scheduler.AfterFunc(s.g.deps.Timing.TypingCoalesce, func() {
		s.g.mu.Lock()
		defer s.g.mu.Unlock()
		delete(s.typing, channel)
	})
	return true
}

func (s *socket) refuseTyping(ctx context.Context, reason string, code domain.ErrorCode, ref *string) {
	s.log().LogAttrs(ctx, slog.LevelDebug, "typing.rejected", slog.String("reason", reason))
	s.sendError(ctx, code, ref)
}
