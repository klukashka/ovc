package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/notifications"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/worker"
)

const (
	publicOrigin = "https://ovc.example"
	tutorEmail   = "tutor@example.org"
)

var authStart = time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)

type countingRandom struct {
	mutex   sync.Mutex
	counter uint64
	pending []byte
}

func (r *countingRandom) Read(buffer []byte) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for len(r.pending) < len(buffer) {
		r.counter++
		block := sha256.Sum256(binary.BigEndian.AppendUint64(nil, r.counter))
		r.pending = append(r.pending, block[:]...)
	}
	copy(buffer, r.pending)
	r.pending = r.pending[len(buffer):]
	return len(buffer), nil
}

type countingHasher struct {
	inner         *core.PasswordHasher
	verifications atomic.Int32
	failure       error
	afterVerify   func()
}

func (h *countingHasher) Hash(ctx context.Context, password string) (string, error) {
	return h.inner.Hash(ctx, password)
}

func (h *countingHasher) Verify(ctx context.Context, password string, encoded string) (core.Verification, error) {
	h.verifications.Add(1)
	if h.failure != nil {
		return core.Verification{}, h.failure
	}
	verification, err := h.inner.Verify(ctx, password, encoded)
	if h.afterVerify != nil {
		h.afterVerify()
	}
	return verification, err
}

func (h *countingHasher) VerifyNothing(ctx context.Context, password string) error {
	h.verifications.Add(1)
	return h.inner.VerifyNothing(ctx, password)
}

type authHarness struct {
	server   *httptest.Server
	store    *repositories.Store
	clock    *clock.Shared
	hasher   *core.PasswordHasher
	counting *countingHasher
	logs     *safeBuffer
	database *testdb.Database
	sink     *mail.Sink
	worker   *worker.Worker
}

func cheapArgon2() core.Argon2Params {
	return core.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}
}

func newAuthHarness(t *testing.T) *authHarness {
	t.Helper()
	return newAuthHarnessBehind(t, nil)
}

