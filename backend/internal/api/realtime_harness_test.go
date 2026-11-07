package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testcontrol"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

const (
	socketDeadline  = 15 * time.Second
	settledInterval = 5 * time.Millisecond
)

type manualNow struct {
	mu sync.Mutex
	at time.Time
}

func (m *manualNow) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.at
}

func (m *manualNow) advance(by time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.at = m.at.Add(by)
}

type liveTuning struct {
	gateway        func(*realtime.Dependencies)
	listener       func(*listener.Timing)
	testControl    bool
	lockContention bool
}

func (tuning liveTuning) database(t *testing.T) *testdb.Database {
	t.Helper()
	if tuning.lockContention {
		return testdb.NewForLockContention(t)
	}
	return testdb.New(t)
}

type liveHarness struct {
	*authHarness
	gateway      *realtime.Gateway
	adapter      *listener.Listener
	realtimeDeps realtime.Dependencies
}

func quietTiming() realtime.Timing {
	timing := realtime.DefaultTiming()
	timing.LapseCheckEvery = time.Hour
	timing.SweepEvery = time.Hour
	return timing
}

func newLiveHarness(t *testing.T, tuning liveTuning) *liveHarness {
	t.Helper()
	var gateway *realtime.Gateway
	var captured realtime.Dependencies
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	h := newAuthHarnessWithDatabase(t, tuning.database(t), trusted, api.NewHandler, func(local *teststorage.Storage) storage.Store { return local.Store },
		func(options *api.Options, shared *clock.Shared, logger *slog.Logger, _ *testdb.Database) {
			deps := realtime.Dependencies{
				Sessions: options.Auth.Accounts, Chat: options.Chat, Scheduler: realtime.SystemScheduler{}, Clock: shared, Now: clock.System{}.Now,
				NewID: core.NewRandomID, Logger: logger, PublicOrigin: publicOrigin, Timing: quietTiming(), Limits: realtime.DefaultLimits(),
			}
			if tuning.gateway != nil {
				tuning.gateway(&deps)
			}
			captured = deps
			created, err := realtime.New(deps)
			if err != nil {
				t.Fatal(err)
			}
			gateway = created
			options.Realtime = created
			if tuning.testControl {
				options.Mount = testcontrol.New(testcontrol.Dependencies{Logger: logger, Clock: shared, Realtime: created}).Mount
			}
		})
	instance, err := core.NewID[domain.InstanceID](core.CryptoRandom())
	if err != nil {
		t.Fatal(err)
	}
	timing := listener.DefaultTiming()
	if tuning.listener != nil {
		tuning.listener(&timing)
	}
	adapter, err := listener.New(listener.Dependencies{
		DatabaseURL: h.database.URL, Instance: instance, Heartbeats: listener.ConnectionHeartbeats{}, Consumer: gateway,
		Logger: slog.New(core.NewContextHandler(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))), Timing: timing,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	gateway.Start(ctx, adapter)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		adapter.Run(ctx)
	}()
	t.Cleanup(func() {
		shutdown, done := context.WithTimeout(context.Background(), socketDeadline)
		defer done()
		gateway.Shutdown(shutdown)
		cancel()
		<-stopped
	})
	live := &liveHarness{authHarness: h, gateway: gateway, adapter: adapter, realtimeDeps: captured}
	live.awaitListener(t, func(_ listener.Generation, connected bool) bool { return connected })
	return live
}

func (h *liveHarness) awaitListener(t *testing.T, condition func(listener.Generation, bool) bool) {
	t.Helper()
	h.waitFor(t, "the listener state", func() bool { return condition(h.adapter.State()) })
}

func (h *authHarness) waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(socketDeadline)
	defer deadline.Stop()
	poll := time.NewTicker(settledInterval)
	defer poll.Stop()
	for !condition() {
		select {
		case <-poll.C:
		case <-deadline.C:
			t.Fatalf("%s did not happen within %s", what, socketDeadline)
		}
	}
}

func (h *authHarness) countLogs(fragments ...string) int {
	count := 0
	for _, line := range strings.Split(h.logs.String(), "\n") {
		matched := true
		for _, fragment := range fragments {
			matched = matched && strings.Contains(line, fragment)
		}
		if matched && line != "" {
			count++
		}
	}
	return count
}

func (h *authHarness) awaitLogs(t *testing.T, at int, fragments ...string) {
	t.Helper()
	h.waitFor(t, fmt.Sprintf("%d records with %q", at, fragments), func() bool { return h.countLogs(fragments...) >= at })
}

func (h *authHarness) socketURL() string {
	return "ws" + strings.TrimPrefix(h.server.URL, "http") + "/api/v1/ws"
}

func originHeader() http.Header {
	return http.Header{"Origin": []string{publicOrigin}}
}

