package realtime

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
)

type Sessions interface {
	VerifyAccess(ctx context.Context, token string) (accounts.Access, error)
	SessionStandings(ctx context.Context, sessions []domain.SessionID) (map[domain.SessionID]accounts.SessionStanding, error)
}

type ListenerState interface {
	State() (listener.Generation, bool)
}

type Clock interface {
	Instant(ctx context.Context) (time.Time, error)
}

type Chat interface {
	PersistPresence(ctx context.Context, instance domain.PresenceInstanceID, users []domain.UserID, online bool) ([]domain.UserID, error)
	AnnouncePresence(ctx context.Context, account domain.UserID, online bool) error
	Typing(ctx context.Context, typist domain.UserID, channel domain.ChannelID) error
	CheckTyping(ctx context.Context, typist domain.UserID, channel domain.ChannelID) error
}

type Timing struct {
	AuthWithin      time.Duration
	TokenGrace      time.Duration
	LapseCheckEvery time.Duration
	SweepEvery      time.Duration
	LookupTimeout   time.Duration
	LookupEvery     time.Duration
	WriteTimeout    time.Duration
	PingEvery       time.Duration
	PongWithin      time.Duration
	RateWindow      time.Duration
	ViolationWindow time.Duration
	ShutdownWait    time.Duration
	PresenceEvery   time.Duration
	PresenceTimeout time.Duration
	PresenceSettle  time.Duration
	TypingCoalesce  time.Duration
}

func DefaultTiming() Timing {
	return Timing{
		AuthWithin:      5 * time.Second,
		TokenGrace:      30 * time.Second,
		LapseCheckEvery: time.Second,
		SweepEvery:      60 * time.Second,
		LookupTimeout:   2 * time.Second,
		LookupEvery:     time.Second,
		WriteTimeout:    10 * time.Second,
		PingEvery:       25 * time.Second,
		PongWithin:      60 * time.Second,
		RateWindow:      time.Second,
		ViolationWindow: 10 * time.Second,
		ShutdownWait:    10 * time.Second,
		PresenceEvery:   time.Minute,
		PresenceTimeout: 5 * time.Second,
		PresenceSettle:  5 * time.Second,
		TypingCoalesce:  2 * time.Second,
	}
}

type Limits struct {
	Unauthenticated int
	PerAddress      int
	PerAccount      int
	FramesPerWindow int
	Violations      int
	Accounts        int
}

func DefaultLimits() Limits {
	return Limits{Unauthenticated: 500, PerAddress: 20, PerAccount: 20, FramesPerWindow: 20, Violations: 5, Accounts: 4096}
}

type Dependencies struct {
	Sessions         Sessions
	Chat             Chat
	Scheduler        Scheduler
	Clock            Clock
	Now              func() time.Time
	NewID            func() string
	Logger           *slog.Logger
	PublicOrigin     string
	Timing           Timing
	Limits           Limits
	AuthLookupPassed func(ctx context.Context)
}

var (
	ErrInvalidOrigin       = errors.New("the gateway needs the public origin as an absolute URL")
	ErrMissingDependencies = errors.New("the gateway needs its sessions, chat service, scheduler, clocks, identifier source and logger")
	ErrInvalidTiming       = errors.New("the gateway's timing and limits must be positive")
)

const maxFrameBytes = 16 * 1024

type UpgradeRefusal string

const (
	UpgradeAccepted    UpgradeRefusal = ""
	UpgradeBusy        UpgradeRefusal = "busy"
	UpgradeAddressBusy UpgradeRefusal = "address_busy"
)

type account struct {
	sockets    map[*socket]struct{}
	rechecks   uint64
	rechecking bool
	again      bool
	presence
}

type Gateway struct {
	deps          Dependencies
	originHosts   []string
	mu            sync.Mutex
	listener      ListenerState
	closing       bool
	pending       int
	byAddress     map[netip.Addr]int
	accounts      map[domain.UserID]*account
	open          map[*socket]struct{}
	served        sync.WaitGroup
	stop          context.CancelFunc
	loops         sync.WaitGroup
	rechecks      sync.WaitGroup
	presenceRun   sync.WaitGroup
	instance      domain.PresenceInstanceID
	presenceQueue chan domain.UserID
	stopPresence  context.CancelFunc
}

