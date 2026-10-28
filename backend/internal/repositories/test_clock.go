package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ovc/backend/internal/dbgen"
)

type TestClock struct {
	queries *dbgen.Queries
}

func (r TestClock) ReadTestClock(ctx context.Context) (time.Time, bool, error) {
	instant, err := r.queries.ReadTestClock(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("read the test clock: %w", err)
	}
	return instant, true, nil
}
