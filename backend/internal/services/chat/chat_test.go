package chat_test

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/testdb"
)

const (
	seedTutor = `INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme,
    consent_version, consent_at, unverified_since, created_at, updated_at)
VALUES ($1, 'tutor', 'active', 'tutor@example.org', $2, '$argon2id$v=19$m=19456,t=2,p=1$fixture', 'A tutor', 'en', 'Europe/Moscow',
    'system', '2026-10', $2, $2, $2, $2)`
	seedProfile = `INSERT INTO tutor_profiles (user_id, code, file_quota_bytes, recording_quota_bytes, created_at, updated_at)
VALUES ($1, 'ABCDEFGH', 1, 1, $2, $2)`
	seedGroupChannels = `WITH created AS (
    INSERT INTO groups (id, tutor_id, name, colour, created_at, updated_at)
    SELECT gen_random_uuid(), $1, 'Group ' || n, 'moss', $2, $2 FROM generate_series(1, $3) AS n
    RETURNING id
), opened AS (
    INSERT INTO channels (id, kind, tutor_id, group_id, last_seq, last_activity_at, created_at, updated_at)
    SELECT gen_random_uuid(), 'group', $1, created.id, $4, $2, $2, $2 FROM created
    RETURNING id
)
INSERT INTO channel_members (channel_id, user_id, created_at, updated_at) SELECT opened.id, $1, $2, $2 FROM opened`
	queryEventPrefix = "db.query."
)

type statementCounter struct {
	statements atomic.Int64
}

func (c *statementCounter) Enabled(context.Context, slog.Level) bool { return true }

func (c *statementCounter) Handle(_ context.Context, record slog.Record) error {
	if strings.HasPrefix(record.Message, queryEventPrefix) {
		c.statements.Add(1)
	}
	return nil
}

func (c *statementCounter) WithAttrs([]slog.Attr) slog.Handler { return c }

func (c *statementCounter) WithGroup(string) slog.Handler { return c }

func TestUnreadChannelsAreReadInOneQuery(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	ctx := context.Background()
	instant := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	tutor, other := domain.UserID{1}, domain.UserID{2}
	if _, err := database.Pool.Exec(ctx, seedTutor, tutor, instant); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, seedProfile, tutor, instant); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, seedGroupChannels, tutor, instant, 101, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, seedGroupChannels, tutor, instant, 5, 0); err != nil {
		t.Fatal(err)
	}
	counter := &statementCounter{}
	pool, err := core.OpenPool(ctx, core.Settings{DatabaseURL: database.URL, DBPoolSize: 2}, slog.New(counter), func() time.Time { return instant })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	service := chat.New(chat.Dependencies{Scheduler: core.SystemScheduler{}, Store: repositories.NewStore(pool), Logger: slog.New(slog.DiscardHandler)})
	before := counter.statements.Load()

	unread, unreadErr := service.UnreadChannels(ctx, tutor)
	statements := counter.statements.Load() - before
	none, noneErr := service.UnreadChannels(ctx, other)

	if unreadErr != nil || noneErr != nil {
		t.Fatal(unreadErr, noneErr)
	}
	if unread != domain.UnreadChannelsCap || none != 0 {
		t.Fatalf("the unread count is %d for the tutor and %d for another account", unread, none)
	}
	if statements != 1 {
		t.Fatalf("the unread count took %d statements", statements)
	}
}
