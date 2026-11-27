package testclock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"ovc/backend/internal/adapters/clock"
)

type Pause struct {
	reached chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (p *Pause) Reached() <-chan struct{} {
	return p.reached
}

func (p *Pause) Resume() {
	p.once.Do(func() { close(p.resume) })
}

type Holding struct {
	source clock.Source
	mu     sync.Mutex
	pause  *Pause
}

func NewHolding(source clock.Source) *Holding {
	return &Holding{source: source}
}

func (h *Holding) PauseNextInstant() *Pause {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pause != nil {
		panic("the next clock instant is already paused")
	}
	h.pause = &Pause{reached: make(chan struct{}), resume: make(chan struct{})}
	return h.pause
}

func (h *Holding) Instant(ctx context.Context) (time.Time, error) {
	h.mu.Lock()
	pause := h.pause
	h.pause = nil
	h.mu.Unlock()
	if pause != nil {
		close(pause.reached)
		select {
		case <-pause.resume:
		case <-ctx.Done():
			return time.Time{}, fmt.Errorf("wait for a paused clock: %w", ctx.Err())
		}
		if err := ctx.Err(); err != nil {
			return time.Time{}, fmt.Errorf("resume a paused clock: %w", err)
		}
	}
	return h.source.Instant(ctx)
}

func (h *Holding) InstantWithin(ctx context.Context, reader clock.TestClockReader) (time.Time, error) {
	return h.source.InstantWithin(ctx, reader)
}