func New(deps Dependencies) (*Gateway, error) {
	if deps.Sessions == nil || deps.Chat == nil || deps.Scheduler == nil || deps.Clock == nil || deps.Now == nil || deps.NewID == nil ||
		deps.Logger == nil {
		return nil, ErrMissingDependencies
	}
	parsed, err := url.Parse(deps.PublicOrigin)
	if err != nil || parsed.Host == "" || parsed.Scheme == "" {
		return nil, ErrInvalidOrigin
	}
	if !positive(deps.Timing) || deps.Limits.Unauthenticated <= 0 || deps.Limits.PerAddress <= 0 || deps.Limits.PerAccount <= 0 ||
		deps.Limits.FramesPerWindow <= 0 || deps.Limits.Violations <= 0 || deps.Limits.Accounts <= 0 {
		return nil, ErrInvalidTiming
	}
	encoded, err := hex.DecodeString(deps.NewID())
	if err != nil || len(encoded) != 16 {
		return nil, ErrMissingDependencies
	}
	var instance domain.PresenceInstanceID
	copy(instance[:], encoded)
	return &Gateway{
		deps:          deps,
		instance:      instance,
		presenceQueue: make(chan domain.UserID, deps.Limits.Accounts),
		stopPresence:  func() {},
		originHosts:   []string{escapePattern(parsed.Host)},
		byAddress:     map[netip.Addr]int{},
		accounts:      map[domain.UserID]*account{},
		open:          map[*socket]struct{}{},
		stop:          func() {},
	}, nil
}

func positive(timing Timing) bool {
	for _, duration := range []time.Duration{
		timing.AuthWithin, timing.TokenGrace, timing.LapseCheckEvery, timing.SweepEvery, timing.LookupTimeout, timing.LookupEvery,
		timing.WriteTimeout, timing.PingEvery, timing.PongWithin, timing.RateWindow, timing.ViolationWindow, timing.ShutdownWait,
		timing.PresenceEvery, timing.PresenceTimeout, timing.PresenceSettle, timing.TypingCoalesce,
	} {
		if duration <= 0 {
			return false
		}
	}
	return true
}

