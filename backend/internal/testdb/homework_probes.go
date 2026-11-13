package testdb

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HomeworkProbe struct{ query pgx.TraceQueryStartData }

func (p HomeworkProbe) SQL() string { return p.query.SQL }

type HomeworkProbeTrace struct {
	mu      sync.Mutex
	queries []HomeworkProbe
}

func (p *HomeworkProbeTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, query pgx.TraceQueryStartData) context.Context {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queries = append(p.queries, HomeworkProbe{query: query})
	return ctx
}

func (p *HomeworkProbeTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (p *HomeworkProbeTrace) Take() []HomeworkProbe {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := p.queries
	p.queries = nil
	return result
}

func NewHomeworkProbePool(t testing.TB, database *Database) (*pgxpool.Pool, *HomeworkProbeTrace) {
	t.Helper()
	trace := &HomeworkProbeTrace{}
	config := database.Pool.Config()
	config.ConnConfig.Tracer = trace
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, trace
}

func AssertHomeworkProbeIndex(t testing.TB, pool *pgxpool.Pool, probe HomeworkProbe, expected string) {
	t.Helper()
	rows, err := pool.Query(t.Context(), "EXPLAIN "+probe.query.SQL, probe.query.Args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), expected) {
		t.Fatalf("probe does not use %s: %s", expected, plan.String())
	}
}
