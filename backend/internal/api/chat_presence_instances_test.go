package api_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
	"ovc/backend/internal/services/chat"
)

func (h *liveHarness) anotherInstance(t *testing.T) *liveHarness {
	t.Helper()
	gateway, err := realtime.New(h.realtimeDeps)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := core.NewID[domain.InstanceID](core.CryptoRandom())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := listener.New(listener.Dependencies{DatabaseURL: h.database.URL, Instance: instance, Heartbeats: listener.ConnectionHeartbeats{}, Consumer: gateway, Logger: slog.New(slog.DiscardHandler), Timing: listener.DefaultTiming()})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reservation, refusal := gateway.Reserve(netip.MustParseAddr("127.0.0.1"))
		if refusal != realtime.UpgradeAccepted {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		gateway.Serve(w, r, reservation)
	}))
	shared := *h.authHarness
	shared.server = server
	sibling := &liveHarness{authHarness: &shared, gateway: gateway, adapter: adapter, realtimeDeps: h.realtimeDeps}
	ctx, cancel := context.WithCancel(context.Background())
	gateway.Start(ctx, adapter)
	stopped := make(chan struct{})
	go func() { defer close(stopped); adapter.Run(ctx) }()
	t.Cleanup(func() { gateway.Shutdown(context.Background()); cancel(); <-stopped; server.Close() })
	sibling.awaitListener(t, func(_ listener.Generation, connected bool) bool { return connected })
	return sibling
}

func TestPresenceRemainsOnlineWhileAnotherInstanceHasASocket(t *testing.T) {
	t.Parallel()
	h, scheduler := newScheduledHarness(t, quietTiming())
	other := h.anotherInstance(t)
	parties := h.groupOfTwoStudents(t)
	first := h.connect(t, parties.firstToken)
	last := other.connect(t, parties.firstToken)
	log := h.recordFrames(t)

	if err := first.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitRegistry(t, parties.first, 0)
	scheduler.advance(realtime.DefaultTiming().PresenceSettle)
	frames := log.settle(t, h.authHarness)
	retained, _ := h.presenceOf(t, parties.first)
	h.sent(t, parties.tutorToken, parties.groupChannel, "While the other API is connected")
	pending := h.digestPending(t, parties.first)
	if err := last.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	other.awaitRegistry(t, parties.first, 0)
	released, _ := h.presenceOf(t, parties.first)

	if !retained.OnlineUntil.After(authStart) || pending {
		t.Fatalf("other instance lost presence: %+v, digest pending %t", retained, pending)
	}
	for _, frame := range presenceFrames(frames) {
		if frame.kind == presenceFrame(false, parties.first).kind && frame.id == presenceFrame(false, parties.first).id {
			t.Fatalf("a live account was announced offline: %+v", frame)
		}
	}
	if released.OnlineUntil.After(authStart) {
		t.Fatalf("last instance retained online presence: %+v", released)
	}
}

func TestReadyWaitsForCommittedPresence(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	if _, err := h.database.Pool.Exec(context.Background(), "INSERT INTO user_presence VALUES ($1,$2,$2)", user, authStart); err != nil {
		t.Fatal(err)
	}
	release := h.holdRow(t, presenceRowLock, user.String())
	client := h.open(t)

	client.auth(t, token)
	h.waitForLockWaiters(t, 1)
	readyEarly := false
	select {
	case <-client.stream():
		readyEarly = true
	default:
	}
	release()
	client.expect(t, domain.FrameReady)
	stored, _ := h.presenceOf(t, user)

	if readyEarly || !stored.OnlineUntil.After(authStart) {
		t.Fatalf("ready preceded durable presence: early %t, stored %+v", readyEarly, stored)
	}
}

