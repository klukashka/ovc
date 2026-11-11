package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"ovc/backend/internal/core"
)

const (
	defaultDownloadStallWindow = 30 * time.Second
	defaultDownloadStallBytes  = 16 << 10
	stallWriteChunksPerMinimum = 4
)

var (
	errDownloadStalled = errors.New("the download delivered too little to its client")
	errDownloadCrowded = errors.New("the client address already holds as many long downloads as it may")
)

type deliverySample struct {
	active    time.Duration
	delivered int64
}

type deliveryWindow struct {
	span      time.Duration
	minimum   int64
	bucket    time.Duration
	active    time.Duration
	delivered int64
	samples   []deliverySample
}

func newDeliveryWindow(span time.Duration, minimum int64) deliveryWindow {
	return deliveryWindow{span: span, minimum: minimum, bucket: stallCheckInterval(span)}
}

func stallCheckInterval(span time.Duration) time.Duration {
	return max(span/stallChecksPerWindow, minimumStallCheck)
}

func (w *deliveryWindow) deliveredBy(active time.Duration) int64 {
	var delivered int64
	for _, sample := range w.samples {
		if sample.active > active {
			break
		}
		delivered = sample.delivered
	}
	return delivered
}

func (w *deliveryWindow) prune() {
	keep := 0
	for keep+1 < len(w.samples) && w.samples[keep+1].active <= w.active-w.span {
		keep++
	}
	w.samples = w.samples[keep:]
}

func (w *deliveryWindow) record(spent time.Duration, delivered int) {
	w.active += spent
	w.delivered += int64(delivered)
	sample := deliverySample{active: w.active, delivered: w.delivered}
	if last := len(w.samples) - 1; last >= 0 && w.samples[last].active/w.bucket == sample.active/w.bucket {
		w.samples[last] = sample
	} else {
		w.samples = append(w.samples, sample)
	}
	w.prune()
}

func (w *deliveryWindow) stalled(inFlight time.Duration) bool {
	active := w.active + inFlight
	return active >= w.span && w.delivered-w.deliveredBy(active-w.span) < w.minimum
}

type DownloadTiming struct {
	StallWindow      time.Duration
	StallBytes       int64
	PerAddress       int
	ActivePerAddress int
	LongAfter        time.Duration
}

func (t DownloadTiming) withDefaults() DownloadTiming {
	if t.StallWindow <= 0 {
		t.StallWindow = defaultDownloadStallWindow
	}
	if t.StallBytes <= 0 {
		t.StallBytes = defaultDownloadStallBytes
	}
	if t.PerAddress <= 0 {
		t.PerAddress = defaultLongDownloadsPerAddress
	}
	if t.ActivePerAddress <= 0 {
		t.ActivePerAddress = defaultDownloadsPerAddress
	}
	if t.LongAfter <= 0 {
		t.LongAfter = defaultLongDownloadAfter
	}
	return t
}

type downloadStallGuard struct {
	writer       io.Writer
	mutex        sync.Mutex
	timer        *time.Timer
	stopped      bool
	stalled      bool
	crowded      bool
	writing      bool
	writingSince time.Time
	window       deliveryWindow
	chunk        int
	controller   *http.ResponseController
	ctx          context.Context
	logger       *slog.Logger
	now          func() time.Time
}

func (r *router) guardDownloadAgainstStalls(ctx context.Context, writer io.Writer, controller *http.ResponseController) *downloadStallGuard {
	guard := &downloadStallGuard{
		writer: writer, window: newDeliveryWindow(r.downloads.StallWindow, r.downloads.StallBytes),
		chunk: int(max(r.downloads.StallBytes/stallWriteChunksPerMinimum, 1)), controller: controller, ctx: ctx, logger: r.logger, now: r.now,
	}
	guard.mutex.Lock()
	defer guard.mutex.Unlock()
	guard.timer = time.AfterFunc(guard.window.bucket, guard.check)
	return guard
}

func (g *downloadStallGuard) check() {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.LogPanic(g.ctx, g.logger, "download.stall_guard", fmt.Sprintf("%T", recovered))
		}
	}()
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.stopped {
		return
	}
	if g.writing && g.window.stalled(g.now().Sub(g.writingSince)) {
		g.stalled = true
		if err := g.controller.SetWriteDeadline(g.now()); err != nil && !errors.Is(err, http.ErrNotSupported) {
			g.logger.LogAttrs(g.ctx, slog.LevelWarn, "http.deadline.failed", slog.String("kind", "write"))
		}
		return
	}
	g.timer.Reset(g.window.bucket)
}

func (g *downloadStallGuard) refuseCrowded() {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.stopped {
		return
	}
	g.crowded = true
	if err := g.controller.SetWriteDeadline(g.now()); err != nil && !errors.Is(err, http.ErrNotSupported) {
		g.logger.LogAttrs(g.ctx, slog.LevelWarn, "http.deadline.failed", slog.String("kind", "write"))
	}
}

func (g *downloadStallGuard) stop() {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.stopped = true
	g.timer.Stop()
}

func (g *downloadStallGuard) Write(data []byte) (int, error) {
	written := 0
	for written < len(data) {
		chunk, err := g.writeChunk(data[written:min(len(data), written+g.chunk)])
		written += chunk
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

func (g *downloadStallGuard) writeChunk(data []byte) (int, error) {
	g.mutex.Lock()
	g.writing, g.writingSince = true, g.now()
	g.mutex.Unlock()
	written, err := g.writer.Write(data)
	g.mutex.Lock()
	g.window.record(g.now().Sub(g.writingSince), written)
	g.writing = false
	stalled, crowded := g.stalled, g.crowded
	g.mutex.Unlock()
	switch {
	case err != nil && crowded:
		return written, fmt.Errorf("%w: %w", errDownloadCrowded, err)
	case err != nil && stalled:
		return written, fmt.Errorf("%w: %w", errDownloadStalled, err)
	}
	return written, err
}
