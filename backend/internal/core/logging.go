package core

import (
	"context"
	"io"
	"log"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const (
	maxRecordBytes  = 16 * 1024
	maxValueBytes   = 4 * 1024
	truncatedMarker = "...[truncated]"
	queueCapacity   = 4096
)

type contextKey int

const (
	requestIDKey contextKey = iota
)

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestID(ctx context.Context) string {
	value, ok := ctx.Value(requestIDKey).(string)
	if !ok {
		return ""
	}
	return value
}

type Logging struct {
	Logger *slog.Logger
	async  *asyncState
	sink   slog.Handler
}

func NewLogging(out io.Writer, level slog.Level, format LogFormat) *Logging {
	writer := &boundedWriter{out: out, limit: maxRecordBytes}
	options := &slog.HandlerOptions{Level: level, ReplaceAttr: truncateLongValues}
	var sink slog.Handler
	if format == LogFormatConsole {
		sink = slog.NewTextHandler(writer, options)
	} else {
		sink = slog.NewJSONHandler(writer, options)
	}
	state := &asyncState{queue: make(chan queuedRecord, queueCapacity), done: make(chan struct{})}
	go state.drain()
	handler := &contextHandler{next: &asyncHandler{inner: sink, state: state}}
	return &Logging{Logger: slog.New(handler), async: state, sink: sink}
}

func (l *Logging) Close() {
	dropped := l.async.close()
	if dropped == 0 {
		return
	}
	record := slog.NewRecord(time.Time{}, slog.LevelWarn, "logging.records_dropped", 0)
	record.AddAttrs(slog.Int64("dropped_records", dropped))
	if err := l.sink.Handle(context.Background(), record); err != nil {
		return
	}
}

func (l *Logging) ServerErrorLog(event string) *log.Logger {
	return log.New(&eventWriter{logger: l.Logger, event: event}, "", 0)
}

func truncateLongValues(_ []string, attr slog.Attr) slog.Attr {
	if attr.Value.Kind() != slog.KindString {
		return attr
	}
	text := attr.Value.String()
	if len(text) <= maxValueBytes {
		return attr
	}
	cut := maxValueBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return slog.String(attr.Key, text[:cut]+truncatedMarker)
}

type eventWriter struct {
	logger *slog.Logger
	event  string
}

func (w *eventWriter) Write(p []byte) (int, error) {
	w.logger.Error(w.event, slog.String("kind", serverErrorKind(string(p))))
	return len(p), nil
}

func serverErrorKind(message string) string {
	switch {
	case strings.Contains(message, "TLS handshake"):
		return "tls_handshake"
	case strings.Contains(message, "Accept error"):
		return "accept"
	case strings.Contains(message, "panic"):
		return "panic"
	}
	return "server"
}

type contextHandler struct {
	next slog.Handler
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if requestID := RequestID(ctx); requestID != "" {
		record.AddAttrs(slog.String("request_id", requestID))
	}
	return h.next.Handle(ctx, record)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{next: h.next.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{next: h.next.WithGroup(name)}
}

type queuedRecord struct {
	handler slog.Handler
	record  slog.Record
}

type asyncState struct {
	mu      sync.RWMutex
	closed  bool
	queue   chan queuedRecord
	done    chan struct{}
	dropped atomic.Int64
}

func (s *asyncState) drain() {
	defer close(s.done)
	for item := range s.queue {
		if err := item.handler.Handle(context.Background(), item.record); err != nil {
			s.dropped.Add(1)
		}
	}
}

func (s *asyncState) enqueue(item queuedRecord) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return false
	}
	select {
	case s.queue <- item:
		return true
	default:
		return false
	}
}

func (s *asyncState) close() int64 {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.queue)
	}
	s.mu.Unlock()
	<-s.done
	return s.dropped.Swap(0)
}

type asyncHandler struct {
	inner slog.Handler
	state *asyncState
}

func (h *asyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *asyncHandler) Handle(_ context.Context, record slog.Record) error {
	item := queuedRecord{handler: h.inner, record: record.Clone()}
	previouslyDropped := h.state.dropped.Swap(0)
	if previouslyDropped > 0 {
		item.record.AddAttrs(slog.Int64("dropped_records", previouslyDropped))
	}
	if !h.state.enqueue(item) {
		h.state.dropped.Add(previouslyDropped + 1)
	}
	return nil
}

func (h *asyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &asyncHandler{inner: h.inner.WithAttrs(attrs), state: h.state}
}

func (h *asyncHandler) WithGroup(name string) slog.Handler {
	return &asyncHandler{inner: h.inner.WithGroup(name), state: h.state}
}

type boundedWriter struct {
	mu    sync.Mutex
	out   io.Writer
	limit int
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(p) <= w.limit {
		return w.out.Write(p)
	}
	keep := w.limit - len(truncatedMarker) - 1
	bounded := make([]byte, 0, w.limit)
	bounded = append(bounded, p[:keep]...)
	bounded = append(bounded, truncatedMarker...)
	bounded = append(bounded, '\n')
	if _, err := w.out.Write(bounded); err != nil {
		return 0, err
	}
	return len(p), nil
}
