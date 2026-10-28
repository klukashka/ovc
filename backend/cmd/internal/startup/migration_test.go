package startup_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"log/slog"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/tutoring"
	"ovc/backend/internal/testdb"
)

const (
	lastSlice2Migration = 3
	insertSlice2User    = `INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme,
    consent_version, consent_at, unverified_since, created_at, updated_at)
VALUES ($1, $2, 'active', $3, $4, '$argon2id$v=19$m=19456,t=2,p=1$fixture', 'An account', 'en', 'Europe/Moscow', 'system',
    '2026-10', $4, $4, $4, $4)`
	insertSlice2Profile = `INSERT INTO tutor_profiles (user_id, file_quota_bytes, recording_quota_bytes, created_at, updated_at)
VALUES ($1, 1, 1, $2, $2)`
)

func TestMigrateGivesExistingTutorsAValidCode(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.NewMigratedTo(t, lastSlice2Migration)
	created := time.Date(2026, time.October, 21, 9, 0, 0, 0, time.UTC)
	tutors := []domain.UserID{{1}, {2}, {3}, {4}, {5}}
	for index, tutor := range tutors {
		if _, err := database.Pool.Exec(context.Background(), insertSlice2User, tutor, "tutor", "tutor"+string(rune('a'+index))+"@example.org", created); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Pool.Exec(context.Background(), insertSlice2Profile, tutor, created); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Pool.Exec(context.Background(), insertSlice2User, domain.UserID{9}, "student", "student@example.org", created); err != nil {
		t.Fatal(err)
	}

	migrated := runBinary(t, "api", []string{"OVC_DATABASE_URL=" + database.URL.Reveal()}, "migrate")

	if migrated.exitCode != 0 {
		t.Fatalf("migrate: %d %s", migrated.exitCode, migrated.stderr)
	}
	rows, err := database.Pool.Query(context.Background(), "SELECT code FROM tutor_profiles")
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			t.Fatal(err)
		}
		if normalised, valid := domain.NormaliseTutorCode(code); !valid || string(normalised) != code {
			t.Errorf("tutor code %q is not a canonical code", code)
		}
		codes[code] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(codes) != len(tutors) {
		t.Fatalf("%d distinct codes for %d tutors", len(codes), len(tutors))
	}
}

func tutorCodeSource(user domain.UserID) io.Reader {
	seed := sha256.Sum256(user[:])
	return bytes.NewReader(bytes.Repeat(seed[:], 4))
}

const (
	lastSlice3SchemaMigration = 4
	insertInterimUser         = `INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme,
    consent_version, consent_at, unverified_since, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, '$argon2id$v=19$m=19456,t=2,p=1$fixture', 'An account', 'en', 'Europe/Moscow', 'system',
    '2026-10', $6, $6, $6, $6)`
	insertInterimProfile = `INSERT INTO tutor_profiles (user_id, code, file_quota_bytes, recording_quota_bytes, created_at, updated_at)
VALUES ($1, 'ABCDEFGH', 1, 1, $2, $2)`
	insertInterimLink = `INSERT INTO relationships (id, tutor_id, student_id, state, display_name, colour, linked_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'A student', 'clay', $5, $5, $5)`
)

type interimSeed struct {
	tutor       domain.UserID
	confirmed   domain.UserID
	unconfirmed domain.UserID
	active      domain.UserID
	reclaimed   domain.RelationshipID
	provisional domain.RelationshipID
	kept        domain.RelationshipID
}