func TestReadyFailsWhenPresenceCannotCommit(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) { deps.Timing.PresenceTimeout = presenceTestBudget }})
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	if _, err := h.database.Pool.Exec(context.Background(), "INSERT INTO user_presence VALUES ($1,$2,$2)", user, authStart); err != nil {
		t.Fatal(err)
	}
	release := h.holdRow(t, presenceRowLock, user.String())
	client := h.open(t)

	client.auth(t, token)
	frame, closed := client.next(t)
	release()

	if closed != 1012 || frame.Type == domain.FrameReady {
		t.Fatalf("failed presence returned frame %s, close %d", frame.Type, closed)
	}
}

func TestAFailedPresenceReleaseRemainsPending(t *testing.T) {
	t.Parallel()
	scheduler := &manualScheduler{}
	h := newLiveHarness(t, liveTuning{testControl: true, gateway: func(deps *realtime.Dependencies) {
		deps.Scheduler = scheduler
		deps.Timing.PresenceTimeout = presenceTestBudget
	}})
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	client := h.connect(t, token)
	release := h.holdRow(t, presenceRowLock, user.String())

	if err := client.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitLogs(t, 1, `"msg":"presence.release.pending"`)
	pending := h.send(t, http.MethodPost, "/__test__/presence", fmt.Sprintf(`{"user":%q}`, user.String()), map[string]string{"Content-Type": "application/json"})
	release()
	scheduler.advance(realtime.DefaultTiming().PresenceEvery)
	h.awaitRegistry(t, user, 0)
	recovered := h.send(t, http.MethodPost, "/__test__/presence", fmt.Sprintf(`{"user":%q}`, user.String()), map[string]string{"Content-Type": "application/json"})
	stored, _ := h.presenceOf(t, user)

	h.mustAnswer(t, pending, http.StatusServiceUnavailable)
	h.mustAnswer(t, recovered, http.StatusOK)
	if stored.OnlineUntil.After(authStart) {
		t.Fatalf("release was lost: %+v", stored)
	}
}

func TestPresenceLeasesExpireAfterAnInstanceStopsExtending(t *testing.T) {
	t.Parallel()
	h, _ := newScheduledHarness(t, quietTiming())
	other := h.anotherInstance(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	h.connect(t, token)
	last := other.connect(t, token)

	h.advance(t, 60*time.Second)
	if err := other.gateway.ExtendPresence(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.advance(t, 31*time.Second)
	retained, _ := h.presenceOf(t, user)
	if err := last.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	other.awaitRegistry(t, user, 0)
	expired, _ := h.presenceOf(t, user)

	if !retained.OnlineUntil.After(authStart.Add(91 * time.Second)) {
		t.Fatalf("live instance expired with the stalled instance: %+v", retained)
	}
	if expired.OnlineUntil.After(authStart.Add(91 * time.Second)) {
		t.Fatalf("stalled instance remained online beyond its deadline: %+v", expired)
	}
}

func TestDelayedPresenceEmissionReadsTheLatestAggregate(t *testing.T) {
	t.Parallel()
	h, scheduler := newScheduledHarness(t, quietTiming())
	other := h.anotherInstance(t)
	parties := h.groupOfTwoStudents(t)
	first := h.connect(t, parties.firstToken)
	h.awaitAnnounced(t, 1, parties.first, true)
	log := h.recordFrames(t)

	if err := first.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitRegistry(t, parties.first, 0)
	other.connect(t, parties.firstToken)
	scheduler.advance(realtime.DefaultTiming().PresenceSettle)
	h.awaitAnnounced(t, 2, parties.first, true)
	frames := log.settle(t, h.authHarness)

	for _, frame := range presenceFrames(frames) {
		if frame.kind == presenceFrame(false, parties.first).kind && frame.id == parties.first.String() {
			t.Fatalf("delayed emission used stale local offline: %+v", frame)
		}
	}
}

func TestSettingTheClockFlushesLocalPresenceEvenAtTheSameInstant(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{testControl: true})
	other := h.anotherInstance(t)
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	h.connect(t, token)
	other.connect(t, token)
	target := authStart.Add(10 * time.Minute)

	first := h.send(t, http.MethodPost, "/__test__/clock", fmt.Sprintf(`{"set":%q}`, target.Format(time.RFC3339)), map[string]string{"Content-Type": "application/json"})
	if err := other.gateway.ExtendPresence(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := h.send(t, http.MethodPost, "/__test__/clock", fmt.Sprintf(`{"set":%q}`, target.Format(time.RFC3339)), map[string]string{"Content-Type": "application/json"})
	stored, _ := h.presenceOf(t, user)

	h.mustAnswer(t, first, http.StatusOK)
	h.mustAnswer(t, second, http.StatusOK)
	if !stored.OnlineUntil.Equal(target.Add(domain.PresenceWindow)) {
		t.Fatalf("Set did not flush active leases: %+v", stored)
	}
}

func TestReconnectBeforePendingReleaseRecoveryKeepsTheNewLease(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) { deps.Timing.PresenceTimeout = 200 * time.Millisecond }})
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	first := h.connect(t, token)
	release := h.holdRow(t, presenceRowLock, user.String())

	if err := first.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitLogs(t, 1, `"msg":"presence.release.pending"`)
	next := h.open(t)
	next.auth(t, token)
	h.waitForLockWaiters(t, 1)
	release()
	next.expect(t, domain.FrameReady)
	stored, _ := h.presenceOf(t, user)
	registry := h.gateway.Presence(user)

	if !stored.OnlineUntil.After(authStart) || !registry.Online || registry.Pending || registry.Sockets != 1 {
		t.Fatalf("pending close overwrote reconnect: %+v, %+v", stored, registry)
	}
}