func newAuthHarnessBehind(t *testing.T, trusted []netip.Prefix) *authHarness {
	t.Helper()
	database := testdb.New(t)
	random := &countingRandom{}
	logs := &safeBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	shared := clock.NewShared(database.Pool)
	if _, err := shared.Set(context.Background(), authStart); err != nil {
		t.Fatal(err)
	}
	hasher, err := core.NewPasswordHasher(cheapArgon2(), random, 4, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	store := repositories.NewStore(database.Pool)
	counting := &countingHasher{inner: hasher}
	service := accounts.New(accounts.Dependencies{
		Store: store, Clock: shared, Random: random, Hasher: counting, Keys: keys, Logger: logger,
		Tokens: core.NewAccessTokens(keys, publicOrigin, 15*time.Minute), RefreshTTL: 30 * 24 * time.Hour,
		TutorQuotas: core.TutorQuotas{FileBytes: 5_000_000_000, RecordingBytes: 50_000_000_000},
	})
	handler, err := api.NewHandler(api.Options{
		Logger: logger, Now: clock.System{}.Now, NewRequestID: core.NewRandomID,
		Readiness: func(context.Context) error { return nil },
		Auth:      api.Auth{Accounts: service, PublicOrigin: publicOrigin, TrustedProxies: trusted},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	sink := mail.NewSink(database.Pool, random)
	accountEmails, err := notifications.NewAccountEmails(store, random, publicOrigin)
	if err != nil {
		t.Fatal(err)
	}
	notifier, err := notifications.New(sink, publicOrigin, accountEmails.Builders())
	if err != nil {
		t.Fatal(err)
	}
	lanes := []worker.Lane{{Name: domain.LaneEmail, Concurrency: 10, Perform: notifier.Deliver}}
	background := worker.New(logger, shared, outbox.New(store, random, logger), lanes, nil, worker.DefaultTiming())
	return &authHarness{
		server: server, store: store, clock: shared, hasher: hasher, counting: counting, logs: logs,
		database: database, sink: sink, worker: background,
	}
}

func (h *authHarness) seedUser(t *testing.T, id byte, email string, password string, params core.Argon2Params) domain.UserID {
	t.Helper()
	hasher := h.hasher
	if params != cheapArgon2() {
		other, err := core.NewPasswordHasher(params, &countingRandom{counter: 1 << 20}, 1, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		hasher = other
	}
	encoded, err := hasher.Hash(context.Background(), password)
	if err != nil {
		t.Fatal(err)
	}
	user := domain.UserID{id}
	err = h.store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: user, Role: domain.RoleTutor, State: domain.AccountActive, Email: email, EmailVerifiedAt: &authStart,
		PasswordHash: encoded, Name: "A language tutor", Language: domain.LanguageEnglish, Timezone: "Europe/Moscow",
		Theme: domain.ThemeSystem, ConsentVersion: "2026-10", ConsentAt: authStart, CreatedAt: authStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

type response struct {
	status  int
	body    string
	headers http.Header
}

func (h *authHarness) send(t *testing.T, method string, path string, body string, headers map[string]string) response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), method, h.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := result.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	var text strings.Builder
	buffer := make([]byte, 4096)
	for {
		read, readErr := result.Body.Read(buffer)
		text.Write(buffer[:read])
		if readErr != nil {
			break
		}
	}
	return response{status: result.StatusCode, body: text.String(), headers: result.Header}
}

func (h *authHarness) quietLogin(email string, password string) (int, error) {
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, h.server.URL+"/api/v1/auth/login", strings.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Origin", publicOrigin)
	request.Header.Set("Content-Type", "application/json")
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	return result.StatusCode, result.Body.Close()
}

func (h *authHarness) quietRefresh(cookie string) (int, string, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, h.server.URL+"/api/v1/auth/refresh", nil)
	if err != nil {
		return 0, "", err
	}
	request.Header.Set("Origin", publicOrigin)
	request.Header.Set("Cookie", "__Host-ovc_refresh="+cookie)
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, "", err
	}
	value := ""
	for _, set := range result.Cookies() {
		if set.Name == "__Host-ovc_refresh" {
			value = set.Value
		}
	}
	return result.StatusCode, value, result.Body.Close()
}

func (h *authHarness) login(t *testing.T, email string, password string) response {
	t.Helper()
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	return h.send(t, http.MethodPost, "/api/v1/auth/login", body, map[string]string{"Origin": publicOrigin, "Content-Type": "application/json"})
}

func (h *authHarness) refresh(t *testing.T, cookie string) response {
	t.Helper()
	headers := map[string]string{"Origin": publicOrigin}
	if cookie != "" {
		headers["Cookie"] = "__Host-ovc_refresh=" + cookie
	}
	return h.send(t, http.MethodPost, "/api/v1/auth/refresh", "", headers)
}

func (h *authHarness) me(t *testing.T, token string) response {
	t.Helper()
	return h.send(t, http.MethodGet, "/api/v1/users/me", "", map[string]string{"Authorization": "Bearer " + token})
}

func (h *authHarness) advance(t *testing.T, by time.Duration) {
	t.Helper()
	if _, err := h.clock.Advance(context.Background(), by); err != nil {
		t.Fatal(err)
	}
}

func accessToken(t *testing.T, result response) string {
	t.Helper()
	var decoded struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || decoded.AccessToken == "" || decoded.ExpiresIn != 900 {
		t.Fatalf("no access token in %d %s", result.status, result.body)
	}
	return decoded.AccessToken
}

func refreshValue(t *testing.T, result response) string {
	t.Helper()
	for _, cookie := range (&http.Response{Header: result.headers}).Cookies() {
		if cookie.Name == "__Host-ovc_refresh" {
			return cookie.Value
		}
	}
	t.Fatalf("no refresh cookie in %v", result.headers)
	return ""
}

func code(t *testing.T, result response) string {
	t.Helper()
	var decoded struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil {
		t.Fatalf("not a structured error: %d %s", result.status, result.body)
	}
	return decoded.Error.Code
}

func TestLoginSetsExactlyTheHostOnlyRefreshCookie(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())

	result := h.login(t, "Tutor@Example.org ", "correct horse battery")

	cookies := result.headers.Values("Set-Cookie")
	if result.status != http.StatusOK || len(cookies) != 1 || result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("login %d, cookies %v, cache %q", result.status, cookies, result.headers.Get("Cache-Control"))
	}
	for _, attribute := range []string{"__Host-ovc_refresh=", "Path=/", "Max-Age=2592000", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(cookies[0], attribute) {
			t.Fatalf("cookie %q lacks %s", cookies[0], attribute)
		}
	}
	if strings.Contains(strings.ToLower(cookies[0]), "domain=") {
		t.Fatalf("cookie %q sets a domain", cookies[0])
	}
	if me := h.me(t, accessToken(t, result)); me.status != http.StatusOK || !strings.Contains(me.body, `"email":"tutor@example.org"`) {
		t.Fatalf("me %d %s", me.status, me.body)
	}
}

