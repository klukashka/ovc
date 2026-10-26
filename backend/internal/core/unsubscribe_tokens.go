package core

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"ovc/backend/internal/domain"
)

const (
	minimumUnsubscribeKeyBytes = 32
	unsubscribeKeyInfo         = "unsubscribe"
	unsubscribeTokenVersion    = "v1"
	unsubscribeUserBytes       = 16
	unsubscribeExpiryBytes     = 8
	unsubscribePayloadBytes    = unsubscribeUserBytes + 1 + unsubscribeExpiryBytes
	unsubscribeMACBytes        = sha256.Size
	maximumUnsubscribeText     = 128
)

var (
	ErrUnsubscribeTokenMalformed = errors.New("the unsubscribe token is malformed")
	ErrUnsubscribeTokenSignature = errors.New("the unsubscribe token signature does not match")
	ErrUnsubscribeTokenExpired   = errors.New("the unsubscribe token has expired")
	ErrUnsubscribeTokensUnset    = errors.New("the unsubscribe token signer was not built from a key")
)

func (s *Settings) loadUnsubscribeKey(lookup Lookup) []error {
	value, _ := lookup(varUnsubscribeKey)
	s.UnsubscribeKey = NewSecret(value)
	if len(value) < minimumUnsubscribeKeyBytes {
		return []error{&SettingsError{Variable: varUnsubscribeKey, Problem: "is required and must be at least 32 bytes, generated with openssl rand -base64 32"}}
	}
	for _, other := range []string{varJWTSecret, varFieldKey} {
		if otherValue, set := lookup(other); set && subtle.ConstantTimeCompare([]byte(value), []byte(otherValue)) == 1 {
			return []error{&SettingsError{Variable: varUnsubscribeKey, Problem: "must differ from " + other}}
		}
	}
	return nil
}

type UnsubscribeClaims struct {
	UserID    domain.UserID
	Group     domain.EmailGroup
	ExpiresAt time.Time
}

type UnsubscribeTokens struct {
	key func() []byte
}

func NewUnsubscribeTokens(secret Secret) (UnsubscribeTokens, error) {
	if len(secret.Reveal()) < minimumUnsubscribeKeyBytes {
		return UnsubscribeTokens{}, ErrUnsubscribeTokensUnset
	}
	key, err := hkdf.Key(sha256.New, []byte(secret.Reveal()), nil, unsubscribeKeyInfo, derivedKeyBytes)
	if err != nil {
		return UnsubscribeTokens{}, fmt.Errorf("derive the unsubscribe key: %w", err)
	}
	return UnsubscribeTokens{key: func() []byte { return key }}, nil
}

func (t UnsubscribeTokens) Mint(user domain.UserID, group domain.EmailGroup, sentAt time.Time) (string, error) {
	if t.key == nil {
		return "", ErrUnsubscribeTokensUnset
	}
	code, known := group.TokenCode()
	if !known {
		return "", fmt.Errorf("mint an unsubscribe token for the group %q: %w", group, ErrUnsubscribeTokenMalformed)
	}
	expiry := sentAt.Add(domain.UnsubscribeTokenLifetime).Unix()
	if expiry < 0 {
		return "", fmt.Errorf("mint an unsubscribe token expiring before 1970: %w", ErrUnsubscribeTokenMalformed)
	}
	payload := make([]byte, 0, unsubscribePayloadBytes)
	payload = append(payload, user[:]...)
	payload = append(payload, code)
	payload = binary.BigEndian.AppendUint64(payload, uint64(expiry))
	encoding := base64.RawURLEncoding
	return unsubscribeTokenVersion + "." + encoding.EncodeToString(payload) + "." + encoding.EncodeToString(t.mac(payload)), nil
}

func (t UnsubscribeTokens) Verify(text string, now time.Time) (UnsubscribeClaims, error) {
	if t.key == nil {
		return UnsubscribeClaims{}, ErrUnsubscribeTokensUnset
	}
	payload, mac, ok := splitUnsubscribeToken(text)
	if !ok {
		return UnsubscribeClaims{}, ErrUnsubscribeTokenMalformed
	}
	if !hmac.Equal(mac, t.mac(payload)) {
		return UnsubscribeClaims{}, ErrUnsubscribeTokenSignature
	}
	var user domain.UserID
	copy(user[:], payload[:unsubscribeUserBytes])
	group, known := domain.EmailGroupFromTokenCode(payload[unsubscribeUserBytes])
	expiry := binary.BigEndian.Uint64(payload[unsubscribeUserBytes+1:])
	if !known || expiry > math.MaxInt64 {
		return UnsubscribeClaims{}, ErrUnsubscribeTokenMalformed
	}
	claims := UnsubscribeClaims{UserID: user, Group: group, ExpiresAt: time.Unix(int64(expiry), 0).UTC()}
	if !now.Before(claims.ExpiresAt) {
		return claims, ErrUnsubscribeTokenExpired
	}
	return claims, nil
}

func splitUnsubscribeToken(text string) ([]byte, []byte, bool) {
	if len(text) > maximumUnsubscribeText {
		return nil, nil, false
	}
	parts := strings.Split(text, ".")
	if len(parts) != 3 || parts[0] != unsubscribeTokenVersion || !inBase64URLAlphabet(parts[1]) || !inBase64URLAlphabet(parts[2]) {
		return nil, nil, false
	}
	encoding := base64.RawURLEncoding.Strict()
	payload, payloadErr := encoding.DecodeString(parts[1])
	mac, macErr := encoding.DecodeString(parts[2])
	if payloadErr != nil || macErr != nil || len(payload) != unsubscribePayloadBytes || len(mac) != unsubscribeMACBytes {
		return nil, nil, false
	}
	return payload, mac, true
}

func inBase64URLAlphabet(part string) bool {
	for index := range len(part) {
		character := part[index]
		letter := (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z')
		if !letter && (character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

func (t UnsubscribeTokens) mac(payload []byte) []byte {
	mac := hmac.New(sha256.New, t.key())
	mac.Write(payload)
	return mac.Sum(nil)
}

func (UnsubscribeTokens) String() string {
	return redacted
}

func (UnsubscribeTokens) GoString() string {
	return redacted
}

func (UnsubscribeTokens) Format(state fmt.State, _ rune) {
	writeRedacted(state)
}

func (UnsubscribeTokens) LogValue() slog.Value {
	return slog.StringValue(redacted)
}
