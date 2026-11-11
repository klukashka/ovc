package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"ovc/backend/internal/core"
)

const (
	defaultLongDownloadsPerAddress = 8
	defaultDownloadsPerAddress     = 24
	defaultLongDownloadAfter       = 5 * time.Second
	downloadIPv6PrefixBits         = 64
)

type addressDownloads struct {
	mutex       sync.Mutex
	limit       int
	activeLimit int
	held        map[netip.Prefix]int
	active      map[netip.Prefix]int
}

func newAddressDownloads(limit int, activeLimit int) *addressDownloads {
	return &addressDownloads{limit: limit, activeLimit: activeLimit, held: map[netip.Prefix]int{}, active: map[netip.Prefix]int{}}
}

func downloadPrefix(address netip.Addr) netip.Prefix {
	bits := address.BitLen()
	if address.Is6() {
		bits = downloadIPv6PrefixBits
	}
	prefix, err := address.Prefix(bits)
	if err != nil {
		panic("a client address has no prefix of its own length")
	}
	return prefix
}

func (d *addressDownloads) admit(address netip.Addr) (func(), bool) {
	prefix := downloadPrefix(address)
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.held[prefix] >= d.limit || d.active[prefix] >= d.activeLimit {
		return nil, false
	}
	d.active[prefix]++
	return sync.OnceFunc(func() { d.leave(prefix) }), true
}

func (d *addressDownloads) leave(prefix netip.Prefix) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	decrement(d.active, prefix)
}

func (d *addressDownloads) claim(prefix netip.Prefix) bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.held[prefix] >= d.limit {
		return false
	}
	d.held[prefix]++
	return true
}

func (d *addressDownloads) release(prefix netip.Prefix) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	decrement(d.held, prefix)
}

func decrement(counts map[netip.Prefix]int, prefix netip.Prefix) {
	if counts[prefix] <= 1 {
		delete(counts, prefix)
		return
	}
	counts[prefix]--
}

type longDownload struct {
	mutex   sync.Mutex
	owner   *addressDownloads
	prefix  netip.Prefix
	timer   *time.Timer
	counted bool
	stopped bool
	refuse  func()
	ctx     context.Context
	logger  *slog.Logger
}

func (r *router) watchLongDownload(ctx context.Context, address netip.Addr, refuse func()) *longDownload {
	watch := &longDownload{owner: r.addresses, prefix: downloadPrefix(address), refuse: refuse, ctx: ctx, logger: r.logger}
	watch.mutex.Lock()
	defer watch.mutex.Unlock()
	watch.timer = time.AfterFunc(r.downloads.LongAfter, watch.promote)
	return watch
}

func (w *longDownload) promote() {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.LogPanic(w.ctx, w.logger, "download.address_watch", fmt.Sprintf("%T", recovered))
		}
	}()
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.stopped {
		return
	}
	if !w.owner.claim(w.prefix) {
		w.refuse()
		return
	}
	w.counted = true
	w.logger.LogAttrs(w.ctx, slog.LevelDebug, "files.get.long_running")
}

func (w *longDownload) stop() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.stopped = true
	w.timer.Stop()
	if w.counted {
		w.counted = false
		w.owner.release(w.prefix)
	}
}
