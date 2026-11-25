package chat

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	readSignalWindow  = time.Second
	readSignalTimeout = 5 * time.Second
)

type readKey struct {
	reader  domain.UserID
	channel domain.ChannelID
}

type readDecision int

const (
	readIgnore readDecision = iota
	readRunNow
	readRunTrailing
	readRemove
)

type readWindow struct {
	generation uint64
	dirty      bool
}

type readWindows struct {
	closed     bool
	generation uint64
	open       map[readKey]*readWindow
}

func newReadWindows() readWindows {
	return readWindows{open: map[readKey]*readWindow{}}
}

func (w *readWindows) exposed(key readKey) (readDecision, uint64) {
	if w.closed {
		return readIgnore, 0
	}
	if window, found := w.open[key]; found {
		window.dirty = true
		return readIgnore, window.generation
	}
	w.generation++
	w.open[key] = &readWindow{generation: w.generation}
	return readRunNow, w.generation
}

func (w *readWindows) ended(key readKey, generation uint64) readDecision {
	window, found := w.open[key]
	if w.closed || !found || window.generation != generation {
		return readIgnore
	}
	if window.dirty {
		window.dirty = false
		return readRunTrailing
	}
	delete(w.open, key)
	return readRemove
}

func (w *readWindows) close() {
	w.closed = true
	clear(w.open)
}

type readSignals struct {
	mu        sync.Mutex
	windows   readWindows
	timers    map[readKey]core.Timer
	scheduler core.Scheduler
	logger    *slog.Logger
	run       func(context.Context, readKey)
	running   sync.WaitGroup
}

func newReadSignals(scheduler core.Scheduler, logger *slog.Logger, run func(context.Context, readKey)) *readSignals {
	return &readSignals{windows: newReadWindows(), timers: map[readKey]core.Timer{}, scheduler: scheduler, logger: logger, run: run}
}

func (s *readSignals) exposed(ctx context.Context, key readKey) {
	s.mu.Lock()
	decision, generation := s.windows.exposed(key)
	if decision == readRunNow {
		s.arm(key, generation)
	}
	s.mu.Unlock()
	if decision != readRunNow {
		return
	}
	runContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), readSignalTimeout)
	defer cancel()
	s.run(runContext, key)
}

func (s *readSignals) arm(key readKey, generation uint64) {
	s.timers[key] = s.scheduler.AfterFunc(readSignalWindow, func() { s.ended(key, generation) })
}

func (s *readSignals) ended(key readKey, generation uint64) {
	s.mu.Lock()
	decision := s.windows.ended(key, generation)
	switch decision {
	case readRunTrailing:
		s.arm(key, generation)
		s.running.Add(1)
	case readRemove:
		delete(s.timers, key)
	case readIgnore, readRunNow:
	}
	s.mu.Unlock()
	if decision != readRunTrailing {
		return
	}
	core.Go(context.Background(), s.logger, "chat-read-signal", func(ctx context.Context) {
		defer s.running.Done()
		runContext, cancel := context.WithTimeout(ctx, readSignalTimeout)
		defer cancel()
		s.run(runContext, key)
	})
}

func (s *readSignals) close() {
	s.mu.Lock()
	s.windows.close()
	for _, timer := range s.timers {
		timer.Stop()
	}
	clear(s.timers)
	s.mu.Unlock()
	s.running.Wait()
}

func (s *Service) Close() {
	s.reads.close()
}

func (s *Service) readExposed(ctx context.Context, repos repositories.Repositories, reader domain.UserID, channel domain.ChannelID) {
	repos.AfterCommit(ctx, func(committed context.Context) {
		s.reads.exposed(committed, readKey{reader: reader, channel: channel})
	})
}

func (s *Service) signalRead(ctx context.Context, key readKey) {
	var suppressed domain.ReadSignalSuppression
	var recipient domain.UserID
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		suppressed = ""
		if err := repos.Messages.ShareChannel(ctx, key.channel); errors.Is(err, repositories.ErrNotFound) {
			suppressed = domain.ReadSignalGone
			return nil
		} else if err != nil {
			return err
		}
		peer, err := repos.Messages.DirectPeer(ctx, key.channel, key.reader)
		if errors.Is(err, repositories.ErrNotFound) {
			suppressed = domain.ReadSignalGone
			return nil
		}
		if err != nil {
			return err
		}
		recipient = peer
		scope, err := repos.Messages.ReadSignalScope(ctx, key.channel, key.reader, peer)
		switch {
		case errors.Is(err, repositories.ErrNotFound):
			suppressed = domain.ReadSignalGone
			return nil
		case err != nil:
			return err
		case !scope.Writable:
			suppressed = domain.ReadSignalReadOnly
			return nil
		case !scope.BothActive:
			suppressed = domain.ReadSignalInactive
			return nil
		}
		return repos.Signals.Chat(ctx, domain.ChatSignal{Users: []domain.UserID{peer}, Event: domain.FrameReadUpdated, Channel: key.channel})
	})
	switch {
	case err != nil:
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "chat.read_signal_failed", channelAttribute(key.channel), userAttribute(key.reader),
			slog.String("kind", core.ErrorKind(err)))
	case suppressed != "":
		s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "chat.read_signal_suppressed", channelAttribute(key.channel), userAttribute(key.reader),
			slog.String("reason", string(suppressed)))
	default:
		s.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "chat.read_signal_sent", channelAttribute(key.channel), userAttribute(key.reader),
			slog.String("target_user_id", recipient.String()))
	}
}
