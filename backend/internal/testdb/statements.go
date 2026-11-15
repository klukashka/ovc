package testdb

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const statementStatisticsLibrary = "pg_stat_statements"

type Statement struct {
	Query string
	Calls int64
}

type StatementWindow struct {
	database *Database
	calls    map[int64]Statement
	evicted  int64
}

func WatchStatements(t testing.TB, database *Database) *StatementWindow {
	t.Helper()
	var preloaded string
	if err := database.Pool.QueryRow(t.Context(), "SELECT current_setting('shared_preload_libraries')").Scan(&preloaded); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preloaded, statementStatisticsLibrary) {
		t.Fatalf("the test PostgreSQL does not preload %s; recreate the Compose postgres service so it starts with the setting from docker-compose.yml", statementStatisticsLibrary)
	}
	if _, err := database.Pool.Exec(t.Context(), "CREATE EXTENSION IF NOT EXISTS pg_stat_statements"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(t.Context(), "SELECT pg_stat_statements_reset()"); err != nil {
		t.Fatal(err)
	}
	window := &StatementWindow{database: database}
	window.calls, window.evicted = statementSnapshot(t, database.Pool)
	return window
}

func (w *StatementWindow) Executed(t testing.TB) []Statement {
	t.Helper()
	after, evicted := statementSnapshot(t, w.database.Pool)
	if evicted != w.evicted {
		t.Fatalf("pg_stat_statements evicted entries while the statements were counted (%d to %d deallocations); raise pg_stat_statements.max", w.evicted, evicted)
	}
	var executed []Statement
	for id, statement := range after {
		calls := statement.Calls - w.calls[id].Calls
		if calls > 0 && !strings.Contains(statement.Query, statementStatisticsLibrary) {
			executed = append(executed, Statement{Query: statement.Query, Calls: calls})
		}
	}
	return executed
}

func statementSnapshot(t testing.TB, pool *pgxpool.Pool) (map[int64]Statement, int64) {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT queryid, query, calls FROM pg_stat_statements
 WHERE toplevel AND dbid = (SELECT oid FROM pg_database WHERE datname = current_database())`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	statements := map[int64]Statement{}
	for rows.Next() {
		var id int64
		var statement Statement
		if err := rows.Scan(&id, &statement.Query, &statement.Calls); err != nil {
			t.Fatal(err)
		}
		statements[id] = statement
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var evicted int64
	if err := pool.QueryRow(t.Context(), "SELECT dealloc FROM pg_stat_statements_info").Scan(&evicted); err != nil {
		t.Fatal(err)
	}
	return statements, evicted
}

func GenericPlan(t testing.TB, database *Database, query string) string {
	t.Helper()
	return explain(t, database, "EXPLAIN (GENERIC_PLAN) "+query)
}

func explain(t testing.TB, database *Database, script string) string {
	t.Helper()
	connection, err := database.Pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	results, err := connection.Conn().PgConn().Exec(t.Context(), script).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	for _, result := range results {
		for _, row := range result.Rows {
			plan.Write(row[0])
			plan.WriteByte('\n')
		}
	}
	return plan.String()
}
