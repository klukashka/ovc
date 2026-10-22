package core_test

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

var (
	sentinelFieldKeyBytes = []byte(sentinel("field-key-0123456789abc"))
	sentinelFieldKey      = base64.StdEncoding.EncodeToString(sentinelFieldKeyBytes)
)

func fieldCipher(t *testing.T, keyBytes []byte, random core.RandomSource) core.FieldCipher {
	t.Helper()
	built, err := core.NewFieldCipher(core.NewSecret(base64.StdEncoding.EncodeToString(keyBytes)), random)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func flippedAt(sealed []byte, index int) []byte {
	changed := bytes.Clone(sealed)
	changed[index] ^= 0x01
	return changed
}

func referenceSeal(t *testing.T, keyBytes []byte, nonce []byte, additionalData []byte, plaintext []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	return aead.Seal(bytes.Clone(nonce), nonce, plaintext, additionalData)
}

func TestFieldCipherRoundTripsAndRefusesTampering(t *testing.T) {
	firstNonce := bytes.Repeat([]byte{7}, 12)
	secondNonce := bytes.Repeat([]byte{8}, 12)
	sealer := fieldCipher(t, sentinelFieldKeyBytes, bytes.NewReader(append(bytes.Clone(firstNonce), secondNonce...)))
	otherKey := fieldCipher(t, bytes.Repeat([]byte{9}, 32), bytes.NewReader(nil))
	invitation := domain.InvitationID{1, 2, 3}
	plaintext := []byte("invitation-token-plaintext")
	expected := referenceSeal(t, sentinelFieldKeyBytes, firstNonce, append([]byte("ovc.invitation-token.v1"), invitation[:]...), plaintext)
	withoutLabel := referenceSeal(t, sentinelFieldKeyBytes, firstNonce, invitation[:], plaintext)

	first, firstErr := sealer.SealForInvitation(invitation, plaintext)
	second, secondErr := sealer.SealForInvitation(invitation, plaintext)
	opened, openErr := sealer.OpenForInvitation(invitation, first)
	_, wrongInvitationErr := sealer.OpenForInvitation(domain.InvitationID{1, 2, 4}, first)
	_, wrongKeyErr := otherKey.OpenForInvitation(invitation, first)
	_, unlabelledErr := sealer.OpenForInvitation(invitation, withoutLabel)
	tampered := map[string][]byte{
		"nonce":      flippedAt(first, 0),
		"ciphertext": flippedAt(first, 12),
		"tag":        flippedAt(first, len(first)-1),
		"truncated":  first[:len(first)-1],
		"tag only":   first[12:],
		"empty":      {},
	}

	if firstErr != nil || secondErr != nil || openErr != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("round trip: %v %v %v %q", firstErr, secondErr, openErr, opened)
	}
	if !bytes.Equal(first, expected) {
		t.Fatal("the sealed value must be the nonce, then AES-256-GCM over the label and the invitation identifier")
	}
	if !bytes.Equal(second[:12], secondNonce) {
		t.Fatal("every seal must read a fresh nonce from the injected random source")
	}
	if len(first) != 12+len(plaintext)+16 || bytes.Contains(first, plaintext) {
		t.Fatalf("sealed value has %d bytes or carries the plaintext", len(first))
	}
	for name, err := range map[string]error{"wrong invitation": wrongInvitationErr, "wrong key": wrongKeyErr, "without the label": unlabelledErr} {
		if !errors.Is(err, core.ErrFieldCiphertextInvalid) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for name, sealed := range tampered {
		if result, err := sealer.OpenForInvitation(invitation, sealed); !errors.Is(err, core.ErrFieldCiphertextInvalid) || result != nil {
			t.Fatalf("%s: opened %q, %v", name, result, err)
		}
	}
}

func TestFieldCipherRefusesAnInvalidKeyAFailingRandomSourceAndAZeroValue(t *testing.T) {
	invalidKeys := []string{"", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 16)), "not base64"}
	broken := fieldCipher(t, sentinelFieldKeyBytes, bytes.NewReader(nil))
	var zero core.FieldCipher

	var keyErrors []error
	for _, key := range invalidKeys {
		_, err := core.NewFieldCipher(core.NewSecret(key), bytes.NewReader(nil))
		keyErrors = append(keyErrors, err)
	}
	_, sealErr := broken.SealForInvitation(domain.InvitationID{1}, []byte("token"))
	_, zeroSealErr := zero.SealForInvitation(domain.InvitationID{1}, []byte("token"))
	_, zeroOpenErr := zero.OpenForInvitation(domain.InvitationID{1}, make([]byte, 64))

	for index, err := range keyErrors {
		if !errors.Is(err, core.ErrFieldKeyInvalid) {
			t.Fatalf("key of %d characters: %v", len(invalidKeys[index]), err)
		}
	}
	if sealErr == nil {
		t.Fatal("a seal without a nonce must fail")
	}
	if !errors.Is(zeroSealErr, core.ErrFieldCipherUnset) || !errors.Is(zeroOpenErr, core.ErrFieldCipherUnset) {
		t.Fatalf("zero value: %v, %v", zeroSealErr, zeroOpenErr)
	}
}

