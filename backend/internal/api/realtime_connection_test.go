package api_test

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
)

func upgradeHeaders() http.Header {
	return http.Header{
		"Connection":            []string{"Upgrade"},
		"Upgrade":               []string{"websocket"},
		"Sec-Websocket-Version": []string{"13"},
		"Sec-Websocket-Key":     []string{base64.StdEncoding.EncodeToString(make([]byte, 16))},
	}
}

func (h *authHarness) upgradeRequest(t *testing.T, method string, header http.Header, host string) response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), method, h.server.URL+"/api/v1/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header = header
	if host != "" {
		request.Host = host
	}
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := answer.Body.Close(); err != nil {
		t.Fatal(err)
	}
	return response{status: answer.StatusCode, body: string(body), headers: answer.Header}
}

func (h *authHarness) mustRefuse(t *testing.T, result response, status int, errorCode string) {
	t.Helper()
	if result.status != status || code(t, result) != errorCode || result.headers.Get("Cache-Control") != "no-store" ||
		result.headers.Get("Content-Type") != "application/json" {
		t.Fatalf("answered %d %s %v, want %d %s", result.status, result.body, result.headers, status, errorCode)
	}
}

func TestWrongOrMissingOriginIsRefusedBeforeTheUpgrade(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	withOrigins := func(host string, origins ...string) (http.Header, string) {
		header := upgradeHeaders()
		header["Origin"] = origins
		return header, host
	}
	cases := map[string]func() (http.Header, string){
		"another site":              func() (http.Header, string) { return withOrigins("", "https://attacker.example") },
		"the conferencing host":     func() (http.Header, string) { return withOrigins("", "https://video.ovc.example") },
		"plain http on the host":    func() (http.Header, string) { return withOrigins("", "http://ovc.example") },
		"an origin equal to a Host": func() (http.Header, string) { return withOrigins("attacker.example", "https://attacker.example") },
		"two origins":               func() (http.Header, string) { return withOrigins("", publicOrigin, publicOrigin) },
		"two origins, one foreign":  func() (http.Header, string) { return withOrigins("", publicOrigin, "https://attacker.example") },
		"no origin":                 func() (http.Header, string) { return withOrigins("") },
	}

	results := map[string]response{}
	for name, build := range cases {
		header, host := build()
		if len(header["Origin"]) == 0 {
			delete(header, "Origin")
		}
		results[name] = h.upgradeRequest(t, http.MethodGet, header, host)
	}

	for name, result := range results {
		t.Run(name, func(t *testing.T) {
			h.mustRefuse(t, result, http.StatusForbidden, "forbidden")
		})
	}
	if h.countLogs(`"ws.upgrade.refused"`, `"reason":"origin"`) != len(cases) {
		t.Fatalf("not every refusal was recorded:\n%s", h.logs.String())
	}
}

func TestNonUpgradeRequestGetsTheErrorBody(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	partial := upgradeHeaders()
	partial.Del("Sec-Websocket-Key")
	partial.Set("Origin", publicOrigin)

	plain := h.upgradeRequest(t, http.MethodGet, http.Header{"Origin": []string{publicOrigin}}, "")
	withoutKey := h.upgradeRequest(t, http.MethodGet, partial, "")

	h.mustRefuse(t, plain, http.StatusBadRequest, "bad_request")
	h.mustRefuse(t, withoutKey, http.StatusBadRequest, "bad_request")
}

