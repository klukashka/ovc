package testhasher

import (
	"context"
	"testing"

	"ovc/backend/internal/core"
)

type Hasher interface {
	Hash(ctx context.Context, password string) (string, error)
	Verify(ctx context.Context, password string, encoded string) (core.Verification, error)
	VerifyNothing(ctx context.Context, password string) error
}

type Subject struct {
	Current Hasher
	Older   Hasher
	Foreign Hasher
}

type Scenario func(t *testing.T, subject Subject)

const (
	password      = "correct horse battery"
	wrongPassword = "wrong horse battery"
)

func hashed(t *testing.T, hasher Hasher, value string) string {
	t.Helper()
	encoded, err := hasher.Hash(context.Background(), value)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return encoded
}

var Contract = map[string]Scenario{
	"a hashed password matches and is current": func(t *testing.T, subject Subject) {
		encoded := hashed(t, subject.Current, password)

		verification, err := subject.Current.Verify(context.Background(), password, encoded)

		if err != nil || !verification.Matches || verification.Stale {
			t.Fatalf("got %+v, %v", verification, err)
		}
	},
	"a wrong password does not match": func(t *testing.T, subject Subject) {
		encoded := hashed(t, subject.Current, password)

		verification, err := subject.Current.Verify(context.Background(), wrongPassword, encoded)

		if err != nil || verification.Matches {
			t.Fatalf("got %+v, %v", verification, err)
		}
	},
	"a hash made with an older configuration matches and is stale": func(t *testing.T, subject Subject) {
		encoded := hashed(t, subject.Older, password)

		verification, err := subject.Current.Verify(context.Background(), password, encoded)

		if err != nil || !verification.Matches || !verification.Stale {
			t.Fatalf("got %+v, %v", verification, err)
		}
	},
	"two hashes of one password differ": func(t *testing.T, subject Subject) {
		first := hashed(t, subject.Current, password)

		second := hashed(t, subject.Current, password)

		if first == second {
			t.Fatal("two hashes of one password are equal")
		}
	},
	"verifying nothing succeeds": func(t *testing.T, subject Subject) {
		err := subject.Current.VerifyNothing(context.Background(), password)

		if err != nil {
			t.Fatalf("got %v", err)
		}
	},
	"a hash from another scheme is refused": func(t *testing.T, subject Subject) {
		encoded := hashed(t, subject.Foreign, password)

		verification, err := subject.Current.Verify(context.Background(), password, encoded)

		if err == nil && verification.Matches {
			t.Fatal("a hash from another scheme verified")
		}
	},
	"a malformed hash is refused": func(t *testing.T, subject Subject) {
		_, err := subject.Current.Verify(context.Background(), password, "not a password hash")

		if err == nil {
			t.Fatal("a malformed hash was accepted")
		}
	},
}
