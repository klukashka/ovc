package api_test

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
	"ovc/backend/internal/repositories"
)

const absenceWindow = 300 * time.Millisecond

func TestNoFrameIsDeliveredWhileARecheckIsPending(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	held := h.holdSessionLookups(t)
	h.recheckSignal(t, user)
	h.awaitBlockedLookups(t, 1)

	h.signal(t, func(ctx context.Context, signals repositories.Signals) error {
		return signals.Notification(ctx, user, domain.NotificationID{7})
	})
	h.entityChanged(t, []domain.UserID{user}, 8)
	h.awaitLogs(t, 1, `"realtime.delivery"`, `"kind":"entity"`, `"sockets":1`)
	client.expectNothingWithin(t, absenceWindow)
	held.release()

	notification := client.expect(t, domain.FrameNotificationNew)
	entity := client.expect(t, domain.FrameEntityChanged)
	notificationID, entityID := domain.NotificationID{7}, domain.EntityID{8}
	if string(notification.Payload) != `{"id":"`+notificationID.String()+`"}` ||
		string(entity.Payload) != `{"kind":"relationship","id":"`+entityID.String()+`"}` {
		t.Fatalf("the frames are %s and %s", notification.Payload, entity.Payload)
	}
}

func TestSuspendedFramesCountAgainstTheQueueBound(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	h.holdSessionLookups(t)
	h.recheckSignal(t, user)
	h.awaitBlockedLookups(t, 1)

	h.floodEntities(t, user, 257)

	frame, closed := client.next(t)
	if closed != websocket.StatusCode(domain.CloseTooMany) {
		t.Fatalf("a suspended socket received %s before closing with %d", frame.Type, closed)
	}
	h.awaitLogs(t, 1, `"ws.backpressure.closed"`, `"reason":"queue"`)
}

func TestRechecksForOneAccountAreCoalesced(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	held := h.holdSessionLookups(t)
	h.recheckSignal(t, user)
	h.awaitBlockedLookups(t, 1)

	for range 4 {
		h.recheckSignal(t, user)
	}
	h.awaitLogs(t, 5, `"realtime.delivery"`, `"kind":"recheck"`)
	held.release()

	client.expectAlive(t)
	h.awaitLogs(t, 2, `"msg":"realtime.recheck"`)
	if rounds := h.countLogs(`"msg":"realtime.recheck"`); rounds != 2 {
		t.Fatalf("five signals during one re-check ran %d rounds", rounds)
	}
}

func TestAFailedRecheckClosesWith1012(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) { deps.Timing.LookupTimeout = 200 * time.Millisecond }})
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	h.holdSessionLookups(t)

	h.recheckSignal(t, user)

	client.expectClose(t, domain.CloseReconnect)
	h.awaitLogs(t, 1, `"realtime.recheck.failed"`)
}

func TestSlowConsumerIsClosedWith4429(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) { deps.Timing.WriteTimeout = 200 * time.Millisecond }})
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
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

	for range 100 {
		h.floodEntities(t, user, 100)
	}

	h.awaitLogs(t, 1, `"ws.backpressure.closed"`)
	h.awaitLogs(t, 1, `"ws.connection.closed"`, `"code":4429`)
	for {
		result := <-stalled.stream()
		if result.err != nil {
			status := websocket.CloseStatus(result.err)
			if status != -1 && status != websocket.StatusCode(domain.CloseTooMany) {
				t.Fatalf("closed with %d", status)
			}
			break
		}
	}
}

func TestListenerLossClosesSocketsWith1012AndRefusesAuthUntilRestored(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{listener: func(timing *listener.Timing) {
		timing.MinBackoff = 3 * time.Second
		timing.MaxBackoff = 3 * time.Second
	}})
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	before, _ := h.adapter.State()

	h.terminateListener(t)
	h.awaitListener(t, func(_ listener.Generation, live bool) bool { return !live })
	refused := h.open(t)
	refused.auth(t, token)
	h.awaitLogs(t, 1, `"ws.auth.refused"`, `"reason":"listener_down"`)
	h.awaitListener(t, func(generation listener.Generation, live bool) bool { return live && generation > before })
	restored := h.open(t)
	restored.auth(t, token)

	client.expectClose(t, domain.CloseReconnect)
	refused.expectClose(t, domain.CloseTryLater)
	restored.expect(t, domain.FrameReady)
}

func TestSweepClosesSocketsOfExpiredSessions(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) { deps.Timing.SweepEvery = 20 * time.Millisecond }})
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)

	h.advance(t, 31*24*time.Hour)

	client.expectClose(t, domain.CloseUnauthenticated)
	h.awaitLogs(t, 1, `"realtime.sweep"`, `"closed":1`)
}

func TestShutdownClosesSocketsWith1001(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	clients := []*socketClient{h.connect(t, token), h.open(t)}
	ctx, cancel := context.WithTimeout(context.Background(), socketDeadline)
	defer cancel()

	h.gateway.Shutdown(ctx)

	for _, client := range clients {
		client.expectClose(t, domain.CloseShutdown)
	}
	_, refused := h.openFrom(t, "198.51.100.9")
	if refused != http.StatusServiceUnavailable {
		t.Fatalf("an upgrade during shutdown answered %d", refused)
	}
}

