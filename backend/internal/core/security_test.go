package core_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/core"
)

type sequenceRandom struct {
	next byte
}

func (r *sequenceRandom) Read(buffer []byte) (int, error) {
	for index := range buffer {
		r.next++
		buffer[index] = r.next
	}
	return len(buffer), nil
}

var tokenNow = time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)

func hasher(t *testing.T, params core.Argon2Params) *core.PasswordHasher {
	t.Helper()
	created, err := core.NewPasswordHasher(params, &sequenceRandom{}, 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func keys(t *testing.T, secret string) core.Keys {
	t.Helper()
	derived, err := core.DeriveKeys(core.NewSecret(secret))
	if err != nil {
		t.Fatal(err)
	}
	return derived
}

func TestProductionArgon2ParametersFollowOWASP(t *testing.T) {
	params := core.ProductionArgon2Params()

	matches := params == core.Argon2Params{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1}

	if !matches {
		t.Fatalf("production parameters %+v", params)
	}
}

func TestPasswordHashVerifiesOnlyTheRightPasswordAndReportsStaleParameters(t *testing.T) {
	cheap := core.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}
	encoded, err := hasher(t, cheap).Hash(context.Background(), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}

	right, rightErr := hasher(t, cheap).Verify(context.Background(), "correct horse battery", encoded)
	wrong, wrongErr := hasher(t, cheap).Verify(context.Background(), "correct horse batterz", encoded)
	stale, staleErr := hasher(t, core.Argon2Params{MemoryKiB: 128, Iterations: 1, Parallelism: 1}).Verify(context.Background(), "correct horse battery", encoded)

	if rightErr != nil || wrongErr != nil || staleErr != nil || !right.Matches || right.Stale || wrong.Matches || !stale.Matches || !stale.Stale {
		t.Fatalf("right %+v, wrong %+v, stale %+v", right, wrong, stale)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatalf("encoded %q", encoded)
	}
}

func TestMalformedPasswordHashesAreRefused(t *testing.T) {
	cheap := hasher(t, core.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1})
	salt := base64.RawStdEncoding.EncodeToString(make([]byte, 16))
	key := base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	cases := []string{
		"",
		"plain-text-password",
		"$argon2i$v=19$m=64,t=1,p=1$" + salt + "$" + key,
		"$argon2id$v=18$m=64,t=1,p=1$" + salt + "$" + key,
		"$argon2id$v=19$m=4194304,t=1,p=1$" + salt + "$" + key,
		"$argon2id$v=19$m=64,t=99,p=1$" + salt + "$" + key,
		"$argon2id$v=19$m=64,t=1,p=0$" + salt + "$" + key,
		"$argon2id$v=19$m=64,t=1,p=1$" + salt + "$" + key[:20],
		"$argon2id$v=19$m=64,t=1,p=1$" + salt + "=$" + key,
	}

	for _, encoded := range cases {
		_, err := cheap.Verify(context.Background(), "password", encoded)

		if err == nil {
			t.Errorf("accepted %q", encoded)
		}
	}
}

func TestAccessTokensRefuseForgeriesAndExpireWithLeeway(t *testing.T) {
	tokens := core.NewAccessTokens(keys(t, "token-signing-material-0123456789ab"), "https://ovc.example", 15*time.Minute)
	other := core.NewAccessTokens(keys(t, "other-signing-material-0123456789ab"), "https://ovc.example", 15*time.Minute)
	foreignIssuer := core.NewAccessTokens(keys(t, "token-signing-material-0123456789ab"), "https://attacker.example", 15*time.Minute)
	claims := core.AccessClaims{UserID: [16]byte{1}, SessionID: [16]byte{2}}
	issue := func(source *core.AccessTokens) string {
		token, err := source.Issue(claims, tokenNow)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	valid := issue(tokens)
	parts := strings.Split(valid, ".")
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + parts[1] + "."

	verified, err := tokens.Verify(valid, tokenNow.Add(15*time.Minute+4*time.Second))
	_, expired := tokens.Verify(valid, tokenNow.Add(15*time.Minute+6*time.Second))
	forgeries := []error{}
	for _, forged := range []string{issue(other), issue(foreignIssuer), unsigned, parts[0] + "." + parts[1], "garbage"} {
		_, forgedErr := tokens.Verify(forged, tokenNow)
		forgeries = append(forgeries, forgedErr)
	}

	if err != nil || verified != claims {
		t.Fatalf("within leeway: %+v, %v", verified, err)
	}
	if !errors.Is(expired, core.ErrAccessTokenExpired) {
		t.Fatalf("past leeway: %v", expired)
	}
	for _, forgedErr := range forgeries {
		if !errors.Is(forgedErr, core.ErrAccessTokenInvalid) {
			t.Fatalf("forgeries: %v", forgeries)
		}
	}
}

func TestDerivedKeysNeverPrintAndSeparatePurposes(t *testing.T) {
	derived := keys(t, "key-printing-material-0123456789abcd")
	var logged bytes.Buffer
	slog.New(slog.NewTextHandler(&logged, nil)).Info("keys", slog.Any("keys", derived))

	formatted := fmt.Sprintf("%v %+v %#v %s", derived, derived, derived, derived) + logged.String()

	if strings.Count(formatted, "[redacted]") != 5 {
		t.Fatalf("keys formatted as %q", formatted)
	}
	if bytes.Equal(derived.ThrottleSubject("a", "x"), derived.ThrottleSubject("b", "x")) ||
		bytes.Equal(derived.RefreshMAC([16]byte{1}, 1), derived.RefreshMAC([16]byte{1}, 2)) {
		t.Fatal("derived values do not separate purposes or generations")
	}
}