func TestLoginAnswersIdenticallyForAnUnknownEmailAWrongPasswordAndADeletedAccount(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	deleted := h.seedUser(t, 2, "deleted@example.org", "correct horse battery", cheapArgon2())
	if marked, err := h.store.Repositories().Users.MarkDeleted(context.Background(), deleted, authStart); err != nil || !marked {
		t.Fatalf("mark deleted: %v %v", marked, err)
	}

	unknown := h.login(t, "nobody@example.org", "correct horse battery")
	wrong := h.login(t, tutorEmail, "wrong horse battery")
	gone := h.login(t, "deleted@example.org", "correct horse battery")

	if verified := h.counting.verifications.Load(); verified != 3 {
		t.Fatalf("%d verifications; every path must hash once", verified)
	}
	for _, result := range []response{unknown, wrong, gone} {
		if result.status != http.StatusUnauthorized || code(t, result) != "invalid_credentials" || result.headers.Get("Set-Cookie") != "" {
			t.Fatalf("got %d %s", result.status, result.body)
		}
	}
	if strings.Contains(h.logs.String(), tutorEmail) || strings.Contains(h.logs.String(), "nobody@example.org") {
		t.Fatal("an email reached the logs")
	}
}

func TestLoginThrottlesTheSixthFailureWithin15Minutes(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	h.advance(t, 5*time.Minute+30*time.Second)
	for range 5 {
		if result := h.login(t, tutorEmail, "wrong horse battery"); result.status != http.StatusUnauthorized {
			t.Fatalf("failure answered %d", result.status)
		}
	}

	sixth := h.login(t, tutorEmail, "wrong horse battery")
	correct := h.login(t, tutorEmail, "correct horse battery")
	h.advance(t, 9*time.Minute+30*time.Second)
	later := h.login(t, tutorEmail, "correct horse battery")

	if sixth.status != http.StatusTooManyRequests || code(t, sixth) != "rate_limited" || sixth.headers.Get("Retry-After") != "570" {
		t.Fatalf("sixth %d %s retry %q", sixth.status, sixth.body, sixth.headers.Get("Retry-After"))
	}
	if correct.status != http.StatusTooManyRequests || later.status != http.StatusOK {
		t.Fatalf("correct while throttled %d, after the window %d", correct.status, later.status)
	}
}

func TestParallelWrongPasswordsVerifyAtMostFivePasswords(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	statuses := make([]int, 20)

	var wait sync.WaitGroup
	for index := range statuses {
		wait.Go(func() {
			status, err := h.quietLogin(tutorEmail, "wrong horse battery")
			if err != nil {
				t.Error(err)
			}
			statuses[index] = status
		})
	}
	wait.Wait()

	throttled := 0
	for _, status := range statuses {
		if status == http.StatusTooManyRequests {
			throttled++
		}
	}
	if verified := h.counting.verifications.Load(); verified > 5 || throttled != 15 {
		t.Fatalf("%d password verifications and %d throttled of 20 attempts: %v", verified, throttled, statuses)
	}
}

func TestTheTwentyFirstFailureFromOneAddressIsThrottled(t *testing.T) {
	h := newAuthHarness(t)
	for attempt := range 20 {
		if result := h.login(t, fmt.Sprintf("person%d@example.org", attempt), "wrong horse battery"); result.status != http.StatusUnauthorized {
			t.Fatalf("attempt %d answered %d", attempt, result.status)
		}
	}

	result := h.login(t, "another@example.org", "wrong horse battery")

	if result.status != http.StatusTooManyRequests || !strings.Contains(h.logs.String(), `"scope":"address"`) {
		t.Fatalf("the 21st failure answered %d", result.status)
	}
	if verified := h.counting.verifications.Load(); verified != 20 {
		t.Fatalf("%d verifications; the throttled attempt must not hash", verified)
	}
}

