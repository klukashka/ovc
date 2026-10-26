package listener_test

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func entityIDs(signals []domain.Signal) []domain.EntityID {
	var ids []domain.EntityID
	for _, signal := range signals {
		if entity, ok := signal.(domain.EntitySignal); ok {
			ids = append(ids, entity.ID)
		}
	}
	return ids
}

func waitForEntity(t *testing.T, f *fixture, id domain.EntityID) []domain.Signal {
	t.Helper()
	events := f.recorder.waitFor(t, "an entity signal", func(events []event) bool {
		return slices.Contains(entityIDs(signalsOf(events)), id)
	})
	return signalsOf(events)
}

var errRollBack = errors.New("roll back on purpose")

type gate struct {
	wait chan struct{}
	once sync.Once
}

func (g *gate) open() {
	g.once.Do(func() { close(g.wait) })
}

func releaseOnCleanup(t *testing.T) *gate {
	t.Helper()
	release := &gate{wait: make(chan struct{})}
	t.Cleanup(release.open)
	return release
}

func TestSignalsAreDeliveredOnlyAfterCommit(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	held := domain.EntityID{1}
	signalled := make(chan struct{})
	release := releaseOnCleanup(t)
	committed := make(chan error, 1)
	go func() {
		committed <- f.store.WithTx(context.Background(), func(r repositories.Repositories) error {
			if err := r.Signals.Entity(context.Background(), []domain.UserID{{1}}, domain.EntityRelationship, held); err != nil {
				return err
			}
			close(signalled)
			<-release.wait
			return nil
		})
	}()
	<-signalled

	beforeCommit := signalsOf(f.sentinel(t))
	release.open()
	if err := <-committed; err != nil {
		t.Fatal(err)
	}
	afterCommit := waitForEntity(t, f, held)

	if slices.Contains(entityIDs(beforeCommit), held) {
		t.Fatal("the signal was delivered before its transaction committed")
	}
	if !slices.Contains(entityIDs(afterCommit), held) {
		t.Fatal("the signal was not delivered after the commit")
	}
}

func TestARolledBackTransactionSendsNoSignal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	rolledBack := domain.EntityID{2}

	err := f.store.WithTx(context.Background(), func(r repositories.Repositories) error {
		if err := r.Signals.Entity(context.Background(), []domain.UserID{{1}}, domain.EntityRelationship, rolledBack); err != nil {
			return err
		}
		return errRollBack
	})
	delivered := signalsOf(f.sentinel(t))

	if !errors.Is(err, errRollBack) {
		t.Fatalf("the transaction ended with %v", err)
	}
	if len(delivered) != 0 {
		t.Fatalf("a rolled-back transaction delivered %d signals", len(delivered))
	}
}

func TestSignalsArriveInCommitOrder(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	first, second := domain.EntityID{3}, domain.EntityID{4}
	signalled := make(chan struct{})
	release := releaseOnCleanup(t)
	committed := make(chan error, 1)
	go func() {
		committed <- f.store.WithTx(context.Background(), func(r repositories.Repositories) error {
			if err := r.Signals.Entity(context.Background(), []domain.UserID{{1}}, domain.EntityJoinRequest, first); err != nil {
				return err
			}
			close(signalled)
			<-release.wait
			return nil
		})
	}()
	<-signalled

	err := f.store.WithTx(context.Background(), func(r repositories.Repositories) error {
		return r.Signals.Entity(context.Background(), []domain.UserID{{1}}, domain.EntityJoinRequest, second)
	})
	release.open()
	if err := errors.Join(err, <-committed); err != nil {
		t.Fatal(err)
	}
	delivered := entityIDs(waitForEntity(t, f, first))

	if !slices.Equal(delivered, []domain.EntityID{second, first}) {
		t.Fatalf("signals arrived as %v, want the later-started but earlier-committed one first", delivered)
	}
}

func TestLargeRecipientListsAreSplitUnderThePayloadLimit(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	raw := listenRaw(t, f)
	recipients := make([]domain.UserID, 0, 251)
	for index := range 250 {
		recipients = append(recipients, domain.UserID{byte(index), byte(index >> 8), 0xaa})
	}
	recipients = append(recipients, recipients[0])
	target := domain.EntityID{5}

	err := f.store.Repositories().Signals.Entity(context.Background(), recipients, domain.EntityRelationship, target)
	payloads := []string{raw.next(t), raw.next(t), raw.next(t)}
	events := f.recorder.waitFor(t, "three entity signals", func(events []event) bool {
		return len(entityIDs(signalsOf(events))) == 3
	})

	if err != nil {
		t.Fatal(err)
	}
	var delivered []domain.UserID
	var sizes []int
	for _, signal := range signalsOf(events) {
		entity, ok := signal.(domain.EntitySignal)
		if !ok || entity.ID != target {
			t.Fatalf("unexpected signal %T", signal)
		}
		sizes = append(sizes, len(entity.Users))
		delivered = append(delivered, entity.Users...)
	}
	if !slices.Equal(sizes, []int{100, 100, 50}) || !slices.Equal(delivered, recipients[:250]) {
		t.Fatalf("recipients were split into %v", sizes)
	}
	for _, payload := range payloads {
		if len(payload) > domain.MaxSignalBytes {
			t.Fatalf("a payload holds %d bytes", len(payload))
		}
	}
}

