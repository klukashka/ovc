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
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/notifications"
	"ovc/backend/internal/worker"
)

const (
	maxBodyBytes      = 4 * 1024
	maxAdvanceSeconds = 10 * 365 * 24 * 60 * 60
	maxRecipientBytes = 254
)

var errTrailingData = errors.New("trailing data after the JSON body")

type Routes struct {
	logger      *slog.Logger
	clock       *clock.Shared
	worker      *worker.Worker
	sink        *mail.Sink
	preferences *notifications.Preferences
}

func New(logger *slog.Logger, shared *clock.Shared, w *worker.Worker, sink *mail.Sink, preferences *notifications.Preferences) *Routes {
	return &Routes{logger: logger, clock: shared, worker: w, sink: sink, preferences: preferences}
}

func (routes *Routes) Mount(mux *http.ServeMux) {
	mux.Handle("POST /__test__/mail", routes.guard(routes.mail))
	mux.Handle("POST /__test__/unsubscribe-token", routes.guard(routes.unsubscribeToken))
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
	Subject string    `json:"subject"`
	Text    string    `json:"text"`
	HTML    string    `json:"html"`
	Links   []string  `json:"links"`
	SentAt  time.Time `json:"sent_at"`
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
		response = append(response, mailMessage{Subject: message.Subject, Text: message.Text, HTML: message.HTML, Links: message.Links, SentAt: message.SentAt})
	}
	writeJSON(ctx, routes.logger, w, response)
}

type unsubscribeTokenRequest struct {
	User  string            `json:"user"`
	Group domain.EmailGroup `json:"group"`
}

type unsubscribeTokenResponse struct {
	Token string `json:"token"`
}

func (routes *Routes) unsubscribeToken(w http.ResponseWriter, r *http.Request) {
	var request unsubscribeTokenRequest
	if !decode(routes.logger, w, r, &request) {
		return
	}
	ctx := r.Context()
	user, ok := domain.ParseUUID[domain.UserID](request.User)
	var problems []schemas.FieldError
	if !ok {
		problems = append(problems, schemas.FieldError{Path: "user", Code: string(domain.FieldInvalid)})
	}
	if !request.Group.Known() {
		problems = append(problems, schemas.FieldError{Path: "group", Code: string(domain.FieldInvalid)})
	}
	if len(problems) > 0 {
		writeError(ctx, routes.logger, w, domain.ErrorValidationFailed, problems)
		return
	}
	token, err := routes.preferences.MintUnsubscribeToken(ctx, user, request.Group)
	if err != nil {
		routes.fail(ctx, w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(ctx, routes.logger, w, unsubscribeTokenResponse{Token: token})
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
		writeJSON(ctx, routes.logger, w, clockResponse{Now: stored})
	case request.Set == nil && request.AdvanceSeconds != nil && (*request.AdvanceSeconds < 1 || *request.AdvanceSeconds > maxAdvanceSeconds):
		writeError(ctx, routes.logger, w, domain.ErrorValidationFailed, []schemas.FieldError{{Path: "advance_seconds", Code: "out_of_range"}})
	case request.Set == nil && request.AdvanceSeconds != nil:
		now, err := routes.clock.Advance(ctx, time.Duration(*request.AdvanceSeconds)*time.Second)
		if err != nil {
			routes.fail(ctx, w, err)
			return
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

func decode[T clockRequest | mailRequest | unsubscribeTokenRequest | struct{}](logger *slog.Logger, w http.ResponseWriter, r *http.Request, target *T) bool {
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

func writeJSON[T clockResponse | tickResponse | mailResponse | unsubscribeTokenResponse](ctx context.Context, logger *slog.Logger, w http.ResponseWriter, body T) {
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
