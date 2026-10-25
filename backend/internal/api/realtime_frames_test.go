package api_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
)

func newManualHarness(t *testing.T) (*liveHarness, *manualNow) {
	t.Helper()
	now := &manualNow{at: clock.System{}.Now()}
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) { deps.Now = now.Now }})
	return h, now
}

func TestOversizedFrameClosesWith1009(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)

	client.sendText(t, `{"type":"ping","ref":"`+strings.Repeat("a", 16*1024)+`"}`)

	client.expectClose(t, domain.CloseFrameTooLarge)
	h.awaitLogs(t, 1, `"ws.connection.closed"`, `"code":1009`)
}

func TestBinaryFrameClosesWith1003(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	ctx, cancel := context.WithTimeout(context.Background(), socketDeadline)
	defer cancel()

	if err := client.conn.Write(ctx, websocket.MessageBinary, []byte(`{"type":"ping"}`)); err != nil {
		t.Fatal(err)
	}

	client.expectClose(t, domain.CloseBinaryFrame)
	h.awaitLogs(t, 1, `"ws.frame.rejected"`, `"reason":"binary"`)
}

func TestFrameFloodGetsErrorFramesThenCloses4429(t *testing.T) {
	t.Parallel()
	h, _ := newManualHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	for range 20 {
		client.ping(t, "within-rate")
	}
	for range 20 {
		client.expect(t, domain.FramePong)
	}
	for range 4 {
		client.requireRateLimited(t)
	}

	client.ping(t, "fifth-violation")

	client.expectClose(t, domain.CloseTooMany)
	h.awaitLogs(t, 1, `"ws.connection.closed"`, `"reason":"frame_rate"`)
}

func TestFrameChecksPerformAtMostOneLookupPerSecond(t *testing.T) {
	t.Parallel()
	h, now := newManualHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	now.advance(2 * time.Second)
	client.ping(t, "looked-up")
	client.expect(t, domain.FramePong)

	h.advance(t, 31*24*time.Hour)
	for range 5 {
		client.ping(t, "same-second")
	}
	answered := 0
	for range 5 {
		client.expect(t, domain.FramePong)
		answered++
	}
	now.advance(time.Second)
	client.ping(t, "next-second")

	client.expectClose(t, domain.CloseUnauthenticated)
	if answered != 5 {
		t.Fatalf("%d frames in the same second were answered", answered)
	}
}

func TestPingAnswersPongAndRechecksTheSession(t *testing.T) {
	t.Parallel()
	h, now := newManualHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)

	client.ping(t, "ref.1-A_b")
	pong := client.expect(t, domain.FramePong)
	h.advance(t, 31*24*time.Hour)
	now.advance(time.Second)
	client.ping(t, "expired")

	if pong.Ref == nil || *pong.Ref != "ref.1-A_b" || pong.Payload != nil {
		t.Fatalf("the pong is %+v", pong)
	}
	client.expectClose(t, domain.CloseUnauthenticated)
	if h.countLogs(`"pong"`)+h.countLogs(`"ping"`) != 0 {
		t.Fatal("ping or pong was recorded")
	}
}

func TestUnknownOrReservedFrameTypesGetAnErrorAndKeepTheSocket(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	frames := []string{
		`{"type":"nonsense","ref":"r1"}`,
		`{"type":"message.send","ref":"r2","payload":{"body":"hello"}}`,
		`{"type":"typing.start","ref":"r3"}`,
		`{"type":"ping","ref":"r4","extra":true}`,
		`{"type":"ping","ref":"r5","payload":{}}`,
		`{"type":"ping"} {"type":"ping"}`,
		`not json`,
	}

	for _, frame := range frames {
		client.sendText(t, frame)
	}

	refs := []string{}
	requestIDs := map[string]bool{}
	for range frames {
		errorCode, requestID, ref := errorPayload(t, client.expect(t, domain.FrameError))
		if errorCode != domain.ErrorBadRequest {
			t.Fatalf("got %s", errorCode)
		}
		requestIDs[requestID] = true
		if ref != nil {
			refs = append(refs, *ref)
		}
	}
	client.ping(t, "still-open")
	client.expect(t, domain.FramePong)
	if strings.Join(refs, ",") != "r1,r2,r3,r5" || len(requestIDs) != len(frames) {
		t.Fatalf("echoed refs %v and %d request identifiers", refs, len(requestIDs))
	}
	h.awaitLogs(t, 3, `"ws.frame.rejected"`, `"reason":"unknown_type"`)
	h.awaitLogs(t, 4, `"ws.frame.rejected"`, `"reason":"malformed"`)
}

func TestMalformedRefIsRefusedAndNeverLogged(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	refused := []string{"has space-sentinel", strings.Repeat("r", 65), "quote\"sentinel", ""}

	for _, ref := range refused {
		client.ping(t, ref)
	}
	client.ping(t, "accepted-sentinel")

	for range refused {
		errorCode, _, ref := errorPayload(t, client.expect(t, domain.FrameError))
		if errorCode != domain.ErrorBadRequest || ref != nil {
			t.Fatalf("a malformed ref got %s with ref %v", errorCode, ref)
		}
	}
	client.expect(t, domain.FramePong)
	h.awaitLogs(t, len(refused), `"ws.frame.rejected"`, `"reason":"malformed"`)
	if strings.Contains(h.logs.String(), "sentinel") || strings.Contains(h.logs.String(), strings.Repeat("r", 65)) {
		t.Fatal("a ref reached the records")
	}
}

func TestFramesBeforeAuthCloseWith4401(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	first := []string{`{"type":"ping","ref":"early"}`, `{"type":"nonsense"}`, `not json`}

	for _, frame := range first {
		client := h.open(t)
		client.sendText(t, frame)

		client.expectClose(t, domain.CloseUnauthenticated)
	}

	h.awaitLogs(t, len(first), `"ws.frame.rejected"`, `"reason":"before_auth"`)
}
