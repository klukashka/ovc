package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/export"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/library"
	"ovc/backend/internal/services/meetings"
	"ovc/backend/internal/services/notifications"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

const (
	noStore = "no-store"
)

type Audience string

const (
	AudiencePublic              Audience = "public"
	AudienceUser                Audience = "user"
	AudienceUnverifiedOK        Audience = "unverified ok"
	AudienceTutor               Audience = "tutor"
	AudienceTutorUnverifiedOK   Audience = "tutor, unverified ok"
	AudienceStudent             Audience = "student"
	AudienceStudentUnverifiedOK Audience = "student, unverified ok"
	AudienceRefreshCookie       Audience = "refresh cookie"
	AudienceOptionalUser        Audience = "public, optional bearer"
	AudienceFirstFrame          Audience = "user, unverified ok, by first frame"
	AudienceProvider            Audience = "provider"
)

type audienceRule struct {
	role             domain.Role
	allowsUnverified bool
}

var authenticatedAudiences = map[Audience]audienceRule{
	AudienceUser:                {},
	AudienceUnverifiedOK:        {allowsUnverified: true},
	AudienceTutor:               {role: domain.RoleTutor},
	AudienceTutorUnverifiedOK:   {role: domain.RoleTutor, allowsUnverified: true},
	AudienceStudent:             {role: domain.RoleStudent},
	AudienceStudentUnverifiedOK: {role: domain.RoleStudent, allowsUnverified: true},
}

func (a Audience) Role() (domain.Role, bool) {
	rule, authenticated := authenticatedAudiences[a]
	return rule.role, authenticated && rule.role != ""
}

type OperationAudience struct {
	ID       string
	Method   string
	Path     string
	Audience Audience
}

type principalKey struct{}

type Auth struct {
	Accounts       *accounts.Service
	PublicOrigin   string
	TrustedProxies []netip.Prefix
}

type router struct {
	api         huma.API
	auth        Auth
	files       *files.Service
	tutoring    *tutoring.Service
	schedule    *schedule.Service
	preferences *notifications.Preferences
	inbox       *notifications.Inbox
	chat        *chat.Service
	library     *library.Service
	homework    *homework.Service
	exports     *export.Service
	meetings    *meetings.Lifecycle
	uploads     UploadTiming
	downloads   DownloadTiming
	addresses   *addressDownloads
	now         func() time.Time
	logger      *slog.Logger
	operations  []OperationAudience
}

func register[Input any, Output any](r *router, op huma.Operation, audience Audience, handler func(context.Context, *Input) (*Output, error)) {
	if rule, authenticated := authenticatedAudiences[audience]; authenticated {
		op.Middlewares = append(huma.Middlewares{r.authenticate, r.admit(rule)}, op.Middlewares...)
	}
	if audience == AudienceOptionalUser {
		op.Middlewares = append(huma.Middlewares{r.authenticateWhenPresented}, op.Middlewares...)
	}
	r.operations = append(r.operations, OperationAudience{ID: op.OperationID, Method: op.Method, Path: op.Path, Audience: audience})
	huma.Register(r.api, op, handler)
}

func (rule audienceRule) refusal(principal accounts.Principal) (domain.ErrorCode, bool) {
	if rule.role != "" && principal.Role != rule.role {
		return domain.ErrorNotFound, true
	}
	if !rule.allowsUnverified && principal.State != domain.AccountActive {
		return domain.ErrorEmailNotVerified, true
	}
	return "", false
}

func (r *router) admit(rule audienceRule) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if code, refused := rule.refusal(principalFrom(ctx.Context())); refused {
			r.writeError(ctx, code)
			return
		}
		next(ctx)
	}
}

func principalFrom(ctx context.Context) accounts.Principal {
	principal, found := ctx.Value(principalKey{}).(accounts.Principal)
	if !found {
		panic("an authenticated operation ran without a principal")
	}
	return principal
}

func optionalPrincipal(ctx context.Context) (accounts.Principal, bool) {
	principal, found := ctx.Value(principalKey{}).(accounts.Principal)
	return principal, found
}

func (r *router) authenticateWhenPresented(ctx huma.Context, next func(huma.Context)) {
	if ctx.Header("Authorization") == "" {
		next(ctx)
		return
	}
	r.authenticate(ctx, next)
}

func (r *router) authenticate(ctx huma.Context, next func(huma.Context)) {
	token, found := strings.CutPrefix(ctx.Header("Authorization"), "Bearer ")
	if !found || token == "" {
		r.writeError(ctx, domain.ErrorUnauthenticated)
		return
	}
	principal, err := r.auth.Accounts.Authenticate(ctx.Context(), token)
	if err != nil {
		r.writeError(ctx, r.authenticationCode(ctx.Context(), err))
		return
	}
	core.BindRequestActor(ctx.Context(), principal.UserID.String(), string(principal.Role))
	next(huma.WithValue(ctx, principalKey{}, principal))
}

