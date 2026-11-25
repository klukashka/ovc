package chat_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/testdb"
)

type presenceCleanupResult struct {
	deleted int64
	err     error
}

func TestPresenceCleanupRechecksExpiryAfterConcurrentRenewal(t *testing.T) {
	t.Parallel()
	database := testdb.NewForLockContention(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	instant := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	account, instance := domain.UserID{1}, domain.PresenceInstanceID{1}
	seedExpiredPresence(t, ctx, database, account, instance, instant, seedTutor)
	source := clock.NewShared(database.Pool)
	if _, err := source.Set(ctx, instant); err != nil {
		t.Fatal(err)
	}
	service := chat.New(chat.Dependencies{Scheduler: core.SystemScheduler{}, Store: repositories.NewStore(database.Pool), Clock: source, Logger: slog.New(slog.DiscardHandler)})
	held, err := database.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	committed := false
	defer func() {
		if !committed {
			if err := held.Rollback(context.Background()); err != nil {
				t.Error(err)
			}
		}
	}()
	if _, err := held.Exec(ctx, `SELECT user_id FROM user_presence_instances WHERE user_id = $1 FOR UPDATE`, account); err != nil {
		t.Fatal(err)
	}
	blocker := held.Conn().PgConn().PID()
	renewed := make(chan error, 1)
	cleaned := make(chan presenceCleanupResult, 1)

	go func() {
		_, writeErr := service.PersistPresence(ctx, instance, []domain.UserID{account}, true)
		renewed <- writeErr
	}()
	renewalPID := waitPresenceBlockedBy(t, ctx, database, blocker)
	go func() {
		deleted, cleanupErr := service.ExpirePresence(ctx, instant)
		cleaned <- presenceCleanupResult{deleted: deleted, err: cleanupErr}
	}()
	waitPresenceBlockedBy(t, ctx, database, renewalPID)
	if err := held.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	committed = true
	renewalErr := <-renewed
	cleanup := <-cleaned

	if renewalErr != nil || cleanup.err != nil || cleanup.deleted != 0 {
		t.Fatalf("renewal %v, cleanup deleted %d: %v", renewalErr, cleanup.deleted, cleanup.err)
	}
	var until, seen time.Time
	if err := database.Pool.QueryRow(ctx, `SELECT online_until, last_seen_at FROM user_presence_instances WHERE user_id = $1 AND instance_id = $2`, account, instance).Scan(&until, &seen); err != nil {
		t.Fatal(err)
	}
	if !until.Equal(instant.Add(domain.PresenceWindow)) || !seen.Equal(instant) {
		t.Fatalf("renewed lease changed: until %s, seen %s", until, seen)
	}
}

func TestPresenceCleanupSkipsConcurrentlyPurgedAccountsAndContinuesBatch(t *testing.T) {
	t.Parallel()
	database := testdb.NewForLockContention(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	instant := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	purged, retained := domain.UserID{1}, domain.UserID{2}
	seedExpiredPresence(t, ctx, database, purged, domain.PresenceInstanceID{1}, instant, seedTutor)
	seedExpiredPresence(t, ctx, database, retained, domain.PresenceInstanceID{2}, instant, strings.Replace(seedTutor, "tutor@example.org", "other@example.org", 1))
	service := chat.New(chat.Dependencies{Scheduler: core.SystemScheduler{}, Store: repositories.NewStore(database.Pool), Logger: slog.New(slog.DiscardHandler)})
	held, err := database.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	committed := false
	defer func() {
		if !committed {
			if err := held.Rollback(context.Background()); err != nil {
				t.Error(err)
			}
		}
	}()
	if _, err := held.Exec(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, purged); err != nil {
		t.Fatal(err)
	}
	cleaned := make(chan presenceCleanupResult, 1)

	go func() {
		deleted, cleanupErr := service.ExpirePresence(ctx, instant)
		cleaned <- presenceCleanupResult{deleted: deleted, err: cleanupErr}
	}()
	waitPresenceBlockedBy(t, ctx, database, held.Conn().PgConn().PID())
	if _, err := held.Exec(ctx, `DELETE FROM users WHERE id = $1`, purged); err != nil {
		t.Fatal(err)
	}
	if err := held.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	committed = true
	cleanup := <-cleaned

	if cleanup.err != nil || cleanup.deleted != 1 {
		t.Fatalf("cleanup deleted %d: %v", cleanup.deleted, cleanup.err)
	}
	var leases, summaries int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM user_presence_instances`).Scan(&leases); err != nil {
		t.Fatal(err)
	}
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM user_presence`).Scan(&summaries); err != nil {
		t.Fatal(err)
	}
	var until, seen time.Time
	if err := database.Pool.QueryRow(ctx, `SELECT online_until, last_seen_at FROM user_presence WHERE user_id = $1`, retained).Scan(&until, &seen); err != nil {
		t.Fatal(err)
	}
	if leases != 0 || summaries != 1 || !until.Equal(instant) || !seen.Equal(instant.Add(-time.Minute)) {
		t.Fatalf("cleanup history/cascade: leases %d, summaries %d, until %s, seen %s", leases, summaries, until, seen)
	}
}

func seedExpiredPresence(t *testing.T, ctx context.Context, database *testdb.Database, account domain.UserID, instance domain.PresenceInstanceID, instant time.Time, seed string) {
	t.Helper()
	if _, err := database.Pool.Exec(ctx, seed, account, instant); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO user_presence VALUES ($1, $2, $3)`, account, instant, instant.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO user_presence_instances VALUES ($1, $2, $3, $4)`, account, instance, instant, instant.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func waitPresenceBlockedBy(t *testing.T, ctx context.Context, database *testdb.Database, blocker uint32) uint32 {
	t.Helper()
	for {
		var waiting uint32
		err := database.Pool.QueryRow(ctx, `SELECT coalesce((SELECT pid FROM pg_stat_activity WHERE datname = current_database() AND $1::int = ANY(pg_blocking_pids(pid)) ORDER BY pid LIMIT 1), 0)::int`, blocker).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting != 0 {
			return waiting
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
	}
}
