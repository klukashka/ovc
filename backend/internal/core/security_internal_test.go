package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type countingBytes struct {
	next byte
}

func (r *countingBytes) Read(buffer []byte) (int, error) {
	for index := range buffer {
		r.next++
		buffer[index] = r.next
	}
	return len(buffer), nil
}

func TestHashingWaitsBoundedThenReportsBusyOrTheCallersCancellation(t *testing.T) {
	hasher, err := NewPasswordHasher(Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}, &countingBytes{}, 1, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	hasher.slots <- struct{}{}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	_, busy := hasher.Hash(context.Background(), "password")
	_, abandoned := hasher.Hash(cancelled, "password")

	if !errors.Is(busy, ErrHashingBusy) || errors.Is(abandoned, ErrHashingBusy) || !errors.Is(abandoned, context.Canceled) {
		t.Fatalf("busy %v, abandoned %v", busy, abandoned)
	}
}

func TestAccessTokensSignedWithAnotherHMACMethodAreRefused(t *testing.T) {
	keys, err := DeriveKeys(NewSecret("token-signing-material-0123456789ab"))
	if err != nil {
		t.Fatal(err)
	}
	tokens := NewAccessTokens(keys, "https://ovc.example", 15*time.Minute)
	now := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	claims := accessClaims{Session: "02000000000000000000000000000000", RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "https://ovc.example", Subject: "01000000000000000000000000000000", Audience: jwt.ClaimStrings{accessAudience},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
	}}

	var results []error
	for _, method := range []jwt.SigningMethod{jwt.SigningMethodHS384, jwt.SigningMethodHS512} {
		signed, signErr := jwt.NewWithClaims(method, claims).SignedString(keys.access())
		if signErr != nil {
			t.Fatal(signErr)
		}
		_, verifyErr := tokens.Verify(signed, now)
		results = append(results, verifyErr)
	}

	for _, result := range results {
		if !errors.Is(result, ErrAccessTokenInvalid) {
			t.Fatalf("another HMAC method was accepted: %v", results)
		}
	}
}
