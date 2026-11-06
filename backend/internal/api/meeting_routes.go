package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/meetings"
)

type lessonJoinOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.LessonJoin
}

type endedHintInput struct {
	RoutingKey     string `path:"routing_key"`
	RecordingMarks string `query:"recordingmarks"`
}

type endedHintOutput struct {
	CacheControl string `header:"Cache-Control"`
}

func (r *router) registerMeetings() {
	join := operation("lessons-join", http.MethodPost, lessonsPath+"/{id}/join")
	join.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests, http.StatusServiceUnavailable)
	register(r, join, AudienceUnverifiedOK, r.joinLesson)

	end := operation("lessons-end", http.MethodPost, lessonsPath+"/{id}/end")
	end.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests, http.StatusServiceUnavailable)
	register(r, end, AudienceTutor, r.endLesson)

	restart := operation("lessons-restart-room", http.MethodPost, lessonsPath+"/{id}/restart-room")
	restart.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests, http.StatusServiceUnavailable)
	register(r, restart, AudienceTutor, r.restartRoom)

	hint := operation("integrations-bbb-ended", http.MethodGet, "/api/v1/integrations/bbb/{routing_key}/ended")
	hint.DefaultStatus = http.StatusOK
	register(r, hint, AudienceProvider, r.endedHint)
}

func actorFrom(ctx context.Context) meetings.Actor {
	principal := principalFrom(ctx)
	return meetings.Actor{UserID: principal.UserID, SessionID: principal.SessionID, Role: principal.Role}
}

func (r *router) joinLesson(ctx context.Context, input *lessonInput) (*lessonJoinOutput, error) {
	id, valid := domain.ParseUUID[domain.LessonID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if principal := principalFrom(ctx); principal.Role == domain.RoleTutor {
		if refusal, refused := authenticatedAudiences[AudienceTutor].refusal(principal); refused {
			return nil, NewError(ctx, refusal)
		}
	}
	ticket, err := r.meetings.Join(ctx, actorFrom(ctx), id)
	if err != nil {
		return nil, r.meetingError(ctx, "lessons.join.failed", err)
	}
	return &lessonJoinOutput{CacheControl: noStore, Body: schemas.LessonJoin{URL: ticket.Link.Reveal(), EmbedAllowed: ticket.EmbedAllowed}}, nil
}

func (r *router) endLesson(ctx context.Context, input *lessonInput) (*lessonOutput, error) {
	id, valid := domain.ParseUUID[domain.LessonID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err := r.meetings.End(ctx, actorFrom(ctx), id); err != nil {
		return nil, r.meetingError(ctx, "lessons.end.failed", err)
	}
	principal := principalFrom(ctx)
	lesson, err := r.schedule.Lesson(ctx, principal.UserID, principal.Role, id)
	if err != nil {
		return nil, r.scheduleError(ctx, "lessons.end.failed", err)
	}
	return &lessonOutput{CacheControl: noStore, Body: lessonBody(lesson)}, nil
}

func (r *router) restartRoom(ctx context.Context, input *lessonInput) (*lessonJoinOutput, error) {
	id, valid := domain.ParseUUID[domain.LessonID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	ticket, err := r.meetings.Restart(ctx, actorFrom(ctx), id)
	if err != nil {
		return nil, r.meetingError(ctx, "lessons.restart_room.failed", err)
	}
	return &lessonJoinOutput{CacheControl: noStore, Body: schemas.LessonJoin{URL: ticket.Link.Reveal(), EmbedAllowed: ticket.EmbedAllowed}}, nil
}

func (r *router) endedHint(ctx context.Context, input *endedHintInput) (*endedHintOutput, error) {
	r.meetings.AcceptEndedHint(ctx, input.RoutingKey, input.RecordingMarks == "true")
	return &endedHintOutput{CacheControl: noStore}, nil
}

func (r *router) meetingError(ctx context.Context, event string, err error) error {
	var throttled *meetings.ThrottledError
	switch {
	case errors.Is(err, meetings.ErrNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, meetings.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	case errors.Is(err, meetings.ErrOutsideJoinWindow):
		return NewError(ctx, domain.ErrorOutsideJoinWindow)
	case errors.Is(err, meetings.ErrLessonNotLive):
		return NewError(ctx, domain.ErrorLessonNotLive)
	case errors.Is(err, meetings.ErrConflict):
		return NewError(ctx, domain.ErrorConflict)
	case errors.Is(err, meetings.ErrProviderUnavailable):
		return NewError(ctx, domain.ErrorProviderUnavailable)
	case errors.Is(err, meetings.ErrAdmissionCapacity), errors.Is(err, meetings.ErrVideoUnavailable):
		return NewError(ctx, domain.ErrorServiceUnavailable)
	case errors.As(err, &throttled):
		seconds := int64(math.Ceil(throttled.RetryAfter.Seconds()))
		return NewError(ctx, domain.ErrorRateLimited).withHeader("Retry-After", strconv.FormatInt(max(seconds, 1), 10))
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return NewError(ctx, domain.ErrorServiceUnavailable)
	}
	return r.internal(ctx, event, err)
}
