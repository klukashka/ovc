package api_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
)

const presenceTestBudget = 2 * time.Second

func TestPendingPresenceAccountsHaveATotalCapacity(t *testing.T) {
	t.Parallel()
	scheduler := &manualScheduler{}
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) {
		deps.Scheduler = scheduler
		deps.Limits.Accounts = 2
		deps.Timing.PresenceTimeout = presenceTestBudget
	}})
	first, firstToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, firstPresenceEmail)
	second, secondToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, secondPresenceEmail)
	third, thirdToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, formerPresenceEmail)
	firstSocket := h.connect(t, firstToken)
	secondSocket := h.connect(t, secondToken)
	h.awaitAnnounced(t, 1, first, true)
	h.awaitAnnounced(t, 1, second, true)
	scheduler.advance(realtime.DefaultTiming().PresenceSettle)
	releaseFirst := h.holdRow(t, presenceRowLock, first.String())
	releaseSecond := h.holdRow(t, presenceRowLock, second.String())

	if err := firstSocket.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	if err := secondSocket.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitLogs(t, 2, `"msg":"presence.release.pending"`)
	pendingFirst := h.gateway.Presence(first)
	pendingSecond := h.gateway.Presence(second)
	refused := h.open(t)
	started := clock.System{}.Now()
	refused.auth(t, thirdToken)
	frame, closed := refused.next(t)
	refusalDuration := time.Since(started)
	retainedFirst := h.gateway.Presence(first)
	retainedSecond := h.gateway.Presence(second)
	thirdLeases := h.countRows(t, "SELECT count(user_id) FROM user_presence_instances WHERE user_id = $1::uuid", third.String())
	releaseFirst()
	releaseSecond()
	if err := h.gateway.ExtendPresence(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.awaitAnnounced(t, 1, first, false)
	h.awaitAnnounced(t, 1, second, false)
	scheduler.advance(realtime.DefaultTiming().PresenceSettle)
	h.awaitRegistry(t, first, 0)
	h.awaitRegistry(t, second, 0)
	recovered := h.connect(t, thirdToken)
	thirdPresence := h.gateway.Presence(third)
	remainingFirst := h.countRows(t, "SELECT count(user_id) FROM user_presence_instances WHERE user_id = $1::uuid", first.String())
	remainingSecond := h.countRows(t, "SELECT count(user_id) FROM user_presence_instances WHERE user_id = $1::uuid", second.String())

	if !pendingFirst.Pending || !pendingSecond.Pending || pendingFirst.Online || pendingSecond.Online ||
		pendingFirst.Sockets != 0 || pendingSecond.Sockets != 0 {
		t.Fatalf("failed releases lost pending state: first %+v, second %+v", pendingFirst, pendingSecond)
	}
	if domain.CloseCode(closed) != domain.CloseTooMany || frame.Type == domain.FrameReady || refusalDuration >= time.Second {
		t.Fatalf("capacity admission returned frame %s, close %d after %s", frame.Type, closed, refusalDuration)
	}
	if !retainedFirst.Pending || !retainedSecond.Pending || thirdLeases != 0 {
		t.Fatalf("refusal displaced accepted releases: first %+v, second %+v, third leases %d", retainedFirst, retainedSecond, thirdLeases)
	}
	if recovered == nil || !thirdPresence.Online || thirdPresence.Sockets != 1 || remainingFirst != 0 || remainingSecond != 0 {
		t.Fatalf("capacity did not recover after durable release: third %+v, retained leases %d/%d", thirdPresence, remainingFirst, remainingSecond)
	}
}

func TestPresenceOperationsRespectAdmissionAndShutdownDeadlines(t *testing.T) {
	t.Parallel()
	scheduler := &manualScheduler{}
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) {
		deps.Scheduler = scheduler
		deps.Timing.PresenceTimeout = presenceTestBudget
	}})
	connected, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, firstPresenceEmail)
	waiting, waitingToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, secondPresenceEmail)
	h.connect(t, token)
	h.awaitTickers(t, scheduler, realtime.DefaultTiming().PresenceEvery, 1)
	if _, err := h.database.Pool.Exec(context.Background(), "INSERT INTO user_presence VALUES ($1, $2, $2)", waiting, authStart); err != nil {
		t.Fatal(err)
	}
	release := h.holdRow(t, presenceRowLock, waiting.String())
	client := h.open(t)

	started := clock.System{}.Now()
	client.auth(t, waitingToken)
	frame, closed := client.next(t)
	admissionDuration := time.Since(started)
	release()
	shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	h.gateway.Shutdown(shutdown)
	before := h.countLogs(`"msg":"presence.`)
	storedBefore, _ := h.presenceOf(t, connected)
	scheduler.advance(24 * time.Hour)
	after := h.countLogs(`"msg":"presence.`)
	storedAfter, _ := h.presenceOf(t, connected)
	leases := h.countRows(t, "SELECT count(user_id) FROM user_presence_instances WHERE user_id = $1::uuid", connected.String())

	if domain.CloseCode(closed) != domain.CloseReconnect || frame.Type == domain.FrameReady || admissionDuration >= 2*presenceTestBudget {
		t.Fatalf("blocked readiness returned frame %s, close %d after %s", frame.Type, closed, admissionDuration)
	}
	if scheduler.runningTickers(realtime.DefaultTiming().PresenceEvery) != 0 ||
		scheduler.pendingTimers(realtime.DefaultTiming().PresenceSettle) != 0 || before != after {
		t.Fatalf("presence work survived shutdown: records before %d, after %d", before, after)
	}
	if leases != 0 || storedAfter != storedBefore || storedAfter.OnlineUntil.After(authStart) {
		t.Fatalf("shutdown did not durably release presence: leases %d, before %+v, after %+v", leases, storedBefore, storedAfter)
	}
}

func TestPresenceHasFourConcurrentWriters(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) { deps.Timing.PresenceTimeout = 2 * time.Second }})
	clients := make([]*socketClient, 0, 5)
	tokens := make([]string, 0, 5)
	for index := range 5 {
		user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, byte(index+1), fmt.Sprintf("writer-%d@example.org", index))
		if _, err := h.database.Pool.Exec(context.Background(), "INSERT INTO user_presence VALUES ($1,$2,$2)", user, authStart); err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
		clients = append(clients, h.open(t))
	}
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM user_presence WHERE user_id <> $1::uuid FOR UPDATE", (domain.UserID{}).String())

	for index, client := range clients {
		client.auth(t, tokens[index])
	}
	h.waitForLockWaiters(t, 4)
	waiting := h.countRows(t, "SELECT count(*) FROM pg_stat_activity WHERE datname=$1::text AND cardinality(pg_blocking_pids(pid)) > 0", h.database.Name)
	release()
	for _, client := range clients {
		client.expect(t, domain.FrameReady)
	}

	if waiting != 4 {
		t.Fatalf("presence used %d concurrent blocked writers, want four", waiting)
	}
}
