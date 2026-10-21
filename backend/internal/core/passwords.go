package core

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	ErrHashingBusy       = errors.New("password hashing is saturated")
	errMalformedPassword = errors.New("the stored password hash is malformed")
)

const (
	argon2SaltBytes = 16
	argon2KeyBytes  = 32
)

type Argon2Params struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
}

func ProductionArgon2Params() Argon2Params {
	return Argon2Params{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1}
}

type PasswordHasher struct {
	params Argon2Params
	random RandomSource
	slots  chan struct{}
	wait   time.Duration
	dummy  string
}

type Verification struct {
	Matches bool
	Stale   bool
}

func NewPasswordHasher(params Argon2Params, random RandomSource, concurrency int, wait time.Duration) (*PasswordHasher, error) {
	hasher := &PasswordHasher{params: params, random: random, slots: make(chan struct{}, max(concurrency, 1)), wait: wait}
	placeholder := make([]byte, 32)
	if _, err := random.Read(placeholder); err != nil {
		return nil, fmt.Errorf("read a placeholder password: %w", err)
	}
	dummy, err := hasher.encode(string(placeholder))
	if err != nil {
		return nil, err
	}
	hasher.dummy = dummy
	return hasher, nil
}

func (h *PasswordHasher) Hash(ctx context.Context, password string) (string, error) {
	release, err := h.acquire(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	return h.encode(password)
}

func (h *PasswordHasher) Verify(ctx context.Context, password string, encoded string) (Verification, error) {
	params, salt, expected, err := parsePHC(encoded)
	if err != nil {
		return Verification{}, err
	}
	release, err := h.acquire(ctx)
	if err != nil {
		return Verification{}, err
	}
	defer release()
	actual := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, argon2KeyBytes)
	matches := subtle.ConstantTimeCompare(actual, expected) == 1
	stale := params != h.params
	return Verification{Matches: matches, Stale: stale}, nil
}

func (h *PasswordHasher) VerifyNothing(ctx context.Context, password string) error {
	_, err := h.Verify(ctx, password, h.dummy)
	return err
}

func (h *PasswordHasher) acquire(ctx context.Context) (func(), error) {
	waiting, cancel := context.WithTimeout(ctx, h.wait)
	defer cancel()
	select {
	case h.slots <- struct{}{}:
		return func() { <-h.slots }, nil
	case <-waiting.Done():
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("wait for a hashing slot: %w", err)
		}
		return nil, ErrHashingBusy
	}
}

func (h *PasswordHasher) encode(password string) (string, error) {
	salt := make([]byte, argon2SaltBytes)
	if _, err := h.random.Read(salt); err != nil {
		return "", fmt.Errorf("read a password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, h.params.Iterations, h.params.MemoryKiB, h.params.Parallelism, argon2KeyBytes)
	encoding := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, h.params.MemoryKiB, h.params.Iterations,
		h.params.Parallelism, encoding.EncodeToString(salt), encoding.EncodeToString(key)), nil
}

func parsePHC(encoded string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return Argon2Params{}, nil, nil, errMalformedPassword
	}
	params, err := parsePHCParams(parts[3])
	if err != nil {
		return Argon2Params{}, nil, nil, err
	}
	salt, saltErr := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	key, keyErr := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if saltErr != nil || keyErr != nil || len(salt) != argon2SaltBytes || len(key) != argon2KeyBytes {
		return Argon2Params{}, nil, nil, errMalformedPassword
	}
	return params, salt, key, nil
}

func parsePHCParams(text string) (Argon2Params, error) {
	fields := strings.Split(text, ",")
	if len(fields) != 3 || !strings.HasPrefix(fields[0], "m=") || !strings.HasPrefix(fields[1], "t=") || !strings.HasPrefix(fields[2], "p=") {
		return Argon2Params{}, errMalformedPassword
	}
	memory, memoryErr := strconv.ParseUint(fields[0][2:], 10, 32)
	iterations, iterationsErr := strconv.ParseUint(fields[1][2:], 10, 32)
	parallelism, parallelismErr := strconv.ParseUint(fields[2][2:], 10, 8)
	if memoryErr != nil || iterationsErr != nil || parallelismErr != nil || memory < 8 || memory > 1<<21 || iterations < 1 || iterations > 16 || parallelism < 1 {
		return Argon2Params{}, errMalformedPassword
	}
	return Argon2Params{MemoryKiB: uint32(memory), Iterations: uint32(iterations), Parallelism: uint8(parallelism)}, nil
}
