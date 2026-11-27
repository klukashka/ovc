package api_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
)

const (
	stalledWriteTimeout = 750 * time.Millisecond
	floodChunk          = 80
)

func stalledWriteTiming() realtime.Timing {
	timing := quietTiming()
	timing.WriteTimeout = stalledWriteTimeout
	return timing
}

func (h *liveHarness) stalledSocket(t *testing.T, token string) *socketClient {
	t.Helper()
	conn, _, err := h.dial(t, originHeader(), smallReceiveBuffer())
	if err != nil {
		t.Fatal(err)
	}
	stalled := newSocketClient(conn)
	stalled.auth(t, token)
	ctx, cancel := context.WithTimeout(context.Background(), socketDeadline)
	defer cancel()
	if _, ready, err := conn.Read(ctx); err != nil || !strings.Contains(string(ready), `"ready"`) {
		t.Fatalf("no ready frame: %s %v", ready, err)
	}
	return stalled
}

func (h *liveHarness) floodDelivered(t *testing.T, user domain.UserID, flooded *int) {
	t.Helper()
	h.floodEntities(t, user, floodChunk)
	*flooded += floodChunk
	h.awaitLogs(t, *flooded, `"realtime.delivery"`, `"kind":"entity"`)
}

func (s *manualScheduler) pendingTimer(after time.Duration) *manualTimer {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, timer := range s.timers {
		if !timer.done && timer.after == after {
			return timer
		}
	}
	return nil
}

func (h *liveHarness) awaitStalledWrite(t *testing.T, scheduler *manualScheduler, user domain.UserID) {
	t.Helper()
	flooded := 0
	h.waitFor(t, "a write that stays pending across a delivered chunk", func() bool {
		pending := scheduler.pendingTimer(stalledWriteTimeout)
		h.floodDelivered(t, user, &flooded)
		return pending != nil && scheduler.pendingTimer(stalledWriteTimeout) == pending
	})
}

func (h *liveHarness) fireWriteTimersUntilClosed(t *testing.T, scheduler *manualScheduler, user domain.UserID) {
	t.Helper()
	flooded := 0
	h.waitFor(t, "a socket closed by its write timer", func() bool {
		if h.countLogs(`"ws.connection.closed"`) > 0 {
			return true
		}
		if scheduler.pendingTimers(stalledWriteTimeout) == 0 {
			h.floodDelivered(t, user, &flooded)
			return false
		}
		scheduler.advance(stalledWriteTimeout)
		return false
	})
}

func TestAStalledWriteClosesWith4429WhenItsTimerFires(t *testing.T) {
	t.Parallel()
	h, scheduler := newScheduledHarness(t, stalledWriteTiming())
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.stalledSocket(t, token)

	h.fireWriteTimersUntilClosed(t, scheduler, user)

	h.awaitLogs(t, 1, `"ws.backpressure.closed"`, `"reason":"write_timeout"`)
	closed := h.countLogs(`"ws.connection.closed"`)
	timedOut := h.countLogs(`"ws.connection.closed"`, `"code":4429`, `"reason":"write_timeout"`)
	if closed != 1 || timedOut != 1 {
		t.Fatalf("closed records %d, of them 4429 write_timeout %d", closed, timedOut)
	}
	if backpressure := h.countLogs(`"ws.backpressure.closed"`); backpressure != 1 {
		t.Fatalf("backpressure records %d", backpressure)
	}
}

func TestAListenerLossDuringAStalledWriteStillSends1012(t *testing.T) {
	t.Parallel()
	h, scheduler := newScheduledHarness(t, stalledWriteTiming())
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	stalled := h.stalledSocket(t, token)
	h.awaitStalledWrite(t, scheduler, user)

	h.terminateListener(t)

	stalled.expectClose(t, domain.CloseReconnect)
	h.awaitLogs(t, 1, `"ws.connection.closed"`, `"code":1012`)
}

func TestAStalledSocketClosedByAListenerLossIsDroppedWithoutReading(t *testing.T) {
	t.Parallel()
	h, scheduler := newScheduledHarness(t, stalledWriteTiming())
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.stalledSocket(t, token)
	h.awaitStalledWrite(t, scheduler, user)

	h.terminateListener(t)

	h.awaitLogs(t, 1, `"ws.connection.closed"`, `"code":1012`)
	if timers := scheduler.pendingTimers(stalledWriteTimeout); timers != 0 {
		t.Fatalf("the dropped socket's write still holds %d pending write timers", timers)
	}
}