func TestBlockedAccountIsRevealedOnlyAfterTheCorrectPassword(t *testing.T) {
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	if blocked, err := h.store.Repositories().Users.Block(context.Background(), user, authStart); err != nil || !blocked {
		t.Fatalf("block: %v %v", blocked, err)
	}

	wrong := h.login(t, tutorEmail, "wrong horse battery")
	correct := h.login(t, tutorEmail, "correct horse battery")

	if wrong.status != http.StatusUnauthorized || code(t, wrong) != "invalid_credentials" {
		t.Fatalf("wrong password %d %s", wrong.status, wrong.body)
	}
	if correct.status != http.StatusForbidden || code(t, correct) != "account_blocked" {
		t.Fatalf("correct password %d %s", correct.status, correct.body)
	}
}

func TestLoginRehashesAPasswordWithStaleParameters(t *testing.T) {
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", core.Argon2Params{MemoryKiB: 32, Iterations: 1, Parallelism: 1})

	result := h.login(t, tutorEmail, "correct horse battery")

	stored, err := h.store.Repositories().Users.ByID(context.Background(), user)
	if result.status != http.StatusOK || err != nil || !strings.HasPrefix(stored.PasswordHash, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatalf("login %d, stored %q, %v", result.status, stored.PasswordHash, err)
	}
}

func TestParallelSignInsWithAStaleHashAllSucceed(t *testing.T) {
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", core.Argon2Params{MemoryKiB: 32, Iterations: 1, Parallelism: 1})
	statuses := make([]int, 4)

	var wait sync.WaitGroup
	for index := range statuses {
		wait.Go(func() {
			status, err := h.quietLogin(tutorEmail, "correct horse battery")
			if err != nil {
				t.Error(err)
			}
			statuses[index] = status
		})
	}
	wait.Wait()

	stored, err := h.store.Repositories().Users.ByID(context.Background(), user)
	for _, status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("parallel sign-ins with a stale hash answered %v", statuses)
		}
	}
	if err != nil || !strings.HasPrefix(stored.PasswordHash, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatalf("stored %q, %v", stored.PasswordHash, err)
	}
}

func TestRefreshReuseRevokesTheSession(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	first := h.login(t, tutorEmail, "correct horse battery")
	t0 := refreshValue(t, first)
	t1 := refreshValue(t, h.refresh(t, t0))
	second := h.refresh(t, t1)
	t2 := refreshValue(t, second)

	replay := h.refresh(t, t0)
	afterReplay := h.refresh(t, t2)
	me := h.me(t, accessToken(t, second))

	if replay.status != http.StatusUnauthorized || code(t, replay) != "unauthenticated" || !strings.Contains(replay.headers.Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("replay %d %s %v", replay.status, replay.body, replay.headers)
	}
	if afterReplay.status != http.StatusUnauthorized || me.status != http.StatusUnauthorized {
		t.Fatalf("after reuse: refresh %d, me %d", afterReplay.status, me.status)
	}
	if !strings.Contains(h.logs.String(), `"msg":"auth.refresh.reuse_detected"`) {
		t.Fatal("no reuse record")
	}
}

func TestForgedStaleGenerationIsRefusedWithoutRevoking(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	current := refreshValue(t, h.refresh(t, refreshValue(t, h.login(t, tutorEmail, "correct horse battery"))))
	parts := strings.Split(current, ".")
	forgeries := []string{
		strings.Join([]string{parts[0], "1", parts[2], parts[3]}, "."),
		strings.Join([]string{parts[0], "1", parts[2], strings.Repeat("A", 43)}, "."),
		parts[0] + ".1",
		"not-a-cookie",
	}

	var statuses []int
	for _, forged := range forgeries {
		statuses = append(statuses, h.refresh(t, forged).status)
	}
	still := h.refresh(t, current)

	for _, status := range statuses {
		if status != http.StatusUnauthorized {
			t.Fatalf("forgeries answered %v", statuses)
		}
	}
	if still.status != http.StatusOK || strings.Contains(h.logs.String(), "reuse_detected") {
		t.Fatalf("the session was revoked by a forgery: %d", still.status)
	}
}

