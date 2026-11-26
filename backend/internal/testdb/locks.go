package testdb

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	LockedRowPosition = "tableoid::oid, (ctid::text::point)[0]::int4, (ctid::text::point)[1]::int2"

	RowDirectWaiters = `
    SELECT direct.pid FROM pg_locks AS direct
    WHERE NOT direct.granted AND (
        (direct.locktype = 'transactionid' AND direct.transactionid IN (
            SELECT held.transactionid FROM pg_locks AS held WHERE held.pid = $1 AND held.locktype = 'transactionid' AND held.granted))
        OR (direct.locktype = 'tuple' AND direct.database = (SELECT oid FROM pg_database WHERE datname = current_database())
            AND direct.relation = $2 AND direct.page = $3 AND direct.tuple = $4))
`

	transactionWaiters = `
    SELECT waiting.pid FROM pg_locks AS waiting
    WHERE NOT waiting.granted AND waiting.locktype = 'transactionid' AND waiting.transactionid IN (
        SELECT held.transactionid FROM pg_locks AS held WHERE held.pid = $1 AND held.locktype = 'transactionid' AND held.granted)
`

	advisoryWaiters = `
    SELECT waiting.pid FROM pg_locks AS waiting
    JOIN pg_locks AS held ON held.locktype = 'advisory' AND held.granted AND held.pid = $1
        AND held.database = waiting.database AND held.classid = waiting.classid AND held.objid = waiting.objid
        AND held.objsubid = waiting.objsubid
    WHERE NOT waiting.granted AND waiting.locktype = 'advisory'
`

	lockWaitDeadline = 10 * time.Second
	lockWaitPoll     = 5 * time.Millisecond
)

type HeldRow struct {
	Holder   int32
	Relation uint32
	Page     int32
	Tuple    int16
}

type RowHold struct {
	Row      HeldRow
	holder   pgx.Tx
	released bool
}

func (d *Database) HoldRow(tb testing.TB, lockQuery string, key string) *RowHold {
	tb.Helper()
	holder, err := d.Pool.Begin(context.Background())
	if err != nil {
		tb.Fatal(err)
	}
	hold := &RowHold{holder: holder}
	tb.Cleanup(func() {
		if hold.released {
			return
		}
		if err := holder.Rollback(context.Background()); err != nil {
			tb.Error(err)
		}
	})
	if err := holder.QueryRow(context.Background(), lockQuery, key).Scan(&hold.Row.Relation, &hold.Row.Page, &hold.Row.Tuple); err != nil {
		tb.Fatal(err)
	}
	if err := holder.QueryRow(context.Background(), "SELECT pg_backend_pid()").Scan(&hold.Row.Holder); err != nil {
		tb.Fatal(err)
	}
	return hold
}

func (h *RowHold) Release(tb testing.TB) {
	tb.Helper()
	h.released = true
	if err := h.holder.Commit(context.Background()); err != nil {
		tb.Fatal(err)
	}
}

func (d *Database) DirectWaitersOn(tb testing.TB, row HeldRow) []int32 {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), lockWaitDeadline)
	defer cancel()
	rows, err := d.Pool.Query(ctx, RowDirectWaiters, row.Holder, row.Relation, row.Page, row.Tuple)
	return backendsOf(tb, rows, err)
}

func (d *Database) TransactionWaitersOf(tb testing.TB, holder int32) []int32 {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), lockWaitDeadline)
	defer cancel()
	rows, err := d.Pool.Query(ctx, transactionWaiters, holder)
	return backendsOf(tb, rows, err)
}

func (d *Database) AdvisoryWaitersOf(tb testing.TB, holder int32) []int32 {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), lockWaitDeadline)
	defer cancel()
	rows, err := d.Pool.Query(ctx, advisoryWaiters, holder)
	return backendsOf(tb, rows, err)
}

func (d *Database) AwaitAdvisoryWaitersOf(tb testing.TB, holder int32, count int) []int32 {
	tb.Helper()
	return d.awaitBackends(tb, count, func() []int32 { return d.AdvisoryWaitersOf(tb, holder) })
}

func (d *Database) AwaitDirectWaitersOn(tb testing.TB, row HeldRow, count int) []int32 {
	tb.Helper()
	return d.awaitBackends(tb, count, func() []int32 { return d.DirectWaitersOn(tb, row) })
}

func (d *Database) AwaitTransactionWaitersOf(tb testing.TB, holder int32, count int) []int32 {
	tb.Helper()
	return d.awaitBackends(tb, count, func() []int32 { return d.TransactionWaitersOf(tb, holder) })
}

func (d *Database) awaitBackends(tb testing.TB, count int, read func() []int32) []int32 {
	tb.Helper()
	deadline := time.NewTimer(lockWaitDeadline)
	defer deadline.Stop()
	poll := time.NewTicker(lockWaitPoll)
	defer poll.Stop()
	for {
		backends := read()
		if len(backends) >= count {
			return backends
		}
		select {
		case <-deadline.C:
			tb.Fatalf("%d backends wait on the held lock instead of %d", len(backends), count)
			return nil
		case <-poll.C:
		}
	}
}

func backendsOf(tb testing.TB, rows pgx.Rows, err error) []int32 {
	tb.Helper()
	if err != nil {
		tb.Fatal(err)
	}
	backends, err := pgx.CollectRows(rows, pgx.RowTo[int32])
	if err != nil {
		tb.Fatal(err)
	}
	slices.Sort(backends)
	return slices.Compact(backends)
}
