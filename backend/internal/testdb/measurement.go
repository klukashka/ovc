//go:build slice6_measurement

package testdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MeasuredQuery struct {
	data pgx.TraceQueryStartData
}

type QueryMeasurements struct {
	mutex   sync.Mutex
	active  bool
	queries []MeasuredQuery
}

func (q *QueryMeasurements) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.active {
		q.queries = append(q.queries, MeasuredQuery{data: data})
	}
	return ctx
}

func (q *QueryMeasurements) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (q *QueryMeasurements) Start() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.queries = nil
	q.active = true
}

func (q *QueryMeasurements) Finish() []MeasuredQuery {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.active = false
	queries := q.queries
	q.queries = nil
	return queries
}

func NewQueryMeasurement(t *testing.T) (*Database, *QueryMeasurements) {
	t.Helper()
	database := NewForLockContention(t)
	tracer := &QueryMeasurements{}
	configuration := database.Pool.Config()
	configuration.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), configuration)
	if err != nil {
		t.Fatal("measurement traced pool could not open")
	}
	t.Cleanup(pool.Close)
	instrumented := &Database{Name: database.Name, URL: database.URL, Pool: pool}
	return instrumented, tracer
}

func LogMeasurementQueryPlans(t *testing.T, database *Database, queries []MeasuredQuery, label string) {
	t.Helper()
	t.Logf("event=measurement.query_count route=%s queries=%d", label, len(queries))
	for index, query := range queries {
		statement := strings.TrimSpace(query.data.SQL)
		if strings.HasPrefix(statement, "-- name:") {
			_, statement, _ = strings.Cut(statement, "\n")
		}
		if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(statement)), "SELECT ") ||
			(!strings.Contains(statement, "FROM messages") && !strings.Contains(statement, "FROM active_channel_members") && !strings.Contains(statement, "FROM channels")) {
			continue
		}
		hash := sha256.Sum256([]byte(query.data.SQL))
		identity := hex.EncodeToString(hash[:])
		rows, err := database.Pool.Query(t.Context(), "EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) "+statement, query.data.Args...)
		if err != nil {
			t.Fatal("measurement query plan failed")
		}
		for rows.Next() {
			var plan string
			if err := rows.Scan(&plan); err != nil {
				rows.Close()
				t.Fatal("measurement query plan row failed")
			}
			t.Logf("event=measurement.query_plan route=%s ordinal=%d sql_sha256=%s plan=%s", label, index, identity, plan)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal("measurement query plan stream failed")
		}
	}
}
