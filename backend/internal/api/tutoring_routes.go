package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/tutoring"
)

type tutorCodeOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.TutorCode
}

func (r *router) registerTutoring() {
	code := operation("tutor-code", http.MethodGet, "/api/v1/tutor/code")
	code.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, code, AudienceTutorUnverifiedOK, r.tutorCode)

	regenerate := operation("tutor-code-regenerate", http.MethodPost, "/api/v1/tutor/code/regenerate")
	regenerate.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests)
	register(r, regenerate, AudienceTutor, r.regenerateTutorCode)
}

func (r *router) tutorCode(ctx context.Context, _ *struct{}) (*tutorCodeOutput, error) {
	current, err := r.tutoring.Code(ctx, principalFrom(ctx).UserID)
	if err != nil {
		return nil, r.internal(ctx, "tutor.code.failed", err)
	}
	return tutorCodeBody(current), nil
}

func (r *router) regenerateTutorCode(ctx context.Context, _ *struct{}) (*tutorCodeOutput, error) {
	regenerated, err := r.tutoring.RegenerateCode(ctx, principalFrom(ctx).UserID)
	var throttled *tutoring.ThrottledError
	switch {
	case err == nil:
		return tutorCodeBody(regenerated), nil
	case errors.As(err, &throttled):
		seconds := int64(math.Ceil(throttled.RetryAfter.Seconds()))
		return nil, NewError(ctx, domain.ErrorRateLimited).withHeader("Retry-After", strconv.FormatInt(max(seconds, 1), 10))
	}
	return nil, r.internal(ctx, "tutor.code.regenerate.failed", err)
}

func tutorCodeBody(code tutoring.TutorCode) *tutorCodeOutput {
	return &tutorCodeOutput{CacheControl: noStore, Body: schemas.TutorCode{Code: code.Code.Display(), Active: code.Active}}
}