func (h *authHarness) dial(t *testing.T, header http.Header, dialer *net.Dialer) (*websocket.Conn, response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), socketDeadline)
	defer cancel()
	client := &http.Client{}
	if dialer != nil {
		client.Transport = &http.Transport{DialContext: dialer.DialContext}
	}
	conn, answer, err := websocket.Dial(ctx, h.socketURL(), &websocket.DialOptions{HTTPHeader: header, HTTPClient: client})
	upgrade := response{}
	if answer != nil {
		upgrade = response{status: answer.StatusCode, headers: answer.Header}
		if answer.Body != nil {
			if closeErr := answer.Body.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
		}
	}
	if conn != nil {
		conn.SetReadLimit(1 << 20)
		t.Cleanup(func() {
			if closeErr := conn.CloseNow(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
				t.Logf("closing a test socket: %v", closeErr)
			}
		})
	}
	return conn, upgrade, err
}

type readResult struct {
	kind websocket.MessageType
	data []byte
	err  error
}

type socketClient struct {
	conn     *websocket.Conn
	once     sync.Once
	results  chan readResult
	presence []serverFrame
}

func newSocketClient(conn *websocket.Conn) *socketClient {
	return &socketClient{conn: conn, results: make(chan readResult, 1024)}
}

func (c *socketClient) stream() <-chan readResult {
	c.once.Do(func() {
		go func() {
			for {
				kind, data, err := c.conn.Read(context.Background())
				c.results <- readResult{kind: kind, data: data, err: err}
				if err != nil {
					return
				}
			}
		}()
	})
	return c.results
}

func (h *authHarness) open(t *testing.T) *socketClient {
	t.Helper()
	conn, answer, err := h.dial(t, originHeader(), nil)
	if err != nil {
		t.Fatalf("the upgrade failed with %d: %v", answer.status, err)
	}
	return newSocketClient(conn)
}

func (h *authHarness) openFrom(t *testing.T, address string) (*socketClient, int) {
	t.Helper()
	header := originHeader()
	header.Set("X-Forwarded-For", address)
	conn, answer, err := h.dial(t, header, nil)
	if err != nil {
		if answer.status == 0 {
			t.Fatal(err)
		}
		return nil, answer.status
	}
	return newSocketClient(conn), http.StatusSwitchingProtocols
}

func (h *authHarness) connect(t *testing.T, token string) *socketClient {
	t.Helper()
	client := h.open(t)
	client.auth(t, token)
	client.expect(t, domain.FrameReady)
	return client
}

func (c *socketClient) sendText(t *testing.T, text string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), socketDeadline)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, []byte(text)); err != nil {
		t.Fatal(err)
	}
}

func (c *socketClient) auth(t *testing.T, token string) {
	t.Helper()
	c.sendText(t, fmt.Sprintf(`{"type":"auth","payload":{"access_token":%q}}`, token))
}

func (c *socketClient) ping(t *testing.T, ref string) {
	t.Helper()
	c.sendText(t, fmt.Sprintf(`{"type":"ping","ref":%q}`, ref))
}

type serverFrame struct {
	Type    domain.FrameType `json:"type"`
	Ref     *string          `json:"ref"`
	Payload json.RawMessage  `json:"payload"`
}

