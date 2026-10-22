package startup_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"testing"
	"time"

	"ovc/backend/internal/domain"
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
