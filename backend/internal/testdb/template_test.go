package testdb

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"ovc/backend/internal/core"
)

const remoteAdminURL = "postgres://ovc@203.0.113.7:5432/ovc_dev?sslmode=disable"

func TestTemplateDatabaseIsCreatedOnlyOnACheckedTestServer(t *testing.T) {
	t.Parallel()
	remote := core.NewSecret(remoteAdminURL)

	server, source, err := migratedTemplate(context.Background(), remote)

	if !errors.Is(err, core.ErrDatabaseNotLocal) {
		t.Fatalf("a remote server must be refused before the template step, got %v", err)
	}
	if server.pool != nil || source != (template{}) {
		t.Fatal("a refused server must yield neither an administrative pool nor a template")
	}
}

func TestProcessTemplateIsMigratedAndClosedToConnections(t *testing.T) {
	t.Parallel()
	admin := adminURL(t)

	server, source, err := migratedTemplate(context.Background(), admin)

	if err != nil {
		t.Fatalf("prepare the template: %s", describe(err))
	}
	if !templateName.MatchString(source.name) {
		t.Fatalf("the template name %s does not carry the fixture's prefix, creation time and suffix", source.name)
	}
	exists, allowsConnections := templateState(t, server, source.name)
	if !exists || allowsConnections {
		t.Fatalf("the template must exist on the checked server and refuse connections, exists=%t allows=%t", exists, allowsConnections)
	}
}

func TestClonesNeverUseADatabaseTheFixtureDidNotCreate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := requireServer(t, ctx, adminURL(t))
	foreign := createForeignDatabase(t, server, newTemplateName(requireServerNow(t, server)))
	markDatabase(t, server, foreign, "foreign_marker")

	database := New(t)

	if relationExists(t, database, "foreign_marker") {
		t.Fatal("a clone was made from a template-named database this process did not create")
	}
	if !relationExists(t, database, "users") || !relationExists(t, database, "notifications") {
		t.Fatal("a clone does not carry the migrated schema of the process's own template")
	}
}

func TestClonedDatabasesAreIndependent(t *testing.T) {
	t.Parallel()
	first := New(t)
	second := New(t)

	if _, err := first.Pool.Exec(context.Background(), "CREATE TABLE independence_probe (id integer)"); err != nil {
		t.Fatalf("create the probe table: %s", core.ErrorKind(err))
	}
	if _, err := first.Pool.Exec(context.Background(), "INSERT INTO test_clock (id, instant) VALUES (1, now())"); err != nil {
		t.Fatalf("set the test clock: %s", core.ErrorKind(err))
	}

	if !relationExists(t, first, "independence_probe") || relationExists(t, second, "independence_probe") {
		t.Fatal("a table created in one clone must appear only in that clone")
	}
	if rowCount(t, first, "test_clock") != 1 || rowCount(t, second, "test_clock") != 0 {
		t.Fatal("a row written in one clone must appear only in that clone")
	}
}

func TestStaleTemplatesOfACrashedRunAreDropped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := requireServer(t, ctx, adminURL(t))
	now := requireServerNow(t, server)
	stale := createForeignDatabase(t, server, newTemplateName(now.Add(-2*staleTemplateAge)))
	fresh := createForeignDatabase(t, server, newTemplateName(now.Add(-staleTemplateAge/2)))
	unrelatedOld := createForeignDatabase(t, server, "ovc_"+strconv.FormatInt(now.Add(-2*staleTemplateAge).Unix(), 10)+"_test")
	malformed := createForeignDatabase(t, server, templatePrefix+strconv.FormatInt(now.Add(-2*staleTemplateAge).Unix(), 10)+"_kept_test")

	err := dropStaleTemplates(ctx, server, now)

	if err != nil {
		t.Fatalf("drop stale templates: %s", describe(err))
	}
	if databaseExists(t, server, stale) {
		t.Fatal("a template older than an hour must be dropped")
	}
	for _, kept := range []string{fresh, unrelatedOld, malformed} {
		if !databaseExists(t, server, kept) {
			t.Fatalf("database %s is not a stale template and must be kept", kept)
		}
	}
}

func requireServerNow(t *testing.T, server testServer) time.Time {
	t.Helper()
	now, err := serverNow(context.Background(), server)
	if err != nil {
		t.Fatalf("server clock: %s", describe(err))
	}
	return now
}

func createForeignDatabase(t *testing.T, server testServer, name string) string {
	t.Helper()
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := server.pool.Exec(context.Background(), "CREATE DATABASE "+identifier); err != nil {
		t.Fatalf("create database %s: %s", name, core.ErrorKind(err))
	}
	t.Cleanup(func() { drop(t, server.pool, name, identifier) })
	return name
}

func markDatabase(t *testing.T, server testServer, name string, table string) {
	t.Helper()
	databaseURL, err := withDatabaseName(server.admin, name)
	if err != nil {
		t.Fatalf("database URL: %v", err)
	}
	connection, err := pgx.Connect(context.Background(), databaseURL.Reveal())
	if err != nil {
		t.Fatalf("connect to %s: %s", name, core.ErrorKind(err))
	}
	_, execErr := connection.Exec(context.Background(), "CREATE TABLE "+pgx.Identifier{table}.Sanitize()+" (id integer)")
	if err := errors.Join(execErr, connection.Close(context.Background())); err != nil {
		t.Fatalf("mark database %s: %s", name, core.ErrorKind(err))
	}
}

func relationExists(t *testing.T, database *Database, relation string) bool {
	t.Helper()
	var exists bool
	if err := database.Pool.QueryRow(context.Background(), "SELECT to_regclass($1) IS NOT NULL", relation).Scan(&exists); err != nil {
		t.Fatalf("look up %s: %s", relation, core.ErrorKind(err))
	}
	return exists
}

func rowCount(t *testing.T, database *Database, table string) int {
	t.Helper()
	var count int
	if err := database.Pool.QueryRow(context.Background(), "SELECT count(id) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil {
		t.Fatalf("count %s: %s", table, core.ErrorKind(err))
	}
	return count
}

func databaseExists(t *testing.T, server testServer, name string) bool {
	t.Helper()
	exists, _ := templateState(t, server, name)
	return exists
}

func templateState(t *testing.T, server testServer, name string) (bool, bool) {
	t.Helper()
	var allowsConnections bool
	err := server.pool.QueryRow(context.Background(), "SELECT datallowconn FROM pg_database WHERE datname = $1", name).Scan(&allowsConnections)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, false
	case err != nil:
		t.Fatalf("look up database %s: %s", name, core.ErrorKind(err))
	}
	return true, allowsConnections
}
