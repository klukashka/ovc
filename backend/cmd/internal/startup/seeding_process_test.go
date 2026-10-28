package startup_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
)

type seededAccount struct {
	User        string `json:"user"`
	Password    string `json:"password"`
	AccessToken string `json:"access_token"`
}

type seedingExchanges struct {
	created      exchange
	me           exchange
	refreshed    exchange
	signedIn     exchange
	session      exchange
	duplicate    exchange
	realAccount  exchange
	invalidInput exchange
}

func seedingCalls(t *testing.T, ctx context.Context, base string) seedingExchanges {
	t.Helper()
	jsonHeaders := map[string]string{"Content-Type": "application/json"}
	browserHeaders := map[string]string{"Content-Type": "application/json", "Origin": "https://ovc.example"}
	newTutor := `{"role":"tutor","name":"A seeded tutor","email":"Seeded@Example.org","language":"en","timezone":"Europe/Moscow"}`
	var exchanges seedingExchanges
	exchanges.created = call(t, ctx, http.MethodPost, base+"/__test__/accounts", newTutor, jsonHeaders)
	var account seededAccount
	if err := decodeJSON(exchanges.created.body, &account); err != nil {
		t.Fatalf("seeding answered %d %s", exchanges.created.status, exchanges.created.body)
	}
	exchanges.me = call(t, ctx, http.MethodGet, base+"/api/v1/users/me", "", map[string]string{"Authorization": "Bearer " + account.AccessToken})
	cookie, _, _ := strings.Cut(exchanges.created.header.Get("Set-Cookie"), ";")
	exchanges.refreshed = call(t, ctx, http.MethodPost, base+"/api/v1/auth/refresh", "", map[string]string{"Origin": "https://ovc.example", "Cookie": cookie})
	exchanges.signedIn = call(t, ctx, http.MethodPost, base+"/api/v1/auth/login", `{"email":"seeded@example.org","password":"`+account.Password+`"}`, browserHeaders)
	exchanges.session = call(t, ctx, http.MethodPost, base+"/__test__/session", `{"email":"seeded@example.org"}`, jsonHeaders)
	exchanges.duplicate = call(t, ctx, http.MethodPost, base+"/__test__/accounts", newTutor, jsonHeaders)
	exchanges.realAccount = call(t, ctx, http.MethodPost, base+"/__test__/session", `{"email":"tutor@example.org"}`, jsonHeaders)
	exchanges.invalidInput = call(t, ctx, http.MethodPost, base+"/__test__/accounts", `{"role":"operator","name":"","email":"x","language":"de","timezone":""}`, jsonHeaders)
	return exchanges
}

func startSeedingServer(t *testing.T, ctx context.Context, database *testdb.Database, flag string) (string, func() (string, error)) {
	t.Helper()
	address := freeAddress(t)
	env := []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=" + flag}
	server, output := startServer(t, ctx, env)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("server output:\n%s", output.String())
		}
	})
	waitForStatus(t, ctx, "http://"+address+"/api/v1/health/ready")
	stop := func() (string, error) {
		stopErr := errors.Join(server.Process.Signal(syscall.SIGTERM), server.Wait())
		return output.String(), stopErr
	}
	return "http://" + address, stop
}

func TestSeedingRoutesCreateConfirmedAccountsWithRealSessions(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.New(t)
	seedTutor(t, database, "correct horse battery")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	base, stop := startSeedingServer(t, ctx, database, "1")

	exchanges := seedingCalls(t, ctx, base)
	serverOutput, stopErr := stop()

	if exchanges.created.status != http.StatusOK || exchanges.created.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("seeding answered %d %s", exchanges.created.status, exchanges.created.body)
	}
	for _, attribute := range []string{"__Host-ovc_refresh=", "Path=/", "Max-Age=2592000", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(exchanges.created.header.Get("Set-Cookie"), attribute) {
			t.Fatalf("the refresh cookie lacks %s: %s", attribute, exchanges.created.header.Get("Set-Cookie"))
		}
	}
	if exchanges.me.status != http.StatusOK || !strings.Contains(exchanges.me.body, `"state":"active"`) || !strings.Contains(exchanges.me.body, `"role":"tutor"`) {
		t.Fatalf("the seeded token read the profile as %d %s", exchanges.me.status, exchanges.me.body)
	}
	if exchanges.refreshed.status != http.StatusOK || exchanges.signedIn.status != http.StatusOK || exchanges.session.status != http.StatusOK {
		t.Fatalf("refresh %d %s, real sign-in %d %s, seeded session %d %s", exchanges.refreshed.status, exchanges.refreshed.body,
			exchanges.signedIn.status, exchanges.signedIn.body, exchanges.session.status, exchanges.session.body)
	}
	if exchanges.duplicate.status != http.StatusConflict || exchanges.realAccount.status != http.StatusNotFound {
		t.Fatalf("a duplicate answered %d %s, an account with its own password %d %s", exchanges.duplicate.status, exchanges.duplicate.body,
			exchanges.realAccount.status, exchanges.realAccount.body)
	}
	for _, path := range []string{"body.role", "body.name", "body.email", "body.language"} {
		if exchanges.invalidInput.status != http.StatusUnprocessableEntity || !strings.Contains(exchanges.invalidInput.body, `"path":"`+path+`"`) {
			t.Fatalf("invalid input answered %d %s", exchanges.invalidInput.status, exchanges.invalidInput.body)
		}
	}
	if stopErr != nil {
		t.Fatalf("stopping the server: %v", stopErr)
	}
	unanswered := answeredElsewhere(serverOutput, exchanges.created, exchanges.me, exchanges.refreshed, exchanges.signedIn, exchanges.session,
		exchanges.duplicate, exchanges.realAccount, exchanges.invalidInput)
	if len(unanswered) != 0 {
		t.Fatalf("another process answered the requests %v", unanswered)
	}
}