func TestUnauthenticatedSocketLimitsAnswer503(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) {
		deps.Limits.Unauthenticated = 3
		deps.Limits.PerAddress = 2
	}})
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	first, _ := h.openFrom(t, "198.51.100.1")
	h.openFrom(t, "198.51.100.1")

	_, addressFull := h.openFrom(t, "198.51.100.1")
	h.openFrom(t, "198.51.100.2")
	_, instanceFull := h.openFrom(t, "198.51.100.3")
	header := upgradeHeaders()
	header.Set("Origin", publicOrigin)
	header.Set("X-Forwarded-For", "198.51.100.3")
	refused := h.upgradeRequest(t, http.MethodGet, header, "")
	first.auth(t, token)
	first.expect(t, domain.FrameReady)
	_, afterAuth := h.openFrom(t, "198.51.100.1")

	if addressFull != http.StatusServiceUnavailable || instanceFull != http.StatusServiceUnavailable {
		t.Fatalf("a full address answered %d and a full instance %d", addressFull, instanceFull)
	}
	h.mustRefuse(t, refused, http.StatusServiceUnavailable, "service_unavailable")
	if refused.headers.Get("Retry-After") != "5" {
		t.Fatalf("Retry-After is %q", refused.headers.Get("Retry-After"))
	}
	if afterAuth != http.StatusSwitchingProtocols {
		t.Fatalf("an authenticated socket kept its unauthenticated slot: %d", afterAuth)
	}
	if h.countLogs(`"ws.upgrade.refused"`, `"reason":"address_busy"`) != 1 || h.countLogs(`"ws.upgrade.refused"`, `"reason":"busy"`) != 2 {
		t.Fatalf("the refusals were not recorded:\n%s", h.logs.String())
	}
}

func TestPerAddressSocketLimitCountsMappedIPv4AsIPv4AndIPv6BySlash64(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) {
		deps.Limits.PerAddress = 2
	}})
	h.openFrom(t, "198.51.100.7")
	h.openFrom(t, "::ffff:198.51.100.7")
	h.openFrom(t, "2001:db8:1:2::1")
	h.openFrom(t, "2001:db8:1:2:ffff::9")

	_, thirdIPv4 := h.openFrom(t, "198.51.100.7")
	_, thirdMapped := h.openFrom(t, "::ffff:198.51.100.7")
	_, thirdInPrefix := h.openFrom(t, "2001:db8:1:2:abcd::3")
	_, otherPrefix := h.openFrom(t, "2001:db8:1:3::1")

	if thirdIPv4 != http.StatusServiceUnavailable || thirdMapped != http.StatusServiceUnavailable {
		t.Fatalf("a third socket of one IPv4 address answered %d plain and %d mapped", thirdIPv4, thirdMapped)
	}
	if thirdInPrefix != http.StatusServiceUnavailable {
		t.Fatalf("a third socket of one IPv6 /64 answered %d", thirdInPrefix)
	}
	if otherPrefix != http.StatusSwitchingProtocols {
		t.Fatalf("a socket of another IPv6 /64 answered %d", otherPrefix)
	}
	if h.countLogs(`"ws.upgrade.refused"`, `"reason":"address_busy"`) != 3 {
		t.Fatalf("the refusals were not recorded:\n%s", h.logs.String())
	}
}

func TestTheWebSocketRouteHasTheOuterHandlers(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	header := originHeader()
	header.Set("X-Request-ID", "socket-request-1")
	conn, answer, err := h.dial(t, header, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := newSocketClient(conn)
	client.auth(t, token)
	client.requireReady(t)

	closeErr := conn.Close(1000, "done")

	if closeErr != nil {
		t.Fatal(closeErr)
	}

	if answer.headers.Get("X-Request-ID") != "socket-request-1" || answer.headers.Get("Referrer-Policy") != "no-referrer" ||
		answer.headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("the upgrade lacks the outer headers: %v", answer.headers)
	}
	h.awaitLogs(t, 1, `"http.request.completed"`, `"route":"GET /api/v1/ws"`, `"status":101`, `"request_id":"socket-request-1"`)
	h.awaitLogs(t, 1, `"ws.connection.closed"`, `"code":1000`)
}

func TestSocketWithoutAnAuthFrameClosesWith4401(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) { deps.Timing.AuthWithin = 100 * time.Millisecond }})

	client := h.open(t)

	client.expectClose(t, domain.CloseUnauthenticated)
	h.awaitLogs(t, 1, `"ws.auth.refused"`, `"reason":"timeout"`)
}

