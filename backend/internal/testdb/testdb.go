package testdb

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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

type poolSize int32

const (
	defaultPoolSize           poolSize = 4
	lockContentionPoolSize    poolSize = 6
	templateMigrationPoolSize poolSize = 2
)

func New(tb testing.TB) *Database {
	tb.Helper()
	return newNamed(tb, "ovc_"+core.NewRandomID()[:12]+"_test", true, defaultPoolSize)
}

func NewForLockContention(tb testing.TB) *Database {
	tb.Helper()
	return newNamed(tb, "ovc_"+core.NewRandomID()[:12]+"_test", true, lockContentionPoolSize)
}

func NewMigratedTo(tb testing.TB, version int64) *Database {
	tb.Helper()
	database := NewNamed(tb, "ovc_"+core.NewRandomID()[:12]+"_test", false)
	if err := runMigrations(context.Background(), database.Pool, func(ctx context.Context, provider *goose.Provider) error {
		_, err := provider.UpTo(ctx, version)
		return err
	}); err != nil {
		tb.Fatalf("apply migrations up to %d: %s", version, describe(err))
	}
	return database
}

func NewNamed(tb testing.TB, name string, migrate bool) *Database {
	tb.Helper()
	return newNamed(tb, name, migrate, defaultPoolSize)
}

func newNamed(tb testing.TB, name string, migrate bool, size poolSize) *Database {
	tb.Helper()
	ctx := context.Background()
	admin := adminURL(tb)
	identifier := pgx.Identifier{name}.Sanitize()
	var server testServer
	if migrate {
		server = cloneFromProcessTemplate(tb, ctx, admin, name, identifier)
	} else {
		server = createEmpty(tb, ctx, admin, name, identifier)
	}
	tb.Cleanup(func() { drop(tb, server.pool, name, identifier) })

	databaseURL, err := withDatabaseName(server.admin, name)
	if err != nil {
		tb.Fatalf("test database URL: %v", err)
	}
	pool, err := openBoundedPool(ctx, databaseURL, size)
	if err != nil {
		tb.Fatalf("open the pool for test database %s: %s", name, core.ErrorKind(err))
	}
	tb.Cleanup(pool.Close)
	if err := verifyReachedDatabase(ctx, pool, name); err != nil {
		tb.Fatalf("test database %s: %s", name, describe(err))
	}
	return &Database{Name: name, URL: databaseURL, Pool: pool}
}

func createEmpty(tb testing.TB, ctx context.Context, admin core.Secret, name string, identifier string) testServer {
	tb.Helper()
	server := requireServer(tb, ctx, admin)
	if _, err := server.pool.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		tb.Fatalf("create test database %s: %s", name, core.ErrorKind(err))
	}
	return server
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

type testServer struct {
	admin core.Secret
	pool  *pgxpool.Pool
}

func checkServer(ctx context.Context, admin core.Secret) (testServer, error) {
	if err := core.VerifyLocalDatabase(ctx, admin); err != nil {
		return testServer{}, err
	}
	pool, err := adminPool(admin)
	if err != nil {
		return testServer{}, &stepError{step: "open the pool for the test database server", err: err}
	}
	return testServer{admin: admin, pool: pool}, nil
}

func requireServer(tb testing.TB, ctx context.Context, admin core.Secret) testServer {
	tb.Helper()
	server, err := checkServer(ctx, admin)
	failOnServerCheck(tb, err)
	return server
}

func failOnServerCheck(tb testing.TB, err error) {
	tb.Helper()
	var settingsError *core.SettingsError
	switch {
	case err == nil:
	case errors.Is(err, core.ErrDatabaseNotLocal), errors.As(err, &settingsError):
		tb.Fatalf("test database: %v", err)
	default:
		tb.Fatalf("check the test database server: %s", describe(err))
	}
}

var sharedAdmin struct {
	once  sync.Once
	admin string
	pool  *pgxpool.Pool
	err   error
}

var errAdminURLChanged = errors.New("the administrative pool was opened for another test database server URL")

func adminPool(admin core.Secret) (*pgxpool.Pool, error) {
	sharedAdmin.once.Do(func() {
		sharedAdmin.admin = admin.Reveal()
		sharedAdmin.pool, sharedAdmin.err = openAdministrativePool(context.Background(), admin)
	})
	if sharedAdmin.err == nil && sharedAdmin.admin != admin.Reveal() {
		return nil, errAdminURLChanged
	}
	return sharedAdmin.pool, sharedAdmin.err
}

type template struct {
	name       string
	identifier string
}

const (
	templatePrefix   = "ovc_template_"
	templateSuffix   = "_test"
	staleTemplateAge = time.Hour
	templateLockKey  = int64(0x6f76635f746d706c)
	templateLockWait = time.Minute
)

var templateName = regexp.MustCompile(`^ovc_template_([0-9]{1,18})_[0-9a-f]{12}_test$`)

var processTemplate struct {
	once     sync.Once
	template template
	err      error
}