func TestRealtimeRecordsCarryNoTokenOrPersonalData(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	client.ping(t, "ref-sentinel-ping")
	client.expect(t, domain.FramePong)
	client.auth(t, token)
	client.expect(t, domain.FrameReady)
	client.sendText(t, `{"type":"nonsense","ref":"ref-sentinel-error","payload":{"body":"payload-sentinel"}}`)
	client.expect(t, domain.FrameError)
	h.entityChanged(t, []domain.UserID{user}, 3)
	client.expect(t, domain.FrameEntityChanged)
	refused := h.open(t)
	refused.auth(t, token+"tampered")
	refused.expectClose(t, domain.CloseUnauthenticated)

	h.mustAnswer(t, h.send(t, http.MethodPost, "/api/v1/auth/logout", "", bearer(token)), http.StatusNoContent)
	client.expectClose(t, domain.CloseUnauthenticated)

	h.awaitLogs(t, 2, `"ws.connection.closed"`)
	records := h.logs.String()
	for _, forbidden := range []string{token, "ref-sentinel", "payload-sentinel", tutorEmail, "A language tutor"} {
		if strings.Contains(records, forbidden) {
			t.Fatalf("a record holds %q", forbidden)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(records), "\n") {
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("a record is not JSON: %s", line)
		}
		message := string(record["msg"])
		if strings.HasPrefix(message, `"ws.`) && !strings.Contains(line, `"connection_id"`) && !strings.HasPrefix(message, `"ws.upgrade`) {
			t.Fatalf("a socket record has no connection identifier: %s", line)
		}
	}
}

func TestEveryFrameTypeHasAGatewayTest(t *testing.T) {
	t.Parallel()
	producedBy := map[string]string{
		string(domain.FrameAuth):              "TestUnverifiedAccountMayConnect",
		string(domain.FramePing):              "TestPingAnswersPongAndRechecksTheSession",
		string(domain.FrameReady):             "TestUnverifiedAccountMayConnect",
		string(domain.FrameNotificationNew):   "TestNoFrameIsDeliveredWhileARecheckIsPending",
		string(domain.FrameEntityChanged):     "TestNoFrameIsDeliveredWhileARecheckIsPending",
		string(domain.FramePong):              "TestPingAnswersPongAndRechecksTheSession",
		string(domain.FrameError):             "TestUnknownOrReservedFrameTypesGetAnErrorAndKeepTheSocket",
		string(domain.FrameMessageNew):        "TestMessageFramesCarryIdentifiersOnly",
		string(domain.FrameMessageUpdated):    "TestMessageFramesCarryIdentifiersOnly",
		string(domain.FrameMessageDeleted):    "TestMessageFramesCarryIdentifiersOnly",
		string(domain.FrameReadUpdated):       "TestMessageFramesCarryIdentifiersOnly",
		string(domain.FrameTypingStart):       "TestTypingIsCoalescedBeforeTheDatabaseAndRelayedToOtherConnectedMembers",
		string(domain.FrameTypingStop):        "TestTypingFromANonMemberOrIntoAReadOnlyChannelIsRefused",
		string(domain.FrameTyping):            "TestStudentsReceiveAnonymousTypingAboutOtherStudents",
		string(domain.FramePresence):          "TestPresenceFramesReachOnlyUsersSharingAWritableChannel",
		closeKey(domain.CloseClient):          "TestTheWebSocketRouteHasTheOuterHandlers",
		closeKey(domain.CloseShutdown):        "TestShutdownClosesSocketsWith1001",
		closeKey(domain.CloseBinaryFrame):     "TestBinaryFrameClosesWith1003",
		closeKey(domain.CloseFrameTooLarge):   "TestOversizedFrameClosesWith1009",
		closeKey(domain.CloseReconnect):       "TestListenerLossClosesSocketsWith1012AndRefusesAuthUntilRestored",
		closeKey(domain.CloseTryLater):        "TestListenerGenerationChangeDuringAuthCloses1013",
		closeKey(domain.CloseUnauthenticated): "TestSocketWithoutAnAuthFrameClosesWith4401",
		closeKey(domain.CloseTooMany):         "TestTheTwentyFirstSocketOfAnAccountClosesWith4429",
	}
	defined := testFunctionNames(t)

	var missing []string
	for _, kind := range append(domain.ClientFrameTypes(), domain.ServerFrameTypes()...) {
		if !defined[producedBy[string(kind)]] {
			missing = append(missing, string(kind))
		}
	}
	for _, code := range domain.CloseCodes() {
		if !defined[producedBy[closeKey(code)]] {
			missing = append(missing, closeKey(code))
		}
	}

	if len(missing) > 0 {
		t.Fatalf("no gateway test produces %v", missing)
	}
}

func closeKey(code domain.CloseCode) string {
	return "close " + strconv.Itoa(int(code))
}

func testFunctionNames(t *testing.T) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, path := range paths {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && function.Recv == nil && strings.HasPrefix(function.Name.Name, "Test") {
				names[function.Name.Name] = true
			}
		}
	}
	return names
}
