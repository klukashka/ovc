package testdb

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const acquireDeadline = 10 * time.Second

func (d *Database) HoldAllConnectionsBut(tb testing.TB, left int32) func() {
	tb.Helper()
	count := d.Pool.Config().MaxConns - left
	held := make([]*pgxpool.Conn, 0, count)
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		for _, connection := range held {
			connection.Release()
		}
	}
	tb.Cleanup(release)
	acquiring, cancel := context.WithTimeout(context.Background(), acquireDeadline)
	defer cancel()
	for range count {
		connection, err := d.Pool.Acquire(acquiring)
		if err != nil {
			tb.Fatalf("hold %d of the pool's connections: %v", count, err)
		}
		held = append(held, connection)
	}
	return release
}
