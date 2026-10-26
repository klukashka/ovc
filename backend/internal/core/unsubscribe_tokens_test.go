package core_test

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

var (
	sentinelUnsubscribeSecret = sentinel("unsubscribe-signing-material-01")
	unsubscribeUser           = domain.UserID{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x01}
)

func unsubscribeTokens(t *testing.T, secret string) core.UnsubscribeTokens {
	t.Helper()
	built, err := core.NewUnsubscribeTokens(core.NewSecret(secret))
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func referenceUnsubscribeToken(t *testing.T, secret string, payload []byte) string {
	t.Helper()
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, "unsubscribe", 32)
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return "v1." + base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func unsubscribePayload(user domain.UserID, group byte, expiry uint64) []byte {
	payload := append(bytes.Clone(user[:]), group)
	return binary.BigEndian.AppendUint64(payload, expiry)
}

func unixSeconds(instant time.Time) uint64 {
	seconds := instant.Unix()
	if seconds < 0 {
		return 0
	}
	return uint64(seconds)
}

func replacedPart(token string, index int, replacement string) string {
	parts := strings.Split(token, ".")
	parts[index] = replacement
	return strings.Join(parts, ".")
}

func flippedPayloadByte(t *testing.T, token string, index int) string {
	t.Helper()
	payload, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
	if err != nil {
		t.Fatal(err)
	}
	payload[index] ^= 0x01
	return replacedPart(token, 1, base64.RawURLEncoding.EncodeToString(payload))
}

func TestUnsubscribeTokenRoundTripsAndRefusesTampering(t *testing.T) {
	tokens := unsubscribeTokens(t, sentinelUnsubscribeSecret)
	homeworkCode, _ := domain.EmailGroupHomework.TokenCode()
	expiry := unixSeconds(tokenNow.Add(90 * 24 * time.Hour))
	minted, mintErr := tokens.Mint(unsubscribeUser, domain.EmailGroupHomework, tokenNow)
	if mintErr != nil {
		t.Fatal(mintErr)
	}
	otherSecret := unsubscribeTokens(t, sentinel("another-unsubscribe-material-01"))
	foreign, foreignErr := otherSecret.Mint(unsubscribeUser, domain.EmailGroupHomework, tokenNow)
	if foreignErr != nil {
		t.Fatal(foreignErr)
	}
	accessKeys := keys(t, sentinelUnsubscribeSecret)
	signatureCases := map[string]string{
		"another secret":        foreign,
		"a flipped mac byte":    replacedPart(minted, 2, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, 32))),
		"another account":       flippedPayloadByte(t, minted, 0),
		"another group":         flippedPayloadByte(t, minted, 16),
		"a later expiry":        flippedPayloadByte(t, minted, 24),
		"another purpose's key": replacedPart(minted, 2, base64.RawURLEncoding.EncodeToString(accessKeys.ThrottleSubject("unsubscribe", ""))),
	}
	malformedCases := map[string]string{
		"empty":                   "",
		"another version":         replacedPart(minted, 0, "v2"),
		"padded":                  minted + "=",
		"standard base64":         replacedPart(minted, 1, base64.StdEncoding.EncodeToString(unsubscribePayload(unsubscribeUser, homeworkCode, expiry))),
		"a fourth part":           minted + ".x",
		"two parts":               strings.Join(strings.Split(minted, ".")[:2], "."),
		"a short payload":         replacedPart(minted, 1, base64.RawURLEncoding.EncodeToString(make([]byte, 24))),
		"a truncated mac":         replacedPart(minted, 2, strings.Split(minted, ".")[2][:42]),
		"an unknown group":        referenceUnsubscribeToken(t, sentinelUnsubscribeSecret, unsubscribePayload(unsubscribeUser, 7, expiry)),
		"an expiry beyond int64":  referenceUnsubscribeToken(t, sentinelUnsubscribeSecret, unsubscribePayload(unsubscribeUser, homeworkCode, 1<<63)),
		"an oversized token text": minted + strings.Repeat("A", 200),
	}

	claims, verifyErr := tokens.Verify(minted, tokenNow)
	signatureResults := map[string]error{}
	for name, token := range signatureCases {
		_, signatureResults[name] = tokens.Verify(token, tokenNow)
	}
	malformedResults := map[string]error{}
	for name, token := range malformedCases {
		_, malformedResults[name] = tokens.Verify(token, tokenNow)
	}

	if verifyErr != nil || claims.UserID != unsubscribeUser || claims.Group != domain.EmailGroupHomework || !claims.ExpiresAt.Equal(tokenNow.Add(90*24*time.Hour)) {
		t.Fatalf("the minted token verified as %+v, %v", claims, verifyErr)
	}
	pinned := map[domain.EmailGroup]byte{
		domain.EmailGroupLessonReminders: 1, domain.EmailGroupLessonChanges: 2, domain.EmailGroupHomework: 3,
		domain.EmailGroupResults: 4, domain.EmailGroupSubmissions: 5, domain.EmailGroupMessageDigest: 6,
	}
	for group, want := range pinned {
		if got, known := group.TokenCode(); !known || got != want {
			t.Fatalf("the group %s is coded %d in tokens instead of %d", group, got, want)
		}
	}
	if minted != referenceUnsubscribeToken(t, sentinelUnsubscribeSecret, unsubscribePayload(unsubscribeUser, homeworkCode, expiry)) {
		t.Fatalf("the token %q does not follow the layout and the HKDF-derived HMAC-SHA-256", minted)
	}
	for name, err := range signatureResults {
		if !errors.Is(err, core.ErrUnsubscribeTokenSignature) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, err := range malformedResults {
		if !errors.Is(err, core.ErrUnsubscribeTokenMalformed) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

const base64URLAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

func withTrailingBitSet(t *testing.T, token string, index int) string {
	t.Helper()
	part := strings.Split(token, ".")[index]
	last := strings.IndexByte(base64URLAlphabet, part[len(part)-1])
	respelled := part[:len(part)-1] + string(base64URLAlphabet[last|1])
	original, originalErr := base64.RawURLEncoding.DecodeString(part)
	lenient, lenientErr := base64.RawURLEncoding.DecodeString(respelled)
	if originalErr != nil || lenientErr != nil || !bytes.Equal(original, lenient) || respelled == part {
		t.Fatalf("the respelled part %q is not a second spelling of %q", respelled, part)
	}
	return replacedPart(token, index, respelled)
}

func withInserted(token string, index int, at int, inserted string) string {
	part := strings.Split(token, ".")[index]
	return replacedPart(token, index, part[:at]+inserted+part[at:])
}

func TestUnsubscribeTokenRefusesNewlinesAndNonZeroTrailingBits(t *testing.T) {
	tokens := unsubscribeTokens(t, sentinelUnsubscribeSecret)
	minted, err := tokens.Mint(unsubscribeUser, domain.EmailGroupHomework, tokenNow)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"a newline in the payload":           withInserted(minted, 1, 10, "\n"),
		"a carriage return in the payload":   withInserted(minted, 1, 10, "\r"),
		"a newline in the mac":               withInserted(minted, 2, 20, "\n"),
		"a trailing newline":                 minted + "\n",
		"a space in the payload":             withInserted(minted, 1, 10, " "),
		"trailing bits set in the payload":   withTrailingBitSet(t, minted, 1),
		"trailing bits set in the mac":       withTrailingBitSet(t, minted, 2),
		"a newline before the mac separator": withInserted(minted, 1, len(strings.Split(minted, ".")[1]), "\n"),
	}

	results := map[string]error{}
	for name, token := range cases {
		_, results[name] = tokens.Verify(token, tokenNow)
	}

	for name, verifyErr := range results {
		if !errors.Is(verifyErr, core.ErrUnsubscribeTokenMalformed) {
			t.Errorf("%s: %v", name, verifyErr)
		}
	}
}

func TestUnsubscribeTokenExpiresAfter90Days(t *testing.T) {
	tokens := unsubscribeTokens(t, sentinelUnsubscribeSecret)
	minted, err := tokens.Mint(unsubscribeUser, domain.EmailGroupLessonReminders, tokenNow)
	if err != nil {
		t.Fatal(err)
	}
	var zero core.UnsubscribeTokens

	_, lastSecond := tokens.Verify(minted, tokenNow.Add(90*24*time.Hour-time.Second))
	expiredClaims, atExpiry := tokens.Verify(minted, tokenNow.Add(90*24*time.Hour))
	_, unset := zero.Verify(minted, tokenNow)
	_, unsetMint := zero.Mint(unsubscribeUser, domain.EmailGroupLessonReminders, tokenNow)

	if lastSecond != nil {
		t.Fatalf("the token was refused a second before 90 days: %v", lastSecond)
	}
	if !errors.Is(atExpiry, core.ErrUnsubscribeTokenExpired) || expiredClaims.UserID != unsubscribeUser || expiredClaims.Group != domain.EmailGroupLessonReminders {
		t.Fatalf("at 90 days: %+v %v", expiredClaims, atExpiry)
	}
	if !errors.Is(unset, core.ErrUnsubscribeTokensUnset) || !errors.Is(unsetMint, core.ErrUnsubscribeTokensUnset) {
		t.Fatalf("a signer without a key verified %v and minted %v", unset, unsetMint)
	}
}

func TestUnsubscribeSigningSecretIsRedactedInEveryFormat(t *testing.T) {
	settings, err := core.Load(lookupFrom(validValues()), core.ProcessAPI)
	if err != nil {
		t.Fatal(err)
	}
	built, err := core.NewUnsubscribeTokens(settings.UnsubscribeKey)
	if err != nil {
		t.Fatal(err)
	}
	derived, err := hkdf.Key(sha256.New, []byte(sentinelUnsubscribeSecret), nil, "unsubscribe", 32)
	if err != nil {
		t.Fatal(err)
	}
	holder := struct {
		settings core.Settings
		tokens   core.UnsubscribeTokens
	}{settings: settings, tokens: built}
	var logged, text bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logged, nil))
	textLogger := slog.New(slog.NewTextHandler(&text, nil))

	var formatted strings.Builder
	for _, verb := range []string{"v", "s", "q", "x", "X", "d", "p"} {
		for _, flag := range []string{"", "+", "#"} {
			format := "%" + flag + verb + " "
			writeFormatted(t, &formatted, format, settings)
			writeFormatted(t, &formatted, format, settings.UnsubscribeKey)
			writeFormatted(t, &formatted, format, built)
			writeFormatted(t, &formatted, format, holder)
		}
	}
	encoded, encodeErr := json.Marshal(settings)
	logger.Info("unsubscribe key", slog.Any("settings", settings), slog.Any("key", settings.UnsubscribeKey), slog.Any("tokens", built))
	textLogger.Info("unsubscribe key", slog.Any("holder", holder), slog.Any("tokens", built))

	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	outputs := map[string]string{"fmt": formatted.String(), "json": string(encoded), "slog json": logged.String(), "slog text": text.String()}
	for name, output := range outputs {
		for _, secret := range []string{sentinelUnsubscribeSecret, fmt.Sprintf("%x", sentinelUnsubscribeSecret), fmt.Sprintf("%x", derived), string(derived)} {
			if strings.Contains(output, secret) {
				t.Fatalf("%s output exposes the unsubscribe signing secret", name)
			}
		}
	}
	if settings.UnsubscribeKey.Reveal() != sentinelUnsubscribeSecret {
		t.Fatal("the secret must stay available to Reveal")
	}
}

