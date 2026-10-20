package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

const (
	requestIDHeader = "X-Request-ID"
	requestDeadline = 30 * time.Second
	unmatchedRoute  = "unmatched"
	healthPrefix    = "GET /api/v1/health/"
)

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type outermost struct {
	next         http.Handler
	logger       *slog.Logger
	now          func() time.Time
	newRequestID func() string
}

func (m *outermost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := m.now()
	requestID := m.requestID(r)
	ctx := core.WithRequestID(r.Context(), requestID)
	r = r.WithContext(ctx)
	setBaseHeaders(w.Header(), requestID)
	recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK, ctx: ctx}

	defer func() {
		recovered := recover()
		if recovered == nil {
			m.logAccess(ctx, r, recorder, start)
			return
		}
		if recoveredErr, ok := recovered.(error); ok && errors.Is(recoveredErr, http.ErrAbortHandler) {
			panic(http.ErrAbortHandler)
		}
		core.LogPanic(ctx, m.logger, "http", fmt.Sprintf("%T", recovered))
		if recorder.wroteHeader {
			m.logAccess(ctx, r, recorder, start)
			panic(http.ErrAbortHandler)
		}
		headers := w.Header()
		for name := range headers {
			headers.Del(name)
		}
		setBaseHeaders(headers, requestID)
		writeError(ctx, recorder, domain.ErrorInternal)
		m.logAccess(ctx, r, recorder, start)
	}()

	controller := http.NewResponseController(recorder)
	if err := controller.SetReadDeadline(start.Add(requestDeadline)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		m.logger.LogAttrs(ctx, slog.LevelWarn, "http.deadline.failed", slog.String("kind", "read"))
	}
	if err := controller.SetWriteDeadline(start.Add(requestDeadline)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		m.logger.LogAttrs(ctx, slog.LevelWarn, "http.deadline.failed", slog.String("kind", "write"))
	}
	m.next.ServeHTTP(recorder, r)
}

func setBaseHeaders(headers http.Header, requestID string) {
	headers.Set(requestIDHeader, requestID)
	headers.Set("Referrer-Policy", "no-referrer")
	headers.Set("X-Content-Type-Options", "nosniff")
}

func (m *outermost) requestID(r *http.Request) string {
	values := r.Header.Values(requestIDHeader)
	if len(values) == 1 && validRequestID.MatchString(values[0]) {
		return values[0]
	}
	return m.newRequestID()
}

func (m *outermost) logAccess(ctx context.Context, r *http.Request, recorder *responseRecorder, start time.Time) {
	route := r.Pattern
	if route == "" {
		route = unmatchedRoute
	}
	level := slog.LevelInfo
	if strings.HasPrefix(route, healthPrefix) {
		level = slog.LevelDebug
	}
	m.logger.LogAttrs(ctx, level, "http.request.completed",
		slog.String("method", r.Method),
		slog.String("route", route),
		slog.Int("status", recorder.status),
		slog.Int64("duration_ms", m.now().Sub(start).Milliseconds()),
	)
}

type responseRecorder struct {
	http.ResponseWriter
	ctx         context.Context
	status      int
	wroteHeader bool
	intercepted bool
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	if (status == http.StatusNotFound || status == http.StatusMethodNotAllowed) && strings.HasPrefix(r.Header().Get("Content-Type"), "text/plain") {
		r.intercepted = true
		r.wroteHeader = true
		r.status = status
		writeError(r.ctx, r.ResponseWriter, domain.ErrorCodeForStatus(status))
		return
	}
	r.wroteHeader = true
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(body []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	if r.intercepted {
		return len(body), nil
	}
	return r.ResponseWriter.Write(body)
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
