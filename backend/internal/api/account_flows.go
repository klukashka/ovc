package api

import (
	"context"
	"errors"
	"math"
	"mime"
	"net/http"
	"net/netip"
	"strconv"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
)

type registerInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.RegisterRequest
}

type verifyEmailInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.VerifyEmailRequest
}

type verifyEmailOutput struct {
	SetCookie    string `header:"Set-Cookie"`
	CacheControl string `header:"Cache-Control"`
	Body         schemas.EmailConfirmed
}

type resendInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.ResendConfirmationRequest
}

type forgotInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.ForgotPasswordRequest
}

type resetInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.ResetPasswordRequest
}

type changeInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.ChangePasswordRequest
}

type acceptedOutput struct {
	CacheControl string `header:"Cache-Control"`
}

type resetOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.PasswordReset
}

var (
	bodyErrors   = []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity}
	serverErrors = []int{http.StatusInternalServerError, http.StatusServiceUnavailable}
)

func errorsOf(statuses ...int) []int {
	all := append(append([]int{}, bodyErrors...), statuses...)
	return append(all, serverErrors...)
}

func (r *router) registerAccountFlows() {
	signUp := operation("auth-register", http.MethodPost, "/api/v1/auth/register")
	signUp.DefaultStatus = http.StatusAccepted
	signUp.Middlewares = append(signUp.Middlewares, r.recordClientAddress)
	signUp.Errors = errorsOf(http.StatusTooManyRequests)
	register(r, signUp, AudiencePublic, r.signUp)

	verify := operation("auth-verify-email", http.MethodPost, "/api/v1/auth/verify-email")
	verify.Middlewares = append(verify.Middlewares, r.requireOrigin)
	verify.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden)
	register(r, verify, AudienceOptionalUser, r.verifyEmail)

	resend := operation("auth-verify-email-resend", http.MethodPost, "/api/v1/auth/verify-email/resend")
	resend.DefaultStatus = http.StatusAccepted
	resend.Middlewares = append(resend.Middlewares, r.recordClientAddress)
	resend.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests)
	register(r, resend, AudienceOptionalUser, r.resend)

	forgot := operation("auth-password-forgot", http.MethodPost, "/api/v1/auth/password/forgot")
	forgot.DefaultStatus = http.StatusAccepted
	forgot.Middlewares = append(forgot.Middlewares, r.recordClientAddress)
	forgot.Errors = errorsOf(http.StatusTooManyRequests)
	register(r, forgot, AudiencePublic, r.forgot)

	reset := operation("auth-password-reset", http.MethodPost, "/api/v1/auth/password/reset")
	reset.Errors = errorsOf()
	register(r, reset, AudiencePublic, r.reset)

	change := operation("auth-password-change", http.MethodPost, "/api/v1/auth/password/change")
	change.DefaultStatus = http.StatusNoContent
	change.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests)
	register(r, change, AudienceUnverifiedOK, r.change)
}

func (r *router) signUp(ctx context.Context, input *registerInput) (*acceptedOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	err := r.auth.Accounts.Register(ctx, accounts.Registration{
		Role: input.Body.Role, Name: input.Body.Name, Email: input.Body.Email, Password: input.Body.Password,
		Language: input.Body.Language, Timezone: input.Body.Timezone, Consent: input.Body.Consent, TutorCode: input.Body.TutorCode,
	}, clientAddress(ctx))
	if err != nil {
		return nil, r.flowError(ctx, "auth.register.failed", err)
	}
	return &acceptedOutput{CacheControl: noStore}, nil
}

func (r *router) verifyEmail(ctx context.Context, input *verifyEmailInput) (*verifyEmailOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	var caller *accounts.Principal
	if principal, found := optionalPrincipal(ctx); found {
		caller = &principal
	}
	confirmation, err := r.auth.Accounts.ConfirmEmail(ctx, caller, input.Body.Token, input.Body.NewPassword)
	if err != nil {
		return nil, r.flowError(ctx, "auth.verify_email.failed", err)
	}
	output := &verifyEmailOutput{CacheControl: noStore, Body: schemas.EmailConfirmed{Outcome: confirmation.Outcome}}
	if confirmation.SignIn != nil {
		output.SetCookie = refreshCookie(confirmation.SignIn.RefreshCookie, confirmation.SignIn.RefreshLifetime)
		output.Body.AccessToken = confirmation.SignIn.AccessToken
		output.Body.ExpiresIn = int64(confirmation.SignIn.ExpiresIn.Seconds())
	}
	return output, nil
}