func TestShutdownWithPendingPresenceReportsIncompleteRelease(t *testing.T) {
	t.Parallel()
	scheduler := &manualScheduler{}
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) {
		deps.Scheduler = scheduler
		deps.Timing.PresenceTimeout = presenceTestBudget
	}})
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	client := h.connect(t, token)
	release := h.holdRow(t, presenceRowLock, user.String())

	if err := client.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitLogs(t, 1, `"msg":"presence.release.pending"`)
	h.gateway.Shutdown(context.Background())
	pending := h.gateway.Presence(user)
	before := h.countLogs(`"msg":"presence.`)
	scheduler.advance(24 * time.Hour)
	after := h.countLogs(`"msg":"presence.`)
	release()
	stored, _ := h.presenceOf(t, user)

	if !pending.Pending || pending.Sockets != 0 || !stored.OnlineUntil.After(authStart) {
		t.Fatalf("shutdown hid its failed release: %+v, %+v", pending, stored)
	}
	if h.countLogs(`"msg":"presence.shutdown.incomplete"`, `"unreleased_accounts":1`) == 0 || before != after {
		t.Fatalf("incomplete shutdown was not recorded safely or timer survived: %d -> %d", before, after)
	}
}

func TestChatPresenceFailureRecordsCarryNoBodiesNamesOrAddresses(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) { deps.Timing.PresenceTimeout = presenceTestBudget }})
	parties := h.groupOfTwoStudents(t)
	client := h.connect(t, parties.firstToken)
	release := h.holdRow(t, presenceRowLock, parties.first.String())
	service := chat.New(chat.Dependencies{Store: h.store, Clock: h.clock, Logger: h.realtimeDeps.Logger})

	if err := client.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitLogs(t, 1, `"msg":"presence.release.pending"`)
	h.gateway.Shutdown(context.Background())
	release()
	_, cleanupErr := service.ExpirePresence(context.Background(), authStart.Add(91*time.Second))
	records := h.logs.String()

	if cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
	for index, value := range []string{bodySentinel, editSentinel, fileSentinel, replaySentinel, firstPresenceEmail, secondPresenceEmail, parties.firstToken, parties.tutorToken, "SELECT user_id", "?t="} {
		if strings.Contains(records, value) {
			t.Fatalf("presence diagnostics contain forbidden content item %d", index)
		}
	}
	for _, event := range []string{"presence.batch.failed", "presence.release.pending", "presence.shutdown.incomplete"} {
		if h.countLogs(`"msg":"`+event+`"`, `"kind"`) == 0 {
			t.Fatalf("missing safe failure event %s", event)
		}
	}
}
