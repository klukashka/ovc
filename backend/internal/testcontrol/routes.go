package testcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/worker"
)

const (
	maxBodyBytes      = 4 * 1024
	maxAdvanceSeconds = 10 * 365 * 24 * 60 * 60
)

var errTrailingData = errors.New("trailing data after the JSON body")

type Routes struct {
	logger *slog.Logger
	clock  *clock.Shared
	worker *worker.Worker
}

func New(logger *slog.Logger, shared *clock.Shared, w *worker.Worker) *Routes {
	return &Routes{logger: logger, clock: shared, worker: w}
}

func (routes *Routes) Mount(mux *http.ServeMux) {
	mux.Handle("POST /__test__/clock", routes.guard(routes.setClock))
	mux.Handle("POST /__test__/tick", routes.guard(routes.tick))
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
	if !decode(w, r, &request) {
		return
	}
	ctx := r.Context()
	switch {
	case request.Set != nil && request.AdvanceSeconds == nil:
		if err := routes.clock.Set(ctx, *request.Set); err != nil {
			routes.fail(ctx, w, err)
			return
		}
		writeJSON(w, clockResponse{Now: request.Set.UTC()})
	case request.Set == nil && request.AdvanceSeconds != nil && (*request.AdvanceSeconds < 1 || *request.AdvanceSeconds > maxAdvanceSeconds):
		writeError(ctx, w, domain.ErrorValidationFailed, []schemas.FieldError{{Path: "advance_seconds", Code: "out_of_range"}})
	case request.Set == nil && request.AdvanceSeconds != nil:
		now, err := routes.clock.Advance(ctx, time.Duration(*request.AdvanceSeconds)*time.Second)
		if err != nil {
			routes.fail(ctx, w, err)
			return
		}
		writeJSON(w, clockResponse{Now: now})
	default:
		writeError(ctx, w, domain.ErrorValidationFailed, []schemas.FieldError{{Path: "body", Code: "set_or_advance_seconds"}})
	}
}

func (routes *Routes) tick(w http.ResponseWriter, r *http.Request) {
	var request struct{}
	if !decode(w, r, &request) {
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
	writeJSON(w, tickResponse{Ticked: now})
}

func (routes *Routes) fail(ctx context.Context, w http.ResponseWriter, err error) {
	if errors.Is(err, clock.ErrTestClockNotSet) {
		writeError(ctx, w, domain.ErrorValidationFailed, []schemas.FieldError{{Path: "clock", Code: "not_set"}})
		return
	}
	routes.logger.LogAttrs(ctx, slog.LevelError, "testcontrol.failed", slog.String("kind", "internal"))
	writeError(ctx, w, domain.ErrorInternal, nil)
}

func (routes *Routes) guard(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if !isLoopbackHost(r.Host) {
			writeError(ctx, w, domain.ErrorNotFound, nil)
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeError(ctx, w, domain.ErrorUnsupportedMediaType, nil)
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

func decode[T clockRequest | struct{}](w http.ResponseWriter, r *http.Request, target *T) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil && decoder.More() {
		err = errTrailingData
	}
	if err == nil {
		return true
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(r.Context(), w, domain.ErrorPayloadTooLarge, nil)
		return false
	}
	writeError(r.Context(), w, domain.ErrorBadRequest, nil)
	return false
}

func writeJSON[T clockResponse | tickResponse](w http.ResponseWriter, body T) {
	w.Header().Set("Content-Type", "application/json")
	encoded, err := json.Marshal(body)
	if err != nil {
		return
	}
	if _, err := w.Write(encoded); err != nil {
		return
	}
}

func writeError(ctx context.Context, w http.ResponseWriter, code domain.ErrorCode, fields []schemas.FieldError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code.Status())
	encoded, err := json.Marshal(schemas.NewErrorEnvelope(code, core.RequestID(ctx), fields))
	if err != nil {
		return
	}
	if _, err := w.Write(encoded); err != nil {
		return
	}
}
