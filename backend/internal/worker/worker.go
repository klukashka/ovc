package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/clock"
)

const tickInterval = time.Second

type Worker struct {
	logger *slog.Logger
	clock  clock.Source
}

func New(logger *slog.Logger, source clock.Source) *Worker {
	return &Worker{logger: logger, clock: source}
}

func (w *Worker) Tick(ctx context.Context, now time.Time) error {
	w.logger.LogAttrs(ctx, slog.LevelDebug, "worker.tick", slog.Time("now", now))
	return nil
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			now, err := w.clock.Instant(ctx)
			if err != nil {
				return fmt.Errorf("read the clock: %w", err)
			}
			if err := w.Tick(ctx, now); err != nil {
				return fmt.Errorf("tick at %s: %w", now.Format(time.RFC3339), err)
			}
		}
	}
}
