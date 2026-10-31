package chat_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/worker"
)

func TestPresenceExpiryCleanupIsBoundedAndPreservesRenewedLeases(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	ctx := context.Background()
	instant := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	account := domain.UserID{1}
	if _, err := database.Pool.Exec(ctx, seedTutor, account, instant); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO user_presence VALUES ($1, $2, $3)`, account, instant.Add(time.Minute), instant.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO user_presence_instances SELECT $1, gen_random_uuid(), $2, $3 FROM generate_series(1, 1001)`, account, instant, instant.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	live := domain.PresenceInstanceID{2}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO user_presence_instances VALUES ($1, $2, $3, $4)`, account, live, instant.Add(time.Minute), instant.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	store := repositories.NewStore(database.Pool)
	logger := slog.New(slog.DiscardHandler)
	service := chat.New(chat.Dependencies{Store: store, Logger: logger})
	queue := outbox.New(store, nil, logger)
	jobs := []worker.Job{{Name: domain.JobExpirePresence, Every: time.Minute, Run: service.ExpirePresence}}
	running := worker.New(logger, clock.System{}, queue, nil, jobs, worker.DefaultTiming())

	runErr := running.Tick(ctx, instant)

	if runErr != nil {
		t.Fatal(runErr)
	}
	var leases int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM user_presence_instances WHERE user_id = $1`, account).Scan(&leases); err != nil {
		t.Fatal(err)
	}
	var until, seen time.Time
	if err := database.Pool.QueryRow(ctx, `SELECT online_until, last_seen_at FROM user_presence WHERE user_id = $1`, account).Scan(&until, &seen); err != nil {
		t.Fatal(err)
	}
	if leases != 1 || !until.Equal(instant.Add(time.Minute)) || !seen.Equal(instant.Add(-time.Second)) {
		t.Fatalf("cleanup: leases %d, until %s, seen %s", leases, until, seen)
	}
}

func TestPresenceCleanupDeadlineIncludesUserLockWait(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	ctx := context.Background()
	instant := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	account := domain.UserID{1}
	if _, err := database.Pool.Exec(ctx, seedTutor, account, instant); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO user_presence VALUES ($1, $2, $2)`, account, instant); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO user_presence_instances VALUES ($1, $2, $3, $3)`, account, domain.PresenceInstanceID{1}, instant); err != nil {
		t.Fatal(err)
	}
	locked, err := database.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if rollbackErr := locked.Rollback(ctx); rollbackErr != nil {
			t.Error(rollbackErr)
		}
	})
	if _, err := locked.Exec(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, account); err != nil {
		t.Fatal(err)
	}
	service := chat.New(chat.Dependencies{Store: repositories.NewStore(database.Pool), Logger: slog.New(slog.DiscardHandler), PresenceCleanup: chat.PresenceCleanupSettings{Wall: clock.System{}.Now, Budget: 100 * time.Millisecond, Batch: 500}})

	deleted, cleanupErr := service.ExpirePresence(ctx, instant)

	if deleted != 0 || !errors.Is(cleanupErr, context.DeadlineExceeded) {
		t.Fatalf("cleanup: deleted %d, error %v", deleted, cleanupErr)
	}
	var leases int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM user_presence_instances`).Scan(&leases); err != nil {
		t.Fatal(err)
	}
	if leases != 1 {
		t.Fatalf("cleanup lost %d leases", 1-leases)
	}
}

func TestPresenceInstanceWritesPreserveOtherLeasesAndActivity(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	ctx := context.Background()
	instant := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	account := domain.UserID{1}
	if _, err := database.Pool.Exec(ctx, seedTutor, account, instant); err != nil {
		t.Fatal(err)
	}
	source := clock.NewShared(database.Pool)
	service := chat.New(chat.Dependencies{Store: repositories.NewStore(database.Pool), Clock: source, Logger: slog.New(slog.DiscardHandler)})
	first, second := domain.PresenceInstanceID{1}, domain.PresenceInstanceID{2}
	write := func(instance domain.PresenceInstanceID, at time.Time, online bool) {
		t.Helper()
		if _, err := source.Set(ctx, at); err != nil {
			t.Fatal(err)
		}
		if _, err := service.PersistPresence(ctx, instance, []domain.UserID{account}, online); err != nil {
			t.Fatal(err)
		}
	}
	write(first, instant, true)
	write(second, instant.Add(20*time.Second), true)

	write(first, instant.Add(-time.Second), true)
	write(second, instant.Add(5*time.Second), false)

	var until, seen time.Time
	if err := database.Pool.QueryRow(ctx, `SELECT online_until, last_seen_at FROM user_presence WHERE user_id = $1`, account).Scan(&until, &seen); err != nil {
		t.Fatal(err)
	}
	if !until.Equal(instant.Add(domain.PresenceWindow)) || !seen.Equal(instant.Add(20*time.Second)) {
		t.Fatalf("other-instance release or older extension changed summary: until %s, seen %s", until, seen)
	}
	var leases int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM user_presence_instances WHERE user_id = $1`, account).Scan(&leases); err != nil {
		t.Fatal(err)
	}
	if leases != 1 {
		t.Fatalf("retained leases: %d", leases)
	}
}

