package files

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"ovc/backend/internal/adapters/clock"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/teststorage"
)

func TestUploadCleanupRechecksLateCreateDuringDelete(t *testing.T) {
	t.Parallel()
	reached, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := sync.OnceFunc(func() { close(resume) })
	h := newUploadProtocolFixture(t, func(local *teststorage.Storage) storage.Store {
		target, err := url.Parse(local.Settings.Endpoint)
		if err != nil {
			t.Fatal(err)
		}
		forward := httputil.NewSingleHostReverseProxy(target)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete && !r.URL.Query().Has("uploadId") {
				once.Do(func() { close(reached); <-resume })
			}
			forward.ServeHTTP(w, r)
		}))
		t.Cleanup(server.Close)
		t.Cleanup(release)
		settings := local.Settings
		settings.Endpoint = server.URL
		return storage.NewS3(settings)
	})
	opened := openUpload{id: domain.AttachmentID{9}, key: "chat/late-multipart", created: h.now}
	if err := h.service.deps.Store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		return repos.UploadIntents.Create(context.Background(), opened.id, opened.key, domain.UploadCapacityChat, h.now)
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.service.discardChatUpload(context.Background(), opened, false); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- h.worker.Tick(context.Background(), h.now) }()
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("the real object Delete did not reach its barrier")
	}

	upload, err := h.objects.CreateMultipartUpload(context.Background(), opened.key, objectContentType)
	if err != nil {
		t.Fatal(err)
	}
	opened.upload = upload
	allowed, recordedErr := h.service.recordUploadID(context.Background(), opened, h.now)
	release()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup did not finish")
	}
	incomplete, listErr := h.objects.IncompleteUploads(context.Background(), opened.key)
	_, remaining := h.service.deps.Store.Repositories().UploadIntents.ByID(context.Background(), opened.id)

	if allowed || recordedErr != nil || listErr != nil || len(incomplete) != 0 || !errors.Is(remaining, repositories.ErrNotFound) || h.capacity(t) != 0 {
		t.Fatalf("late identity escaped cleanup: allowed %t record %v list %v incomplete %d remaining %v", allowed, recordedErr, listErr, len(incomplete), remaining)
	}
}

const lockObservationDeadline = 30 * time.Second

func TestUploadSettlementDeadlineCoversPoolWaits(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	h.service.deps.Uploads.Settlement = 100 * time.Millisecond
	opened := h.intent(t, domain.AttachmentID{5}, false)
	release := h.blockSettlement(t, "pool", opened.id)
	defer release()
	ctx, cancel := h.service.settlementContext(context.Background(), opened.id)
	defer cancel()

	started := clock.System{}.Now()
	finished := make(chan error, 1)
	go func() { finished <- h.service.discardChatUpload(ctx, opened, false) }()
	guard := time.NewTimer(lockObservationDeadline)
	defer guard.Stop()
	var err error
	select {
	case err = <-finished:
	case <-guard.C:
		t.Fatal("pool wait outlived its settlement budget")
	}
	elapsed := time.Since(started)

	if !errors.Is(err, context.DeadlineExceeded) || elapsed < 50*time.Millisecond {
		t.Fatalf("pool wait escaped settlement budget: %v after %s", err, elapsed)
	}
}

func TestUploadSettlementBoundsItsClockAndIntentLockWaits(t *testing.T) {
	for _, kind := range []string{"clock", "intent"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			h := newUploadProtocolFixture(t)
			lifetime, stop := context.WithCancel(context.Background())
			defer stop()
			deps := h.service.deps
			deps.Lifetime = lifetime
			deps.Uploads.Settlement = time.Minute
			service := New(deps)
			opened := h.intent(t, domain.AttachmentID{5}, false)
			release := h.blockSettlement(t, kind, opened.id)
			defer release()
			settling, cancel := service.settlementContext(context.Background(), opened.id)
			defer cancel()
			finished := make(chan error, 1)
			go func() { finished <- service.discardChatUpload(settling, opened, false) }()
			observing, stopObserving := context.WithTimeout(context.Background(), lockObservationDeadline)
			defer stopObserving()
			h.awaitDatabaseLock(t, observing)

			deadline, bounded := settling.Deadline()
			stop()
			var err error
			select {
			case err = <-finished:
			case <-observing.Done():
				t.Fatalf("%s wait outlived its settlement context", kind)
			}

			if !bounded || time.Until(deadline) > deps.Uploads.Settlement {
				t.Fatalf("%s settlement has no deadline within its budget", kind)
			}
			if err == nil {
				t.Fatalf("%s wait ended without an error", kind)
			}
		})
	}
}

func (h uploadProtocolFixture) blockSettlement(t *testing.T, kind string, id domain.AttachmentID) func() {
	t.Helper()
	if kind == "pool" {
		released := make([]func(), 0, h.database.Pool.Config().MaxConns)
		for range h.database.Pool.Config().MaxConns {
			connection, err := h.database.Pool.Acquire(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			released = append(released, connection.Release)
		}
		return func() {
			for _, release := range released {
				release()
			}
		}
	}
	tx, err := h.database.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if kind == "clock" {
		_, err = tx.Exec(context.Background(), "LOCK TABLE test_clock IN ACCESS EXCLUSIVE MODE")
	} else {
		_, err = tx.Exec(context.Background(), "SELECT attachment_id FROM upload_intents WHERE attachment_id = $1 FOR UPDATE", id)
	}
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	}
}

func TestUploadSettlementHonorsEarlierProcessDrain(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	lifetime, stop := context.WithCancel(context.Background())
	deps := h.service.deps
	deps.Lifetime = lifetime
	deps.Uploads.Settlement = time.Minute
	service := New(deps)
	opened := h.intent(t, domain.AttachmentID{6}, false)
	release := h.blockSettlement(t, "intent", opened.id)
	defer release()
	settling, cancel := service.settlementContext(context.Background(), opened.id)
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- service.discardChatUpload(settling, opened, false) }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		var waiting int
		if err := h.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'").Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("the intent lock was never awaited")
		default:
		}
	}

	stop()
	var err error
	select {
	case err = <-finished:
	case <-time.After(time.Second):
		t.Fatal("settlement outlived the process drain")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("drain cancellation was lost: %v", err)
	}
}

func (h uploadProtocolFixture) awaitDatabaseLock(t *testing.T, ctx context.Context) {
	t.Helper()
	for {
		var waiting int
		if err := h.database.Pool.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'").Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			return
		}
	}
}
