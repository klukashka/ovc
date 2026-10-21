package api

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"ovc/backend/internal/core"
)

const readinessReuseWindow = time.Second

var errReadinessPanicked = errors.New("the readiness check panicked")

type sharedReadiness struct {
	check     ReadinessFunc
	now       func() time.Time
	logger    *slog.Logger
	mu        sync.Mutex
	inFlight  chan struct{}
	result    error
	checkedAt time.Time
	checked   bool
	joined    int
}

func (s *sharedReadiness) run(ctx context.Context) error {
	s.mu.Lock()
	if s.checked && s.now().Sub(s.checkedAt) < readinessReuseWindow {
		result := s.result
		s.mu.Unlock()
		return result
	}
	if s.inFlight == nil {
		done := make(chan struct{})
		s.inFlight = done
		core.Go(context.Background(), s.logger, "readiness-check", func(checkContext context.Context) {
			s.perform(checkContext, done)
		})
	}
	wait := s.inFlight
	s.joined++
	s.mu.Unlock()

	select {
	case <-wait:
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.result
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *sharedReadiness) joinedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.joined
}

func (s *sharedReadiness) perform(ctx context.Context, done chan struct{}) {
	result := errReadinessPanicked
	defer func() {
		s.mu.Lock()
		s.result = result
		s.checkedAt = s.now()
		s.checked = true
		s.inFlight = nil
		s.mu.Unlock()
		close(done)
	}()
	result = s.check(ctx)
}
