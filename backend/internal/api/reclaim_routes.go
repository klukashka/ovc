package api

import (
	"context"
	"errors"
	"net/http"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/tutoring"
)

const reclaimPath = "/api/v1/users/me/reclaim"

type reclaimListOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.ReclaimList
}

type reclaimChoiceInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.ReclaimChoice
}

type reclaimDecidedOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.ReclaimDecided
}

func (r *router) registerReclaimRoutes() {
	list := operation("users-me-reclaim", http.MethodGet, reclaimPath)
	list.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden)
	register(r, list, AudienceUser, r.reclaimItems)

	decide := operation("users-me-reclaim-decide", http.MethodPost, reclaimPath)
	decide.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, decide, AudienceUser, r.decideReclaim)
}

func (r *router) reclaimItems(ctx context.Context, _ *struct{}) (*reclaimListOutput, error) {
	list, err := r.tutoring.ReclaimItems(ctx, principalFrom(ctx))
	if err != nil {
		return nil, r.reclaimError(ctx, "users.me.reclaim.failed", err)
	}
	body := schemas.ReclaimList{
		Links: make([]schemas.ReclaimLink, 0, len(list.Links)), Requests: make([]schemas.ReclaimRequest, 0, len(list.Requests)),
		ChooseBy: utc(list.ChooseBy),
	}
	for _, link := range list.Links {
		body.Links = append(body.Links, schemas.ReclaimLink{RelationshipID: link.RelationshipID.String(), Tutor: reclaimTutorBody(link.Tutor)})
	}
	for _, request := range list.Requests {
		body.Requests = append(body.Requests, schemas.ReclaimRequest{RequestID: request.RequestID.String(), Tutor: reclaimTutorBody(request.Tutor)})
	}
	return &reclaimListOutput{CacheControl: noStore, Body: body}, nil
}

func reclaimTutorBody(tutor tutoring.ReclaimTutor) schemas.ReclaimTutor {
	return schemas.ReclaimTutor{Name: tutor.Name, AvatarID: optionalID(tutor.AvatarID), Subjects: append([]string{}, tutor.Subjects...)}
}

func (r *router) decideReclaim(ctx context.Context, input *reclaimChoiceInput) (*reclaimDecidedOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	outcome, err := r.tutoring.DecideReclaim(ctx, principalFrom(ctx), tutoring.ReclaimChoice{
		KeepLinks: input.Body.KeepLinks, DropLinks: input.Body.DropLinks,
		KeepRequests: input.Body.KeepRequests, DropRequests: input.Body.DropRequests,
	})
	if err != nil {
		return nil, r.reclaimError(ctx, "users.me.reclaim.decide_failed", err)
	}
	return &reclaimDecidedOutput{CacheControl: noStore, Body: schemas.ReclaimDecided{Outcome: outcome}}, nil
}

func (r *router) reclaimError(ctx context.Context, event string, err error) error {
	switch {
	case errors.Is(err, tutoring.ErrReclaimItemNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, accounts.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	}
	return r.tutoringError(ctx, event, err)
}
