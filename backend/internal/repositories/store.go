package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ovc/backend/internal/dbgen"
)

var ErrNotFound = errors.New("not found")

type Repositories struct {
	Users         Users
	TutorProfiles TutorProfiles
	Sessions      Sessions
	EmailTokens   EmailTokens
	RateLimits    RateLimits
	Attachments   Attachments
	Outbox        Outbox
	JobRuns       JobRuns
	Relationships Relationships
	Invitations   Invitations
	JoinRequests  JoinRequests
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Repositories() Repositories {
	return newRepositories(dbgen.New(s.pool))
}

func (s *Store) WithTx(ctx context.Context, work func(Repositories) error) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return work(newRepositories(dbgen.New(tx)))
	})
	if err != nil {
		return fmt.Errorf("transaction: %w", err)
	}
	return nil
}

func newRepositories(queries *dbgen.Queries) Repositories {
	return Repositories{
		Users:         Users{queries: queries},
		TutorProfiles: TutorProfiles{queries: queries},
		Sessions:      Sessions{queries: queries},
		EmailTokens:   EmailTokens{queries: queries},
		RateLimits:    RateLimits{queries: queries},
		Attachments:   Attachments{queries: queries},
		Outbox:        Outbox{queries: queries},
		JobRuns:       JobRuns{queries: queries},
		Relationships: Relationships{queries: queries},
		Invitations:   Invitations{queries: queries},
		JoinRequests:  JoinRequests{queries: queries},
	}
}

func one[Row any](row Row, err error, what string) (Row, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return row, fmt.Errorf("%s: %w", what, ErrNotFound)
	}
	if err != nil {
		return row, fmt.Errorf("%s: %w", what, err)
	}
	return row, nil
}

func applied(rows int64, err error, what string) (bool, error) {
	if err != nil {
		return false, fmt.Errorf("%s: %w", what, err)
	}
	return rows > 0, nil
}

func counted(rows int64, err error, what string) (int64, error) {
	if err != nil {
		return 0, fmt.Errorf("%s: %w", what, err)
	}
	return rows, nil
}

func done(err error, what string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}