func TestSeedingRoutesAreAbsentWithoutTestControl(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	base, stop := startSeedingServer(t, ctx, database, "0")
	jsonHeaders := map[string]string{"Content-Type": "application/json"}

	created := call(t, ctx, http.MethodPost, base+"/__test__/accounts",
		`{"role":"tutor","name":"A seeded tutor","email":"seeded@example.org","language":"en","timezone":"UTC"}`, jsonHeaders)
	session := call(t, ctx, http.MethodPost, base+"/__test__/session", `{"email":"seeded@example.org"}`, jsonHeaders)
	serverOutput, stopErr := stop()

	if created.status != http.StatusNotFound || session.status != http.StatusNotFound || created.header.Get("Set-Cookie") != "" {
		t.Fatalf("without the flag seeding answered %d %s and the session %d %s", created.status, created.body, session.status, session.body)
	}
	if stopErr != nil {
		t.Fatalf("stopping the server: %v", stopErr)
	}
	if unanswered := answeredElsewhere(serverOutput, created, session); len(unanswered) != 0 {
		t.Fatalf("another process answered the requests %v", unanswered)
	}
}

func seedThroughTheRoute(t *testing.T, ctx context.Context, base string, email string) (domain.UserID, string) {
	t.Helper()
	created := call(t, ctx, http.MethodPost, base+"/__test__/accounts",
		`{"role":"student","name":"A seeded student","email":"`+email+`","language":"ru","timezone":"Europe/Moscow"}`, map[string]string{"Content-Type": "application/json"})
	var account seededAccount
	if err := decodeJSON(created.body, &account); err != nil {
		t.Fatalf("seeding answered %d %s", created.status, created.body)
	}
	user, ok := domain.ParseUUID[domain.UserID](account.User)
	if !ok {
		t.Fatalf("seeding answered the user %q", account.User)
	}
	return user, account.Password
}

func TestSeedingHashesOnceWithProductionParametersAndRefusesChangedAccounts(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	base, stop := startSeedingServer(t, ctx, database, "1")
	users := repositories.NewStore(database.Pool).Repositories().Users
	_, password := seedThroughTheRoute(t, ctx, base, "kept@example.org")
	blocked, _ := seedThroughTheRoute(t, ctx, base, "blocked@example.org")
	deleted, _ := seedThroughTheRoute(t, ctx, base, "deleted@example.org")
	changed, _ := seedThroughTheRoute(t, ctx, base, "changed@example.org")
	now := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	for _, err := range []error{
		appliedOrMissing(users.Block(ctx, blocked, now)),
		appliedOrMissing(users.MarkDeleted(ctx, deleted, now)),
		appliedOrMissing(users.ReplacePassword(ctx, changed, "$argon2id$v=19$m=65536,t=1,p=1$c2FsdA$aGFzaA", now)),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	jsonHeaders := map[string]string{"Content-Type": "application/json"}

	signedIn := call(t, ctx, http.MethodPost, base+"/api/v1/auth/login", `{"email":"kept@example.org","password":"`+password+`"}`,
		map[string]string{"Content-Type": "application/json", "Origin": "https://ovc.example"})
	blockedSession := call(t, ctx, http.MethodPost, base+"/__test__/session", `{"email":"blocked@example.org"}`, jsonHeaders)
	deletedSession := call(t, ctx, http.MethodPost, base+"/__test__/session", `{"email":"deleted@example.org"}`, jsonHeaders)
	changedSession := call(t, ctx, http.MethodPost, base+"/__test__/session", `{"email":"changed@example.org"}`, jsonHeaders)
	kept, keptErr := users.ByEmail(ctx, "kept@example.org")
	other, otherErr := users.ByEmail(ctx, "blocked@example.org")
	serverOutput, stopErr := stop()

	if signedIn.status != http.StatusOK || blockedSession.status != http.StatusForbidden || !strings.Contains(blockedSession.body, "account_blocked") {
		t.Fatalf("real sign-in %d %s, blocked session %d %s", signedIn.status, signedIn.body, blockedSession.status, blockedSession.body)
	}
	if deletedSession.status != http.StatusNotFound || changedSession.status != http.StatusNotFound {
		t.Fatalf("deleted session %d %s, changed session %d %s", deletedSession.status, deletedSession.body, changedSession.status, changedSession.body)
	}
	params := core.ProductionArgon2Params()
	production := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$", params.MemoryKiB, params.Iterations, params.Parallelism)
	if keptErr != nil || otherErr != nil || kept.PasswordHash != other.PasswordHash || !strings.HasPrefix(kept.PasswordHash, production) {
		t.Fatalf("the seeded hashes differ, were rehashed by sign-in or lack the production parameters %s: %v %v", production, keptErr, otherErr)
	}
	if stopErr != nil {
		t.Fatalf("stopping the server: %v", stopErr)
	}
	if unanswered := answeredElsewhere(serverOutput, signedIn, blockedSession, deletedSession, changedSession); len(unanswered) != 0 {
		t.Fatalf("another process answered the requests %v", unanswered)
	}
}

func appliedOrMissing(applied bool, err error) error {
	if err != nil {
		return err
	}
	if !applied {
		return errors.New("the account change did not apply")
	}
	return nil
}
