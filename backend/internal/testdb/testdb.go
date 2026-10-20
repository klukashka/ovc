package testdb

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
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

func NewNamed(tb testing.TB, name string, migrate bool) *Database {
	tb.Helper()
	admin, err := core.TestDatabaseAdminURLFromEnvironment()
	if err != nil {
		tb.Fatalf("test database: %v", err)
	}
	ctx := context.Background()
	adminConnection, err := pgx.Connect(ctx, admin.Reveal())
	if err != nil {
		tb.Fatalf("connect to the test database server: %v", err)
	}
	defer closeConnection(tb, adminConnection)
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := adminConnection.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		tb.Fatalf("create test database: %v", err)
	}
	tb.Cleanup(func() { drop(tb, admin, identifier) })

	databaseURL, err := withDatabaseName(admin, name)
	if err != nil {
		tb.Fatalf("test database URL: %v", err)
	}
	pool, err := pgxpool.New(ctx, databaseURL.Reveal())
	if err != nil {
		tb.Fatalf("open test database pool: %v", err)
	}
	tb.Cleanup(pool.Close)
	if migrate {
		applyMigrations(tb, pool)
	}
	return &Database{Name: name, URL: databaseURL, Pool: pool}
}

func OpenPool(tb testing.TB, databaseURL core.Secret) *pgxpool.Pool {
	tb.Helper()
	pool, err := pgxpool.New(context.Background(), databaseURL.Reveal())
	if err != nil {
		tb.Fatalf("open pool: %v", err)
	}
	tb.Cleanup(pool.Close)
	return pool
}

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
	database := stdlib.OpenDBFromPool(pool)
	defer func() {
		if err := database.Close(); err != nil {
			tb.Errorf("close migration connection: %v", err)
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
	if _, err := provider.Up(context.Background()); err != nil {
		tb.Fatalf("apply migrations: %v", err)
	}
}

func drop(tb testing.TB, admin core.Secret, identifier string) {
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, admin.Reveal())
	if err != nil {
		tb.Errorf("connect to drop test database: %v", err)
		return
	}
	defer closeConnection(tb, connection)
	if _, err := connection.Exec(ctx, "DROP DATABASE IF EXISTS "+identifier+" WITH (FORCE)"); err != nil {
		tb.Errorf("drop test database: %v", err)
	}
}

func closeConnection(tb testing.TB, connection *pgx.Conn) {
	if err := connection.Close(context.Background()); err != nil {
		tb.Errorf("close admin connection: %v", err)
	}
}
