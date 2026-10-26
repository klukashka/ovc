package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
)

type phase int

const (
	phaseAwaiting phase = iota
	phaseAuthenticating
	phaseReady
)

const (
	closeAbnormal    = 1006
	closedDuringAuth = "closed"
)

var validRef = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type socket struct {
	g           *Gateway
	conn        *websocket.Conn
	id          string
	logger      atomic.Pointer[slog.Logger]
	opened      time.Time
	reservation Reservation
	done        chan struct{}
	finished    chan struct{}
	wake        chan struct{}

	reserved   bool
	phase      phase
	closed     bool
	code       domain.CloseCode
	reason     string
	user       domain.UserID
	session    domain.SessionID
	expiresAt  time.Time
	generation listener.Generation
	suspended  bool
	queue      outbound

	framesIn    int
	framesOut   int
	lookedUp    time.Time
	windowStart time.Time
	windowCount int
	violations  []time.Time
}

func (g *Gateway) newSocket(conn *websocket.Conn, reservation Reservation) *socket {
	id := g.deps.NewID()
	s := &socket{
		g: g, conn: conn, id: id, opened: g.deps.Now(), reservation: reservation, reserved: true,
		done: make(chan struct{}), finished: make(chan struct{}), wake: make(chan struct{}, 1),
	}
	s.logger.Store(g.deps.Logger.With(slog.String("connection_id", id)))
	return s
}

func (s *socket) log() *slog.Logger {
	return s.logger.Load()
}

func (s *socket) wakeWriter() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *socket) closeWith(code domain.CloseCode, reason string) {
	s.g.mu.Lock()
	defer s.g.mu.Unlock()
	s.g.closeLocked(s, code, reason)
}

func (s *socket) isClosed() bool {
	s.g.mu.Lock()
	defer s.g.mu.Unlock()
	return s.closed
}

func (s *socket) run() {
	ctx := context.Background()
	s.log().LogAttrs(ctx, slog.LevelDebug, "ws.connection.opened")
	authTimer := time.AfterFunc(s.g.deps.Timing.AuthWithin, func() {
		core.Go(ctx, s.log(), "realtime.auth_timeout", s.closeUnauthenticated)
	})
	defer authTimer.Stop()
	writing := make(chan struct{})
	core.Go(ctx, s.log(), "realtime.writer", func(context.Context) {
		defer close(writing)
		s.write()
	})
	pinging := make(chan struct{})
	core.Go(ctx, s.log(), "realtime.pinger", func(context.Context) {
		defer close(pinging)
		s.ping()
	})
	if err := <-core.Run(ctx, s.log(), "realtime.reader", func() error {
		s.read(ctx)
		return nil
	}); err != nil {
		s.closeWith(closeAbnormal, "reader_panicked")
	}
	s.closeWith(closeAbnormal, "connection")
	<-s.finished
	<-writing
	<-pinging
	s.g.mu.Lock()
	code, reason := s.code, s.reason
	s.g.mu.Unlock()
	s.log().LogAttrs(ctx, slog.LevelInfo, "ws.connection.closed",
		slog.Int("code", int(code)), slog.String("reason", reason),
		slog.Int64("duration_ms", s.g.deps.Now().Sub(s.opened).Milliseconds()),
		slog.Int("frames_in", s.framesIn), slog.Int("frames_out", s.countOut()))
}

func (s *socket) closeUnauthenticated(ctx context.Context) {
	s.g.mu.Lock()
	defer s.g.mu.Unlock()
	if s.phase == phaseAwaiting && !s.closed {
		s.log().LogAttrs(ctx, slog.LevelInfo, "ws.auth.refused", slog.String("reason", "timeout"))
		s.g.closeLocked(s, domain.CloseUnauthenticated, "timeout")
	}
}

func (s *socket) countOut() int {
	s.g.mu.Lock()
	defer s.g.mu.Unlock()
	return s.framesOut
}

func (s *socket) read(ctx context.Context) {
	for {
		kind, data, err := s.conn.Read(ctx)
		if err != nil {
			s.readFailed(err)
			return
		}
		s.framesIn++
		if kind != websocket.MessageText {
			s.log().LogAttrs(ctx, slog.LevelInfo, "ws.frame.rejected", slog.String("reason", "binary"))
			s.closeWith(domain.CloseBinaryFrame, "binary")
			return
		}
		frameContext := core.WithRequestID(ctx, s.g.deps.NewID())
		if !s.handle(frameContext, data) {
			return
		}
	}
}