func TestTriggerPayloadsParseWithTheListenersParser(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	raw := listenRaw(t, f)
	revoked, blocked, deleted, removed := domain.UserID{1}, domain.UserID{2}, domain.UserID{3}, domain.UserID{4}
	for _, user := range []domain.UserID{revoked, blocked, deleted, removed} {
		f.createUser(t, user, domain.AccountActive)
	}
	f.createSession(t, domain.SessionID{1}, revoked)
	sessions, users := f.store.Repositories().Sessions, f.store.Repositories().Users

	_, revokeErr := sessions.Revoke(context.Background(), domain.SessionID{1}, instant)
	_, blockErr := users.Block(context.Background(), blocked, instant)
	_, deleteErr := users.MarkDeleted(context.Background(), deleted, instant)
	f.exec(t, "DELETE FROM users WHERE id = $1::uuid", removed.String())
	payloads := []string{raw.next(t), raw.next(t), raw.next(t), raw.next(t)}
	delivered := rechecksOf(signalsOf(f.sentinel(t)))

	if err := errors.Join(revokeErr, blockErr, deleteErr); err != nil {
		t.Fatal(err)
	}
	for index, user := range []domain.UserID{revoked, blocked, deleted, removed} {
		parsed, err := domain.ParseSignal(payloads[index])
		recheck, ok := parsed.(domain.RecheckSignal)
		if err != nil || !ok || !slices.Equal(recheck.Users, []domain.UserID{user}) {
			t.Fatalf("trigger payload %d does not parse as a recheck of its account: %v", index, err)
		}
	}
	if !slices.EqualFunc(delivered, [][]domain.UserID{{revoked}, {blocked}, {deleted}, {removed}}, slices.Equal) {
		t.Fatalf("the listener delivered rechecks %v", delivered)
	}
	if rejected := f.logs.records(t, "realtime.signal.rejected"); len(rejected) != 0 {
		t.Fatalf("the listener rejected %d trigger payloads", len(rejected))
	}
}

func TestRevokingEverySessionOfAnAccountSendsOneSignal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := domain.UserID{1}
	f.createUser(t, user, domain.AccountActive)
	for index := range 3 {
		f.createSession(t, domain.SessionID{byte(index + 1)}, user)
	}

	revoked, err := f.store.Repositories().Sessions.RevokeAllOf(context.Background(), user, instant)
	delivered := rechecksOf(signalsOf(f.sentinel(t)))

	if err != nil || revoked != 3 {
		t.Fatalf("revoked %d sessions: %v", revoked, err)
	}
	if !slices.EqualFunc(delivered, [][]domain.UserID{{user}}, slices.Equal) {
		t.Fatalf("revoking every session delivered %v", delivered)
	}
}

func TestRevocationTriggersFireOnlyOnTheirTransitions(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	active, unverified, blocked := domain.UserID{1}, domain.UserID{2}, domain.UserID{3}
	f.createUser(t, active, domain.AccountActive)
	f.createUser(t, unverified, domain.AccountUnverified)
	f.createUser(t, blocked, domain.AccountActive)
	f.createSession(t, domain.SessionID{1}, active)
	f.createSession(t, domain.SessionID{2}, active)
	if _, err := f.store.Repositories().Sessions.Revoke(context.Background(), domain.SessionID{2}, instant); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Repositories().Users.Block(context.Background(), blocked, instant); err != nil {
		t.Fatal(err)
	}
	f.sentinel(t)
	mark, _ := f.recorder.snapshot()

	f.exec(t, "UPDATE sessions SET last_used_at = last_used_at + interval '1 minute' WHERE revoked_at IS NULL AND user_id = $1::uuid", active.String())
	f.exec(t, "UPDATE sessions SET revoked_at = revoked_at + interval '1 minute' WHERE revoked_at IS NOT NULL AND user_id = $1::uuid", active.String())
	f.exec(t, "UPDATE users SET name = 'Another name', state = state WHERE id = $1::uuid", active.String())
	if confirmed, err := f.store.Repositories().Users.ConfirmEmail(context.Background(), unverified, instant); err != nil || !confirmed {
		t.Fatalf("confirming answered %v, %v", confirmed, err)
	}
	f.exec(t, "UPDATE users SET state = state WHERE id = $1::uuid", blocked.String())
	f.exec(t, "UPDATE users SET state = 'active', state_before_block = NULL WHERE id = $1::uuid", blocked.String())
	delivered := signalsOf(f.sentinel(t)[len(mark):])

	if len(delivered) != 0 {
		t.Fatalf("transitions that revoke nothing delivered %d signals", len(delivered))
	}
}

