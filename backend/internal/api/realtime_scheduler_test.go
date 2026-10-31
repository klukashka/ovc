package api_test

import (
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/realtime"
)

type manualScheduler struct {
	mu      sync.Mutex
	elapsed time.Duration
	timers  []*manualTimer
	tickers []*manualTicker
}

type manualTimer struct {
	scheduler *manualScheduler
	after     time.Duration
	due       time.Duration
	run       func()
	done      bool
}

func (timer *manualTimer) Stop() bool {
	timer.scheduler.mu.Lock()
	defer timer.scheduler.mu.Unlock()
	pending := !timer.done
	timer.done = true
	return pending
}

type manualTicker struct {
	scheduler *manualScheduler
	every     time.Duration
	next      time.Duration
	ticks     chan time.Time
	stopped   bool
}

func (ticker *manualTicker) Ticks() <-chan time.Time {
	return ticker.ticks
}

func (ticker *manualTicker) Stop() {
	ticker.scheduler.mu.Lock()
	defer ticker.scheduler.mu.Unlock()
	ticker.stopped = true
}

func (s *manualScheduler) AfterFunc(after time.Duration, run func()) realtime.Timer {
	s.mu.Lock()
	defer s.mu.Unlock()
	timer := &manualTimer{scheduler: s, after: after, due: s.elapsed + after, run: run}
	s.timers = append(s.timers, timer)
	return timer
}

func (s *manualScheduler) NewTicker(every time.Duration) realtime.Ticker {
	s.mu.Lock()
	defer s.mu.Unlock()
	ticker := &manualTicker{scheduler: s, every: every, next: s.elapsed + every, ticks: make(chan time.Time, 1)}
	s.tickers = append(s.tickers, ticker)
	return ticker
}

func (s *manualScheduler) advance(by time.Duration) {
	s.mu.Lock()
	s.elapsed += by
	var due []func()
	for _, timer := range s.timers {
		if !timer.done && timer.due <= s.elapsed {
			timer.done = true
			due = append(due, timer.run)
		}
	}
	for _, ticker := range s.tickers {
		if ticker.stopped || ticker.next > s.elapsed {
			continue
		}
		for ticker.next <= s.elapsed {
			ticker.next += ticker.every
		}
		select {
		case ticker.ticks <- time.Time{}:
		default:
		}
	}
	s.mu.Unlock()
	for _, run := range due {
		run()
	}
}

func (s *manualScheduler) pendingTimers(after time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := 0
	for _, timer := range s.timers {
		if !timer.done && timer.after == after {
			pending++
		}
	}
	return pending
}

func (s *manualScheduler) runningTickers(every time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	running := 0
	for _, ticker := range s.tickers {
		if !ticker.stopped && ticker.every == every {
			running++
		}
	}
	return running
}

func (h *liveHarness) awaitTimers(t *testing.T, scheduler *manualScheduler, after time.Duration, count int) {
	t.Helper()
	h.waitFor(t, "armed timers", func() bool { return scheduler.pendingTimers(after) >= count })
}

func (h *liveHarness) awaitTickers(t *testing.T, scheduler *manualScheduler, every time.Duration, count int) {
	t.Helper()
	h.waitFor(t, "running tickers", func() bool { return scheduler.runningTickers(every) >= count })
}

func newScheduledHarness(t *testing.T, timing realtime.Timing) (*liveHarness, *manualScheduler) {
	t.Helper()
	scheduler := &manualScheduler{}
	h := newLiveHarness(t, liveTuning{gateway: func(deps *realtime.Dependencies) {
		deps.Scheduler = scheduler
		deps.Timing = timing
	}})
	return h, scheduler
}