func escapePattern(host string) string {
	var escaped strings.Builder
	for _, character := range host {
		if strings.ContainsRune(`*?[]\`, character) {
			escaped.WriteRune('\\')
		}
		escaped.WriteRune(character)
	}
	return escaped.String()
}

func (g *Gateway) Start(ctx context.Context, state ListenerState) {
	loopContext, cancel := context.WithCancel(ctx)
	g.mu.Lock()
	g.listener = state
	g.stop = cancel
	g.mu.Unlock()
	presenceContext, stopPresence := context.WithCancel(context.Background())
	g.stopPresence = stopPresence
	g.presenceRun.Add(4)
	for range 4 {
		core.Go(presenceContext, g.deps.Logger, "realtime.presence.writer", func(ctx context.Context) { defer g.presenceRun.Done(); g.presenceWorker(ctx) })
	}
	g.loops.Add(3)
	core.Go(loopContext, g.deps.Logger, "realtime.lapse", func(ctx context.Context) {
		defer g.loops.Done()
		g.every(ctx, g.deps.Timing.LapseCheckEvery, g.closeLapsed)
	})
	core.Go(loopContext, g.deps.Logger, "realtime.sweep", func(ctx context.Context) {
		defer g.loops.Done()
		g.every(ctx, g.deps.Timing.SweepEvery, g.sweep)
	})
	core.Go(loopContext, g.deps.Logger, "realtime.presence", func(ctx context.Context) {
		defer g.loops.Done()
		g.every(ctx, g.deps.Timing.PresenceEvery, g.extendOnSchedule)
	})
}

func (g *Gateway) every(ctx context.Context, interval time.Duration, run func(context.Context)) {
	ticker := g.deps.Scheduler.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.Ticks():
			run(ctx)
		}
	}
}

func (g *Gateway) Shutdown(ctx context.Context) {
	wait, cancel := context.WithTimeout(ctx, g.deps.Timing.ShutdownWait)
	defer cancel()
	g.mu.Lock()
	g.closing = true
	for s := range g.open {
		g.closeLocked(s, domain.CloseShutdown, "shutdown")
	}
	for _, found := range g.accounts {
		if found.settle != nil {
			found.settle.Stop()
			found.settling = false
		}
	}
	stop := g.stop
	g.mu.Unlock()
	stop()
	finished := make(chan struct{})
	core.Go(wait, g.deps.Logger, "realtime.shutdown", func(ctx context.Context) {
		g.served.Wait()
		g.loops.Wait()
		g.rechecks.Wait()
		if err := g.ExtendPresence(ctx); err != nil {
			g.deps.Logger.LogAttrs(ctx, slog.LevelError, "presence.shutdown.incomplete", slog.String("kind", core.ErrorKind(err)), slog.Int("unreleased_accounts", g.pendingPresence()))
		}
		g.stopPresence()
		g.presenceRun.Wait()
		close(finished)
	})
	select {
	case <-finished:
	case <-wait.Done():
		g.stopPresence()
		g.presenceRun.Wait()
		g.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "realtime.shutdown.incomplete", slog.String("kind", core.ErrorKind(wait.Err())))
	}
}

type Reservation struct {
	bucket netip.Addr
}

func (g *Gateway) Reserve(address netip.Addr) (Reservation, UpgradeRefusal) {
	bucket := domain.AddressBucket(address)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closing || g.pending >= g.deps.Limits.Unauthenticated {
		return Reservation{}, UpgradeBusy
	}
	if g.byAddress[bucket] >= g.deps.Limits.PerAddress {
		return Reservation{}, UpgradeAddressBusy
	}
	g.pending++
	g.byAddress[bucket]++
	g.served.Add(1)
	return Reservation{bucket: bucket}, UpgradeAccepted
}

func (g *Gateway) releaseLocked(bucket netip.Addr) {
	g.pending--
	g.byAddress[bucket]--
	if g.byAddress[bucket] <= 0 {
		delete(g.byAddress, bucket)
	}
}

func (g *Gateway) Serve(w http.ResponseWriter, r *http.Request, reservation Reservation) {
	defer g.served.Done()
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: g.originHosts})
	if err != nil {
		g.mu.Lock()
		g.releaseLocked(reservation.bucket)
		g.mu.Unlock()
		g.deps.Logger.LogAttrs(r.Context(), slog.LevelWarn, "ws.upgrade.refused", slog.String("reason", "accept"))
		return
	}
	conn.SetReadLimit(maxFrameBytes)
	s := g.newSocket(conn, reservation)
	g.mu.Lock()
	g.open[s] = struct{}{}
	if g.closing {
		g.closeLocked(s, domain.CloseShutdown, "shutdown")
	}
	g.mu.Unlock()
	s.run()
	g.mu.Lock()
	delete(g.open, s)
	g.mu.Unlock()
}

func (g *Gateway) state() (listener.Generation, bool) {
	if g.listener == nil {
		return 0, false
	}
	return g.listener.State()
}

func (g *Gateway) accountLocked(user domain.UserID) *account {
	found, ok := g.accounts[user]
	if !ok {
		found = &account{sockets: map[*socket]struct{}{}, presence: presence{changed: make(chan struct{})}}
		g.accounts[user] = found
	}
	return found
}

func (g *Gateway) forgetLocked(user domain.UserID, found *account) {
	if len(found.sockets) == 0 && !found.rechecking && !found.persisting && !found.queued && found.revision == found.persisted && !found.announcePending && !found.settling && g.accounts[user] == found {
		delete(g.accounts, user)
	}
}

type closeMode int

const (
	closeHandshake closeMode = iota
	closeDropped
)

func (g *Gateway) closeLocked(s *socket, code domain.CloseCode, reason string) {
	mode := closeHandshake
	if code == closeAbnormal {
		mode = closeDropped
	}
	g.closeInModeLocked(s, code, reason, mode)
}

func (g *Gateway) closeInModeLocked(s *socket, code domain.CloseCode, reason string, mode closeMode) {
	if s.closed {
		return
	}
	s.closed = true
	for _, timer := range s.typing {
		timer.Stop()
	}
	s.typing = nil
	s.code, s.reason = code, reason
	close(s.done)
	if s.reserved {
		s.reserved = false
		g.releaseLocked(s.reservation.bucket)
	}
	if found, ok := g.accounts[s.user]; ok && s.phase != phaseAwaiting {
		g.leaveLocked(s, found)
	}
	conn, finished, logger := s.conn, s.finished, s.log()
	core.Go(context.Background(), logger, "realtime.close", func(ctx context.Context) {
		defer close(finished)
		var err error
		if mode == closeDropped {
			err = conn.CloseNow()
		} else {
			err = conn.Close(websocket.StatusCode(code), code.Reason())
		}
		if err != nil {
			logger.LogAttrs(ctx, slog.LevelDebug, "ws.close.unconfirmed", slog.String("kind", core.ErrorKind(err)))
		}
	})
}

func (g *Gateway) ListenerConnected(listener.Generation) {}

func (g *Gateway) ListenerLost(generation listener.Generation, _ listener.LossKind) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, found := range g.accounts {
		for s := range found.sockets {
			if s.phase == phaseReady && s.generation <= generation {
				g.closeLocked(s, domain.CloseReconnect, "listener_lost")
			}
		}
	}
}

func (g *Gateway) Signal(signal domain.Signal) {
	started := g.deps.Now()
	switch typed := signal.(type) {
	case domain.RecheckSignal:
		sockets := g.recheck(typed.Users)
		g.logDelivery(domain.SignalRecheck, len(typed.Users), sockets, started)
	case domain.NotificationSignal:
		frame := encode(domain.FrameNotificationNew, nil, &schemas.NotificationNewPayload{ID: typed.Notification.String()})
		sockets := g.deliver([]domain.UserID{typed.User}, frame)
		g.logDelivery(domain.SignalNotification, 1, sockets, started)
	case domain.EntitySignal:
		payload := EntityPayload(typed)
		frame := encode(domain.FrameEntityChanged, nil, &payload)
		sockets := g.deliver(typed.Users, frame)
		g.logDelivery(domain.SignalEntity, len(typed.Users), sockets, started)
	case domain.TypingSignal:
		account := typed.Account.String()
		channel := typed.Channel.String()
		shown := g.deliver(typed.Shown, encode(domain.FrameTyping, nil, &schemas.TypingPayload{ChannelID: channel, UserID: &account}))
		anonymous := g.deliver(typed.Anonymous, encode(domain.FrameTyping, nil, &schemas.TypingPayload{ChannelID: channel}))
		g.logDelivery(domain.SignalTyping, len(typed.Shown)+len(typed.Anonymous), shown+anonymous, started)
	case domain.PresenceSignal:
		frame := encode(domain.FramePresence, nil, &schemas.PresencePayload{UserID: typed.Account.String(), Online: typed.Online})
		sockets := g.deliver(typed.Users, frame)
		g.logDelivery(domain.SignalPresence, len(typed.Users), sockets, started)
	case domain.ChatSignal:
		sockets := g.deliver(typed.Users, chatFrame(typed))
		g.logDelivery(domain.SignalChat, len(typed.Users), sockets, started)
		g.deps.Logger.LogAttrs(context.Background(), slog.LevelDebug, "chat.fanout.completed",
			slog.String("channel_id", typed.Channel.String()), slog.Int("recipients", len(typed.Users)), slog.Int("sockets", sockets),
			slog.Int64("duration_ms", g.deps.Now().Sub(started).Milliseconds()))
	}
}

func chatFrame(signal domain.ChatSignal) []byte {
	channel := signal.Channel.String()
	switch {
	case signal.Event == domain.FrameMessageNew && signal.Message != nil && signal.Seq != nil:
		return encode(signal.Event, nil, &schemas.MessageNewPayload{ChannelID: channel, MessageID: signal.Message.String(), Seq: *signal.Seq})
	case signal.Message != nil:
		return encode(signal.Event, nil, &schemas.MessageChangedPayload{ChannelID: channel, MessageID: signal.Message.String()})
	}
	return encode(signal.Event, nil, &schemas.ReadUpdatedPayload{ChannelID: channel})
}

func (g *Gateway) logDelivery(kind domain.SignalKind, recipients int, sockets int, started time.Time) {
	g.deps.Logger.LogAttrs(context.Background(), slog.LevelDebug, "realtime.delivery",
		slog.String("kind", string(kind)), slog.Int("recipients", recipients), slog.Int("sockets", sockets),
		slog.Int64("duration_ms", g.deps.Now().Sub(started).Milliseconds()))
}

func EntityPayload(signal domain.EntitySignal) schemas.EntityChangedPayload {
	payload := schemas.EntityChangedPayload{Kind: signal.Entity, ID: signal.ID.String()}
	if signal.Issue != nil {
		issue := signal.Issue.String()
		payload.IssueID = &issue
	}
	for _, lesson := range signal.Lessons {
		payload.LessonIDs = append(payload.LessonIDs, lesson.String())
	}
	return payload
}

func encode[Payload any](kind domain.FrameType, ref *string, payload *Payload) []byte {
	encoded, err := json.Marshal(schemas.ServerFrame[Payload]{Type: kind, Ref: ref, Payload: payload})
	if err != nil {
		panic("a server frame could not be encoded")
	}
	return encoded
}

func (g *Gateway) deliver(users []domain.UserID, frame []byte) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	delivered := 0
	for _, user := range users {
		found, ok := g.accounts[user]
		if !ok {
			continue
		}
		for s := range found.sockets {
			if s.phase != phaseReady {
				continue
			}
			g.enqueueLocked(s, frame)
			delivered++
		}
	}
	return delivered
}

func (g *Gateway) enqueueLocked(s *socket, frame []byte) {
	if s.closed {
		return
	}
	if !s.queue.push(frame) {
		s.log().LogAttrs(context.Background(), slog.LevelWarn, "ws.backpressure.closed", slog.String("reason", "queue"))
		g.closeLocked(s, domain.CloseTooMany, "backpressure")
		return
	}
	if !s.suspended {
		s.wakeWriter()
	}
}

func (g *Gateway) recheck(users []domain.UserID) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	marked := 0
	for _, user := range users {
		found, ok := g.accounts[user]
		if !ok {
			continue
		}
		found.rechecks++
		ready := 0
		for s := range found.sockets {
			marked++
			if s.phase == phaseReady {
				s.suspended = true
				ready++
			}
		}
		switch {
		case ready == 0 || g.closing:
		case found.rechecking:
			found.again = true
		default:
			found.rechecking = true
			g.rechecks.Add(1)
			core.Go(context.Background(), g.deps.Logger, "realtime.recheck", func(ctx context.Context) {
				defer g.rechecks.Done()
				g.recheckAccount(ctx, user, found)
			})
		}
	}
	return marked
}

func (g *Gateway) recheckAccount(ctx context.Context, user domain.UserID, found *account) {
	for {
		g.mu.Lock()
		found.again = false
		round := found.rechecks
		var suspended []*socket
		for s := range found.sockets {
			if s.phase == phaseReady && s.suspended {
				suspended = append(suspended, s)
			}
		}
		g.mu.Unlock()
		if len(suspended) > 0 {
			standings, err := g.lookup(ctx, sessionsOf(suspended))
			g.mu.Lock()
			g.settleLocked(ctx, found, suspended, standings, err, round)
			g.mu.Unlock()
			g.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "realtime.recheck", slog.Int("sockets", len(suspended)))
		}
		g.mu.Lock()
		if !found.again {
			found.rechecking = false
			g.forgetLocked(user, found)
			g.mu.Unlock()
			return
		}
		g.mu.Unlock()
	}
}

func (g *Gateway) settleLocked(
	ctx context.Context, found *account, suspended []*socket, standings map[domain.SessionID]accounts.SessionStanding, err error, round uint64,
) {
	if err != nil {
		g.deps.Logger.LogAttrs(ctx, slog.LevelError, "realtime.recheck.failed", slog.String("kind", core.ErrorKind(err)))
	}
	for _, s := range suspended {
		if s.closed {
			continue
		}
		if err != nil {
			g.closeLocked(s, domain.CloseReconnect, "recheck_failed")
			continue
		}
		if refusal := refusalOf(standings, s); refusal != accounts.SocketAllowed {
			g.closeLocked(s, domain.CloseUnauthenticated, string(refusal))
			continue
		}
		if found.rechecks == round {
			s.suspended = false
			s.wakeWriter()
		}
	}
}

func refusalOf(standings map[domain.SessionID]accounts.SessionStanding, s *socket) accounts.SocketRefusal {
	standing, ok := standings[s.session]
	if !ok {
		return accounts.SocketSessionEnded
	}
	if standing.Refusal == accounts.SocketAllowed && standing.UserID != s.user {
		return accounts.SocketSessionEnded
	}
	return standing.Refusal
}

func sessionsOf(sockets []*socket) []domain.SessionID {
	seen := map[domain.SessionID]bool{}
	var sessions []domain.SessionID
	for _, s := range sockets {
		if !seen[s.session] {
			seen[s.session] = true
			sessions = append(sessions, s.session)
		}
	}
	return sessions
}

func (g *Gateway) lookup(ctx context.Context, sessions []domain.SessionID) (map[domain.SessionID]accounts.SessionStanding, error) {
	lookupContext, cancel := context.WithTimeout(ctx, g.deps.Timing.LookupTimeout)
	defer cancel()
	return g.deps.Sessions.SessionStandings(lookupContext, sessions)
}

func (g *Gateway) readySockets() []*socket {
	g.mu.Lock()
	defer g.mu.Unlock()
	var ready []*socket
	for _, found := range g.accounts {
		for s := range found.sockets {
			if s.phase == phaseReady && !s.closed {
				ready = append(ready, s)
			}
		}
	}
	return ready
}

func (g *Gateway) closeLapsed(ctx context.Context) {
	now, err := g.deps.Clock.Instant(ctx)
	if err != nil {
		if ctx.Err() == nil {
			g.deps.Logger.LogAttrs(ctx, slog.LevelError, "realtime.lapse.failed", slog.String("kind", core.ErrorKind(err)))
		}
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	checked, closed := 0, 0
	for _, found := range g.accounts {
		for s := range found.sockets {
			if s.phase != phaseReady || s.closed {
				continue
			}
			checked++
			if !now.Before(s.expiresAt.Add(g.deps.Timing.TokenGrace)) {
				s.log().LogAttrs(ctx, slog.LevelInfo, "ws.token.lapsed")
				g.closeLocked(s, domain.CloseUnauthenticated, "token_lapsed")
				closed++
			}
		}
	}
	g.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "realtime.lapse", slog.Int("checked", checked), slog.Int("closed", closed))
}

func (g *Gateway) sweep(ctx context.Context) {
	ready := g.readySockets()
	if len(ready) == 0 {
		return
	}
	standings, err := g.lookup(ctx, sessionsOf(ready))
	if err != nil {
		if ctx.Err() == nil {
			g.deps.Logger.LogAttrs(ctx, slog.LevelError, "realtime.sweep.failed", slog.String("kind", core.ErrorKind(err)))
		}
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	closed := 0
	for _, s := range ready {
		if refusal := refusalOf(standings, s); refusal != accounts.SocketAllowed && !s.closed {
			g.closeLocked(s, domain.CloseUnauthenticated, string(refusal))
			closed++
		}
	}
	g.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "realtime.sweep", slog.Int("checked", len(ready)), slog.Int("closed", closed))
}

type Presence struct {
	Online  bool
	Sockets int
	Pending bool
}

func (g *Gateway) Presence(user domain.UserID) Presence {
	g.mu.Lock()
	defer g.mu.Unlock()
	sockets := 0
	if found, ok := g.accounts[user]; ok {
		for s := range found.sockets {
			if s.phase == phaseReady && !s.closed {
				sockets++
			}
		}
	}
	found := g.accounts[user]
	pending := found != nil && found.revision != found.persisted
	return Presence{Online: sockets > 0, Sockets: sockets, Pending: pending}
}

var ErrCloseCodeNotAllowed = errors.New("only 4429 and 1012 can be requested")

func (g *Gateway) CloseAccount(user domain.UserID, code domain.CloseCode) (int, error) {
	if code != domain.CloseTooMany && code != domain.CloseReconnect {
		return 0, ErrCloseCodeNotAllowed
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	found, ok := g.accounts[user]
	if !ok {
		return 0, nil
	}
	closed := 0
	for s := range found.sockets {
		if s.phase == phaseReady && !s.closed {
			g.closeLocked(s, code, "test_control")
			closed++
		}
	}
	return closed, nil
}
