package testcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/worker"
)

const (
	maxBodyBytes      = 4 * 1024
	maxAdvanceSeconds = 10 * 365 * 24 * 60 * 60
	maxRecipientBytes = 254
)

var errTrailingData = errors.New("trailing data after the JSON body")

type Routes struct {
	logger   *slog.Logger
	clock    *clock.Shared
	worker   *worker.Worker
	sink     *mail.Sink
	realtime *realtime.Gateway
	seeder   *accounts.Seeder
}

type Dependencies struct {
	Logger   *slog.Logger
	Clock    *clock.Shared
	Worker   *worker.Worker
	Sink     *mail.Sink
	Realtime *realtime.Gateway
	Seeder   *accounts.Seeder
}

func New(deps Dependencies) *Routes {
	return &Routes{logger: deps.Logger, clock: deps.Clock, worker: deps.Worker, sink: deps.Sink, realtime: deps.Realtime, seeder: deps.Seeder}
}

func (routes *Routes) Mount(mux *http.ServeMux) {
	mux.Handle("POST /__test__/mail", routes.guard(routes.mail))
	if routes.seeder != nil {
		mux.Handle("POST /__test__/accounts", routes.guard(routes.seedAccount))
		mux.Handle("POST /__test__/session", routes.guard(routes.seedSession))
	}
	if routes.realtime != nil {
		mux.Handle("POST /__test__/presence", routes.guard(routes.presence))
		mux.Handle("POST /__test__/realtime/close", routes.guard(routes.closeSockets))
	}
	if routes.clock == nil {
		return
	}
	mux.Handle("POST /__test__/clock", routes.guard(routes.setClock))
	mux.Handle("POST /__test__/tick", routes.guard(routes.tick))
}

type mailRequest struct {
	To string `json:"to"`
}

type mailMessage struct {
	Subject   string            `json:"subject"`
	Text      string            `json:"text"`
	HTML      string            `json:"html"`
	Links     []string          `json:"links"`
	SentAt    time.Time         `json:"sent_at"`
	MessageID string            `json:"message_id"`
	Headers   map[string]string `json:"headers"`
}

type mailResponse []mailMessage

func (routes *Routes) mail(w http.ResponseWriter, r *http.Request) {
	var request mailRequest
	if !decode(routes.logger, w, r, &request) {
		return
	}
	ctx := r.Context()
	if request.To == "" || len(request.To) > maxRecipientBytes {
		writeError(ctx, routes.logger, w, domain.ErrorValidationFailed, []schemas.FieldError{{Path: "to", Code: "required"}})
		return
	}
	stored, err := routes.sink.Messages(ctx, request.To)
	if err != nil {
		routes.fail(ctx, w, err)
		return
	}
	response := mailResponse{}
	for _, message := range stored {
		response = append(response, mailMessage{
			Subject: message.Subject, Text: message.Text, HTML: message.HTML, Links: message.Links, SentAt: message.SentAt,
			MessageID: message.MessageID, Headers: message.Headers,
		})
	}
	writeJSON(ctx, routes.logger, w, response)
}

type presenceRequest struct {
	User string `json:"user"`
}

type presenceResponse struct {
	Online  bool `json:"online"`
	Sockets int  `json:"sockets"`
}

type closeRequest struct {
	User string           `json:"user"`
	Code domain.CloseCode `json:"code"`
}

type closeResponse struct {
	Closed int `json:"closed"`
}