func TestInvalidExpiredOrForeignTokensCloseWith4401(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	user, expiring := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	session := h.sessionOf(t, expiring)
	otherKeys, err := core.DeriveKeys(core.NewSecret("another-signing-material-0123456789ab"))
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := core.NewAccessTokens(otherKeys, publicOrigin, 15*time.Minute).Issue(core.AccessClaims{UserID: user, SessionID: session}, authStart.Add(20*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	h.advance(t, 20*time.Minute)
	tokens := map[string]string{"garbage": "not.a.token", "expired": expiring, "foreign": foreign, "empty": ""}

	for name, token := range tokens {
		client := h.open(t)
		client.auth(t, token)

		t.Run(name, func(t *testing.T) {
			client.expectClose(t, domain.CloseUnauthenticated)
		})
	}
	h.awaitLogs(t, 3, `"ws.auth.refused"`, `"reason":"invalid_token"`)
	h.awaitLogs(t, 1, `"ws.auth.refused"`, `"reason":"protocol"`)
}

func TestRevokedSessionOrBlockedAccountIsRefusedAtAuth(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, revoked := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.mustAnswer(t, h.send(t, http.MethodPost, "/api/v1/auth/logout", "", bearer(revoked)), http.StatusNoContent)
	_, blocked := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	if _, err := h.operators().Block(context.Background(), studentEmail); err != nil {
		t.Fatal(err)
	}

	afterSignOut := h.open(t)
	afterSignOut.auth(t, revoked)
	afterBlock := h.open(t)
	afterBlock.auth(t, blocked)

	afterSignOut.expectClose(t, domain.CloseUnauthenticated)
	afterBlock.expectClose(t, domain.CloseUnauthenticated)
	h.awaitLogs(t, 1, `"ws.auth.refused"`, `"reason":"session"`)
	h.awaitLogs(t, 1, `"ws.auth.refused"`, `"reason":"account_state"`)
}

func TestUnverifiedAccountMayConnect(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 1, studentEmail)

	h.connect(t, token)

	if presence := h.gateway.Presence(user); !presence.Online || presence.Sockets != 1 {
		t.Fatalf("presence is %+v", presence)
	}
	h.awaitLogs(t, 1, `"ws.connection.authenticated"`, `"role":"student"`, `"user_id":"`+user.String()+`"`)
}

func TestAuthForAnotherSessionOrAccountClosesWith4401(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	_, first := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	otherSession := accessToken(t, h.login(t, tutorEmail, accountPassword))
	_, otherAccount := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	sameAccount := h.connect(t, first)
	anotherAccount := h.connect(t, first)

	sameAccount.auth(t, otherSession)
	anotherAccount.auth(t, otherAccount)

	sameAccount.expectClose(t, domain.CloseUnauthenticated)
	anotherAccount.expectClose(t, domain.CloseUnauthenticated)
	h.awaitLogs(t, 2, `"ws.auth.refused"`, `"reason":"other_session"`)
}

func TestTheTwentyFirstSocketOfAnAccountClosesWith4429(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	user, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	for range 20 {
		h.connect(t, token)
	}

	extra := h.open(t)
	extra.auth(t, token)

	extra.expectClose(t, domain.CloseTooMany)
	if presence := h.gateway.Presence(user); presence.Sockets != 20 {
		t.Fatalf("presence is %+v", presence)
	}
	h.awaitLogs(t, 1, `"ws.auth.refused"`, `"reason":"too_many_sockets"`)
}

func (h *authHarness) sessionOf(t *testing.T, token string) domain.SessionID {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("not a token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	marker := `"sid":"`
	start := strings.Index(string(payload), marker)
	if start < 0 {
		t.Fatal("the token names no session")
	}
	decoded, err := hex.DecodeString(string(payload)[start+len(marker) : start+len(marker)+32])
	if err != nil {
		t.Fatal(err)
	}
	var session domain.SessionID
	copy(session[:], decoded)
	return session
}
