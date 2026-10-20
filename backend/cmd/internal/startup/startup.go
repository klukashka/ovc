package startup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"ovc/backend/internal/core"
	"ovc/backend/migrations"
)

var ErrMigrationsPending = errors.New("database migrations are pending; run the migrate command first")

func newProvider(db *sql.DB, withLock bool) (*goose.Provider, error) {
	options := []goose.ProviderOption{}
	if withLock {
		locker, err := lock.NewPostgresSessionLocker()
		if err != nil {
			return nil, fmt.Errorf("create the migration lock: %w", err)
		}
		options = append(options, goose.WithSessionLocker(locker))
	}
	files, err := fs.Sub(migrations.Files, ".")
	if err != nil {
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files, options...)
	if err != nil {
		return nil, fmt.Errorf("create the migration provider: %w", err)
	}
	return provider, nil
}

func CheckMigrations(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	provider, err := newProvider(db, false)
	if err != nil {
		return err
	}
	pending, err := provider.HasPending(ctx)
	if err != nil {
		return fmt.Errorf("check pending migrations: %w", err)
	}
	if pending {
		logger.LogAttrs(ctx, slog.LevelError, "migrations.pending")
		return ErrMigrationsPending
	}
	return nil
}

func Migrate(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	logger.LogAttrs(ctx, slog.LevelInfo, "migrate.started")
	provider, err := newProvider(db, true)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "migrate.failed", slog.String("kind", "provider"))
		return err
	}
	results, err := provider.Up(ctx)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "migrate.failed", slog.String("kind", "apply"))
		return fmt.Errorf("apply migrations: %w", err)
	}
	for _, result := range results {
		logger.LogAttrs(ctx, slog.LevelInfo, "migrate.applied", slog.Int64("version", result.Source.Version))
	}
	return nil
}

func Describe(err error) string {
	var settingsError *core.SettingsError
	var refusal *core.TestControlRefusal
	switch {
	case errors.As(err, &settingsError):
		return settingsError.Error()
	case errors.As(err, &refusal):
		return refusal.Error()
	case errors.Is(err, ErrMigrationsPending):
		return ErrMigrationsPending.Error()
	case errors.Is(err, core.ErrPanicked):
		return "a background goroutine panicked; see the panic.recovered record"
	}
	return "start-up failed (" + core.ErrorKind(err) + "); see the process.failed record"
}
