package realtime

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

type presence struct {
	revision        uint64
	persisted       uint64
	changed         chan struct{}
	queued          bool
	persisting      bool
	failed          error
	deadline        time.Time
	announced       bool
	settling        bool
	settle          Timer
	announcePending bool
	gone            bool
}

var ErrPresencePending = errors.New("presence persistence is pending")

func (g *Gateway) liveLocked(found *account) bool {
	for s := range found.sockets {
		if s.phase == phaseReady && !s.closed {
			return true
		}
	}
	return false
}

func (g *Gateway) desiredPresenceLocked(found *account) bool {
	for s := range found.sockets {
		if s.presenceWanted && !s.closed {
			return true
		}
	}
	return false
}

func (g *Gateway) openSocketsLocked(found *account) int { return len(found.sockets) }

func (g *Gateway) arrivedLocked(s *socket, found *account, wasLive bool) {
	if !wasLive {
		g.announceLocked(s.user, found)
	}
}

func (g *Gateway) leaveLocked(s *socket, found *account) {
	delete(found.sockets, s)
	if s.presenceWanted && !found.gone {
		found.revision++
		g.queuePresenceLocked(s.user, found)
	}
	g.forgetLocked(s.user, found)
}

func (g *Gateway) queuePresenceLocked(user domain.UserID, found *account) {
	if found.queued || found.persisting || found.gone {
		return
	}
	found.queued = true
	found.failed = nil
	found.deadline = (clock.System{}).Now().Add(g.deps.Timing.PresenceTimeout)
	g.presenceQueue <- user
}

func (g *Gateway) presenceWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case user := <-g.presenceQueue:
			g.writePresence(ctx, user)
		}
	}
}

func (g *Gateway) writePresence(ctx context.Context, user domain.UserID) {
	g.mu.Lock()
	found, ok := g.accounts[user]
	if !ok {
		g.mu.Unlock()
		return
	}
	found.queued = false
	found.persisting = true
	version, online := found.revision, g.desiredPresenceLocked(found)
	persist, announce := version != found.persisted, found.announcePending
	deadline := found.deadline
	g.mu.Unlock()
	operation, cancel := context.WithDeadline(ctx, deadline)
	var kept []domain.UserID
	var err error
	var announceErr error
	if persist {
		kept, err = g.deps.Chat.PersistPresence(operation, g.instance, []domain.UserID{user}, online)
	}
	if err == nil && announce {
		announceErr = g.deps.Chat.AnnouncePresence(operation, user, online)
	}
	cancel()
	g.mu.Lock()
	defer g.mu.Unlock()
	found.persisting = false
	found.failed = err
	if err != nil {
		g.persistFailed(ctx, 1, online, err)
		if !online {
			g.deps.Logger.LogAttrs(ctx, slog.LevelError, "presence.release.pending", slog.String("kind", core.ErrorKind(err)), slog.Int("pending_accounts", g.pendingPresenceLocked()))
		}
	} else {
		if persist {
			found.persisted = version
			found.gone = len(kept) == 0
			if found.gone {
				found.persisted = found.revision
				found.announcePending = false
			}
		}
		if announce && announceErr == nil {
			found.announcePending = false
		}
		if persist && !online {
			g.announceLocked(user, found)
		}
	}
	if announceErr != nil {
		found.failed = announceErr
		g.deps.Logger.LogAttrs(ctx, slog.LevelError, "presence.announce.failed", slog.String("kind", core.ErrorKind(announceErr)))
	}
	close(found.changed)
	found.changed = make(chan struct{})
	if found.revision != version {
		g.queuePresenceLocked(user, found)
	}
	g.forgetLocked(user, found)
}

func (s *socket) preparePresence(ctx context.Context) error {
	operation, cancel := context.WithTimeout(ctx, s.g.deps.Timing.PresenceTimeout)
	defer cancel()
	s.g.mu.Lock()
	found := s.g.accounts[s.user]
	if s.closed || found == nil {
		s.g.mu.Unlock()
		return ErrPresencePending
	}
	if !s.presenceWanted {
		s.presenceWanted = true
		found.revision++
		s.g.queuePresenceLocked(s.user, found)
	}
	target := found.revision
	s.g.mu.Unlock()
	for {
		s.g.mu.Lock()
		if s.closed || found.gone {
			s.g.mu.Unlock()
			return ErrPresencePending
		}
		if found.persisted >= target {
			s.g.mu.Unlock()
			return nil
		}
		if found.failed != nil {
			err := found.failed
			s.g.mu.Unlock()
			return err
		}
		changed := found.changed
		s.g.mu.Unlock()
		select {
		case <-changed:
		case <-s.done:
			return ErrPresencePending
		case <-operation.Done():
			return operation.Err()
		}
	}
}

func (g *Gateway) persistFailed(ctx context.Context, accounts int, online bool, err error) {
	g.deps.Logger.LogAttrs(ctx, slog.LevelError, "presence.batch.failed", slog.Bool("online", online), slog.Int("accounts", accounts), slog.String("kind", core.ErrorKind(err)))
}

func (g *Gateway) announceLocked(user domain.UserID, found *account) {
	if found.settling || g.closing || found.gone {
		return
	}
	online := g.liveLocked(found)
	if online == found.announced {
		return
	}
	found.announced = online
	found.settling = true
	found.announcePending = true
	found.settle = g.deps.Scheduler.AfterFunc(g.deps.Timing.PresenceSettle, func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		found.settling = false
		if !g.closing && g.accounts[user] == found {
			g.announceLocked(user, found)
		}
		g.forgetLocked(user, found)
	})
	g.queuePresenceLocked(user, found)
}

func (g *Gateway) extendOnSchedule(ctx context.Context) {
	if err := g.ExtendPresence(ctx); err != nil && ctx.Err() == nil {
		g.persistFailed(ctx, 0, true, err)
	}
}

func (g *Gateway) ExtendPresence(ctx context.Context) error {
	operation, cancel := context.WithTimeout(ctx, g.deps.Timing.PresenceTimeout)
	defer cancel()
	g.mu.Lock()
	targets := make(map[*account]uint64, len(g.accounts))
	for user, found := range g.accounts {
		if found.gone {
			continue
		}
		if g.desiredPresenceLocked(found) || found.revision != found.persisted || found.announcePending {
			found.revision++
			targets[found] = found.revision
			g.queuePresenceLocked(user, found)
		}
	}
	g.mu.Unlock()
	for found, target := range targets {
		for {
			g.mu.Lock()
			done, err, changed := found.persisted >= target || found.gone, found.failed, found.changed
			g.mu.Unlock()
			if err != nil {
				return err
			}
			if done {
				break
			}
			select {
			case <-changed:
			case <-operation.Done():
				return operation.Err()
			}
		}
	}
	return nil
}

func (g *Gateway) pendingPresenceLocked() int {
	count := 0
	for _, found := range g.accounts {
		if found.revision != found.persisted {
			count++
		}
	}
	return count
}

func (g *Gateway) pendingPresence() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.pendingPresenceLocked()
}
