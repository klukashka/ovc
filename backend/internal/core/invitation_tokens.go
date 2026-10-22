package core

import (
	"crypto/sha256"
	"encoding/base64"

	"ovc/backend/internal/domain"
)

type InvitationToken struct {
	Raw  []byte
	Text string
	Hash []byte
}

func NewInvitationToken(random RandomSource) (InvitationToken, error) {
	raw, text, err := RandomToken(random, domain.InvitationTokenBytes)
	if err != nil {
		return InvitationToken{}, err
	}
	hash := sha256.Sum256(raw)
	return InvitationToken{Raw: raw, Text: text, Hash: hash[:]}, nil
}

func InvitationTokenHash(text string) ([]byte, bool) {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil || len(raw) != domain.InvitationTokenBytes {
		return nil, false
	}
	hash := sha256.Sum256(raw)
	return hash[:], true
}

func InvitationTokenText(raw []byte) (string, bool) {
	if len(raw) != domain.InvitationTokenBytes {
		return "", false
	}
	return base64.RawURLEncoding.EncodeToString(raw), true
}