func TestUnsubscribeSigningSecretIsRequiredByTheAPIAndTheWorker(t *testing.T) {
	cases := map[string]func(values map[string]string){
		"missing": func(values map[string]string) { delete(values, "OVC_UNSUBSCRIBE_SIGNING_SECRET") },
		"empty":   func(values map[string]string) { values["OVC_UNSUBSCRIBE_SIGNING_SECRET"] = "" },
		"short": func(values map[string]string) {
			values["OVC_UNSUBSCRIBE_SIGNING_SECRET"] = sentinel("short-unsubscribe-key")
		},
		"equal to the JWT secret": func(values map[string]string) { values["OVC_UNSUBSCRIBE_SIGNING_SECRET"] = values["OVC_JWT_SECRET"] },
		"equal to the field key": func(values map[string]string) {
			values["OVC_UNSUBSCRIBE_SIGNING_SECRET"] = values["OVC_FIELD_ENCRYPTION_KEY"]
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			change(values)
			adminValues := validValues()
			delete(adminValues, "OVC_UNSUBSCRIBE_SIGNING_SECRET")

			refusals := map[core.Process]error{}
			for _, process := range []core.Process{core.ProcessAPI, core.ProcessWorker} {
				_, refusals[process] = core.Load(lookupFrom(values), process)
			}
			_, adminErr := core.Load(lookupFrom(adminValues), core.ProcessAdmin)

			for process, refusal := range refusals {
				var settingsError *core.SettingsError
				if !errors.As(refusal, &settingsError) || settingsError.Variable != "OVC_UNSUBSCRIBE_SIGNING_SECRET" {
					t.Fatalf("%s: expected a refusal naming the secret, got %v", process, refusal)
				}
				if strings.Contains(refusal.Error(), "sentinel") || strings.Contains(refusal.Error(), sentinelFieldKey) {
					t.Fatalf("%s: the refusal shows a value", process)
				}
			}
			if adminErr != nil {
				t.Fatalf("the administration command needed the secret: %v", adminErr)
			}
		})
	}
}