func TestRefreshRejectsForeignOrigin(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	cookie := refreshValue(t, h.login(t, tutorEmail, "correct horse battery"))

	results := []response{
		h.send(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"Origin": "https://video.ovc.example", "Cookie": "__Host-ovc_refresh=" + cookie}),
		h.send(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"Cookie": "__Host-ovc_refresh=" + cookie}),
	}

	for _, result := range results {
		if result.status != http.StatusForbidden || code(t, result) != "forbidden" {
			t.Fatalf("got %d %s", result.status, result.body)
		}
	}
	if h.refresh(t, cookie).status != http.StatusOK {
		t.Fatal("a refused origin consumed the cookie")
	}
}

func TestLoginRejectsForeignOriginAndMissingContentType(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	body := `{"email":"tutor@example.org","password":"correct horse battery"}`

	foreign := h.send(t, http.MethodPost, "/api/v1/auth/login", body, map[string]string{"Origin": "https://attacker.example", "Content-Type": "application/json"})
	foreignInvalid := h.send(t, http.MethodPost, "/api/v1/auth/login", "{", map[string]string{"Origin": "https://attacker.example", "Content-Type": "application/json"})
	untyped := h.send(t, http.MethodPost, "/api/v1/auth/login", body, map[string]string{"Origin": publicOrigin})

	if foreign.status != http.StatusForbidden || foreignInvalid.status != http.StatusForbidden || untyped.status != http.StatusUnsupportedMediaType {
		t.Fatalf("foreign %d, untyped %d %s", foreign.status, untyped.status, untyped.body)
	}
}

func TestRefreshWithoutCookieAnswers204(t *testing.T) {
	h := newAuthHarness(t)

	result := h.refresh(t, "")

	if result.status != http.StatusNoContent || result.body != "" || result.headers.Get("Set-Cookie") != "" {
		t.Fatalf("got %d %q %v", result.status, result.body, result.headers)
	}
}

func TestAccessTokenExpiresByTheSharedClock(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	token := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))

	h.advance(t, 14*time.Minute)
	before := h.me(t, token)
	h.advance(t, 2*time.Minute)
	after := h.me(t, token)

	if before.status != http.StatusOK || after.status != http.StatusUnauthorized || code(t, after) != "token_expired" {
		t.Fatalf("before %d, after %d %s", before.status, after.status, after.body)
	}
}

func TestLogoutRevokesTheSessionAndLogoutAllEverySession(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	first := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))
	second := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))
	third := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))

	logout := h.send(t, http.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Authorization": "Bearer " + first})
	afterLogout := []int{h.me(t, first).status, h.me(t, second).status}
	logoutAll := h.send(t, http.MethodPost, "/api/v1/auth/logout-all", "", map[string]string{"Authorization": "Bearer " + second})
	afterAll := []int{h.me(t, second).status, h.me(t, third).status}

	if logout.status != http.StatusNoContent || !strings.Contains(logout.headers.Get("Set-Cookie"), "Max-Age=0") || logoutAll.status != http.StatusNoContent {
		t.Fatalf("logout %d %v, logout-all %d", logout.status, logout.headers, logoutAll.status)
	}
	if afterLogout[0] != http.StatusUnauthorized || afterLogout[1] != http.StatusOK || afterAll[0] != http.StatusUnauthorized || afterAll[1] != http.StatusUnauthorized {
		t.Fatalf("after logout %v, after logout-all %v", afterLogout, afterAll)
	}
}