func seedInterimLinks(t *testing.T, database *testdb.Database) interimSeed {
	t.Helper()
	created := time.Date(2026, time.October, 22, 9, 0, 0, 0, time.UTC)
	seed := interimSeed{
		tutor: domain.UserID{1}, confirmed: domain.UserID{2}, unconfirmed: domain.UserID{3}, active: domain.UserID{4},
		reclaimed: domain.RelationshipID{1}, provisional: domain.RelationshipID{2}, kept: domain.RelationshipID{3},
	}
	users := []struct {
		id       domain.UserID
		role     string
		state    string
		email    string
		verified *time.Time
	}{
		{seed.tutor, "tutor", "active", "tutor@example.org", &created},
		{seed.confirmed, "student", "active", "confirmed@example.org", &created},
		{seed.unconfirmed, "student", "unverified", "unconfirmed@example.org", nil},
		{seed.active, "student", "active", "active@example.org", &created},
	}
	for _, user := range users {
		if _, err := database.Pool.Exec(context.Background(), insertInterimUser, user.id, user.role, user.state, user.email, user.verified, created); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Pool.Exec(context.Background(), insertInterimProfile, seed.tutor, created); err != nil {
		t.Fatal(err)
	}
	links := []struct {
		id      domain.RelationshipID
		student domain.UserID
		state   string
	}{
		{seed.reclaimed, seed.confirmed, "provisional"},
		{seed.provisional, seed.unconfirmed, "provisional"},
		{seed.kept, seed.active, "active"},
	}
	for _, link := range links {
		if _, err := database.Pool.Exec(context.Background(), insertInterimLink, link.id, seed.tutor, link.student, link.state, created); err != nil {
			t.Fatal(err)
		}
	}
	return seed
}

func migrateInterim(t *testing.T, database *testdb.Database) (time.Time, time.Time) {
	t.Helper()
	var before, after time.Time
	if err := database.Pool.QueryRow(context.Background(), "SELECT now()").Scan(&before); err != nil {
		t.Fatal(err)
	}
	migrated := runBinary(t, "api", []string{"OVC_DATABASE_URL=" + database.URL.Reveal()}, "migrate")
	if migrated.exitCode != 0 {
		t.Fatalf("migrate: %d %s", migrated.exitCode, migrated.stderr)
	}
	if err := database.Pool.QueryRow(context.Background(), "SELECT now()").Scan(&after); err != nil {
		t.Fatal(err)
	}
	return before, after
}

func linkState(t *testing.T, database *testdb.Database, id domain.RelationshipID) string {
	t.Helper()
	var state string
	if err := database.Pool.QueryRow(context.Background(), "SELECT state FROM relationships WHERE id = $1", id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func reclaimUntil(t *testing.T, database *testdb.Database, user domain.UserID) *time.Time {
	t.Helper()
	var deadline *time.Time
	if err := database.Pool.QueryRow(context.Background(), "SELECT reclaim_until FROM users WHERE id = $1", user).Scan(&deadline); err != nil {
		t.Fatal(err)
	}
	return deadline
}

func TestInterimMigrationMovesProvisionalLinksOfConfirmedAccounts(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.NewMigratedTo(t, lastSlice3SchemaMigration)
	seed := seedInterimLinks(t, database)

	before, after := migrateInterim(t, database)

	if state := linkState(t, database, seed.reclaimed); state != "reclaim_pending" {
		t.Fatalf("the confirmed account's provisional link is %s", state)
	}
	if linkState(t, database, seed.provisional) != "provisional" || linkState(t, database, seed.kept) != "active" {
		t.Fatal("the migration changed a link it does not concern")
	}
	deadline := reclaimUntil(t, database, seed.confirmed)
	if deadline == nil || deadline.Before(before.Add(domain.ReclaimWindow)) || deadline.After(after.Add(domain.ReclaimWindow)) {
		t.Fatalf("the reclaim deadline is %v, migrated between %v and %v", deadline, before, after)
	}
	for _, user := range []domain.UserID{seed.unconfirmed, seed.active, seed.tutor} {
		if reclaimUntil(t, database, user) != nil {
			t.Fatalf("the migration gave account %s a deadline", user)
		}
	}
}

func TestInterimProvisionalLinkOfAConfirmedAccountShowsNoAccount(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.NewMigratedTo(t, lastSlice3SchemaMigration)
	seed := seedInterimLinks(t, database)
	migrateInterim(t, database)
	fieldKey := sha256.Sum256([]byte("interim-field-key"))
	cipher, err := core.NewFieldCipher(core.NewSecret(base64.StdEncoding.EncodeToString(fieldKey[:])), core.CryptoRandom())
	if err != nil {
		t.Fatal(err)
	}
	service := tutoring.New(tutoring.Dependencies{
		Store: repositories.NewStore(database.Pool), Clock: clock.System{}, Random: core.CryptoRandom(), Cipher: cipher,
		PublicURL: "https://ovc.example", Logger: slog.New(slog.DiscardHandler),
	})

	reclaimed, reclaimedErr := service.Card(context.Background(), seed.tutor, seed.reclaimed)
	provisional, provisionalErr := service.Card(context.Background(), seed.tutor, seed.provisional)

	if reclaimedErr != nil || reclaimed.State != domain.RelationshipReclaimPending || reclaimed.Account != nil {
		t.Fatalf("the interim link's card is %+v (%v)", reclaimed, reclaimedErr)
	}
	if provisionalErr != nil || provisional.State != domain.RelationshipProvisional || provisional.Account == nil || provisional.Account.EmailConfirmed {
		t.Fatalf("the unconfirmed account's card is %+v (%v)", provisional, provisionalErr)
	}
}