func (routes *Routes) presence(w http.ResponseWriter, r *http.Request) {
	var request presenceRequest
	if !decode(routes.logger, w, r, &request) {
		return
	}
	ctx := r.Context()
	user, ok := domain.ParseUUID[domain.UserID](request.User)
	if !ok {
		writeError(ctx, routes.logger, w, domain.ErrorValidationFailed, []schemas.FieldError{{Path: "user", Code: string(domain.FieldInvalid)}})
		return
	}
	presence := routes.realtime.Presence(user)
	if presence.Pending {
		writeError(ctx, routes.logger, w, domain.ErrorServiceUnavailable, nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(ctx, routes.logger, w, presenceResponse{Online: presence.Online, Sockets: presence.Sockets})
}

func (routes *Routes) closeSockets(w http.ResponseWriter, r *http.Request) {
	var request closeRequest
	if !decode(routes.logger, w, r, &request) {
		return
	}
	ctx := r.Context()
	user, ok := domain.ParseUUID[domain.UserID](request.User)
	var problems []schemas.FieldError
	if !ok {
		problems = append(problems, schemas.FieldError{Path: "user", Code: string(domain.FieldInvalid)})
	}
	if request.Code != domain.CloseTooMany && request.Code != domain.CloseReconnect {
		problems = append(problems, schemas.FieldError{Path: "code", Code: string(domain.FieldInvalid)})
	}
	if len(problems) > 0 {
		writeError(ctx, routes.logger, w, domain.ErrorValidationFailed, problems)
		return
	}
	closed, err := routes.realtime.CloseAccount(user, request.Code)
	if err != nil {
		routes.fail(ctx, w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(ctx, routes.logger, w, closeResponse{Closed: closed})
}

type seedAccountRequest struct {
	Role     domain.Role     `json:"role"`
	Name     string          `json:"name"`
	Email    string          `json:"email"`
	Language domain.Language `json:"language"`
	Timezone string          `json:"timezone"`
}

type seedAccountResponse struct {
	User        string `json:"user"`
	Password    string `json:"password"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type seedSessionRequest struct {
	Email string `json:"email"`
}

func (routes *Routes) seedAccount(w http.ResponseWriter, r *http.Request) {
	var request seedAccountRequest
	if !decode(routes.logger, w, r, &request) {
		return
	}
	ctx := r.Context()
	seeded, err := routes.seeder.Create(ctx, accounts.SeedAccount{
		Role: request.Role, Name: request.Name, Email: request.Email, Language: request.Language, Timezone: request.Timezone,
	})
	if err != nil {
		routes.seedFailed(ctx, w, err)
		return
	}
	writeSession(w, seeded.SignIn)
	writeJSON(ctx, routes.logger, w, seedAccountResponse{
		User: seeded.User.String(), Password: routes.seeder.Password(),
		AccessToken: seeded.SignIn.AccessToken, ExpiresIn: int64(seeded.SignIn.ExpiresIn.Seconds()),
	})
}

func (routes *Routes) seedSession(w http.ResponseWriter, r *http.Request) {
	var request seedSessionRequest
	if !decode(routes.logger, w, r, &request) {
		return
	}
	ctx := r.Context()
	if request.Email == "" || len(request.Email) > maxRecipientBytes {
		writeError(ctx, routes.logger, w, domain.ErrorValidationFailed, []schemas.FieldError{{Path: "email", Code: string(domain.FieldRequired)}})
		return
	}
	signIn, err := routes.seeder.SignIn(ctx, request.Email)
	if err != nil {
		routes.seedFailed(ctx, w, err)
		return
	}
	writeSession(w, signIn)
	writeJSON(ctx, routes.logger, w, schemas.AccessToken{AccessToken: signIn.AccessToken, ExpiresIn: int64(signIn.ExpiresIn.Seconds())})
}

func writeSession(w http.ResponseWriter, signIn accounts.SignIn) {
	w.Header().Set("Set-Cookie", schemas.RefreshCookie(signIn.RefreshCookie, signIn.RefreshLifetime))
	w.Header().Set("Cache-Control", "no-store")
}

func (routes *Routes) seedFailed(ctx context.Context, w http.ResponseWriter, err error) {
	var invalid *accounts.ValidationError
	switch {
	case errors.As(err, &invalid):
		fields := make([]schemas.FieldError, 0, len(invalid.Fields))
		for _, field := range invalid.Fields {
			fields = append(fields, schemas.FieldError{Path: field.Path, Code: string(field.Code)})
		}
		writeError(ctx, routes.logger, w, domain.ErrorValidationFailed, fields)
	case errors.Is(err, accounts.ErrSeedEmailTaken):
		writeError(ctx, routes.logger, w, domain.ErrorConflict, nil)
	case errors.Is(err, accounts.ErrNotSeeded):
		writeError(ctx, routes.logger, w, domain.ErrorNotFound, nil)
	case errors.Is(err, accounts.ErrAccountBlocked):
		writeError(ctx, routes.logger, w, domain.ErrorAccountBlocked, nil)
	default:
		routes.fail(ctx, w, err)
	}
}

type clockRequest struct {
	Set            *time.Time `json:"set"`
	AdvanceSeconds *int64     `json:"advance_seconds"`
}

type clockResponse struct {
	Now time.Time `json:"now"`
}

type tickResponse struct {
	Ticked time.Time `json:"ticked"`
}

func (routes *Routes) setClock(w http.ResponseWriter, r *http.Request) {
	var request clockRequest
	if !decode(routes.logger, w, r, &request) {
		return
	}
	ctx := r.Context()
	switch {
	case request.Set != nil && request.AdvanceSeconds == nil:
		stored, err := routes.clock.Set(ctx, *request.Set)
		if err != nil {
			routes.fail(ctx, w, err)
			return
		}
		if routes.realtime != nil {
			if err := routes.realtime.ExtendPresence(ctx); err != nil {
				routes.fail(ctx, w, err)
				return
			}
		}
		writeJSON(ctx, routes.logger, w, clockResponse{Now: stored})
	case request.Set == nil && request.AdvanceSeconds != nil && (*request.AdvanceSeconds < 1 || *request.AdvanceSeconds > maxAdvanceSeconds):
		writeError(ctx, routes.logger, w, domain.ErrorValidationFailed, []schemas.FieldError{{Path: "advance_seconds", Code: "out_of_range"}})
	case request.Set == nil && request.AdvanceSeconds != nil:
		now, err := routes.clock.Advance(ctx, time.Duration(*request.AdvanceSeconds)*time.Second)
		if err != nil {
			routes.fail(ctx, w, err)
			return
		}
		if routes.realtime != nil {
			if err := routes.realtime.ExtendPresence(ctx); err != nil {
				routes.fail(ctx, w, err)
				return
			}
		}
		writeJSON(ctx, routes.logger, w, clockResponse{Now: now})
	default:
		writeError(ctx, routes.logger, w, domain.ErrorValidationFailed, []schemas.FieldError{{Path: "body", Code: "set_or_advance_seconds"}})
	}
}

func (routes *Routes) tick(w http.ResponseWriter, r *http.Request) {
	var request struct{}
	if !decode(routes.logger, w, r, &request) {
		return
	}
	ctx := r.Context()
	now, err := routes.clock.Instant(ctx)
	if err != nil {
		routes.fail(ctx, w, err)
		return
	}
	if err := routes.worker.Tick(ctx, now); err != nil {
		routes.fail(ctx, w, err)
		return
	}
	writeJSON(r.Context(), routes.logger, w, tickResponse{Ticked: now})
}

func (routes *Routes) fail(ctx context.Context, w http.ResponseWriter, err error) {
	if errors.Is(err, clock.ErrTestClockNotSet) {
		writeError(ctx, routes.logger, w, domain.ErrorValidationFailed, []schemas.FieldError{{Path: "clock", Code: "not_set"}})
		return
	}
	routes.logger.LogAttrs(ctx, slog.LevelError, "testcontrol.failed", slog.String("kind", "internal"))
	writeError(ctx, routes.logger, w, domain.ErrorInternal, nil)
}

func (routes *Routes) guard(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if !isLoopbackHost(r.Host) {
			writeError(ctx, routes.logger, w, domain.ErrorNotFound, nil)
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeError(ctx, routes.logger, w, domain.ErrorUnsupportedMediaType, nil)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next(w, r)
	})
}

var loopbackHosts = map[string]bool{"127.0.0.1": true, "localhost": true, "::1": true}

func isLoopbackHost(hostHeader string) bool {
	host, _, err := net.SplitHostPort(hostHeader)
	if err != nil {
		host = strings.TrimSuffix(strings.TrimPrefix(hostHeader, "["), "]")
	}
	return loopbackHosts[host]
}

func decode[T clockRequest | mailRequest | presenceRequest | closeRequest | seedAccountRequest | seedSessionRequest | struct{}](logger *slog.Logger, w http.ResponseWriter, r *http.Request, target *T) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil {
		err = trailingDataError(decoder)
	}
	if errors.Is(err, errTrailingData) {
		if _, drainErr := io.Copy(io.Discard, r.Body); drainErr != nil {
			err = drainErr
		}
	}
	if err == nil {
		return true
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(r.Context(), logger, w, domain.ErrorPayloadTooLarge, nil)
		return false
	}
	writeError(r.Context(), logger, w, domain.ErrorBadRequest, nil)
	return false
}

func trailingDataError(decoder *json.Decoder) error {
	_, err := decoder.Token()
	if errors.Is(err, io.EOF) {
		return nil
	}
	var syntaxError *json.SyntaxError
	if err == nil || errors.As(err, &syntaxError) {
		return errTrailingData
	}
	return err
}

func writeJSON[T clockResponse | tickResponse | mailResponse | presenceResponse | closeResponse | seedAccountResponse | schemas.AccessToken](ctx context.Context, logger *slog.Logger, w http.ResponseWriter, body T) {
	encoded, err := json.Marshal(body)
	if err != nil {
		panic("a test control response could not be encoded")
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(encoded); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "http.response.write_failed", slog.String("kind", core.ErrorKind(err)))
	}
}

func writeError(ctx context.Context, logger *slog.Logger, w http.ResponseWriter, code domain.ErrorCode, fields []schemas.FieldError) {
	encoded, err := json.Marshal(schemas.NewErrorEnvelope(code, core.RequestID(ctx), fields))
	if err != nil {
		panic("the error envelope could not be encoded")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code.Status())
	if _, err := w.Write(encoded); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "http.response.write_failed", slog.String("kind", core.ErrorKind(err)))
	}
}
