package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	statementCurrentDatabase = "SELECT current_database()"
	pingTimeout              = time.Second
	defaultConnectTimeout    = 5 * time.Second
)

var testDatabaseName = regexp.MustCompile(`^[a-z0-9_]+_(test|dev)$`)

type NowFunc func() time.Time

type Database struct {
	Pool *pgxpool.Pool
	SQL  *sql.DB
}

func (d *Database) Close() error {
	err := d.SQL.Close()
	d.Pool.Close()
	if err != nil {
		return fmt.Errorf("close the database handle: %w", err)
	}
	return nil
}

func (d *Database) Ping(ctx context.Context) error {
	return Ping(ctx, d.Pool)
}

func Connect(ctx context.Context, settings Settings, logger *slog.Logger, now NowFunc) (*Database, error) {
	if err := VerifyTestControlHosts(ctx, settings); err != nil {
		return nil, err
	}
	pool, err := OpenPool(ctx, settings, logger, now)
	if err != nil {
		return nil, err
	}
	if err := connectAndPing(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	if err := VerifyTestControlDatabase(ctx, settings, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Database{Pool: pool, SQL: SQLDatabase(pool)}, nil
}

type DatabaseSummary struct {
	Hosts []string
	Name  string
}

func (s DatabaseSummary) LogValue() slog.Value {
	return slog.GroupValue(slog.String("hosts", strings.Join(s.Hosts, ",")), slog.String("name", s.Name))
}

func SummarizeDatabase(url Secret) (DatabaseSummary, error) {
	config, err := pgxpool.ParseConfig(url.Reveal())
	if err != nil {
		return DatabaseSummary{}, &SettingsError{Variable: varDatabaseURL, Problem: "is not a valid PostgreSQL connection string"}
	}
	return DatabaseSummary{Hosts: databaseHosts(config.ConnConfig), Name: config.ConnConfig.Database}, nil
}

func OpenPool(ctx context.Context, settings Settings, logger *slog.Logger, now NowFunc) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(settings.DatabaseURL.Reveal())
	if err != nil {
		return nil, &SettingsError{Variable: varDatabaseURL, Problem: "is not a valid PostgreSQL connection string"}
	}
	config.MaxConns = settings.DBPoolSize
	if config.ConnConfig.ConnectTimeout == 0 {
		config.ConnConfig.ConnectTimeout = defaultConnectTimeout
	}
	config.ConnConfig.Tracer = &queryTracer{logger: logger, now: now}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	return pool, nil
}

func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	pingContext, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := pool.Ping(pingContext); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

func connectAndPing(ctx context.Context, pool *pgxpool.Pool) error {
	connecting, cancel := context.WithTimeout(ctx, pool.Config().ConnConfig.ConnectTimeout)
	defer cancel()
	conn, err := pool.Acquire(connecting)
	if err != nil {
		return fmt.Errorf("connect to the database: %w", err)
	}
	defer conn.Release()
	pingContext, cancelPing := context.WithTimeout(ctx, pingTimeout)
	defer cancelPing()
	if err := conn.Ping(pingContext); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

func SQLDatabase(pool *pgxpool.Pool) *sql.DB {
	return stdlib.OpenDBFromPool(pool)
}

type TestControlRefusal struct {
	Problem string
}

func (e *TestControlRefusal) Error() string {
	return "OVC_TEST_CONTROL=1 or OVC_MAIL_BACKEND=sink refused: " + e.Problem
}

func VerifyTestControlHosts(ctx context.Context, settings Settings) error {
	if !settings.RequiresTestDatabase() {
		return nil
	}
	config, err := pgxpool.ParseConfig(settings.DatabaseURL.Reveal())
	if err != nil {
		return &SettingsError{Variable: varDatabaseURL, Problem: "is not a valid PostgreSQL connection string"}
	}
	if !testDatabaseName.MatchString(config.ConnConfig.Database) {
		return &TestControlRefusal{Problem: "the database name must end in _test or _dev"}
	}
	local, err := allHostsLocal(ctx, config.ConnConfig)
	if err != nil {
		return err
	}
	if !local {
		return &TestControlRefusal{Problem: "the database host must be loopback or a Unix socket"}
	}
	return nil
}

var ErrDatabaseNotLocal = errors.New("the database host must be loopback or a Unix socket")

func VerifyLocalDatabase(ctx context.Context, url Secret) error {
	config, err := pgxpool.ParseConfig(url.Reveal())
	if err != nil {
		return &SettingsError{Variable: varTestDatabaseURL, Problem: "is not a valid PostgreSQL connection string"}
	}
	local, err := allHostsLocal(ctx, config.ConnConfig)
	if err != nil {
		return err
	}
	if !local {
		return ErrDatabaseNotLocal
	}
	return nil
}

func allHostsLocal(ctx context.Context, config *pgx.ConnConfig) (bool, error) {
	for _, host := range databaseHosts(config) {
		local, err := isLocalDatabaseHost(ctx, host)
		if err != nil {
			return false, err
		}
		if !local {
			return false, nil
		}
	}
	return true, nil
}

func VerifyTestControlDatabase(ctx context.Context, settings Settings, pool *pgxpool.Pool) error {
	if !settings.RequiresTestDatabase() {
		return nil
	}
	var name string
	if err := pool.QueryRow(ctx, statementCurrentDatabase).Scan(&name); err != nil {
		return fmt.Errorf("read database name: %w", err)
	}
	if !testDatabaseName.MatchString(name) {
		return &TestControlRefusal{Problem: "the database name must end in _test or _dev"}
	}
	return nil
}

func databaseHosts(config *pgx.ConnConfig) []string {
	hosts := []string{config.Host}
	for _, fallback := range config.Fallbacks {
		if !slices.Contains(hosts, fallback.Host) {
			hosts = append(hosts, fallback.Host)
		}
	}
	return hosts
}

func isLocalDatabaseHost(ctx context.Context, host string) (bool, error) {
	if strings.HasPrefix(host, "/") {
		return true, nil
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.IsLoopback(), nil
	}
	if host != "localhost" {
		return false, nil
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return false, fmt.Errorf("resolve localhost: %w", err)
	}
	for _, address := range addresses {
		if !address.Unmap().IsLoopback() {
			return false, nil
		}
	}
	return len(addresses) > 0, nil
}

type queryStartKey struct{}

type queryTracer struct {
	logger *slog.Logger
	now    NowFunc
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryStartKey{}, t.now())
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	attrs := []slog.Attr{}
	if start, ok := ctx.Value(queryStartKey{}).(time.Time); ok {
		attrs = append(attrs, slog.Int64("duration_ms", t.now().Sub(start).Milliseconds()))
	}
	if data.Err == nil {
		t.logger.LogAttrs(ctx, slog.LevelDebug, "db.query.completed", attrs...)
		return
	}
	attrs = append(attrs, slog.String("kind", queryErrorKind(data.Err)))
	t.logger.LogAttrs(ctx, slog.LevelWarn, "db.query.failed", attrs...)
}

func ErrorKind(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return "network"
	}
	return queryErrorKind(err)
}

func queryErrorKind(err error) string {
	var pgError *pgconn.PgError
	switch {
	case errors.As(err, &pgError):
		return "sqlstate_" + pgError.Code
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, pgx.ErrNoRows):
		return "no_rows"
	}
	return "error"
}