func (r *router) authenticationCode(ctx context.Context, err error) domain.ErrorCode {
	switch {
	case errors.Is(err, accounts.ErrTokenExpired):
		return domain.ErrorTokenExpired
	case errors.Is(err, accounts.ErrAccountBlocked):
		return domain.ErrorAccountBlocked
	case errors.Is(err, accounts.ErrUnauthenticated):
		return domain.ErrorUnauthenticated
	}
	r.logger.LogAttrs(ctx, slog.LevelError, "auth.authenticate.failed", slog.String("kind", core.ErrorKind(err)))
	return domain.ErrorInternal
}

func (r *router) writeError(ctx huma.Context, code domain.ErrorCode) {
	encoded, err := json.Marshal(schemas.NewErrorEnvelope(code, core.RequestID(ctx.Context()), nil))
	if err != nil {
		panic("the error envelope could not be encoded")
	}
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetHeader("Cache-Control", noStore)
	if code.Status() == http.StatusUnauthorized {
		ctx.SetHeader("WWW-Authenticate", "Bearer")
	}
	ctx.SetStatus(code.Status())
	if _, err := ctx.BodyWriter().Write(encoded); err != nil {
		r.logger.LogAttrs(ctx.Context(), slog.LevelWarn, "http.response.write_failed", slog.String("kind", core.ErrorKind(err)))
	}
}

type loginInput struct {
	ContentType string `header:"Content-Type"`
	Body        schemas.LoginRequest
}

type tokenOutput struct {
	Status       int
	SetCookie    string `header:"Set-Cookie"`
	CacheControl string `header:"Cache-Control"`
	Body         *schemas.AccessToken
}

type refreshInput struct {
	Refresh string `cookie:"__Host-ovc_refresh"`
}

type signOutOutput struct {
	SetCookie    string `header:"Set-Cookie"`
	CacheControl string `header:"Cache-Control"`
}

type meOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.Me
}

type clientAddressKey struct{}

func (r *router) registerAuth() {
	login := operation("auth-login", http.MethodPost, "/api/v1/auth/login")
	login.Middlewares = append(login.Middlewares, r.requireOrigin, r.recordClientAddress)
	login.Errors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestEntityTooLarge,
		http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity, http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusServiceUnavailable}
	register(r, login, AudiencePublic, r.login)
	refresh := operation("auth-refresh", http.MethodPost, "/api/v1/auth/refresh")
	refresh.DefaultStatus = http.StatusOK
	refresh.Middlewares = append(refresh.Middlewares, r.requireOrigin)
	refresh.Responses = map[string]*huma.Response{"204": {Description: "No refresh cookie: signed out"}}
	refresh.Errors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError}
	register(r, refresh, AudienceRefreshCookie, r.refresh)
	logout := operation("auth-logout", http.MethodPost, "/api/v1/auth/logout")
	logout.DefaultStatus = http.StatusNoContent
	register(r, logout, AudienceUnverifiedOK, r.logout)
	logoutAll := operation("auth-logout-all", http.MethodPost, "/api/v1/auth/logout-all")
	logoutAll.DefaultStatus = http.StatusNoContent
	register(r, logoutAll, AudienceUnverifiedOK, r.logoutAll)
	register(r, operation("users-me", http.MethodGet, "/api/v1/users/me"), AudienceUnverifiedOK, r.me)
	r.registerAccountFlows()
	r.registerProfileRoutes()
	r.registerReclaimRoutes()
	r.registerPreferenceRoutes()
	r.registerNotificationRoutes()
}

func (r *router) recordClientAddress(ctx huma.Context, next func(huma.Context)) {
	var forwarded []string
	ctx.EachHeader(func(name string, value string) {
		if strings.EqualFold(name, "X-Forwarded-For") {
			forwarded = append(forwarded, value)
		}
	})
	address, known := ClientAddress(ctx.RemoteAddr(), forwarded, r.auth.TrustedProxies)
	if !known {
		r.logger.LogAttrs(ctx.Context(), slog.LevelError, "http.client_address.unknown", slog.String("kind", "peer"))
		r.writeError(ctx, domain.ErrorInternal)
		return
	}
	next(huma.WithValue(ctx, clientAddressKey{}, address))
}

func (r *router) requireOrigin(ctx huma.Context, next func(huma.Context)) {
	var origins []string
	ctx.EachHeader(func(name string, value string) {
		if strings.EqualFold(name, "Origin") {
			origins = append(origins, value)
		}
	})
	if !exactOrigin(origins, r.auth.PublicOrigin) {
		r.writeError(ctx, domain.ErrorForbidden)
		return
	}
	next(ctx)
}

func ClientAddress(remoteAddr string, forwarded []string, trusted []netip.Prefix) (netip.Addr, bool) {
	peer, err := netip.ParseAddrPort(remoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}
	current := peer.Addr().Unmap()
	if !isTrusted(current, trusted) {
		return current, true
	}
	entries := strings.Split(strings.Join(forwarded, ","), ",")
	for index := len(entries) - 1; index >= 0; index-- {
		entry := strings.TrimSpace(entries[index])
		if entry == "" {
			continue
		}
		parsed, err := netip.ParseAddr(entry)
		if err != nil {
			return peer.Addr().Unmap(), true
		}
		current = parsed.Unmap()
		if !isTrusted(current, trusted) {
			return current, true
		}
	}
	return current, true
}

