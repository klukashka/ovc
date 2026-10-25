package api_test

import (
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
)

func newBarrierHarness(t *testing.T) (*liveHarness, *authBarrier) {
	t.Helper()
	barrier := newAuthBarrier()
	h := newLiveHarness(t, liveTuning{
		gateway:  func(deps *realtime.Dependencies) { deps.AuthLookupPassed = barrier.hold },
		listener: func(timing *listener.Timing) { timing.MinBackoff = 50 * time.Millisecond },
	})
	return h, barrier
}

func TestRevocationCommittedDuringAuthClosesWith4401(t *testing.T) {
	t.Parallel()
	h, barrier := newBarrierHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.open(t)
	client.auth(t, token)
	barrier.awaitHeld(t)

	h.mustAnswer(t, h.send(t, http.MethodPost, "/api/v1/auth/logout-all", "", bearer(token)), http.StatusNoContent)
	h.awaitLogs(t, 1, `"realtime.delivery"`, `"kind":"recheck"`, `"sockets":1`)
	close(barrier.release)

	client.expectClose(t, domain.CloseUnauthenticated)
	h.awaitLogs(t, 1, `"ws.auth.refused"`, `"reason":"revoked_during_auth"`)
}

func TestAnotherSessionsSignOutDuringAuthDoesNotCloseTheSocket(t *testing.T) {
	t.Parallel()
	h, barrier := newBarrierHarness(t)
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	other := accessToken(t, h.login(t, tutorEmail, accountPassword))
	client := h.open(t)
	client.auth(t, token)
	barrier.awaitHeld(t)
	h.mustAnswer(t, h.send(t, http.MethodPost, "/api/v1/auth/logout", "", bearer(other)), http.StatusNoContent)
	h.awaitLogs(t, 1, `"realtime.delivery"`, `"kind":"recheck"`, `"sockets":1`)

	close(barrier.release)

	client.expect(t, domain.FrameReady)
	if presence := h.gateway.Presence(user); !presence.Online || presence.Sockets != 1 {
		t.Fatalf("presence is %+v", presence)
	}
}

func TestListenerGenerationChangeDuringAuthCloses1013(t *testing.T) {
	t.Parallel()
	h, barrier := newBarrierHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	before, _ := h.adapter.State()
	client := h.open(t)
	client.auth(t, token)
	barrier.awaitHeld(t)

	h.terminateListener(t)
	h.awaitListener(t, func(generation listener.Generation, live bool) bool { return live && generation > before })
	close(barrier.release)

	client.expectClose(t, domain.CloseTryLater)
	h.awaitLogs(t, 1, `"ws.auth.refused"`, `"reason":"listener_down"`)
}

func (h *authHarness) renewedToken(t *testing.T, cookie string) (string, string) {
	t.Helper()
	renewed := h.refresh(t, cookie)
	return accessToken(t, renewed), refreshValue(t, renewed)
}

func newLapseHarness(t *testing.T) *liveHarness {
	t.Helper()
	return newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) { deps.Timing.LapseCheckEvery = 20 * time.Millisecond }})
}

func (h *liveHarness) lapseCheckAt(t *testing.T, instant time.Time) {
	t.Helper()
	h.at(t, instant)
	seen := h.countLogs(`"realtime.lapse"`)
	h.awaitLogs(t, seen+2, `"realtime.lapse"`)
}

func TestASocketWithoutAFreshTokenClosesWith4401AfterTheGrace(t *testing.T) {
	t.Parallel()
	h := newLapseHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)
	expiry := authStart.Add(15 * time.Minute)
	h.lapseCheckAt(t, expiry.Add(29*time.Second))
	client.requireAlive(t)

	h.at(t, expiry.Add(30*time.Second))

	client.expectClose(t, domain.CloseUnauthenticated)
	h.awaitLogs(t, 1, `"ws.token.lapsed"`)
}

func TestInBandAuthRenewsTheSocket(t *testing.T) {
	t.Parallel()
	h := newLapseHarness(t)
	h.seedUser(t, 1, tutorEmail, accountPassword, currentHash)
	signedIn := h.login(t, tutorEmail, accountPassword)
	client := h.connect(t, accessToken(t, signedIn))
	h.at(t, authStart.Add(10*time.Minute))
	fresh, _ := h.renewedToken(t, refreshValue(t, signedIn))
	client.auth(t, fresh)
	client.requireReady(t)

	h.lapseCheckAt(t, authStart.Add(16*time.Minute))
	client.ping(t, "renewed")

	client.expect(t, domain.FramePong)
	h.awaitLogs(t, 1, `"ws.connection.reauthenticated"`)
}

func TestInBandAuthKeepsTheLaterExpiry(t *testing.T) {
	t.Parallel()
	h := newLapseHarness(t)
	h.seedUser(t, 1, tutorEmail, accountPassword, currentHash)
	signedIn := h.login(t, tutorEmail, accountPassword)
	earlier := accessToken(t, signedIn)
	h.at(t, authStart.Add(10*time.Minute))
	later, _ := h.renewedToken(t, refreshValue(t, signedIn))
	client := h.connect(t, later)
	client.auth(t, earlier)
	client.requireReady(t)

	h.lapseCheckAt(t, authStart.Add(16*time.Minute))
	client.ping(t, "kept")

	client.expect(t, domain.FramePong)
}
