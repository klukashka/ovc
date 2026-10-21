package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/outbox"
)

const (
	tickInterval  = time.Second
	effectTimeout = 30 * time.Second
)

type Timing struct {
	Wall        func() time.Time
	ClaimBudget time.Duration
}

func DefaultTiming() Timing {
	return Timing{Wall: clock.System{}.Now, ClaimBudget: outbox.LeaseDuration - 2*effectTimeout}
}

type Perform func(ctx context.Context, row outbox.Row, now time.Time) (domain.OutboxDoneReason, error)

type Lane struct {
	Name        domain.OutboxLane
	Concurrency int32
	Perform     Perform
}

type Job struct {
	Name  domain.JobName
	Every time.Duration
	Run   func(ctx context.Context, now time.Time) (int64, error)
}

type Worker struct {
	logger *slog.Logger
	clock  clock.Source
	outbox *outbox.Service
	lanes  []Lane
	jobs   []Job
	timing Timing
}

func New(logger *slog.Logger, source clock.Source, service *outbox.Service, lanes []Lane, jobs []Job, timing Timing) *Worker {
	return &Worker{logger: logger, clock: source, outbox: service, lanes: lanes, jobs: jobs, timing: timing}
}

func (w *Worker) Tick(ctx context.Context, now time.Time) error {
	w.logger.LogAttrs(ctx, slog.LevelDebug, "worker.tick", slog.Time("now", now))
	stopClaiming := w.timing.Wall().Add(w.timing.ClaimBudget)
	for _, job := range w.jobs {
		if err := w.runJob(ctx, job, now); err != nil {
			return err
		}
	}
	if err := w.outbox.ReclaimExpired(ctx, now); err != nil {
		return err
	}
	for _, lane := range w.lanes {
		if err := w.drain(ctx, lane, now, stopClaiming); err != nil {
			return err
		}
	}
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
			if err := w.Tick(ctx, now); err != nil && ctx.Err() == nil {
				return fmt.Errorf("tick at %s: %w", now.Format(time.RFC3339), err)
			}
		}
	}
}

func (w *Worker) runJob(ctx context.Context, job Job, now time.Time) error {
	due, err := w.outbox.JobDue(ctx, job.Name, job.Every, now)
	if err != nil || !due {
		return err
	}
	started := w.timing.Wall()
	result := domain.JobResultOK
	processed, runErr := job.Run(ctx, now)
	if runErr != nil {
		result = domain.JobResultFailed
	}
	if err := w.outbox.RecordJob(ctx, job.Name, result, now); err != nil {
		return err
	}
	level := slog.LevelInfo
	if runErr != nil {
		level = slog.LevelError
	}
	w.logger.LogAttrs(ctx, level, "job.run",
		slog.String("job", string(job.Name)),
		slog.String("result", string(result)),
		slog.Int64("batch_size", processed),
		slog.Int64("duration_ms", w.timing.Wall().Sub(started).Milliseconds()),
	)
	if runErr != nil {
		return fmt.Errorf("job %s: %w", job.Name, runErr)
	}
	return nil
}

func (w *Worker) drain(ctx context.Context, lane Lane, now time.Time, stopClaiming time.Time) error {
	for ctx.Err() == nil && w.timing.Wall().Before(stopClaiming) {
		claims, err := w.outbox.Claim(ctx, lane.Name, lane.Concurrency, now)
		if err != nil || len(claims) == 0 {
			return err
		}
		if err := w.performAll(ctx, lane, claims, now); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) performAll(ctx context.Context, lane Lane, claims []outbox.Claim, now time.Time) error {
	finishing := context.WithoutCancel(ctx)
	errs := make([]error, len(claims))
	var wait sync.WaitGroup
	for index, claim := range claims {
		wait.Add(1)
		core.Go(finishing, w.logger, "outbox-effect", func(effectContext context.Context) {
			defer wait.Done()
			errs[index] = w.perform(effectContext, lane, claim, now)
		})
	}
	wait.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) perform(ctx context.Context, lane Lane, claim outbox.Claim, now time.Time) error {
	reason, err := w.attempt(ctx, lane, claim, now)
	if err != nil {
		return w.outbox.Fail(ctx, claim, err, now)
	}
	return w.outbox.Complete(ctx, claim, reason, now)
}

var errEffectPanicked = errors.New("the effect panicked")

func (w *Worker) attempt(ctx context.Context, lane Lane, claim outbox.Claim, now time.Time) (reason domain.OutboxDoneReason, err error) {
	effectContext, cancel := context.WithTimeout(ctx, effectTimeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			core.LogPanic(ctx, w.logger, "outbox-effect", fmt.Sprintf("%T", recovered))
			reason, err = "", errEffectPanicked
		}
	}()
	return lane.Perform(effectContext, claim.Row, now)
}
