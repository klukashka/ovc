package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/outbox"
)

const (
	pollInterval  = time.Second
	effectTimeout = 30 * time.Second
	claimTimeout  = 30 * time.Second
)

type Timing struct {
	Wall        func() time.Time
	ClaimBudget time.Duration
}

func DefaultTiming() Timing {
	return Timing{Wall: clock.System{}.Now, ClaimBudget: outbox.LeaseDuration - 2*effectTimeout}
}

type Perform func(ctx context.Context, row outbox.Row, now time.Time) (domain.OutboxDoneReason, error)

type PerformMeetingRecovery func(context.Context, outbox.Row, time.Time) (outbox.MeetingRecoveryOutcome, error)

type Lane struct {
	Name                   domain.OutboxLane
	Concurrency            int32
	Perform                Perform
	PerformMeetingRecovery PerformMeetingRecovery
}

type Job struct {
	Name    domain.JobName
	Every   time.Duration
	OwnLoop bool
	Run     func(ctx context.Context, now time.Time) (int64, error)
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

type dispatch struct {
	instant      func(ctx context.Context) (time.Time, error)
	mayClaim     func() bool
	untilDrained bool
	stop         context.Context
	abort        context.CancelFunc
}

func (w *Worker) Tick(ctx context.Context, now time.Time) error {
	w.logger.LogAttrs(ctx, slog.LevelDebug, "worker.tick", slog.Time("now", now))
	stopClaiming := w.timing.Wall().Add(w.timing.ClaimBudget)
	if err := w.schedule(ctx, now, w.jobs); err != nil {
		return err
	}
	stop, abort := context.WithCancel(ctx)
	defer abort()
	return w.runLanes(dispatch{
		instant:      func(context.Context) (time.Time, error) { return now, nil },
		mayClaim:     func() bool { return w.timing.Wall().Before(stopClaiming) },
		untilDrained: true,
		stop:         stop,
		abort:        abort,
	})
}

func (w *Worker) Run(ctx context.Context) error {
	stop, abort := context.WithCancel(ctx)
	defer abort()
	shared, alone := w.jobLoops()
	schedulers := []<-chan error{core.Run(stop, w.logger, "worker-scheduler", abortOnFailure(abort, func() error {
		return w.scheduleEverySecond(stop, func(ctx context.Context, now time.Time) error { return w.schedule(ctx, now, shared) })
	}))}
	for _, job := range alone {
		schedulers = append(schedulers, core.Run(stop, w.logger, "worker-job-loop", abortOnFailure(abort, func() error {
			return w.scheduleEverySecond(stop, func(ctx context.Context, now time.Time) error { return w.runJob(ctx, job, now) })
		})))
	}
	lanesErr := w.runLanes(dispatch{
		instant:  w.clock.Instant,
		mayClaim: func() bool { return true },
		stop:     stop,
		abort:    abort,
	})
	abort()
	failures := []error{lanesErr}
	for _, scheduler := range schedulers {
		failures = append(failures, <-scheduler)
	}
	return errors.Join(failures...)
}

func (w *Worker) jobLoops() ([]Job, []Job) {
	var shared, alone []Job
	for _, job := range w.jobs {
		if job.OwnLoop {
			alone = append(alone, job)
			continue
		}
		shared = append(shared, job)
	}
	return shared, alone
}

func abortOnFailure(abort context.CancelFunc, run func() error) func() error {
	return func() error {
		succeeded := false
		defer func() {
			if !succeeded {
				abort()
			}
		}()
		err := run()
		succeeded = err == nil
		return err
	}
}

func (w *Worker) scheduleEverySecond(stop context.Context, pass func(ctx context.Context, now time.Time) error) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop.Done():
			return nil
		case <-ticker.C:
			now, err := w.clock.Instant(stop)
			if err != nil {
				return fmt.Errorf("read the clock: %w", err)
			}
			if err := pass(stop, now); err != nil && stop.Err() == nil {
				return fmt.Errorf("schedule at %s: %w", now.Format(time.RFC3339), err)
			}
		}
	}
}

func (w *Worker) schedule(ctx context.Context, now time.Time, jobs []Job) error {
	for _, job := range jobs {
		if err := w.runJob(ctx, job, now); err != nil {
			return err
		}
	}
	return w.outbox.ReclaimExpired(ctx, now)
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
	attributes := []slog.Attr{
		slog.String("job", string(job.Name)),
		slog.String("result", string(result)),
		slog.Int64("batch_size", processed),
		slog.Int64("duration_ms", w.timing.Wall().Sub(started).Milliseconds()),
	}
	level := slog.LevelInfo
	if runErr != nil {
		level = slog.LevelError
		attributes = append(attributes, slog.String("kind", core.ErrorKind(runErr)))
	}
	w.logger.LogAttrs(ctx, level, "job.run", attributes...)
	return nil
}

func (w *Worker) runLanes(mode dispatch) error {
	results := make([]<-chan error, 0, len(w.lanes))
	for _, lane := range w.lanes {
		results = append(results, core.Run(mode.stop, w.logger, "outbox-lane", abortOnFailure(mode.abort, func() error {
			return w.runLane(lane, mode)
		})))
	}
	failures := make([]error, 0, len(results))
	for _, result := range results {
		failures = append(failures, <-result)
	}
	return errors.Join(failures...)
}

type laneLoop struct {
	lane     Lane
	mode     dispatch
	freed    chan error
	inFlight int32
	failure  error
}