func (c *socketClient) next(t *testing.T) (serverFrame, websocket.StatusCode) {
	t.Helper()
	var result readResult
	select {
	case result = <-c.stream():
	case <-time.After(socketDeadline):
		t.Fatalf("nothing arrived within %s", socketDeadline)
	}
	if result.err != nil {
		status := websocket.CloseStatus(result.err)
		if status == -1 {
			t.Fatalf("reading failed without a close frame: %v", result.err)
		}
		return serverFrame{}, status
	}
	if result.kind != websocket.MessageText {
		t.Fatalf("the server sent a %v frame", result.kind)
	}
	var frame serverFrame
	decoder := json.NewDecoder(strings.NewReader(string(result.data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&frame); err != nil {
		t.Fatalf("the server frame %s does not parse: %v", result.data, err)
	}
	if frame.Type == domain.FramePresence {
		c.presence = append(c.presence, frame)
		return c.next(t)
	}
	return frame, 0
}

func (c *socketClient) expect(t *testing.T, kind domain.FrameType) serverFrame {
	t.Helper()
	frame, closed := c.next(t)
	if closed != 0 || frame.Type != kind {
		t.Fatalf("expected a %s frame, got %q or close %d", kind, frame.Type, closed)
	}
	return frame
}

func (c *socketClient) requireReady(t *testing.T) {
	t.Helper()
	c.expect(t, domain.FrameReady)
}

func (c *socketClient) requireRateLimited(t *testing.T) {
	t.Helper()
	c.ping(t, "flooding")
	if errorCode, _, _ := errorPayload(t, c.expect(t, domain.FrameError)); errorCode != domain.ErrorRateLimited {
		t.Fatalf("a flooding frame got %s", errorCode)
	}
}

func (c *socketClient) requireAlive(t *testing.T) {
	t.Helper()
	c.ping(t, "alive")
	c.expect(t, domain.FramePong)
}

func (c *socketClient) expectClose(t *testing.T, code domain.CloseCode) {
	t.Helper()
	for {
		frame, closed := c.next(t)
		if closed == 0 {
			if frame.Type == domain.FrameReady {
				t.Fatalf("expected close %d, got a ready frame", code)
			}
			continue
		}
		if closed != websocket.StatusCode(code) {
			t.Fatalf("closed with %d, want %d", closed, code)
		}
		return
	}
}

func (c *socketClient) expectNothingWithin(t *testing.T, window time.Duration) {
	t.Helper()
	select {
	case result := <-c.stream():
		t.Fatalf("%s arrived while nothing may: %v", result.data, result.err)
	case <-time.After(window):
	}
}

func errorPayload(t *testing.T, frame serverFrame) (domain.ErrorCode, string, *string) {
	t.Helper()
	var payload struct {
		Code      domain.ErrorCode `json:"code"`
		Message   string           `json:"message"`
		RequestID string           `json:"request_id"`
		Ref       *string          `json:"ref"`
	}
	if frame.Type != domain.FrameError || json.Unmarshal(frame.Payload, &payload) != nil || payload.Message == "" || payload.RequestID == "" {
		t.Fatalf("not an error frame: %s %s", frame.Type, frame.Payload)
	}
	return payload.Code, payload.RequestID, payload.Ref
}

func (h *authHarness) signal(t *testing.T, send func(ctx context.Context, signals repositories.Signals) error) {
	t.Helper()
	ctx := context.Background()
	if err := h.store.WithTx(ctx, func(r repositories.Repositories) error { return send(ctx, r.Signals) }); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) entityChanged(t *testing.T, users []domain.UserID, id byte) {
	t.Helper()
	h.signal(t, func(ctx context.Context, signals repositories.Signals) error {
		return signals.Entity(ctx, users, domain.EntityRelationship, domain.EntityID{id})
	})
}

func (h *authHarness) floodEntities(t *testing.T, user domain.UserID, count int) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(),
		`SELECT pg_notify('ovc_realtime', json_build_object('v', 1, 'k', 'entity', 'u', json_build_array($1::text), 'e', 'relationship',
		'i', gen_random_uuid()::text)::text) FROM generate_series(1, $2::int)`, user.String(), count)
	if err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) recheckSignal(t *testing.T, user domain.UserID) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(),
		`SELECT pg_notify('ovc_realtime', json_build_object('v', 1, 'k', 'recheck', 'u', json_build_array($1::text))::text)`, user.String())
	if err != nil {
		t.Fatal(err)
	}
}

type heldLookups struct {
	release func()
}

func (h *authHarness) holdSessionLookups(t *testing.T) *heldLookups {
	t.Helper()
	ctx := context.Background()
	tx, err := h.database.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "LOCK TABLE sessions IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	held := &heldLookups{release: func() {
		once.Do(func() {
			if err := tx.Rollback(ctx); err != nil {
				t.Error(err)
			}
		})
	}}
	t.Cleanup(held.release)
	return held
}

func (h *authHarness) awaitBlockedLookups(t *testing.T, at int) {
	t.Helper()
	h.waitFor(t, "a session lookup waiting on its lock", func() bool {
		var waiting int
		polling, cancel := context.WithTimeout(context.Background(), pauseDeadline)
		err := h.database.Pool.QueryRow(polling,
			`SELECT count(1) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'
			AND query LIKE '%GetSessionStandings%'`).Scan(&waiting)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		return waiting >= at
	})
}

func (h *liveHarness) terminateListener(t *testing.T) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(),
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND application_name = $1",
		listener.ApplicationName)
	if err != nil {
		t.Fatal(err)
	}
}

type authBarrier struct {
	held    chan struct{}
	release chan struct{}
	once    sync.Once
}

func newAuthBarrier() *authBarrier {
	return &authBarrier{held: make(chan struct{}), release: make(chan struct{})}
}

func (b *authBarrier) hold(ctx context.Context) {
	first := false
	b.once.Do(func() { first = true })
	if !first {
		return
	}
	close(b.held)
	select {
	case <-b.release:
	case <-ctx.Done():
	}
}

func (b *authBarrier) awaitHeld(t *testing.T) {
	t.Helper()
	select {
	case <-b.held:
	case <-time.After(socketDeadline):
		t.Fatal("authentication never reached the barrier")
	}
}

func smallReceiveBuffer() *net.Dialer {
	return &net.Dialer{Control: func(_ string, _ string, raw syscall.RawConn) error {
		var setErr error
		if err := raw.Control(func(descriptor uintptr) {
			setErr = syscall.SetsockoptInt(int(descriptor), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4096)
		}); err != nil {
			return err
		}
		return setErr
	}}
}