func (s *socket) readFailed(err error) {
	s.g.mu.Lock()
	defer s.g.mu.Unlock()
	if s.closed {
		return
	}
	switch {
	case errors.Is(err, websocket.ErrMessageTooBig):
		s.g.closeLocked(s, domain.CloseFrameTooLarge, "frame_too_large")
	case websocket.CloseStatus(err) != -1:
		s.g.closeLocked(s, domain.CloseCode(websocket.CloseStatus(err)), "peer")
	default:
		s.g.closeLocked(s, closeAbnormal, "connection")
	}
}

func (s *socket) handle(ctx context.Context, data []byte) bool {
	if s.isClosed() {
		return false
	}
	frame, ref, parsed := parseFrame(data)
	s.g.mu.Lock()
	current := s.phase
	s.g.mu.Unlock()
	if current == phaseAwaiting {
		return s.authenticateFirst(ctx, frame, parsed)
	}
	if !s.withinRate(ctx, ref) {
		return !s.isClosed()
	}
	if !s.lookupAllowed(ctx) {
		return false
	}
	switch {
	case !parsed:
		s.reject(ctx, "malformed", domain.ErrorBadRequest, ref)
	case frame.Type == domain.FramePing:
		s.send(encode[struct{}](domain.FramePong, ref, nil))
	case frame.Type == domain.FrameAuth:
		return s.authenticateAgain(ctx, frame)
	default:
		s.reject(ctx, "unknown_type", domain.ErrorBadRequest, ref)
	}
	return true
}

func parseFrame(data []byte) (schemas.ClientFrame, *string, bool) {
	var frame schemas.ClientFrame
	if err := decodeStrict(data, &frame); err != nil {
		return frame, nil, false
	}
	if frame.Ref != nil && !validRef.MatchString(*frame.Ref) {
		return frame, nil, false
	}
	known := slices.Contains(domain.ClientFrameTypes(), frame.Type)
	if known && frame.Type == domain.FramePing && frame.Payload != nil {
		return frame, frame.Ref, false
	}
	if frame.Type == "" {
		return frame, frame.Ref, false
	}
	return frame, frame.Ref, true
}

var errTrailingData = errors.New("trailing data after the frame")

func decodeStrict[T schemas.ClientFrame | schemas.AuthPayload](data []byte, target *T) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errTrailingData
	}
	return nil
}

func (s *socket) reject(ctx context.Context, reason string, code domain.ErrorCode, ref *string) {
	s.log().LogAttrs(ctx, slog.LevelInfo, "ws.frame.rejected", slog.String("reason", reason))
	listed := code.Listed()
	s.send(encode(domain.FrameError, nil, &schemas.FrameErrorPayload{
		Code: listed, Message: listed.Message(), RequestID: core.RequestID(ctx), Ref: ref,
	}))
}

func (s *socket) send(frame []byte) {
	s.g.mu.Lock()
	defer s.g.mu.Unlock()
	s.g.enqueueLocked(s, frame)
}

func (s *socket) withinRate(ctx context.Context, ref *string) bool {
	now := s.g.deps.Now()
	timing, limits := s.g.deps.Timing, s.g.deps.Limits
	if s.windowStart.IsZero() || now.Sub(s.windowStart) >= timing.RateWindow {
		s.windowStart, s.windowCount = now, 0
	}
	s.windowCount++
	if s.windowCount <= limits.FramesPerWindow {
		return true
	}
	recent := s.violations[:0]
	for _, at := range s.violations {
		if now.Sub(at) < timing.ViolationWindow {
			recent = append(recent, at)
		}
	}
	s.violations = append(recent, now)
	if len(s.violations) >= limits.Violations {
		s.log().LogAttrs(ctx, slog.LevelInfo, "ws.frame.rejected", slog.String("reason", "rate"))
		s.closeWith(domain.CloseTooMany, "frame_rate")
		return false
	}
	s.reject(ctx, "rate", domain.ErrorRateLimited, ref)
	return false
}

