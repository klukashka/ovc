package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
)

type ClaimLane struct {
	Name  string
	Slots int
	Poll  time.Duration
	Claim func(ctx context.Context, now time.Time, slot int) (func(context.Context) error, error)
}

type claimLoop struct {
	worker  *Worker
	lane    ClaimLane
	free    chan int
	done    chan error
	running int
}

func newClaimLoop(w *Worker, lane ClaimLane) *claimLoop {
	loop := &claimLoop{worker: w, lane: lane, free: make(chan int, lane.Slots), done: make(chan error, lane.Slots)}
	for slot := range lane.Slots {
		loop.free <- slot
	}
	return loop
}

func (l *claimLoop) start(ctx context.Context, now time.Time) (bool, error) {
	var slot int
	select {
	case slot = <-l.free:
	default:
		return false, nil
	}
	build, err := l.lane.Claim(ctx, now, slot)
	if err != nil || build == nil {
		l.free <- slot
		return false, err
	}
	result := core.Run(ctx, l.worker.logger, "worker-claim-"+l.lane.Name, func() error { return build(ctx) })
	l.running++
	go func() {
		err := <-result
		l.free <- slot
		l.done <- err
	}()
	return true, nil
}

func (l *claimLoop) report(ctx context.Context, err error) {
	l.running--
	l.logFailure(ctx, err)
}

func (l *claimLoop) logFailure(ctx context.Context, err error) {
	if err != nil {
		l.worker.logger.LogAttrs(context.WithoutCancel(ctx), slog.LevelError, "worker.claim_lane.failed", slog.String("lane", l.lane.Name), slog.String("kind", core.ErrorKind(err)))
	}
}

func (l *claimLoop) inFlight() int {
	return l.running
}

func (w *Worker) drainClaimLanes(ctx context.Context, now time.Time) error {
	var failures error
	for _, lane := range w.claimLanes {
		loop := newClaimLoop(w, lane)
		for {
			started, err := loop.start(ctx, now)
			if err != nil {
				failures = errors.Join(failures, err)
				break
			}
			if started {
				continue
			}
			if loop.inFlight() == 0 {
				break
			}
			loop.report(ctx, <-loop.done)
		}
		for loop.inFlight() > 0 {
			loop.report(ctx, <-loop.done)
		}
	}
	return failures
}

func (w *Worker) runClaimLane(stop context.Context, lane ClaimLane) error {
	builds, cancelBuilds := context.WithCancel(stop)
	defer cancelBuilds()
	loop := newClaimLoop(w, lane)
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-stop.Done():
			for loop.inFlight() > 0 {
				loop.report(stop, <-loop.done)
			}
			return nil
		case err := <-loop.done:
			loop.report(stop, err)
			timer.Reset(0)
		case <-timer.C:
			now, err := w.clock.Instant(stop)
			if err != nil {
				if stop.Err() != nil {
					continue
				}
				cancelBuilds()
				for loop.inFlight() > 0 {
					loop.report(stop, <-loop.done)
				}
				return err
			}
			started, err := loop.start(builds, now)
			if err != nil && stop.Err() == nil {
				loop.logFailure(stop, err)
			}
			delay := lane.Poll
			if started {
				delay = 0
			}
			timer.Reset(delay)
		}
	}
}
