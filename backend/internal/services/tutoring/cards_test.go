package tutoring_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/tutoring"
	"ovc/backend/internal/testdb"
)

var subMicrosecond = time.Date(2030, time.March, 10, 9, 0, 0, 1234567, time.UTC)

func fixturePasswordHash() string {
	digest := sha256.Sum256([]byte("fixture"))
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%x", digest)
}

type fixedClock struct {
	instant time.Time
}

func (c fixedClock) Instant(context.Context) (time.Time, error) {
	return c.instant, nil
}

type cardFixture struct {
	database *testdb.Database
	store    *repositories.Store
	service  *tutoring.Service
	tutor    domain.UserID
}

func newCardFixture(t *testing.T, instant time.Time) cardFixture {
	t.Helper()
	database := testdb.New(t)
	store := repositories.NewStore(database.Pool)
	fieldKey := sha256.Sum256([]byte("test-field-key"))
	cipher, err := core.NewFieldCipher(core.NewSecret(base64.StdEncoding.EncodeToString(fieldKey[:])), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service := tutoring.New(tutoring.Dependencies{
		Store: store, Clock: fixedClock{instant: instant}, Random: rand.Reader, Keys: keys, Cipher: cipher,
		PublicURL: "https://ovc.example", Logger: slog.New(slog.DiscardHandler),
	})
	tutor := domain.UserID{1}
	err = store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: tutor, Role: domain.RoleTutor, State: domain.AccountActive, Email: "tutor@example.org", EmailVerifiedAt: &instant,
		PasswordHash: fixturePasswordHash(), Name: "A language tutor",
		Language: domain.LanguageEnglish, Timezone: "Europe/Moscow", Theme: domain.ThemeSystem, ConsentVersion: "2026-10",
		ConsentAt: instant, CreatedAt: instant,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := repositories.NewTutorProfile{UserID: tutor, FileQuotaBytes: 1, RecordingQuotaBytes: 1, CreatedAt: instant}
	if err := store.Repositories().TutorProfiles.Create(context.Background(), profile, bytes.NewReader([]byte{2, 3, 4, 5, 6, 7, 8, 9})); err != nil {
		t.Fatal(err)
	}
	return cardFixture{database: database, store: store, service: service, tutor: tutor}
}

func TestCreatedCardMatchesItsStoredRowAtASubMicrosecondInstant(t *testing.T) {
	f := newCardFixture(t, subMicrosecond)

	created, err := f.service.CreateCard(context.Background(), f.tutor, tutoring.NewCard{DisplayName: "A student"})
	if err != nil {
		t.Fatal(err)
	}
	read, err := f.service.Card(context.Background(), f.tutor, created.ID)

	if err != nil {
		t.Fatal(err)
	}
	if !created.CreatedAt.Equal(read.CreatedAt) || created.CreatedAt.Nanosecond()%1000 != 0 {
		t.Fatalf("created at %s, read back at %s", created.CreatedAt.Format(time.RFC3339Nano), read.CreatedAt.Format(time.RFC3339Nano))
	}
	if !created.Invitation.ExpiresAt.Equal(read.Invitation.ExpiresAt) || *created.Invitation.URL != *read.Invitation.URL {
		t.Fatalf("the created invitation %+v reads back as %+v", created.Invitation, read.Invitation)
	}
}

const (
	seedCards = `INSERT INTO relationships (id, tutor_id, state, display_name, created_at, updated_at)
SELECT gen_random_uuid(), $1, 'invited', 'A student', $2, $2 FROM generate_series(1, $3)`
	seedInvitations = `INSERT INTO invitations (id, relationship_id, token_hash, token_encrypted, expires_at, created_at)
SELECT gen_random_uuid(), id, sha256(convert_to(id::text, 'UTF8')), 'ciphertext', $2, $2 FROM relationships WHERE tutor_id = $1`
)

func (f cardFixture) seedExpiredInvitations(t *testing.T, count int, expiry time.Time) {
	t.Helper()
	if _, err := f.database.Pool.Exec(context.Background(), seedCards, f.tutor, expiry, count); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.Pool.Exec(context.Background(), seedInvitations, f.tutor, expiry); err != nil {
		t.Fatal(err)
	}
}

func TestInvitationExpiryClearsMoreThanOneBatchPerRun(t *testing.T) {
	expiry := subMicrosecond.Add(-time.Hour)
	f := newCardFixture(t, subMicrosecond)
	f.seedExpiredInvitations(t, 1203, expiry)

	cleared, err := f.service.ExpireInvitations(context.Background(), subMicrosecond)

	if err != nil {
		t.Fatal(err)
	}
	var remaining int64
	if err := f.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM invitations WHERE token_encrypted IS NOT NULL").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if cleared != 1203 || remaining != 0 {
		t.Fatalf("one run cleared %d ciphertexts and left %d", cleared, remaining)
	}
}