func (s *socket) lookupAllowed(ctx context.Context) bool {
	now := s.g.deps.Now()
	if !s.lookedUp.IsZero() && now.Sub(s.lookedUp) < s.g.deps.Timing.LookupEvery {
		return true
	}
	standings, err := s.g.lookup(ctx, []domain.SessionID{s.session})
	if err != nil {
		s.log().LogAttrs(ctx, slog.LevelError, "realtime.recheck.failed", slog.String("kind", core.ErrorKind(err)))
		s.closeWith(domain.CloseReconnect, "lookup_failed")
		return false
	}
	if refusal := refusalOf(standings, s); refusal != accounts.SocketAllowed {
		s.closeWith(domain.CloseUnauthenticated, string(refusal))
		return false
	}
	s.lookedUp = now
	return true
}

func (s *socket) refuseAuth(ctx context.Context, reason string, code domain.CloseCode) bool {
	s.log().LogAttrs(ctx, slog.LevelInfo, "ws.auth.refused", slog.String("reason", reason))
	s.closeWith(code, reason)
	return false
}

func (s *socket) verify(ctx context.Context, frame schemas.ClientFrame) (accounts.Access, string, error) {
	var payload schemas.AuthPayload
	if frame.Payload == nil || decodeStrict(frame.Payload, &payload) != nil || payload.AccessToken == "" {
		return accounts.Access{}, "protocol", nil
	}
	access, err := s.g.deps.Sessions.VerifyAccess(ctx, payload.AccessToken)
	switch {
	case errors.Is(err, accounts.ErrUnauthenticated) || errors.Is(err, accounts.ErrTokenExpired):
		return accounts.Access{}, "invalid_token", nil
	case err != nil:
		return accounts.Access{}, "", err
	}
	return access, "", nil
}

func (s *socket) authenticateFirst(ctx context.Context, frame schemas.ClientFrame, parsed bool) bool {
	if !parsed || frame.Type != domain.FrameAuth {
		s.log().LogAttrs(ctx, slog.LevelInfo, "ws.frame.rejected", slog.String("reason", "before_auth"))
		s.closeWith(domain.CloseUnauthenticated, "before_auth")
		return false
	}
	access, refusal, err := s.verify(ctx, frame)
	if err != nil {
		s.log().LogAttrs(ctx, slog.LevelError, "realtime.recheck.failed", slog.String("kind", core.ErrorKind(err)))
		s.closeWith(domain.CloseReconnect, "verify_failed")
		return false
	}
	if refusal != "" {
		return s.refuseAuth(ctx, refusal, domain.CloseUnauthenticated)
	}
	registered, refusal, code := s.register(access)
	if refusal == closedDuringAuth {
		return false
	}
	if refusal != "" {
		return s.refuseAuth(ctx, refusal, code)
	}
	round := registered
	for {
		standings, err := s.g.lookup(ctx, []domain.SessionID{access.SessionID})
		if err != nil {
			s.log().LogAttrs(ctx, slog.LevelError, "realtime.recheck.failed", slog.String("kind", core.ErrorKind(err)))
			s.closeWith(domain.CloseReconnect, "lookup_failed")
			return false
		}
		if refusal := refusalOf(standings, s); refusal != accounts.SocketAllowed {
			if s.rechecksSince(registered) {
				return s.refuseAuth(ctx, "revoked_during_auth", domain.CloseUnauthenticated)
			}
			return s.refuseAuth(ctx, string(refusal), domain.CloseUnauthenticated)
		}
		if s.g.deps.AuthLookupPassed != nil {
			s.g.deps.AuthLookupPassed(ctx)
		}
		next, outcome := s.becomeReady(round)
		switch outcome {
		case readyGranted:
			s.lookedUp = s.g.deps.Now()
			s.logger.Store(s.log().With(slog.String("user_id", access.UserID.String()), slog.String("role", string(standings[access.SessionID].Role))))
			s.log().LogAttrs(ctx, slog.LevelInfo, "ws.connection.authenticated")
			return true
		case readyListenerChanged:
			return s.refuseAuth(ctx, "listener_down", domain.CloseTryLater)
		case readyClosed:
			return false
		case readyRecheckArrived:
			round = next
		}
	}
}

func (s *socket) rechecksSince(round uint64) bool {
	s.g.mu.Lock()
	defer s.g.mu.Unlock()
	found, ok := s.g.accounts[s.user]
	return ok && found.rechecks != round
}