func (r *router) resend(ctx context.Context, input *resendInput) (*acceptedOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	var err error
	if principal, found := optionalPrincipal(ctx); found {
		err = r.auth.Accounts.ResendConfirmationFor(ctx, principal, clientAddress(ctx))
	} else if input.Body.Email == nil {
		err = &accounts.ValidationError{Fields: []accounts.FieldProblem{{Path: "body.email", Code: domain.FieldRequired}}}
	} else {
		err = r.auth.Accounts.ResendConfirmationTo(ctx, *input.Body.Email, clientAddress(ctx))
	}
	if err != nil {
		return nil, r.flowError(ctx, "auth.resend.failed", err)
	}
	return &acceptedOutput{CacheControl: noStore}, nil
}

func (r *router) forgot(ctx context.Context, input *forgotInput) (*acceptedOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	if err := r.auth.Accounts.ForgotPassword(ctx, input.Body.Email, clientAddress(ctx)); err != nil {
		return nil, r.flowError(ctx, "auth.password.forgot_failed", err)
	}
	return &acceptedOutput{CacheControl: noStore}, nil
}

func (r *router) reset(ctx context.Context, input *resetInput) (*resetOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	outcome, err := r.auth.Accounts.ResetPassword(ctx, input.Body.Token, input.Body.NewPassword)
	if err != nil {
		return nil, r.flowError(ctx, "auth.password.reset_failed", err)
	}
	return &resetOutput{CacheControl: noStore, Body: schemas.PasswordReset{Outcome: outcome}}, nil
}

func (r *router) change(ctx context.Context, input *changeInput) (*acceptedOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	err := r.auth.Accounts.ChangePassword(ctx, principalFrom(ctx), input.Body.CurrentPassword, input.Body.NewPassword)
	if err != nil {
		return nil, r.flowError(ctx, "auth.password.change_failed", err)
	}
	return &acceptedOutput{CacheControl: noStore}, nil
}

func requireJSON(ctx context.Context, contentType string) error {
	if mediaType, _, err := mime.ParseMediaType(contentType); err != nil || mediaType != "application/json" {
		return NewError(ctx, domain.ErrorUnsupportedMediaType)
	}
	return nil
}

func clientAddress(ctx context.Context) netip.Addr {
	address, found := ctx.Value(clientAddressKey{}).(netip.Addr)
	if !found {
		panic("a throttled operation ran without a client address")
	}
	return address
}

func (r *router) flowError(ctx context.Context, event string, err error) error {
	var invalid *accounts.ValidationError
	var throttled *accounts.ThrottledError
	switch {
	case errors.As(err, &invalid):
		fields := make([]schemas.FieldError, 0, len(invalid.Fields))
		for _, field := range invalid.Fields {
			fields = append(fields, schemas.FieldError{Path: field.Path, Code: string(field.Code)})
		}
		return &Error{status: http.StatusUnprocessableEntity, ErrorEnvelope: schemas.NewErrorEnvelope(domain.ErrorValidationFailed, core.RequestID(ctx), fields)}
	case errors.Is(err, domain.ErrInvalidAvatar):
		return &Error{status: http.StatusUnprocessableEntity, ErrorEnvelope: schemas.NewErrorEnvelope(domain.ErrorValidationFailed, core.RequestID(ctx),
			[]schemas.FieldError{{Path: "body.avatar_id", Code: string(domain.FieldInvalid)}})}
	case errors.Is(err, accounts.ErrTokenInvalid):
		return NewError(ctx, domain.ErrorTokenInvalid)
	case errors.Is(err, accounts.ErrInvitationInvalid):
		return NewError(ctx, domain.ErrorInvitationInvalid)
	case errors.Is(err, accounts.ErrInvitationEmailMismatch):
		return NewError(ctx, domain.ErrorInvitationEmailMismatch)
	case errors.Is(err, accounts.ErrTutorCodeInvalid):
		return NewError(ctx, domain.ErrorTutorCodeInvalid)
	case errors.Is(err, accounts.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	case errors.As(err, &throttled):
		seconds := int64(math.Ceil(throttled.RetryAfter.Seconds()))
		return NewError(ctx, domain.ErrorRateLimited).withHeader("Retry-After", strconv.FormatInt(max(seconds, 1), 10))
	case errors.Is(err, accounts.ErrPurgeIncomplete):
		return NewError(ctx, domain.ErrorServiceUnavailable).withHeader("Retry-After", "1")
	case errors.Is(err, core.ErrHashingBusy), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return NewError(ctx, domain.ErrorServiceUnavailable)
	}
	return r.internal(ctx, event, err)
}
