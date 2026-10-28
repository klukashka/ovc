package clock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TestClockReader interface {
	ReadTestClock(ctx context.Context) (time.Time, bool, error)
}

type Source interface {
	Instant(ctx context.Context) (time.Time, error)
	InstantWithin(ctx context.Context, reader TestClockReader) (time.Time, error)
}

type System struct{}

func (System) Now() time.Time {
	return time.Now()
}

func (s System) Instant(_ context.Context) (time.Time, error) {
	return s.Now().UTC(), nil
}

func (s System) InstantWithin(_ context.Context, _ TestClockReader) (time.Time, error) {
	return s.Now().UTC(), nil
}

const (
	statementReadTestClock = "SELECT instant FROM test_clock WHERE id = 1"
	statementSetTestClock  = "INSERT INTO test_clock (id, instant) VALUES (1, $1) ON CONFLICT (id) DO UPDATE SET instant = EXCLUDED.instant RETURNING instant"
	statementAdvanceClock  = "UPDATE test_clock SET instant = instant + make_interval(secs => $1) WHERE id = 1 RETURNING instant"
)

var ErrTestClockNotSet = errors.New("the shared test clock has not been set")

type Shared struct {
	pool *pgxpool.Pool
}

func NewShared(pool *pgxpool.Pool) *Shared {
	return &Shared{pool: pool}
}

func (s *Shared) Instant(ctx context.Context) (time.Time, error) {
	var instant time.Time
	err := s.pool.QueryRow(ctx, statementReadTestClock).Scan(&instant)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrTestClockNotSet
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("read the shared test clock: %w", err)
	}
	return instant.UTC(), nil
}

func (s *Shared) InstantWithin(ctx context.Context, reader TestClockReader) (time.Time, error) {
	instant, found, err := reader.ReadTestClock(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("read the shared test clock in a transaction: %w", err)
	}
	if !found {
		return time.Time{}, ErrTestClockNotSet
	}
	return instant.UTC(), nil
}

func (s *Shared) Set(ctx context.Context, instant time.Time) (time.Time, error) {
	var stored time.Time
	if err := s.pool.QueryRow(ctx, statementSetTestClock, instant.UTC()).Scan(&stored); err != nil {
		return time.Time{}, fmt.Errorf("set the shared test clock: %w", err)
	}
	return stored.UTC(), nil
}

func (s *Shared) Advance(ctx context.Context, by time.Duration) (time.Time, error) {
	var instant time.Time
	err := s.pool.QueryRow(ctx, statementAdvanceClock, by.Seconds()).Scan(&instant)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrTestClockNotSet
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("advance the shared test clock: %w", err)
	}
	return instant.UTC(), nil
}
