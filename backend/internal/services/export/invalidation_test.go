package export_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/export"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/testdb"
)

const insertReadyExport = `INSERT INTO exports (id, user_id, state, language, timezone, requested_at, build_id, storage_key, build_started_at,
    built_at, size_bytes, expires_at, updated_at)
VALUES ($1, $2, 'ready', 'en', 'Europe/Moscow', $3, $4, $5, $3, $3, 1, $6, $3)`

const (
	invalidationDeadline = 10 * time.Second
	insertExportLimit    = "INSERT INTO export_limits (user_id, export_id, allowed_from) VALUES ($1, $2, $3)"
	lockExportOfUser     = "SELECT " + testdb.LockedRowPosition + " FROM exports WHERE user_id = $1::uuid FOR UPDATE"
)

var invalidationStart = time.Date(2030, time.April, 2, 9, 0, 0, 0, time.UTC)

type sharedRecords struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (r *sharedRecords) Write(written []byte) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.buffer.Write(written)
}

func (r *sharedRecords) String() string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.buffer.String()
}

type invalidationRecord struct {
	Message    string `json:"msg"`
	ExportID   string `json:"export_id"`
	Cause      string `json:"cause"`
	Users      int    `json:"users"`
	Locked     int    `json:"locked"`
	Withdrawn  int    `json:"withdrawn"`
	Statements int    `json:"statements"`
}

