package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/tutoring"
)

const joinRequestsPath = "/api/v1/join-requests"

type enterCodeInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.JoinRequestEntry
}

type enterCodeOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.JoinRequestSent
}

type pageInput struct {
	Limit  int32  `query:"limit" default:"30" minimum:"1" maximum:"100"`
	Cursor string `query:"cursor" maxLength:"128"`
}

type tutorRequestsOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.TutorJoinRequestPage
}

type sentRequestsOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.SentJoinRequestPage
}

type requestInput struct {
	ID string `path:"id"`
}

type declineInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.JoinRequestDecline
}

type declineOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.JoinRequestDecision
}

type blockedListOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.BlockedRequesterPage
}

type blockedInput struct {
	StudentID string `path:"studentId"`
}

func (r *router) registerJoinRequests() {
	enter := operation("join-requests-create", http.MethodPost, joinRequestsPath)
	enter.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusTooManyRequests)
	register(r, enter, AudienceUnverifiedOK, r.enterCode)

	list := operation("join-requests-list", http.MethodGet, joinRequestsPath)
	list.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, list, AudienceTutor, r.tutorRequests)

	sent := operation("join-requests-sent", http.MethodGet, joinRequestsPath+"/sent")
	sent.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, sent, AudienceStudentUnverifiedOK, r.sentRequests)

	accept := operation("join-requests-accept", http.MethodPost, joinRequestsPath+"/{id}/accept")
	accept.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	register(r, accept, AudienceTutor, r.acceptRequest)

	decline := operation("join-requests-decline", http.MethodPost, joinRequestsPath+"/{id}/decline")
	decline.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	register(r, decline, AudienceTutor, r.declineRequest)

	blocked := operation("tutor-blocked-requesters-list", http.MethodGet, "/api/v1/tutor/blocked-requesters")
	blocked.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, blocked, AudienceTutor, r.blockedStudents)

	unblock := operation("tutor-blocked-requesters-delete", http.MethodDelete, "/api/v1/tutor/blocked-requesters/{studentId}")
	unblock.DefaultStatus = http.StatusNoContent
	unblock.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, unblock, AudienceTutor, r.unblock)
}

func (r *router) enterCode(ctx context.Context, input *enterCodeInput) (*enterCodeOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	request, err := r.tutoring.EnterCode(ctx, principalFrom(ctx), input.Body.Code)
	if err != nil {
		return nil, r.joinRequestError(ctx, "join_requests.create.failed", err)
	}
	return &enterCodeOutput{CacheControl: noStore, Body: schemas.JoinRequestSent{Request: sentRequestBody(request)}}, nil
}

func (r *router) tutorRequests(ctx context.Context, input *pageInput) (*tutorRequestsOutput, error) {
	page, err := r.tutoring.TutorRequests(ctx, principalFrom(ctx).UserID, tutoring.PageQuery{Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return nil, r.joinRequestError(ctx, "join_requests.list.failed", err)
	}
	body := schemas.TutorJoinRequestPage{Items: make([]schemas.TutorJoinRequest, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		body.Items = append(body.Items, schemas.TutorJoinRequest{
			ID:        item.ID.String(),
			Student:   schemas.RequestingStudent{Name: item.Student.Name, Email: item.Student.Email, AvatarID: optionalID(item.Student.AvatarID)},
			CreatedAt: item.CreatedAt.UTC(), ExpiresAt: item.ExpiresAt.UTC(),
		})
	}
	return &tutorRequestsOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) sentRequests(ctx context.Context, input *pageInput) (*sentRequestsOutput, error) {
	page, err := r.tutoring.SentRequests(ctx, principalFrom(ctx), tutoring.PageQuery{Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return nil, r.joinRequestError(ctx, "join_requests.sent.failed", err)
	}
	body := schemas.SentJoinRequestPage{Items: make([]schemas.SentJoinRequest, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		body.Items = append(body.Items, sentRequestBody(item))
	}
	return &sentRequestsOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) acceptRequest(ctx context.Context, input *requestInput) (*cardOutput, error) {
	id, valid := domain.ParseUUID[domain.JoinRequestID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	card, err := r.tutoring.AcceptRequest(ctx, tutorActorFrom(ctx), id)
	if err != nil {
		return nil, r.joinRequestError(ctx, "join_requests.accept.failed", err)
	}
	return &cardOutput{CacheControl: noStore, Body: cardBody(card)}, nil
}

func (r *router) declineRequest(ctx context.Context, input *declineInput) (*declineOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.JoinRequestID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	block := input.Body.Block != nil && *input.Body.Block
	decision, err := r.tutoring.DeclineRequest(ctx, tutorActorFrom(ctx), id, block)
	if err != nil {
		return nil, r.joinRequestError(ctx, "join_requests.decline.failed", err)
	}
	return &declineOutput{CacheControl: noStore, Body: schemas.JoinRequestDecision{ID: decision.ID.String(), State: decision.State}}, nil
}

func (r *router) blockedStudents(ctx context.Context, input *pageInput) (*blockedListOutput, error) {
	page, err := r.tutoring.BlockedStudents(ctx, principalFrom(ctx).UserID, tutoring.PageQuery{Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return nil, r.joinRequestError(ctx, "tutor.blocked_requesters.list.failed", err)
	}
	body := schemas.BlockedRequesterPage{Items: make([]schemas.BlockedRequester, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		body.Items = append(body.Items, schemas.BlockedRequester{
			StudentID: item.StudentID.String(), Name: item.Name, AvatarID: optionalID(item.AvatarID), BlockedAt: item.BlockedAt.UTC(),
		})
	}
	return &blockedListOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) unblock(ctx context.Context, input *blockedInput) (*withdrawOutput, error) {
	student, valid := domain.ParseUUID[domain.UserID](input.StudentID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err := r.tutoring.Unblock(ctx, tutorActorFrom(ctx), student); err != nil {
		return nil, r.joinRequestError(ctx, "tutor.blocked_requesters.delete.failed", err)
	}
	return &withdrawOutput{CacheControl: noStore}, nil
}

func sentRequestBody(request tutoring.SentJoinRequest) schemas.SentJoinRequest {
	return schemas.SentJoinRequest{ID: request.ID.String(), State: request.State, CreatedAt: request.CreatedAt.UTC(), DecidedAt: utc(request.DecidedAt)}
}

func (r *router) joinRequestError(ctx context.Context, event string, err error) error {
	switch {
	case errors.Is(err, tutoring.ErrRoleMismatch):
		return NewError(ctx, domain.ErrorRoleMismatch)
	case errors.Is(err, accounts.ErrTutorCodeInvalid):
		return NewError(ctx, domain.ErrorTutorCodeInvalid)
	case errors.Is(err, tutoring.ErrAlreadyLinked):
		return NewError(ctx, domain.ErrorAlreadyLinked)
	case errors.Is(err, tutoring.ErrTutorUnavailable):
		seconds := int64(math.Ceil(tutoring.TutorUnavailableRetry.Seconds()))
		return NewError(ctx, domain.ErrorTutorUnavailable).withHeader("Retry-After", strconv.FormatInt(seconds, 10))
	case errors.Is(err, tutoring.ErrJoinRequestNotFound), errors.Is(err, tutoring.ErrBlockedRequesterNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, accounts.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	}
	return r.tutoringError(ctx, event, err)
}
