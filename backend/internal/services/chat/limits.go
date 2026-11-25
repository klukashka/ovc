package chat

import (
	"sync"
	"time"

	"ovc/backend/internal/domain"
)

type fixedWindowLimit struct {
	mutex  sync.Mutex
	limit  int
	length time.Duration
	window time.Time
	counts map[domain.UserID]int
}

func newFixedWindowLimit(limit int, length time.Duration) *fixedWindowLimit {
	return &fixedWindowLimit{limit: limit, length: length, counts: map[domain.UserID]int{}}
}

func newWriteLimit() *fixedWindowLimit {
	return newFixedWindowLimit(domain.MessageWritesPerMinute, domain.MessageWriteWindow)
}

func newSearchLimit() *fixedWindowLimit {
	return newFixedWindowLimit(domain.MessageSearchesPerMinute, domain.MessageSearchWindow)
}

func (w *fixedWindowLimit) take(user domain.UserID, now time.Time) (func(), time.Duration, bool) {
	window, retryAfter := domain.FixedWindow(now, w.length)
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if !window.Equal(w.window) {
		w.window = window
		clear(w.counts)
	}
	if w.counts[user] >= w.limit {
		return nil, retryAfter, false
	}
	w.counts[user]++
	return sync.OnceFunc(func() { w.give(user, window) }), 0, true
}

func (w *fixedWindowLimit) give(user domain.UserID, window time.Time) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if window.Equal(w.window) && w.counts[user] > 0 {
		w.counts[user]--
	}
}
