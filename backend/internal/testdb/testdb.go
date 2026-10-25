package testdb

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"ovc/backend/internal/core"
	"ovc/backend/migrations"
)

var errAdminURLForm = errors.New("OVC_TEST_DATABASE_ADMIN_URL must be a postgres:// URL")

type Database struct {
	Name string
	URL  core.Secret
	Pool *pgxpool.Pool
}

func New(tb testing.TB) *Database {
	tb.Helper()
	return NewNamed(tb, "ovc_"+core.NewRandomID()[:12]+"_test", true)
}

func NewMigratedTo(tb testing.TB, version int64) *Database {
	tb.Helper()
	database := NewNamed(tb, "ovc_"+core.NewRandomID()[:12]+"_test", false)
	runMigrations(tb, database.Pool, func(ctx context.Context, provider *goose.Provider) error {
		_, err := provider.UpTo(ctx, version)
		return err
	})
	return database
}

func NewNamed(tb testing.TB, name string, migrate bool) *Database {
	tb.Helper()
	admin := adminURL(tb)
	ctx := context.Background()
	verifyLocal(tb, ctx, admin)
	server, err := adminPool(admin)
	if err != nil {
		tb.Fatalf("open the pool for the test database server: %s", core.ErrorKind(err))
	}
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := server.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		tb.Fatalf("create test database %s: %s", name, core.ErrorKind(err))
	}
	tb.Cleanup(func() { drop(tb, server, name, identifier) })

	databaseURL, err := withDatabaseName(admin, name)
	if err != nil {
		tb.Fatalf("test database URL: %v", err)
	}
	pool, err := pgxpool.New(ctx, databaseURL.Reveal())
	if err != nil {
		tb.Fatalf("open the pool for test database %s: %s", name, core.ErrorKind(err))
	}
	tb.Cleanup(pool.Close)
	verifyReachedDatabase(tb, ctx, pool, name)
	if migrate {
		applyMigrations(tb, pool)
	}
	return &Database{Name: name, URL: databaseURL, Pool: pool}
}

func adminURL(tb testing.TB) core.Secret {
	tb.Helper()
	admin, err := core.TestDatabaseAdminURLFromEnvironment()
	if err != nil {
		tb.Fatalf("test database: %v", err)
	}
	parsed, err := url.Parse(admin.Reveal())
	if err != nil || parsed.Scheme == "" {
		tb.Fatalf("test database: %v", errAdminURLForm)
	}
	query := parsed.Query()
	if query.Has(sslModeParameter) || query.Has(serviceParameter) || core.TestDatabaseTLSChosenInEnvironment() {
		return admin
	}
	query.Set(sslModeParameter, "disable")
	parsed.RawQuery = query.Encode()
	return core.NewSecret(parsed.String())
}

const (
	sslModeParameter = "sslmode"
	serviceParameter = "service"
)

var sharedAdmin struct {
	once sync.Once
	pool *pgxpool.Pool
	err  error
}

func adminPool(admin core.Secret) (*pgxpool.Pool, error) {
	sharedAdmin.once.Do(func() {
		sharedAdmin.pool, sharedAdmin.err = pgxpool.New(context.Background(), admin.Reveal())
	})
	return sharedAdmin.pool, sharedAdmin.err
}

func verifyReachedDatabase(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, name string) {
	tb.Helper()
	var reached string
	if err := pool.QueryRow(ctx, statementCurrentDatabase).Scan(&reached); err != nil {
		tb.Fatalf("read the name of test database %s: %s", name, core.ErrorKind(err))
	}
	if reached != name {
		tb.Fatalf("test database %s: the pool reached another database", name)
	}
}

func OpenPool(tb testing.TB, databaseURL core.Secret) *pgxpool.Pool {
	tb.Helper()
	pool, err := pgxpool.New(context.Background(), databaseURL.Reveal())
	if err != nil {
		tb.Fatalf("open pool: %s", core.ErrorKind(err))
	}
	tb.Cleanup(pool.Close)
	return pool
}

const statementCurrentDatabase = "SELECT current_database()"

func withDatabaseName(admin core.Secret, name string) (core.Secret, error) {
	parsed, err := url.Parse(admin.Reveal())
	if err != nil || parsed.Scheme == "" {
		return core.Secret{}, errAdminURLForm
	}
	parsed.Path = "/" + name
	return core.NewSecret(parsed.String()), nil
}

func applyMigrations(tb testing.TB, pool *pgxpool.Pool) {
	tb.Helper()
	runMigrations(tb, pool, func(ctx context.Context, provider *goose.Provider) error {
		_, err := provider.Up(ctx)
		return err
	})
}

func runMigrations(tb testing.TB, pool *pgxpool.Pool, apply func(context.Context, *goose.Provider) error) {
	tb.Helper()
	database := stdlib.OpenDBFromPool(pool)
	defer func() {
		if err := database.Close(); err != nil {
			tb.Errorf("close migration connection: %s", core.ErrorKind(err))
		}
	}()
	files, err := fs.Sub(migrations.Files, ".")
	if err != nil {
		tb.Fatalf("open embedded migrations: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, database, files)
	if err != nil {
		tb.Fatalf("create migration provider: %v", err)
	}
	if err := apply(context.Background(), provider); err != nil {
		tb.Fatalf("apply migrations: %s", core.ErrorKind(err))
	}
}

func drop(tb testing.TB, server *pgxpool.Pool, name string, identifier string) {
	if _, err := server.Exec(context.Background(), "DROP DATABASE IF EXISTS "+identifier+" WITH (FORCE)"); err != nil {
		tb.Errorf("drop test database %s: %s", name, core.ErrorKind(err))
	}
}

func verifyLocal(tb testing.TB, ctx context.Context, admin core.Secret) {
	tb.Helper()
	err := core.VerifyLocalDatabase(ctx, admin)
	var settingsError *core.SettingsError
	switch {
	case err == nil:
	case errors.Is(err, core.ErrDatabaseNotLocal), errors.As(err, &settingsError):
		tb.Fatalf("test database: %v", err)
	default:
		tb.Fatalf("check the test database host: %s", core.ErrorKind(err))
	}
}