func TestEveryOperationDeclaresAnAudience(t *testing.T) {
	h := newAuthHarness(t)
	document, err := api.OpenAPIDocument()
	if err != nil {
		t.Fatal(err)
	}
	var published struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(document, &published); err != nil {
		t.Fatal(err)
	}

	operations := api.Operations()

	declared := map[string]api.Audience{}
	for _, operation := range operations {
		declared[strings.ToUpper(operation.Method)+" "+operation.Path] = operation.Audience
	}
	for path, methods := range published.Paths {
		for method := range methods {
			if _, found := declared[strings.ToUpper(method)+" "+path]; !found {
				t.Fatalf("%s %s is published without a declared audience", method, path)
			}
		}
	}
	for _, operation := range operations {
		switch operation.Audience {
		case api.AudiencePublic, api.AudienceRefreshCookie:
		case api.AudienceOptionalUser:
			result := h.send(t, operation.Method, operation.Path, "{}", map[string]string{
				"Authorization": "Bearer forged", "Content-Type": "application/json", "Origin": publicOrigin,
			})
			if result.status != http.StatusUnauthorized || code(t, result) != "unauthenticated" {
				t.Fatalf("%s %s with a forged token answered %d %s", operation.Method, operation.Path, result.status, result.body)
			}
		case api.AudienceUser, api.AudienceUnverifiedOK:
			result := h.send(t, operation.Method, operation.Path, "", nil)
			if result.status != http.StatusUnauthorized || code(t, result) != "unauthenticated" || result.headers.Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("%s %s without a token answered %d %s", operation.Method, operation.Path, result.status, result.body)
			}
		default:
			t.Fatalf("%s declares no known audience: %q", operation.ID, operation.Audience)
		}
	}
}

func TestRefreshExpiresThirtyDaysAfterSignInWhateverTheRotations(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	cookie := refreshValue(t, h.login(t, tutorEmail, "correct horse battery"))
	h.advance(t, 29*24*time.Hour)
	rotated := h.refresh(t, cookie)

	h.advance(t, 24*time.Hour)
	expired := h.refresh(t, refreshValue(t, rotated))

	if rotated.status != http.StatusOK || !strings.Contains(rotated.headers.Get("Set-Cookie"), "Max-Age=86400") {
		t.Fatalf("rotation on day 29: %d %v", rotated.status, rotated.headers.Values("Set-Cookie"))
	}
	if expired.status != http.StatusUnauthorized {
		t.Fatalf("refresh on day 30 answered %d", expired.status)
	}
}

func TestBlockedAccountLosesRefreshAndAccess(t *testing.T) {
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	signedIn := h.login(t, tutorEmail, "correct horse battery")
	if _, err := h.store.Repositories().Users.Block(context.Background(), user, authStart); err != nil {
		t.Fatal(err)
	}

	refresh := h.refresh(t, refreshValue(t, signedIn))
	me := h.me(t, accessToken(t, signedIn))

	if refresh.status != http.StatusForbidden || code(t, refresh) != "account_blocked" || !strings.Contains(refresh.headers.Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("refresh %d %s %v", refresh.status, refresh.body, refresh.headers)
	}
	if me.status != http.StatusForbidden || code(t, me) != "account_blocked" {
		t.Fatalf("me %d %s", me.status, me.body)
	}
}

func TestConcurrentRefreshesWithOneCookieRotateOnceAndRevokeAsReuse(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	cookie := refreshValue(t, h.login(t, tutorEmail, "correct horse battery"))
	statuses := make([]int, 2)
	cookies := make([]string, 2)

	var wait sync.WaitGroup
	for index := range statuses {
		wait.Go(func() {
			status, value, err := h.quietRefresh(cookie)
			if err != nil {
				t.Error(err)
			}
			statuses[index], cookies[index] = status, value
		})
	}
	wait.Wait()

	winner := -1
	for index, status := range statuses {
		if status == http.StatusOK {
			winner = index
		}
	}
	if winner < 0 || statuses[1-winner] != http.StatusUnauthorized {
		t.Fatalf("concurrent refreshes answered %v", statuses)
	}
	if after := h.refresh(t, cookies[winner]); after.status != http.StatusUnauthorized {
		t.Fatalf("the session survived a reused cookie: %d", after.status)
	}
}