func (w *Worker) runLane(lane Lane, mode dispatch) error {
	laneAttribute := slog.String("lane", string(lane.Name))
	w.logger.LogAttrs(mode.stop, slog.LevelDebug, "outbox.lane.started", laneAttribute, slog.Int("slots", int(lane.Concurrency)))
	loop := &laneLoop{lane: lane, mode: mode, freed: make(chan error, lane.Concurrency)}
	for {
		claimed := w.claimKeepingTheLaneOnPanic(loop)
		if loop.inFlight == 0 && loop.finished(claimed) {
			break
		}
		loop.await()
	}
	w.logger.LogAttrs(context.WithoutCancel(mode.stop), slog.LevelDebug, "outbox.lane.stopped", laneAttribute)
	if loop.failure != nil {
		return fmt.Errorf("outbox lane %s: %w", lane.Name, loop.failure)
	}
	return nil
}

func (l *laneLoop) claiming() bool {
	return l.failure == nil && l.mode.stop.Err() == nil && l.mode.mayClaim()
}

func (l *laneLoop) finished(claimed int) bool {
	return !l.claiming() || (l.mode.untilDrained && claimed == 0)
}

func (l *laneLoop) await() {
	if l.inFlight > 0 && (l.mode.untilDrained || l.inFlight == l.lane.Concurrency || !l.claiming()) {
		l.release(<-l.freed)
		return
	}
	select {
	case err := <-l.freed:
		l.release(err)
	case <-l.mode.stop.Done():
	case <-time.After(pollInterval):
	}
}

func (l *laneLoop) release(err error) {
	l.inFlight--
	if err != nil && l.failure == nil {
		l.failure = err
		l.mode.abort()
	}
}

func (w *Worker) claimKeepingTheLaneOnPanic(loop *laneLoop) (claimed int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.LogPanic(context.WithoutCancel(loop.mode.stop), w.logger, "outbox-lane", fmt.Sprintf("%T", recovered))
			loop.failure = errClaimPanicked
			loop.mode.abort()
			claimed = 0
		}
	}()
	return w.claimIntoFreeSlots(loop)
}

func (w *Worker) claimIntoFreeSlots(loop *laneLoop) int {
	free := loop.lane.Concurrency - loop.inFlight
	if free == 0 || !loop.claiming() {
		return 0
	}
	finishing := context.WithoutCancel(loop.mode.stop)
	now, err := loop.mode.instant(finishing)
	if err != nil {
		loop.failure = fmt.Errorf("read the clock: %w", err)
		loop.mode.abort()
		return 0
	}
	claimContext, cancelClaim := context.WithTimeout(finishing, claimTimeout)
	claims, err := w.outbox.Claim(claimContext, loop.lane.Name, free, now)
	cancelClaim()
	if err != nil {
		w.logger.LogAttrs(finishing, slog.LevelError, "outbox.claim.failed",
			slog.String("lane", string(loop.lane.Name)), slog.String("kind", core.ErrorKind(err)))
		loop.failure = err
		loop.mode.abort()
		return 0
	}
	for _, claim := range claims {
		loop.inFlight++
		core.Go(finishing, w.logger, "outbox-effect", func(effectContext context.Context) {
			var result error
			defer func() {
				if recovered := recover(); recovered != nil {
					core.LogPanic(effectContext, w.logger, "outbox-effect", fmt.Sprintf("%T", recovered))
					result = errCompletionPanicked
				}
				loop.freed <- result
			}()
			result = w.perform(effectContext, loop, claim, now)
		})
	}
	return len(claims)
}

var (
	errEffectPanicked     = errors.New("the effect panicked")
	errCompletionPanicked = errors.New("completing the effect panicked")
	errClaimPanicked      = errors.New("claiming panicked")
)

func (w *Worker) perform(ctx context.Context, loop *laneLoop, claim outbox.Claim, claimedAt time.Time) error {
	result, err := w.attempt(ctx, loop.lane, claim, claimedAt)
	now, clockErr := loop.mode.instant(ctx)
	if clockErr != nil {
		return fmt.Errorf("read the clock: %w", clockErr)
	}
	if err != nil {
		return w.outbox.Fail(ctx, claim, err, now)
	}
	if claim.Row.Kind == domain.KindMeetingCleanup {
		return w.outbox.CompleteMeetingRecovery(ctx, claim, result.meetingRecovery, now)
	}
	return w.outbox.Complete(ctx, claim, result.reason, now)
}

type effectResult struct {
	reason          domain.OutboxDoneReason
	meetingRecovery outbox.MeetingRecoveryOutcome
}

var errEffectHandler = errors.New("outbox effect handler does not match its kind and lane")

func (w *Worker) attempt(ctx context.Context, lane Lane, claim outbox.Claim, now time.Time) (result effectResult, err error) {
	effectContext, cancel := context.WithTimeout(ctx, effectTimeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			core.LogPanic(ctx, w.logger, "outbox-effect", fmt.Sprintf("%T", recovered))
			result, err = effectResult{}, errEffectPanicked
		}
	}()
	if claim.Row.Kind == domain.KindMeetingCleanup {
		if lane.Name != domain.LaneProviderCall || lane.PerformMeetingRecovery == nil {
			return result, errEffectHandler
		}
		result.meetingRecovery, err = lane.PerformMeetingRecovery(effectContext, claim.Row, now)
	} else {
		if lane.Perform == nil {
			return result, errEffectHandler
		}
		result.reason, err = lane.Perform(effectContext, claim.Row, now)
	}
	return result, err
}