func isTrusted(address netip.Addr, trusted []netip.Prefix) bool {
	for _, prefix := range trusted {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func (r *router) login(ctx context.Context, input *loginInput) (*tokenOutput, error) {
	if mediaType, _, err := mime.ParseMediaType(input.ContentType); err != nil || mediaType != "application/json" {
		return nil, NewError(ctx, domain.ErrorUnsupportedMediaType)
	}
	signIn, err := r.auth.Accounts.Login(ctx, input.Body.Email, input.Body.Password, clientAddress(ctx))
	if err != nil {
		return nil, r.loginError(ctx, err)
	}
	return tokens(signIn), nil
}

func (r *router) refresh(ctx context.Context, input *refreshInput) (*tokenOutput, error) {
	if input.Refresh == "" {
		return &tokenOutput{Status: http.StatusNoContent, CacheControl: noStore}, nil
	}
	signIn, err := r.auth.Accounts.Refresh(ctx, input.Refresh)
	switch {
	case err == nil:
		return tokens(signIn), nil
	case errors.Is(err, accounts.ErrAccountBlocked):
		return nil, NewError(ctx, domain.ErrorAccountBlocked).withHeader("Set-Cookie", clearedCookie())
	case errors.Is(err, accounts.ErrUnauthenticated):
		return nil, NewError(ctx, domain.ErrorUnauthenticated).withHeader("Set-Cookie", clearedCookie())
	}
	return nil, r.internal(ctx, "auth.refresh.failed", err)
}

func (r *router) logout(ctx context.Context, _ *struct{}) (*signOutOutput, error) {
	if err := r.auth.Accounts.Logout(ctx, principalFrom(ctx)); err != nil {
		return nil, r.internal(ctx, "auth.logout.failed", err)
	}
	return &signOutOutput{SetCookie: clearedCookie(), CacheControl: noStore}, nil
}

func (r *router) logoutAll(ctx context.Context, _ *struct{}) (*signOutOutput, error) {
	if err := r.auth.Accounts.LogoutAll(ctx, principalFrom(ctx)); err != nil {
		return nil, r.flowError(ctx, "auth.logout_all.failed", err)
	}
	return &signOutOutput{SetCookie: clearedCookie(), CacheControl: noStore}, nil
}

func (r *router) me(ctx context.Context, _ *struct{}) (*meOutput, error) {
	profile, err := r.auth.Accounts.Me(ctx, principalFrom(ctx))
	if err != nil {
		return nil, r.internal(ctx, "users.me.failed", err)
	}
	return &meOutput{CacheControl: noStore, Body: meBody(profile)}, nil
}

func (r *router) loginError(ctx context.Context, err error) error {
	var throttled *accounts.ThrottledError
	switch {
	case errors.Is(err, accounts.ErrInvalidCredentials):
		return NewError(ctx, domain.ErrorSignInRefused)
	case errors.Is(err, accounts.ErrAccountBlocked):
		return NewError(ctx, domain.ErrorAccountBlocked)
	case errors.As(err, &throttled):
		seconds := int64(math.Ceil(throttled.RetryAfter.Seconds()))
		return NewError(ctx, domain.ErrorRateLimited).withHeader("Retry-After", strconv.FormatInt(max(seconds, 1), 10))
	case errors.Is(err, core.ErrHashingBusy):
		return NewError(ctx, domain.ErrorServiceUnavailable)
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		r.logger.LogAttrs(ctx, slog.LevelDebug, "auth.login.abandoned", slog.String("kind", core.ErrorKind(err)))
		return NewError(ctx, domain.ErrorServiceUnavailable)
	}
	return r.internal(ctx, "auth.login.error", err)
}

func (r *router) internal(ctx context.Context, event string, err error) error {
	r.logger.LogAttrs(ctx, slog.LevelError, event, slog.String("kind", core.ErrorKind(err)))
	return NewError(ctx, domain.ErrorInternal)
}

func tokens(signIn accounts.SignIn) *tokenOutput {
	return &tokenOutput{
		Status:       http.StatusOK,
		SetCookie:    refreshCookie(signIn.RefreshCookie, signIn.RefreshLifetime),
		CacheControl: noStore,
		Body:         &schemas.AccessToken{AccessToken: signIn.AccessToken, ExpiresIn: int64(signIn.ExpiresIn.Seconds())},
	}
}

func refreshCookie(value string, lifetime time.Duration) string {
	return schemas.RefreshCookie(value, lifetime)
}

func clearedCookie() string {
	return schemas.ClearedRefreshCookie()
}

func tutorActorFrom(ctx context.Context) domain.TutorActor {
	principal := principalFrom(ctx)
	return domain.TutorActor{UserID: principal.UserID, SessionID: principal.SessionID}
}