func migratedTemplate(ctx context.Context, admin core.Secret) (testServer, template, error) {
	server, err := checkServer(ctx, admin)
	if err != nil {
		return testServer{}, template{}, err
	}
	source, err := templateOn(server)
	return server, source, err
}

func templateOn(server testServer) (template, error) {
	processTemplate.once.Do(func() {
		processTemplate.template, processTemplate.err = createTemplateNow(context.Background(), server)
	})
	return processTemplate.template, processTemplate.err
}

func createTemplateNow(ctx context.Context, server testServer) (template, error) {
	now, err := serverNow(ctx, server)
	if err != nil {
		return template{}, err
	}
	return createTemplate(ctx, server, now)
}

func serverNow(ctx context.Context, server testServer) (time.Time, error) {
	var now time.Time
	if err := server.pool.QueryRow(ctx, statementServerNow).Scan(&now); err != nil {
		return time.Time{}, &stepError{step: "read the test database server's clock", err: err}
	}
	return now, nil
}

func newTemplateName(createdAt time.Time) string {
	return templatePrefix + strconv.FormatInt(createdAt.Unix(), 10) + "_" + core.NewRandomID()[:12] + templateSuffix
}

func createTemplate(ctx context.Context, server testServer, now time.Time) (template, error) {
	if err := dropStaleTemplates(ctx, server, now); err != nil {
		return template{}, &stepError{step: "drop stale templates", err: err}
	}
	name := newTemplateName(now)
	created := template{name: name, identifier: pgx.Identifier{name}.Sanitize()}
	if _, err := server.pool.Exec(ctx, "CREATE DATABASE "+created.identifier); err != nil {
		return template{}, &stepError{step: "create the template database", err: err}
	}
	if err := migrateTemplate(ctx, server, created); err != nil {
		if _, dropErr := server.pool.Exec(ctx, "DROP DATABASE IF EXISTS "+created.identifier); dropErr != nil {
			return template{}, errors.Join(err, &stepError{step: "drop the unfinished template database", err: dropErr})
		}
		return template{}, err
	}
	return created, nil
}

func migrateTemplate(ctx context.Context, server testServer, created template) error {
	templateURL, err := withDatabaseName(server.admin, created.name)
	if err != nil {
		return err
	}
	pool, err := openBoundedPool(ctx, templateURL, templateMigrationPoolSize)
	if err != nil {
		return &stepError{step: "open the pool for the template database", err: err}
	}
	migrateErr := verifyReachedDatabase(ctx, pool, created.name)
	if migrateErr == nil {
		migrateErr = runMigrations(ctx, pool, func(ctx context.Context, provider *goose.Provider) error {
			_, err := provider.Up(ctx)
			return err
		})
	}
	pool.Close()
	if migrateErr != nil {
		return &stepError{step: "migrate the template database", err: migrateErr}
	}
	if _, err := server.pool.Exec(ctx, "ALTER DATABASE "+created.identifier+" WITH ALLOW_CONNECTIONS false"); err != nil {
		return &stepError{step: "close the template database to connections", err: err}
	}
	return nil
}

func cloneFromProcessTemplate(tb testing.TB, ctx context.Context, admin core.Secret, name string, identifier string) testServer {
	tb.Helper()
	server, source, err := migratedTemplate(ctx, admin)
	var settingsError *core.SettingsError
	if errors.Is(err, core.ErrDatabaseNotLocal) || errors.As(err, &settingsError) {
		failOnServerCheck(tb, err)
	}
	if err != nil {
		tb.Fatalf("prepare the template database: %s", describe(err))
	}
	if err := clone(ctx, server, source, identifier); err != nil {
		tb.Fatalf("create test database %s from the template: %s", name, describe(err))
	}
	return server
}

func clone(ctx context.Context, server testServer, source template, identifier string) error {
	return underTemplateLock(ctx, server, statementLockTemplatesShared, statementUnlockTemplatesShared, func(connection *pgxpool.Conn) error {
		_, err := connection.Exec(ctx, "CREATE DATABASE "+identifier+" TEMPLATE "+source.identifier)
		return err
	})
}

func dropStaleTemplates(ctx context.Context, server testServer, now time.Time) error {
	return underTemplateLock(ctx, server, statementLockTemplates, statementUnlockTemplates, func(connection *pgxpool.Conn) error {
		names, err := templateNames(ctx, connection)
		if err != nil {
			return err
		}
		for _, name := range names {
			if !isStaleTemplate(name, now) {
				continue
			}
			if _, err := connection.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()); err != nil {
				return &stepError{step: "drop a stale template database", err: err}
			}
		}
		return nil
	})
}

func templateNames(ctx context.Context, connection *pgxpool.Conn) ([]string, error) {
	rows, err := connection.Query(ctx, statementTemplateCandidates, templatePrefix)
	if err != nil {
		return nil, &stepError{step: "list template databases", err: err}
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, &stepError{step: "read template database names", err: err}
	}
	return names, nil
}

