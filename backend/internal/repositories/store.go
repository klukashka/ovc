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
	Users           Users
	TutorProfiles   TutorProfiles
	Sessions        Sessions
	EmailTokens     EmailTokens
	RateLimits      RateLimits
	Attachments     Attachments
	UploadIntents   UploadIntents
	DailyUploads    DailyUploads
	Outbox          Outbox
	JobRuns         JobRuns
	Relationships   Relationships
	Invitations     Invitations
	JoinRequests    JoinRequests
	Blocked         BlockedRequesters
	Groups          Groups
	Preferences     EmailPreferences
	Signals         Signals
	Notifications   Notifications
	Lessons         Lessons
	Meetings        Meetings
	MeetingRecovery MeetingRecovery
	Channels        Channels
	Messages        Messages
	Digests         Digests
	Presence        Presence
	Library         Library
	LessonMaterials LessonMaterials
	TestClock       TestClock

	commit *commitHooks
}

type commitHooks struct {
	hooks []func(context.Context)
}

func (r Repositories) AfterCommit(ctx context.Context, hook func(context.Context)) {
	if r.commit == nil {
		hook(ctx)
		return
	}
	r.commit.hooks = append(r.commit.hooks, hook)
}

func (h *commitHooks) run(ctx context.Context) {
	for _, hook := range h.hooks {
		hook(ctx)
	}
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
	hooks := &commitHooks{}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		repos := newRepositories(dbgen.New(tx))
		repos.commit = hooks
		return work(repos)
	})
	if err != nil {
		return fmt.Errorf("transaction: %w", err)
	}
	hooks.run(ctx)
	return nil
}

func (s *Store) WithReadTx(ctx context.Context, work func(Repositories) error) error {
	options := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := pgx.BeginTxFunc(ctx, s.pool, options, func(tx pgx.Tx) error {
		return work(newRepositories(dbgen.New(tx)))
	})
	if err != nil {
		return fmt.Errorf("read-only transaction: %w", err)
	}
	return nil
}

func newRepositories(queries *dbgen.Queries) Repositories {
	return Repositories{
		Users:           Users{queries: queries},
		TutorProfiles:   TutorProfiles{queries: queries},
		Sessions:        Sessions{queries: queries},
		EmailTokens:     EmailTokens{queries: queries},
		RateLimits:      RateLimits{queries: queries},
		Attachments:     Attachments{queries: queries},
		UploadIntents:   UploadIntents{queries: queries},
		DailyUploads:    DailyUploads{queries: queries},
		Outbox:          Outbox{queries: queries},
		JobRuns:         JobRuns{queries: queries},
		Relationships:   Relationships{queries: queries},
		Invitations:     Invitations{queries: queries},
		JoinRequests:    JoinRequests{queries: queries},
		Blocked:         BlockedRequesters{queries: queries},
		Groups:          Groups{queries: queries},
		Preferences:     EmailPreferences{queries: queries},
		Signals:         Signals{queries: queries},
		Notifications:   Notifications{queries: queries},
		Lessons:         Lessons{queries: queries},
		Meetings:        Meetings{queries: queries},
		MeetingRecovery: MeetingRecovery{queries: queries},
		Channels:        Channels{queries: queries},
		Messages:        Messages{queries: queries},
		Digests:         Digests{queries: queries},
		Presence:        Presence{queries: queries},
		Library:         Library{queries: queries},
		LessonMaterials: LessonMaterials{queries: queries},
		TestClock:       TestClock{queries: queries},
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
