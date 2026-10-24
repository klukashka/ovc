package core

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"ovc/backend/internal/domain"
)

const (
	fieldKeyBytes        = 32
	fieldNonceBytes      = 12
	invitationTokenLabel = "ovc.invitation-token.v1"
)

var (
	ErrFieldKeyInvalid        = errors.New("the field encryption key must be 32 bytes in base64")
	ErrFieldCiphertextInvalid = errors.New("the field ciphertext cannot be opened")
	ErrFieldCipherUnset       = errors.New("the field cipher was not built with NewFieldCipher")
)

func (s *Settings) loadFieldKey(lookup Lookup) []error {
	value, _ := lookup(varFieldKey)
	s.FieldKey = NewSecret(value)
	if value == "" {
		return []error{&SettingsError{Variable: varFieldKey, Problem: "is required: 32 random bytes in base64, generated with openssl rand -base64 32"}}
	}
	if _, err := decodeFieldKey(s.FieldKey); err != nil {
		return []error{&SettingsError{Variable: varFieldKey, Problem: "must be exactly 32 bytes in standard base64, generated with openssl rand -base64 32"}}
	}
	return nil
}

func decodeFieldKey(key Secret) ([]byte, error) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(key.Reveal())
	if err != nil || len(decoded) != fieldKeyBytes {
		return nil, ErrFieldKeyInvalid
	}
	return decoded, nil
}

type FieldCipher struct {
	aead   func() cipher.AEAD
	random RandomSource
}

func NewFieldCipher(key Secret, random RandomSource) (FieldCipher, error) {
	decoded, err := decodeFieldKey(key)
	if err != nil {
		return FieldCipher{}, err
	}
	block, err := aes.NewCipher(decoded)
	if err != nil {
		return FieldCipher{}, fmt.Errorf("create the field cipher block: %w", err)
	}
	aead, err := cipher.NewGCMWithNonceSize(block, fieldNonceBytes)
	if err != nil {
		return FieldCipher{}, fmt.Errorf("create the field cipher mode: %w", err)
	}
	return FieldCipher{aead: func() cipher.AEAD { return aead }, random: random}, nil
}

func invitationAdditionalData(invitation domain.InvitationID) []byte {
	return append([]byte(invitationTokenLabel), invitation[:]...)
}

func (c FieldCipher) SealForInvitation(invitation domain.InvitationID, plaintext []byte) ([]byte, error) {
	if c.aead == nil || c.random == nil {
		return nil, ErrFieldCipherUnset
	}
	aead := c.aead()
	sealed := make([]byte, fieldNonceBytes, fieldNonceBytes+len(plaintext)+aead.Overhead())
	if _, err := io.ReadFull(c.random, sealed); err != nil {
		return nil, fmt.Errorf("read the field cipher nonce: %w", err)
	}
	return aead.Seal(sealed, sealed[:fieldNonceBytes], plaintext, invitationAdditionalData(invitation)), nil
}

func (c FieldCipher) OpenForInvitation(invitation domain.InvitationID, sealed []byte) ([]byte, error) {
	if c.aead == nil {
		return nil, ErrFieldCipherUnset
	}
	aead := c.aead()
	if len(sealed) < fieldNonceBytes+aead.Overhead() {
		return nil, ErrFieldCiphertextInvalid
	}
	plaintext, err := aead.Open(nil, sealed[:fieldNonceBytes], sealed[fieldNonceBytes:], invitationAdditionalData(invitation))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFieldCiphertextInvalid, err)
	}
	return plaintext, nil
}

func (FieldCipher) String() string {
	return redacted
}

func (FieldCipher) GoString() string {
	return redacted
}

func (FieldCipher) Format(state fmt.State, _ rune) {
	writeRedacted(state)
}

func (FieldCipher) LogValue() slog.Value {
	return slog.StringValue(redacted)
}
