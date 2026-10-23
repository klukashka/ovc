package testhasher

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"ovc/backend/internal/core"
)

const (
	storedPrefix = "argon2id"
	scheme       = "fake"
)

var errMalformed = errors.New("the password hash is not a fake hash")

type Pause struct {
	reached chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (p *Pause) Reached() <-chan struct{} {
	return p.reached
}

func (p *Pause) Resume() {
	p.once.Do(func() { close(p.resume) })
}

type Fake struct {
	generation    int
	salts         atomic.Uint64
	hashes        atomic.Int32
	verifications atomic.Int32
	mu            sync.Mutex
	failure       error
	pause         *Pause
}

func New(generation int) *Fake {
	return &Fake{generation: generation}
}

func (f *Fake) Hash(_ context.Context, password string) (string, error) {
	f.hashes.Add(1)
	salt := make([]byte, 8)
	binary.BigEndian.PutUint64(salt, f.salts.Add(1))
	return f.encode(f.generation, salt, password), nil
}

func (f *Fake) Verify(ctx context.Context, password string, encoded string) (core.Verification, error) {
	f.verifications.Add(1)
	if err := f.currentFailure(); err != nil {
		return core.Verification{}, err
	}
	generation, salt, expected, err := parse(encoded)
	if err != nil {
		return core.Verification{}, err
	}
	actual := digest(salt, password)
	verification := core.Verification{Matches: subtle.ConstantTimeCompare(actual, expected) == 1, Stale: generation != f.generation}
	if err := f.waitIfPaused(ctx); err != nil {
		return core.Verification{}, err
	}
	return verification, nil
}

func (f *Fake) VerifyNothing(ctx context.Context, password string) error {
	_, err := f.Verify(ctx, password, f.encode(f.generation, []byte("placeholder"), "placeholder password"))
	return err
}

func (f *Fake) Hashes() int {
	return int(f.hashes.Load())
}

func (f *Fake) Verifications() int {
	return int(f.verifications.Load())
}

func (f *Fake) FailVerifications(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failure = err
}

func (f *Fake) PauseAfterNextVerification() *Pause {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pause = &Pause{reached: make(chan struct{}), resume: make(chan struct{})}
	return f.pause
}

func (f *Fake) currentFailure() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.failure
}

func (f *Fake) waitIfPaused(ctx context.Context) error {
	f.mu.Lock()
	pause := f.pause
	f.pause = nil
	f.mu.Unlock()
	if pause == nil {
		return nil
	}
	close(pause.reached)
	select {
	case <-pause.resume:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for a paused verification: %w", ctx.Err())
	}
}

func (f *Fake) encode(generation int, salt []byte, password string) string {
	return fmt.Sprintf("$%s$%s$g=%d$%s$%s", storedPrefix, scheme, generation, hex.EncodeToString(salt), hex.EncodeToString(digest(salt, password)))
}

func digest(salt []byte, password string) []byte {
	sum := sha256.Sum256(append(append([]byte{}, salt...), password...))
	return sum[:]
}

func parse(encoded string) (int, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != storedPrefix || parts[2] != scheme || !strings.HasPrefix(parts[3], "g=") {
		return 0, nil, nil, errMalformed
	}
	generation, err := strconv.Atoi(strings.TrimPrefix(parts[3], "g="))
	if err != nil {
		return 0, nil, nil, errMalformed
	}
	salt, saltErr := hex.DecodeString(parts[4])
	key, keyErr := hex.DecodeString(parts[5])
	if saltErr != nil || keyErr != nil || len(key) != sha256.Size {
		return 0, nil, nil, errMalformed
	}
	return generation, salt, key, nil
}