func TestAccountFlowsLogNoSecretsOrEmails(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	signedIn := h.login(t, tutorEmail, "correct horse battery")
	token := accessToken(t, signedIn)
	cookie := refreshValue(t, signedIn)

	rotated := refreshValue(t, h.refresh(t, cookie))
	h.refresh(t, cookie)
	h.login(t, tutorEmail, "wrong horse battery")
	h.send(t, http.MethodPost, "/api/v1/auth/logout-all", "", map[string]string{"Authorization": "Bearer " + token})

	logs := h.logs.String()
	for _, secret := range []string{tutorEmail, "correct horse battery", "wrong horse battery", token, cookie, rotated, strings.Split(cookie, ".")[2]} {
		if strings.Contains(logs, secret) {
			t.Fatalf("the logs contain %q", secret)
		}
	}
	for _, event := range []string{"auth.login.succeeded", "auth.refresh.reuse_detected", "auth.login.failed"} {
		if !strings.Contains(logs, `"msg":"`+event+`"`) {
			t.Fatalf("no %s record", event)
		}
	}
}

func TestClientAddressWalksForwardedHeadersFromTheRightPastTrustedProxies(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("127.0.0.1/32")}
	cases := map[string]struct {
		remote    string
		forwarded []string
		want      string
	}{
		"direct client":               {remote: "203.0.113.5:4000", forwarded: []string{"198.51.100.1"}, want: "203.0.113.5"},
		"one trusted proxy":           {remote: "10.0.0.2:4000", forwarded: []string{"198.51.100.1"}, want: "198.51.100.1"},
		"spoofed left entry":          {remote: "10.0.0.2:4000", forwarded: []string{"1.2.3.4, 198.51.100.1"}, want: "198.51.100.1"},
		"chain of trusted proxies":    {remote: "127.0.0.1:4000", forwarded: []string{"198.51.100.1, 10.0.0.9"}, want: "198.51.100.1"},
		"several header lines":        {remote: "10.0.0.2:4000", forwarded: []string{"1.2.3.4", "198.51.100.1, 10.0.0.3"}, want: "198.51.100.1"},
		"unparsable entry":            {remote: "10.0.0.2:4000", forwarded: []string{"198.51.100.1, garbage"}, want: "10.0.0.2"},
		"trusted peer without header": {remote: "10.0.0.2:4000", want: "10.0.0.2"},
		"mapped IPv4 peer":            {remote: "[::ffff:203.0.113.5]:4000", want: "203.0.113.5"},
		"unparsable peer":             {remote: "not-an-address", want: "invalid IP"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			got, known := api.ClientAddress(testCase.remote, testCase.forwarded, trusted)

			if got.String() != testCase.want || known != (testCase.want != "invalid IP") {
				t.Fatalf("got %s, want %s", got, testCase.want)
			}
		})
	}
}