func TestAnOlderReleaseCannotBackdateTheAccountsOfflineInstant(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	ctx := context.Background()
	instant := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	account := domain.UserID{1}
	if _, err := database.Pool.Exec(ctx, seedTutor, account, instant); err != nil {
		t.Fatal(err)
	}
	source := clock.NewShared(database.Pool)
	service := chat.New(chat.Dependencies{Store: repositories.NewStore(database.Pool), Clock: source, Logger: slog.New(slog.DiscardHandler)})
	first, second := domain.PresenceInstanceID{1}, domain.PresenceInstanceID{2}
	write := func(instance domain.PresenceInstanceID, at time.Time, online bool) {
		t.Helper()
		if _, err := source.Set(ctx, at); err != nil {
			t.Fatal(err)
		}
		if _, err := service.PersistPresence(ctx, instance, []domain.UserID{account}, online); err != nil {
			t.Fatal(err)
		}
	}
	write(first, instant, true)
	write(second, instant.Add(20*time.Second), true)

	write(second, instant.Add(30*time.Second), false)
	write(first, instant.Add(10*time.Second), false)
	var until, seen time.Time
	if err := database.Pool.QueryRow(ctx, `SELECT online_until, last_seen_at FROM user_presence WHERE user_id = $1`, account).Scan(&until, &seen); err != nil {
		t.Fatal(err)
	}

	if !until.Equal(instant.Add(30*time.Second)) || !seen.Equal(until) {
		t.Fatalf("older sampled release backdated offline state: until %s, seen %s", until, seen)
	}
}

func TestPresenceCleanupCannotBackdateTheAccountsOfflineInstant(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	ctx := context.Background()
	instant := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	account := domain.UserID{1}
	if _, err := database.Pool.Exec(ctx, seedTutor, account, instant); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO user_presence VALUES ($1,$2,$2)`, account, instant); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO user_presence_instances VALUES ($1,$2,$3,$4),($1,$5,$6,$4)`, account, domain.PresenceInstanceID{1}, instant.Add(-10*time.Second), instant.Add(-time.Minute), domain.PresenceInstanceID{2}, instant.Add(-5*time.Second)); err != nil {
		t.Fatal(err)
	}
	service := chat.New(chat.Dependencies{Store: repositories.NewStore(database.Pool), Logger: slog.New(slog.DiscardHandler), PresenceCleanup: chat.PresenceCleanupSettings{Wall: clock.System{}.Now, Budget: time.Second, Batch: 1}})

	deleted, cleanupErr := service.ExpirePresence(ctx, instant)
	var until, seen time.Time
	if err := database.Pool.QueryRow(ctx, `SELECT online_until, last_seen_at FROM user_presence WHERE user_id = $1`, account).Scan(&until, &seen); err != nil {
		t.Fatal(err)
	}

	if cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
	if deleted != 2 || !until.Equal(instant) || !seen.Equal(instant) {
		t.Fatalf("cleanup backdated activity: deleted %d, until %s, seen %s", deleted, until, seen)
	}
}