func TestMalformedSignalsAreRejectedAndLogged(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := "aaaaaaaa-2222-4333-8444-55555555555b"
	malformed := map[string]string{
		"not JSON":                     "not json",
		"an array":                     `[1]`,
		"another version":              `{"v":2,"k":"recheck","u":["` + user + `"]}`,
		"a string version":             `{"v":"1","k":"recheck","u":["` + user + `"]}`,
		"an unknown kind":              `{"v":1,"k":"revoke","u":["` + user + `"]}`,
		"an unknown field":             `{"v":1,"k":"recheck","u":["` + user + `"],"token":"x"}`,
		"a duplicate field":            `{"v":1,"k":"recheck","u":["` + user + `"],"u":["` + user + `"]}`,
		"trailing data":                `{"v":1,"k":"recheck","u":["` + user + `"]} {}`,
		"an uppercase identifier":      `{"v":1,"k":"recheck","u":["` + strings.ToUpper(user) + `"]}`,
		"no recipients":                `{"v":1,"k":"recheck","u":[]}`,
		"null recipients":              `{"v":1,"k":"recheck","u":null}`,
		"repeated recipients":          `{"v":1,"k":"recheck","u":["` + user + `","` + user + `"]}`,
		"a recheck with an entity":     `{"v":1,"k":"recheck","u":["` + user + `"],"e":"relationship"}`,
		"a notification for two":       `{"v":1,"k":"notification","u":["` + user + `","aaaaaaaa-2222-4333-8444-55555555555c"],"i":"` + user + `"}`,
		"an entity of an unknown kind": `{"v":1,"k":"entity","u":["` + user + `"],"e":"lesson","i":"` + user + `"}`,
		"a heartbeat with recipients":  `{"v":1,"k":"heartbeat","u":["` + user + `"],"i":"` + user + `"}`,
		"an oversized payload":         `{"v":1,"k":"recheck","u":["` + user + `"],"pad":"` + strings.Repeat("x", domain.MaxSignalBytes) + `"}`,
	}
	for _, payload := range malformed {
		f.exec(t, "SELECT pg_notify('ovc_realtime', $1)", payload)
	}

	delivered := signalsOf(f.sentinel(t))

	if len(delivered) != 0 {
		t.Fatalf("%d malformed signals were delivered", len(delivered))
	}
	rejected := f.logs.records(t, "realtime.signal.rejected")
	if len(rejected) != len(malformed) {
		t.Fatalf("%d rejections were logged for %d malformed signals", len(rejected), len(malformed))
	}
	for _, record := range rejected {
		if string(record["level"]) != `"WARN"` || len(record["reason"]) < 3 {
			t.Fatalf("a rejection record has level %s and reason %s", record["level"], record["reason"])
		}
	}
	for _, leaked := range []string{user, "token", "revoke", "not json"} {
		if strings.Contains(f.logs.String(), leaked) {
			t.Fatalf("the log carries payload content %q", leaked)
		}
	}
	if generation, live := f.listener.State(); generation != 1 || !live {
		t.Fatalf("the listener is at generation %d, live %v", generation, live)
	}
}

var signalPayloadShape = regexp.MustCompile(
	`^\{"v" ?: ?1, ?"k" ?: ?"(recheck|notification|entity|heartbeat)"` +
		`(, ?"u" ?: ?\[("[0-9a-f-]{36}"(, ?)?)+\])?(, ?"e" ?: ?"[a-z_]+")?(, ?"i" ?: ?"[0-9a-f-]{36}")?\}$`,
)

func TestSignalPayloadsCarryNoTokenOrPersonalData(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	raw := listenRaw(t, f)
	user := domain.UserID{7}
	f.createUser(t, user, domain.AccountActive)
	f.createSession(t, domain.SessionID{7}, user)
	stored, err := f.store.Repositories().Users.ByID(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	signals := f.store.Repositories().Signals

	sendErr := errors.Join(
		signals.Notification(context.Background(), user, domain.NotificationID{1}),
		signals.Entity(context.Background(), []domain.UserID{user}, domain.EntityRelationship, domain.EntityID{1}),
		listener.ConnectionHeartbeats{}.Heartbeat(context.Background(), raw.connection, domain.InstanceID{1}),
	)
	_, revokeErr := f.store.Repositories().Sessions.Revoke(context.Background(), domain.SessionID{7}, instant)
	_, blockErr := f.store.Repositories().Users.Block(context.Background(), user, instant)
	f.exec(t, "DELETE FROM users WHERE id = $1::uuid", user.String())
	var payloads []string
	for range 6 {
		payloads = append(payloads, raw.next(t))
	}

	if err := errors.Join(sendErr, revokeErr, blockErr); err != nil {
		t.Fatal(err)
	}
	secrets := []string{stored.Email, stored.Name, stored.PasswordHash}
	for _, payload := range payloads {
		if !signalPayloadShape.MatchString(payload) {
			t.Fatalf("a payload carries more than identifiers: %s", payload)
		}
		for _, secret := range secrets {
			if strings.Contains(payload, secret) {
				t.Fatal("a payload carries personal data")
			}
		}
	}
}