func isStaleTemplate(name string, now time.Time) bool {
	match := templateName.FindStringSubmatch(name)
	if match == nil {
		return false
	}
	seconds, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return false
	}
	return time.Unix(seconds, 0).Before(now.Add(-staleTemplateAge))
}

func underTemplateLock(ctx context.Context, server testServer, lock string, unlock string, work func(*pgxpool.Conn) error) error {
	connection, err := server.pool.Acquire(ctx)
	if err != nil {
		return &stepError{step: "acquire a connection for the template lock", err: err}
	}
	lockCtx, cancel := context.WithTimeout(ctx, templateLockWait)
	_, lockErr := connection.Exec(lockCtx, lock, templateLockKey)
	cancel()
	if err := lockErr; err != nil {
		connection.Release()
		return &stepError{step: "take the template lock", err: err}
	}
	workErr := work(connection)
	if _, err := connection.Exec(ctx, unlock, templateLockKey); err != nil {
		closeErr := connection.Conn().Close(ctx)
		connection.Release()
		return errors.Join(workErr, &stepError{step: "release the template lock", err: err}, closeErr)
	}
	connection.Release()
	return workErr
}

const (
	statementCurrentDatabase       = "SELECT current_database()"
	statementServerNow             = "SELECT clock_timestamp()"
	statementLockTemplates         = "SELECT pg_advisory_lock($1)"
	statementUnlockTemplates       = "SELECT pg_advisory_unlock($1)"
	statementLockTemplatesShared   = "SELECT pg_advisory_lock_shared($1)"
	statementUnlockTemplatesShared = "SELECT pg_advisory_unlock_shared($1)"
	statementTemplateCandidates    = "SELECT datname FROM pg_database WHERE starts_with(datname, $1) AND datdba = (SELECT oid FROM pg_roles WHERE rolname = current_user)"
)

func verifyReachedDatabase(ctx context.Context, pool *pgxpool.Pool, name string) error {
	var reached string
	if err := pool.QueryRow(ctx, statementCurrentDatabase).Scan(&reached); err != nil {
		return &stepError{step: "read the database name", err: err}
	}
	if reached != name {
		return errReachedAnotherDatabase
	}
	return nil
}

var errReachedAnotherDatabase = errors.New("the pool reached another database")

type stepError struct {
	step string
	err  error
}

func (e *stepError) Error() string {
	return e.step + ": " + e.err.Error()
}

func (e *stepError) Unwrap() error {
	return e.err
}

func describe(err error) string {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		descriptions := make([]string, 0, len(joined.Unwrap()))
		for _, part := range joined.Unwrap() {
			descriptions = append(descriptions, describe(part))
		}
		return strings.Join(descriptions, "; ")
	}
	var step *stepError
	switch {
	case errors.Is(err, errReachedAnotherDatabase), errors.Is(err, errAdminURLForm), errors.Is(err, errAdminURLChanged):
		return err.Error()
	case errors.As(err, &step):
		return step.step + ": " + describe(step.err)
	}
	return core.ErrorKind(err)
}

func openBoundedPool(ctx context.Context, databaseURL core.Secret, connections poolSize) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL.Reveal())
	if err != nil {
		return nil, fmt.Errorf("parse the test database URL: %w", err)
	}
	config.MaxConns = int32(connections)
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open the test database pool: %w", err)
	}
	return pool, nil
}

const (
	administrativeIdleLifetime = time.Second
	administrativeIdleCheck    = 500 * time.Millisecond
)

func openAdministrativePool(ctx context.Context, admin core.Secret) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(admin.Reveal())
	if err != nil {
		return nil, fmt.Errorf("parse the administrative URL: %w", err)
	}
	config.MaxConnIdleTime = administrativeIdleLifetime
	config.HealthCheckPeriod = administrativeIdleCheck
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open the administrative pool: %w", err)
	}
	return pool, nil
}

func OpenPool(tb testing.TB, databaseURL core.Secret) *pgxpool.Pool {
	tb.Helper()
	pool, err := openBoundedPool(context.Background(), databaseURL, defaultPoolSize)
	if err != nil {
		tb.Fatalf("open pool: %s", core.ErrorKind(err))
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

func runMigrations(ctx context.Context, pool *pgxpool.Pool, apply func(context.Context, *goose.Provider) error) error {
	database := stdlib.OpenDBFromPool(pool)
	files, err := fs.Sub(migrations.Files, ".")
	if err != nil {
		return errors.Join(&stepError{step: "open embedded migrations", err: err}, database.Close())
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, database, files)
	if err != nil {
		return errors.Join(&stepError{step: "create migration provider", err: err}, database.Close())
	}
	return errors.Join(apply(ctx, provider), database.Close())
}

func drop(tb testing.TB, server *pgxpool.Pool, name string, identifier string) {
	if _, err := server.Exec(context.Background(), "DROP DATABASE IF EXISTS "+identifier+" WITH (FORCE)"); err != nil {
		tb.Errorf("drop test database %s: %s", name, core.ErrorKind(err))
	}
}