func TestEveryPublishedOperationDescribesTheErrorEnvelope(t *testing.T) {
	document, err := api.OpenAPIDocument()
	if err != nil {
		t.Fatal(err)
	}
	var published struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]json.RawMessage `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
	}

	decodeErr := json.Unmarshal(document, &published)

	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	for path, methods := range published.Paths {
		for method, operation := range methods {
			described := false
			for status, response := range operation.Responses {
				if (status == "default" || strings.HasPrefix(status, "4") || strings.HasPrefix(status, "5")) && response.Content["application/json"] != nil {
					described = true
				}
			}
			if !described {
				t.Errorf("%s %s publishes no error response", method, path)
			}
		}
	}
}

func TestAThrottledAddressDoesNotLockTheVictimOutElsewhere(t *testing.T) {
	h := newAuthHarnessBehind(t, []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	from := func(address string, email string, password string) response {
		body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		return h.send(t, http.MethodPost, "/api/v1/auth/login", body, map[string]string{
			"Origin": publicOrigin, "Content-Type": "application/json", "X-Forwarded-For": address,
		})
	}
	for attempt := range 20 {
		from("198.51.100.7", fmt.Sprintf("person%d@example.org", attempt), "wrong horse battery")
	}

	var attackerStatuses []int
	for range 6 {
		attackerStatuses = append(attackerStatuses, from("198.51.100.7", tutorEmail, "wrong horse battery").status)
	}
	victim := from("203.0.113.9", tutorEmail, "correct horse battery")

	for _, status := range attackerStatuses {
		if status != http.StatusTooManyRequests {
			t.Fatalf("the exhausted address answered %v", attackerStatuses)
		}
	}
	if victim.status != http.StatusOK {
		t.Fatalf("the victim from another address answered %d %s", victim.status, victim.body)
	}
}

func TestAPasswordChangedDuringVerificationStartsNoSession(t *testing.T) {
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	h.counting.afterVerify = func() {
		replacement, err := h.hasher.Hash(context.Background(), "a brand new password")
		if err != nil {
			t.Error(err)
			return
		}
		current, err := h.store.Repositories().Users.ByID(context.Background(), user)
		if err != nil {
			t.Error(err)
			return
		}
		if _, err := h.store.Repositories().Users.UpdatePasswordHash(context.Background(), repositories.PasswordRehash{
			PasswordHash: replacement, Now: authStart, ID: user, CurrentPasswordHash: current.PasswordHash,
		}); err != nil {
			t.Error(err)
		}
	}

	result := h.login(t, tutorEmail, "correct horse battery")

	if result.status != http.StatusUnauthorized || code(t, result) != "invalid_credentials" || result.headers.Get("Set-Cookie") != "" {
		t.Fatalf("a sign-in raced by a password change answered %d %s", result.status, result.body)
	}
	if !strings.Contains(h.logs.String(), `"reason":"changed"`) {
		t.Fatal("no changed record")
	}
}

func TestAStaleHashRehashLostToAPasswordChangeStartsNoSession(t *testing.T) {
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", core.Argon2Params{MemoryKiB: 32, Iterations: 1, Parallelism: 1})
	changed := false
	h.counting.afterVerify = func() {
		if changed {
			return
		}
		changed = true
		replacement, err := h.hasher.Hash(context.Background(), "a brand new password")
		if err != nil {
			t.Error(err)
			return
		}
		current, err := h.store.Repositories().Users.ByID(context.Background(), user)
		if err != nil {
			t.Error(err)
			return
		}
		if _, err := h.store.Repositories().Users.UpdatePasswordHash(context.Background(), repositories.PasswordRehash{
			PasswordHash: replacement, Now: authStart, ID: user, CurrentPasswordHash: current.PasswordHash,
		}); err != nil {
			t.Error(err)
		}
	}

	result := h.login(t, tutorEmail, "correct horse battery")

	if result.status != http.StatusUnauthorized || code(t, result) != "invalid_credentials" || result.headers.Get("Set-Cookie") != "" {
		t.Fatalf("a stale sign-in raced by a password change answered %d %s", result.status, result.body)
	}
	if verified := h.counting.verifications.Load(); verified != 2 {
		t.Fatalf("%d password verifications, want the second against the replaced hash", verified)
	}
	if !strings.Contains(h.logs.String(), `"reason":"changed"`) {
		t.Fatal("no changed record")
	}
}

func TestABlockDuringVerificationStartsNoSession(t *testing.T) {
	h := newAuthHarness(t)
	user := h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	h.counting.afterVerify = func() {
		if _, err := h.store.Repositories().Users.Block(context.Background(), user, authStart); err != nil {
			t.Error(err)
		}
	}

	result := h.login(t, tutorEmail, "correct horse battery")

	if result.status != http.StatusForbidden || code(t, result) != "account_blocked" || result.headers.Get("Set-Cookie") != "" {
		t.Fatalf("a sign-in raced by a block answered %d %s", result.status, result.body)
	}
	if !strings.Contains(h.logs.String(), `"reason":"blocked"`) {
		t.Fatal("no blocked record")
	}
}

func TestBusyHashingDoesNotSpendSignInAttempts(t *testing.T) {
	h := newAuthHarness(t)
	h.seedUser(t, 1, tutorEmail, "correct horse battery", cheapArgon2())
	h.counting.failure = core.ErrHashingBusy
	var statuses []int
	for range 6 {
		statuses = append(statuses, h.login(t, tutorEmail, "correct horse battery").status)
	}

	h.counting.failure = nil
	result := h.login(t, tutorEmail, "correct horse battery")

	for _, status := range statuses {
		if status != http.StatusServiceUnavailable {
			t.Fatalf("busy hashing answered %v", statuses)
		}
	}
	if result.status != http.StatusOK {
		t.Fatalf("after busy hashing the sign-in answered %d %s", result.status, result.body)
	}
}
