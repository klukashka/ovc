package core

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessAudience = "ovc-api"
	clockLeeway    = 5 * time.Second
)

var (
	ErrAccessTokenExpired = errors.New("the access token has expired")
	ErrAccessTokenInvalid = errors.New("the access token is invalid")
)

type AccessClaims struct {
	UserID    [16]byte
	SessionID [16]byte
}

type AccessTokens struct {
	keys   Keys
	issuer string
	ttl    time.Duration
}

type accessClaims struct {
	Session string `json:"sid"`
	jwt.RegisteredClaims
}

func NewAccessTokens(keys Keys, issuer string, ttl time.Duration) *AccessTokens {
	return &AccessTokens{keys: keys, issuer: issuer, ttl: ttl}
}

func (t *AccessTokens) TTL() time.Duration {
	return t.ttl
}

func (t *AccessTokens) Issue(claims AccessClaims, now time.Time) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims{
		Session: hex.EncodeToString(claims.SessionID[:]),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    t.issuer,
			Subject:   hex.EncodeToString(claims.UserID[:]),
			Audience:  jwt.ClaimStrings{accessAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
		},
	})
	signed, err := token.SignedString(t.keys.access())
	if err != nil {
		return "", fmt.Errorf("sign an access token: %w", err)
	}
	return signed, nil
}

func (t *AccessTokens) Verify(text string, now time.Time) (AccessClaims, error) {
	var claims accessClaims
	token, parts, err := jwt.NewParser().ParseUnverified(text, &claims)
	if err != nil || len(parts) != 3 || token.Method != jwt.SigningMethodHS256 {
		return AccessClaims{}, ErrAccessTokenInvalid
	}
	if err := jwt.SigningMethodHS256.Verify(parts[0]+"."+parts[1], token.Signature, t.keys.access()); err != nil {
		return AccessClaims{}, ErrAccessTokenInvalid
	}
	validator := jwt.NewValidator(
		jwt.WithIssuer(t.issuer),
		jwt.WithAudience(accessAudience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(clockLeeway),
		jwt.WithTimeFunc(func() time.Time { return now }),
	)
	err = validator.Validate(&claims)
	if errors.Is(err, jwt.ErrTokenExpired) {
		return AccessClaims{}, ErrAccessTokenExpired
	}
	if err != nil || claims.IssuedAt == nil {
		return AccessClaims{}, ErrAccessTokenInvalid
	}
	user, userErr := decodeID(claims.Subject)
	session, sessionErr := decodeID(claims.Session)
	if userErr != nil || sessionErr != nil {
		return AccessClaims{}, ErrAccessTokenInvalid
	}
	return AccessClaims{UserID: user, SessionID: session}, nil
}

func decodeID(text string) ([16]byte, error) {
	var id [16]byte
	decoded, err := hex.DecodeString(text)
	if err != nil || len(decoded) != len(id) {
		return id, ErrAccessTokenInvalid
	}
	copy(id[:], decoded)
	return id, nil
}

func RandomToken(random RandomSource, size int) ([]byte, string, error) {
	raw := make([]byte, size)
	if _, err := io.ReadFull(random, raw); err != nil {
		return nil, "", fmt.Errorf("read a random token: %w", err)
	}
	return raw, base64.RawURLEncoding.EncodeToString(raw), nil
}