func TestFieldEncryptionKeyIsRedactedInEveryFormat(t *testing.T) {
	settings, err := core.Load(lookupFrom(validValues()), core.ProcessWorker)
	if err != nil {
		t.Fatal(err)
	}
	built, err := core.NewFieldCipher(settings.FieldKey, bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	holder := struct {
		settings core.Settings
		cipher   core.FieldCipher
	}{settings: settings, cipher: built}
	var logged bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logged, nil))
	var text bytes.Buffer
	textLogger := slog.New(slog.NewTextHandler(&text, nil))

	var formatted strings.Builder
	for _, verb := range []string{"v", "s", "q", "x", "X", "d", "p"} {
		for _, flag := range []string{"", "+", "#"} {
			format := "%" + flag + verb + " "
			writeFormatted(t, &formatted, format, settings)
			writeFormatted(t, &formatted, format, settings.FieldKey)
			writeFormatted(t, &formatted, format, built)
			writeFormatted(t, &formatted, format, holder)
		}
	}
	encoded, encodeErr := json.Marshal(settings)
	logger.Info("field key", slog.Any("settings", settings), slog.Any("key", settings.FieldKey), slog.Any("cipher", built))
	textLogger.Info("field key", slog.Any("holder", holder), slog.Any("cipher", built))

	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	outputs := map[string]string{"fmt": formatted.String(), "json": string(encoded), "slog json": logged.String(), "slog text": text.String()}
	for name, output := range outputs {
		for _, secret := range []string{sentinelFieldKey, string(sentinelFieldKeyBytes), fmt.Sprintf("%x", sentinelFieldKeyBytes)} {
			if strings.Contains(output, secret) {
				t.Fatalf("%s output exposes the field encryption key", name)
			}
		}
	}
	if settings.FieldKey.Reveal() != sentinelFieldKey {
		t.Fatal("the key must stay available to Reveal")
	}
}

func TestFieldEncryptionKeyIsRequiredByTheAPIAndTheWorker(t *testing.T) {
	cases := map[string]string{
		"empty":           "",
		"short":           base64.StdEncoding.EncodeToString([]byte(sentinel("short-key"))),
		"long":            base64.StdEncoding.EncodeToString([]byte(sentinel("long-key-0123456789abcdefghijklmnopqrstuv"))),
		"not base64":      sentinel("field-key-0123456789abcdefghijk!"),
		"unpadded base64": base64.RawStdEncoding.EncodeToString(sentinelFieldKeyBytes),
		"URL-safe base64": base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 32)),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			values["OVC_FIELD_ENCRYPTION_KEY"] = value
			missing := validValues()
			delete(missing, "OVC_FIELD_ENCRYPTION_KEY")

			var refusals []error
			for _, process := range []core.Process{core.ProcessAPI, core.ProcessWorker} {
				for _, lookup := range []core.Lookup{lookupFrom(values), lookupFrom(missing)} {
					_, err := core.Load(lookup, process)
					refusals = append(refusals, err)
				}
			}
			_, adminErr := core.Load(lookupFrom(missing), core.ProcessAdmin)

			for _, refusal := range refusals {
				var settingsError *core.SettingsError
				if !errors.As(refusal, &settingsError) || settingsError.Variable != "OVC_FIELD_ENCRYPTION_KEY" {
					t.Fatalf("expected a refusal naming the key, got %v", refusal)
				}
				if value != "" && strings.Contains(refusal.Error(), value) || strings.Contains(refusal.Error(), "sentinel") {
					t.Fatal("the refusal shows the value")
				}
			}
			if adminErr != nil {
				t.Fatalf("the administration command needed the key: %v", adminErr)
			}
		})
	}
}