func (s *socket) register(access accounts.Access) (uint64, string, domain.CloseCode) {
	s.g.mu.Lock()
	defer s.g.mu.Unlock()
	if s.closed {
		return 0, closedDuringAuth, 0
	}
	generation, live := s.g.state()
	if !live {
		return 0, "listener_down", domain.CloseTryLater
	}
	found := s.g.accountLocked(access.UserID)
	if len(found.sockets) >= s.g.deps.Limits.PerAccount {
		s.g.forgetLocked(access.UserID, found)
		return 0, "too_many_sockets", domain.CloseTooMany
	}
	s.phase = phaseAuthenticating
	s.user, s.session, s.expiresAt, s.generation = access.UserID, access.SessionID, access.ExpiresAt, generation
	s.suspended = true
	found.sockets[s] = struct{}{}
	return found.rechecks, "", 0
}

type readyOutcome int

const (
	readyGranted readyOutcome = iota
	readyListenerChanged
	readyRecheckArrived
	readyClosed
)

func (s *socket) becomeReady(round uint64) (uint64, readyOutcome) {
	s.g.mu.Lock()
	defer s.g.mu.Unlock()
	if s.closed {
		return round, readyClosed
	}
	generation, live := s.g.state()
	if !live || generation != s.generation {
		return round, readyListenerChanged
	}
	found := s.g.accounts[s.user]
	if found.rechecks != round {
		return found.rechecks, readyRecheckArrived
	}
	s.phase = phaseReady
	s.suspended = false
	if s.reserved {
		s.reserved = false
		s.g.releaseLocked(s.reservation.bucket)
	}
	s.g.enqueueLocked(s, encode(domain.FrameReady, nil, &schemas.ReadyPayload{}))
	return round, readyGranted
}

func (s *socket) authenticateAgain(ctx context.Context, frame schemas.ClientFrame) bool {
	access, refusal, err := s.verify(ctx, frame)
	if err != nil {
		s.log().LogAttrs(ctx, slog.LevelError, "realtime.recheck.failed", slog.String("kind", core.ErrorKind(err)))
		s.closeWith(domain.CloseReconnect, "verify_failed")
		return false
	}
	if refusal != "" {
		return s.refuseAuth(ctx, refusal, domain.CloseUnauthenticated)
	}
	s.g.mu.Lock()
	if access.UserID != s.user || access.SessionID != s.session {
		s.g.mu.Unlock()
		return s.refuseAuth(ctx, "other_session", domain.CloseUnauthenticated)
	}
	if access.ExpiresAt.After(s.expiresAt) {
		s.expiresAt = access.ExpiresAt
	}
	s.g.enqueueLocked(s, encode(domain.FrameReady, nil, &schemas.ReadyPayload{}))
	s.g.mu.Unlock()
	s.log().LogAttrs(ctx, slog.LevelDebug, "ws.connection.reauthenticated")
	return true
}

func (s *socket) write() {
	for {
		select {
		case <-s.done:
			return
		case <-s.wake:
		}
		if !s.flush() {
			return
		}
	}
}

func (s *socket) flush() bool {
	for {
		s.g.mu.Lock()
		if s.closed {
			s.g.mu.Unlock()
			return false
		}
		if s.suspended {
			s.g.mu.Unlock()
			return true
		}
		frame, ok := s.queue.pop()
		s.g.mu.Unlock()
		if !ok {
			return true
		}
		ctx, cancel := context.WithTimeout(context.Background(), s.g.deps.Timing.WriteTimeout)
		err := s.conn.Write(ctx, websocket.MessageText, frame)
		cancel()
		if err != nil {
			s.writeFailed(ctx, err)
			return false
		}
		s.g.mu.Lock()
		s.framesOut++
		s.g.mu.Unlock()
	}
}

func (s *socket) writeFailed(ctx context.Context, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		s.log().LogAttrs(ctx, slog.LevelWarn, "ws.backpressure.closed", slog.String("reason", "write_timeout"))
		s.closeWith(domain.CloseTooMany, "write_timeout")
		return
	}
	s.closeWith(closeAbnormal, "write_failed")
}

func (s *socket) ping() {
	ticker := time.NewTicker(s.g.deps.Timing.PingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
		}
		ctx, cancel := context.WithTimeout(context.Background(), s.g.deps.Timing.PongWithin)
		err := s.conn.Ping(ctx)
		cancel()
		if err != nil {
			if !s.isClosed() {
				s.log().LogAttrs(ctx, slog.LevelInfo, "ws.ping.unanswered")
			}
			s.closeWith(closeAbnormal, "ping_timeout")
			return
		}
	}
}
