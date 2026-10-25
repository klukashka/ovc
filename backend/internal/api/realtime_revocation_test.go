package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
)

const revocationBound = 5 * time.Second

func (c *socketClient) expectCloseWithin(t *testing.T, code domain.CloseCode, from time.Time) {
	t.Helper()
	c.expectClose(t, code)
	if elapsed := (clock.System{}).Now().Sub(from); elapsed > revocationBound {
		t.Fatalf("the socket closed %s after the revocation", elapsed)
	}
}

func (c *socketClient) expectAlive(t *testing.T) {
	t.Helper()
	c.ping(t, "alive")
	c.expect(t, domain.FramePong)
}

func TestSignOutClosesOnlyThatSessionsSocketsWithin5Seconds(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, signingOut := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	staying := accessToken(t, h.login(t, tutorEmail, accountPassword))
	revoked := []*socketClient{h.connect(t, signingOut), h.connect(t, signingOut)}
	kept := h.connect(t, staying)

	started := clock.System{}.Now()
	h.mustAnswer(t, h.send(t, http.MethodPost, "/api/v1/auth/logout", "", bearer(signingOut)), http.StatusNoContent)

	for _, client := range revoked {
		client.expectCloseWithin(t, domain.CloseUnauthenticated, started)
	}
	kept.expectAlive(t)
}

func TestSignOutEverywhereClosesEverySocketOfTheAccount(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, here := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	elsewhere := accessToken(t, h.login(t, tutorEmail, accountPassword))
	_, bystander := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	clients := []*socketClient{h.connect(t, here), h.connect(t, elsewhere)}
	unrelated := h.connect(t, bystander)

	started := clock.System{}.Now()
	h.mustAnswer(t, h.send(t, http.MethodPost, "/api/v1/auth/logout-all", "", bearer(here)), http.StatusNoContent)

	for _, client := range clients {
		client.expectCloseWithin(t, domain.CloseUnauthenticated, started)
	}
	unrelated.expectAlive(t)
}

func TestPasswordChangeClosesTheOtherSessionsSockets(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, here := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	elsewhere := accessToken(t, h.login(t, tutorEmail, accountPassword))
	current := h.connect(t, here)
	other := h.connect(t, elsewhere)

	started := clock.System{}.Now()
	changed := h.postJSON(t, "/api/v1/auth/password/change",
		fmt.Sprintf(`{"current_password":%q,"new_password":"a brand new password"}`, accountPassword), bearer(here))
	h.mustAnswer(t, changed, http.StatusNoContent)

	other.expectCloseWithin(t, domain.CloseUnauthenticated, started)
	current.expectAlive(t)
}

func TestReclaimClosesTheSquattersSockets(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.squat(t, tutor, newcomerEmail, newcomerEmail)
	squatter := h.connect(t, accessToken(t, h.login(t, newcomerEmail, newcomerPassword)))
	tutorSocket := h.connect(t, tutor)

	started := clock.System{}.Now()
	h.reclaimed(t, newcomerEmail)

	squatter.expectCloseWithin(t, domain.CloseUnauthenticated, started)
	tutorSocket.expect(t, domain.FrameEntityChanged)
	tutorSocket.expectAlive(t)
}

func TestDeletionClosesTheAccountsSockets(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	client := h.connect(t, token)

	started := clock.System{}.Now()
	h.mustAnswer(t, h.deleteMe(t, token, accountPassword), http.StatusNoContent)

	client.expectCloseWithin(t, domain.CloseUnauthenticated, started)
}

func TestOperatorBlockClosesTheAccountsSockets(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 1, studentEmail)
	client := h.connect(t, token)

	started := clock.System{}.Now()
	if _, err := h.operators().Block(context.Background(), studentEmail); err != nil {
		t.Fatal(err)
	}

	client.expectCloseWithin(t, domain.CloseUnauthenticated, started)
	h.awaitLogs(t, 1, `"ws.connection.closed"`, `"reason":"account_state"`)
}

func TestRefreshReuseClosesTheSessionsSockets(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	h.seedUser(t, 1, tutorEmail, accountPassword, currentHash)
	signedIn := h.login(t, tutorEmail, accountPassword)
	stale := refreshValue(t, signedIn)
	h.renewedToken(t, stale)
	client := h.connect(t, accessToken(t, signedIn))

	started := clock.System{}.Now()
	reused := h.refresh(t, stale)

	if reused.status != http.StatusUnauthorized {
		t.Fatalf("the reused cookie answered %d %s", reused.status, reused.body)
	}
	client.expectCloseWithin(t, domain.CloseUnauthenticated, started)
}

func TestCleanupAndPurgeCloseTheAccountsSockets(t *testing.T) {
	t.Parallel()
	paths := map[string]func(t *testing.T, h *liveHarness, token string){
		"7-day cleanup": func(t *testing.T, h *liveHarness, _ string) {
			h.at(t, authStart.Add(7*24*time.Hour))
			h.tick(t)
		},
		"immediate purge": func(t *testing.T, h *liveHarness, token string) {
			h.mustAnswer(t, h.deleteMe(t, token, newcomerPassword), http.StatusNoContent)
		},
	}
	for name, purge := range paths {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newLiveHarness(t, liveTuning{})
			h.register(t, tutorSignUp(newcomerEmail))
			token := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
			client := h.connect(t, token)

			started := clock.System{}.Now()
			purge(t, h, token)

			client.expectCloseWithin(t, domain.CloseUnauthenticated, started)
		})
	}
}