func (r *sharedRecords) named(t *testing.T, message string) []invalidationRecord {
	t.Helper()
	var found []invalidationRecord
	lines := bufio.NewScanner(strings.NewReader(r.String()))
	lines.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for lines.Scan() {
		var record invalidationRecord
		if err := json.Unmarshal(lines.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if record.Message == message {
			found = append(found, record)
		}
	}
	if err := lines.Err(); err != nil {
		t.Fatal(err)
	}
	return found
}

type invalidationFixture struct {
	database *testdb.Database
	store    *repositories.Store
	effects  export.Effects
	records  *sharedRecords
}

func newInvalidationFixture(t *testing.T, database *testdb.Database) invalidationFixture {
	t.Helper()
	store := repositories.NewStore(database.Pool)
	records := &sharedRecords{}
	logger := slog.New(slog.NewJSONHandler(records, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cleanup := files.New(files.Dependencies{Store: store, Clock: clock.System{}, Random: core.CryptoRandom(), Logger: logger})
	effects := export.NewEffects(export.Dependencies{Store: store, Wall: clock.System{}.Now, Random: core.CryptoRandom(), Logger: logger, Cleanup: cleanup})
	return invalidationFixture{database: database, store: store, effects: effects, records: records}
}

func fixturePasswordHash() string {
	digest := sha256.Sum256([]byte("fixture"))
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%x", digest)
}

func (f invalidationFixture) account(t *testing.T, user domain.UserID) {
	t.Helper()
	err := f.store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: user, Role: domain.RoleStudent, State: domain.AccountActive, Email: fmt.Sprintf("member-%x@example.org", user[:3]),
		EmailVerifiedAt: &invalidationStart, PasswordHash: fixturePasswordHash(), Name: "A student", Language: domain.LanguageEnglish,
		Timezone: "Europe/Moscow", Theme: domain.ThemeSystem, ConsentVersion: "2026-10", ConsentAt: invalidationStart, CreatedAt: invalidationStart,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f invalidationFixture) readyExport(t *testing.T, user domain.UserID) domain.ExportID {
	t.Helper()
	f.account(t, user)
	id := domain.ExportID{0xe0, user[0], user[1], user[2]}
	build := domain.ExportBuildID{0xb0, user[0], user[1], user[2]}
	_, err := f.database.Pool.Exec(context.Background(), insertReadyExport, id, user, invalidationStart, build, domain.ExportStorageKey(build),
		invalidationStart.Add(domain.ExportRetention))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.Pool.Exec(context.Background(), insertExportLimit, user, id, invalidationStart.Add(domain.ExportStartInterval)); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f invalidationFixture) invalidate(ctx context.Context, cause domain.ExportInvalidationCause, users ...domain.UserID) error {
	return f.store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := repos.InvalidateExports(cause, users...); err != nil {
			return err
		}
		return f.effects.Apply(ctx, repos, invalidationStart)
	})
}

func (f invalidationFixture) count(t *testing.T, query string, user domain.UserID) int64 {
	t.Helper()
	var counted int64
	if err := f.database.Pool.QueryRow(context.Background(), query, user).Scan(&counted); err != nil {
		t.Fatal(err)
	}
	return counted
}

func (f invalidationFixture) state(t *testing.T, user domain.UserID) (string, *string) {
	t.Helper()
	var state string
	var failure *string
	if err := f.database.Pool.QueryRow(context.Background(), "SELECT state, failure FROM exports WHERE user_id = $1", user).Scan(&state, &failure); err != nil {
		t.Fatal(err)
	}
	return state, failure
}

func manyUsers(count int) []domain.UserID {
	users := make([]domain.UserID, 0, count)
	for index := range count {
		users = append(users, domain.UserID{0x50, byte(index >> 8), byte(index)})
	}
	return users
}

func TestAnInvalidationOfManyUsersLocksThemInBatchesOfAThousand(t *testing.T) {
	t.Parallel()
	f := newInvalidationFixture(t, testdb.New(t))
	users := manyUsers(2500)
	holders := []domain.UserID{users[3], users[1500], users[2499]}
	for _, user := range holders {
		f.readyExport(t, user)
	}
	untouched := domain.UserID{0x60}
	f.readyExport(t, untouched)

	err := f.invalidate(context.Background(), domain.ExportCauseRelationshipEnded, users...)

	if err != nil {
		t.Fatal(err)
	}
	for _, user := range holders {
		if state, failure := f.state(t, user); state != string(domain.ExportFailed) || failure == nil || *failure != string(domain.ExportFailureWithdrawn) {
			t.Fatalf("the named export of %s is %s", user, state)
		}
	}
	if state, _ := f.state(t, untouched); state != string(domain.ExportReady) {
		t.Fatalf("an export nobody named is %s", state)
	}
	applied := f.records.named(t, "export.invalidation.applied")
	if len(applied) != 1 || applied[0].Statements != 3 || applied[0].Users != 2500 || applied[0].Locked != 3 || applied[0].Withdrawn != 3 {
		t.Fatalf("the applied records are %+v", applied)
	}
	if invalidated := f.records.named(t, "export.invalidated"); len(invalidated) != 3 || invalidated[0].Cause != string(domain.ExportCauseRelationshipEnded) {
		t.Fatalf("the invalidated records are %+v", invalidated)
	}
}

func TestInvalidationsNamingTheSameUsersInAnyOrderDoNotDeadlock(t *testing.T) {
	t.Parallel()
	f := newInvalidationFixture(t, testdb.NewForLockContention(t))
	first, second := domain.UserID{0x71}, domain.UserID{0x72}
	f.readyExport(t, first)
	f.readyExport(t, second)
	hold := f.database.HoldRow(t, lockExportOfUser, second.String())
	ascending := make(chan error, 1)
	go func() {
		ascending <- f.invalidate(context.Background(), domain.ExportCauseRelationshipEnded, first, second)
	}()
	ascendingBackend := f.database.AwaitDirectWaitersOn(t, hold.Row, 1)[0]

	descending := make(chan error, 1)
	go func() {
		descending <- f.invalidate(context.Background(), domain.ExportCauseRelationshipDiscarded, second, first)
	}()
	behindAscending := f.database.AwaitAdvisoryWaitersOf(t, ascendingBackend, 1)
	onTheHeldRow := f.database.DirectWaitersOn(t, hold.Row)
	hold.Release(t)
	results := awaitBoth(t, ascending, descending)

	if len(behindAscending) != 1 || behindAscending[0] == ascendingBackend {
		t.Fatalf("the descending transaction waits on %v, not on the ascending transaction %d", behindAscending, ascendingBackend)
	}
	if len(onTheHeldRow) != 1 || onTheHeldRow[0] != ascendingBackend {
		t.Fatalf("the held row of the higher identifier is awaited by %v, want only the ascending transaction %d", onTheHeldRow, ascendingBackend)
	}
	for _, err := range results {
		if err != nil {
			t.Fatalf("an invalidation failed instead of waiting for the other: %v", err)
		}
	}
	for _, user := range []domain.UserID{first, second} {
		if state, _ := f.state(t, user); state != string(domain.ExportFailed) {
			t.Fatalf("the export of %s is %s", user, state)
		}
	}
}

func awaitBoth(t *testing.T, first <-chan error, second <-chan error) []error {
	t.Helper()
	deadline := time.NewTimer(invalidationDeadline)
	defer deadline.Stop()
	var results []error
	for len(results) < 2 {
		select {
		case err := <-first:
			results = append(results, err)
		case err := <-second:
			results = append(results, err)
		case <-deadline.C:
			t.Fatal("an invalidation did not finish after the held row was released")
		}
	}
	return results
}

func TestAPurgeEntryOverridesAWithdrawalOfTheSameUser(t *testing.T) {
	t.Parallel()
	f := newInvalidationFixture(t, testdb.New(t))
	user := domain.UserID{0x81}
	f.readyExport(t, user)

	err := f.store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		if err := repos.InvalidateExports(domain.ExportCauseRelationshipEnded, user); err != nil {
			return err
		}
		return f.effects.Purge(context.Background(), repos, user, invalidationStart)
	})

	if err != nil {
		t.Fatal(err)
	}
	if rows := f.count(t, "SELECT count(id) FROM exports WHERE user_id = $1", user); rows != 0 {
		t.Fatal("the purge kept the export row")
	}
	if limits := f.count(t, "SELECT count(user_id) FROM export_limits WHERE user_id = $1", user); limits != 0 {
		t.Fatal("the purge kept the export limit row")
	}
	if notified := f.count(t, "SELECT count(id) FROM notifications WHERE user_id = $1", user); notified != 0 {
		t.Fatal("the purged account received an export notification")
	}
	if withdrawals := f.records.named(t, "export.invalidated"); len(withdrawals) != 0 {
		t.Fatalf("the purge recorded withdrawals %+v", withdrawals)
	}
	var queued int64
	if err := f.database.Pool.QueryRow(context.Background(), "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.KindExportObject)).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("the purge queued %d object deletions: %v", queued, err)
	}
}
