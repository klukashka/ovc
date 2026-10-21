package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	closeTimeout    = 5 * time.Second
)

var ErrLoggingCloseTimedOut = errors.New("the log sink did not drain before the close timeout")

type contextKey int

const (
	requestIDKey contextKey = iota
	requestActorKey
)

type requestActor struct {
	mu     sync.Mutex
	userID string
	role   string
}

func (a *requestActor) bound() (string, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.userID, a.role
}

func WithRequestActor(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestActorKey, &requestActor{})
}

func BindRequestActor(ctx context.Context, userID string, role string) {
	actor, ok := ctx.Value(requestActorKey).(*requestActor)
	if !ok {
		panic("a request was authenticated outside the request middleware")
	}
	actor.mu.Lock()
	defer actor.mu.Unlock()
	actor.userID = userID
	actor.role = role
}

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
	writer := &boundedWriter{out: out, limit: maxRecordBytes, format: format}
	options := &slog.HandlerOptions{Level: level, ReplaceAttr: truncateLongValues}
	var sink slog.Handler
	if format == LogFormatConsole {
		sink = slog.NewTextHandler(writer, options)
	} else {
		sink = slog.NewJSONHandler(writer, options)
	}
	state := &asyncState{queue: make(chan queuedRecord, queueCapacity), done: make(chan struct{})}
	go state.drain()
	return &Logging{Logger: slog.New(NewContextHandler(&asyncHandler{inner: sink, state: state})), async: state, sink: sink}
}

func (l *Logging) Close() error {
	dropped, drained := l.async.close(closeTimeout)
	if !drained {
		return ErrLoggingCloseTimedOut
	}
	if dropped == 0 {
		return nil
	}
	record := slog.NewRecord(time.Time{}, slog.LevelWarn, "logging.records_dropped", 0)
	record.AddAttrs(slog.Int64("dropped_records", dropped))
	if err := l.sink.Handle(context.Background(), record); err != nil {
		return fmt.Errorf("report dropped log records: %w", err)
	}
	return nil
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
	next      slog.Handler
	userBound bool
	roleBound bool
}

func NewContextHandler(next slog.Handler) slog.Handler {
	return &contextHandler{next: next}
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if requestID := RequestID(ctx); requestID != "" {
		record.AddAttrs(slog.String("request_id", requestID))
	}
	if actor, ok := ctx.Value(requestActorKey).(*requestActor); ok {
		h.addActor(&record, actor)
	}
	return h.next.Handle(ctx, record)
}

func (h *contextHandler) addActor(record *slog.Record, actor *requestActor) {
	userID, role := actor.bound()
	if userID == "" {
		return
	}
	userNamed, roleNamed := h.userBound, h.roleBound
	record.Attrs(func(attr slog.Attr) bool {
		userNamed = userNamed || attr.Key == "user_id"
		roleNamed = roleNamed || attr.Key == "role"
		return true
	})
	if !userNamed {
		record.AddAttrs(slog.String("user_id", userID))
	}
	if !roleNamed {
		record.AddAttrs(slog.String("role", role))
	}
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := &contextHandler{next: h.next.WithAttrs(attrs), userBound: h.userBound, roleBound: h.roleBound}
	for _, attr := range attrs {
		next.userBound = next.userBound || attr.Key == "user_id"
		next.roleBound = next.roleBound || attr.Key == "role"
	}
	return next
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{next: h.next.WithGroup(name), userBound: h.userBound, roleBound: h.roleBound}
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
		if !handleRecovering(item) {
			s.dropped.Add(1)
		}
	}
}

func handleRecovering(item queuedRecord) (written bool) {
	defer func() {
		if recover() != nil {
			written = false
		}
	}()
	return item.handler.Handle(context.Background(), item.record) == nil
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

func (s *asyncState) close(timeout time.Duration) (int64, bool) {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.queue)
	}
	s.mu.Unlock()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.done:
		return s.dropped.Swap(0), true
	case <-timer.C:
		return s.dropped.Load(), false
	}
}

type asyncHandler struct {
	inner slog.Handler
	state *asyncState
}

func (h *asyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *asyncHandler) Handle(_ context.Context, record slog.Record) error {
	item := queuedRecord{handler: h.inner, record: resolvedRecord(record)}
	previouslyDropped := h.state.dropped.Swap(0)
	if previouslyDropped > 0 {
		item.record.AddAttrs(slog.Int64("dropped_records", previouslyDropped))
	}
	if !h.state.enqueue(item) {
		h.state.dropped.Add(previouslyDropped + 1)
	}
	return nil
}

func resolvedRecord(record slog.Record) slog.Record {
	resolved := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		resolved.AddAttrs(resolvedAttr(attr))
		return true
	})
	return resolved
}

func resolvedAttr(attr slog.Attr) slog.Attr {
	value := attr.Value.Resolve()
	if value.Kind() != slog.KindGroup {
		return slog.Attr{Key: attr.Key, Value: value}
	}
	members := value.Group()
	resolved := make([]slog.Attr, len(members))
	for index, member := range members {
		resolved[index] = resolvedAttr(member)
	}
	return slog.Attr{Key: attr.Key, Value: slog.GroupValue(resolved...)}
}

func (h *asyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &asyncHandler{inner: h.inner.WithAttrs(attrs), state: h.state}
}

func (h *asyncHandler) WithGroup(name string) slog.Handler {
	return &asyncHandler{inner: h.inner.WithGroup(name), state: h.state}
}

type boundedWriter struct {
	mu     sync.Mutex
	out    io.Writer
	limit  int
	format LogFormat
}

type truncatedRecord struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
	Event string `json:"event,omitempty"`
	Bytes int    `json:"original_bytes"`
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(p) <= w.limit {
		return w.out.Write(p)
	}
	replacement, err := w.replacement(p)
	if err != nil {
		return 0, err
	}
	if _, err := w.out.Write(replacement); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *boundedWriter) replacement(record []byte) ([]byte, error) {
	event := oversizedEvent(record, w.format)
	if w.format == LogFormatConsole {
		line := fmt.Sprintf("level=WARN msg=logging.record_truncated event=%q original_bytes=%d\n", event, len(record))
		return []byte(line), nil
	}
	encoded, err := json.Marshal(truncatedRecord{Level: "WARN", Msg: "logging.record_truncated", Event: event, Bytes: len(record)})
	if err != nil {
		return nil, fmt.Errorf("encode the truncated-record notice: %w", err)
	}
	return append(encoded, '\n'), nil
}

func oversizedEvent(record []byte, format LogFormat) string {
	if format == LogFormatConsole {
		start := bytes.Index(record, []byte(" msg="))
		if start < 0 {
			return ""
		}
		rest := record[start+len(" msg="):]
		if end := bytes.IndexByte(rest, ' '); end >= 0 {
			rest = rest[:end]
		}
		return truncateEvent(string(rest))
	}
	decoder := json.NewDecoder(bytes.NewReader(record))
	if _, err := decoder.Token(); err != nil {
		return ""
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return ""
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return ""
		}
		if key == "msg" {
			var message string
			if err := json.Unmarshal(value, &message); err != nil {
				return ""
			}
			return truncateEvent(message)
		}
	}
	return ""
}

func truncateEvent(event string) string {
	const maxEventBytes = 128
	if len(event) <= maxEventBytes {
		return event
	}
	cut := maxEventBytes
	for cut > 0 && !utf8.RuneStart(event[cut]) {
		cut--
	}
	return event[:cut]
}
