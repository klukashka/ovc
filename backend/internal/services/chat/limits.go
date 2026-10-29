package chat

import (
	"sync"
	"time"

	"ovc/backend/internal/domain"
)

type writeLimit struct {
	mutex  sync.Mutex
	window time.Time
	counts map[domain.UserID]int
}

func newWriteLimit() *writeLimit {
	return &writeLimit{counts: map[domain.UserID]int{}}
}

func (w *writeLimit) take(user domain.UserID, now time.Time) (func(), time.Duration, bool) {
	window, retryAfter := domain.FixedWindow(now, domain.MessageWriteWindow)
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if !window.Equal(w.window) {
		w.window = window
		clear(w.counts)
	}
	if w.counts[user] >= domain.MessageWritesPerMinute {
		return nil, retryAfter, false
	}
	w.counts[user]++
	return sync.OnceFunc(func() { w.give(user, window) }), 0, true
}

func (w *writeLimit) give(user domain.UserID, window time.Time) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if window.Equal(w.window) && w.counts[user] > 0 {
		w.counts[user]--
	}
}
