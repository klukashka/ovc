package files

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/teststorage"
)

func TestUploadSettlementAccountAndChannelWaitsReleasePermit(t *testing.T) {
	for _, kind := range []string{"account", "channel"} {
		t.Run(kind, func(t *testing.T) {
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
					if r.Method == http.MethodPost && r.URL.Query().Has("uploadId") {
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
			lifetime, stop := context.WithCancel(context.Background())
			defer stop()
			deps := h.service.deps
			deps.Lifetime = lifetime
			deps.Uploads.Settlement = domain.UploadSettlementBudget
			service := New(deps)
			principal, channel := h.tutor(t)
			for range 3 {
				releasePermit, err := service.gate.enter(principal.UserID)
				if err != nil {
					t.Fatal(err)
				}
				defer releasePermit()
			}
			finished := make(chan error, 1)
			go func() {
				_, err := service.UploadChat(context.Background(), ChatUpload{Principal: principal, Channel: channel, Filename: "held.txt", Declared: 5, Body: bytes.NewBufferString("hello"), Finish: func() error { return nil }})
				finished <- err
			}()
			guard := time.NewTimer(lockObservationDeadline)
			defer guard.Stop()
			select {
			case <-reached:
			case <-guard.C:
				t.Fatal("Complete did not reach barrier")
			}
			held, err := h.database.Pool.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			released := false
			defer func() {
				if !released {
					if err := held.Rollback(context.Background()); err != nil {
						t.Error(err)
					}
				}
			}()
			if kind == "account" {
				_, err = held.Exec(context.Background(), "SELECT user_id FROM tutor_profiles WHERE user_id = $1 FOR UPDATE", principal.UserID)
			} else {
				_, err = held.Exec(context.Background(), "SELECT id FROM channels WHERE id = $1 FOR UPDATE", channel)
			}
			if err != nil {
				t.Fatal(err)
			}

			release()
			observing, cancel := context.WithTimeout(context.Background(), lockObservationDeadline)
			defer cancel()
			h.awaitDatabaseLock(t, observing)
			stop()
			var uploadErr error
			select {
			case uploadErr = <-finished:
			case <-observing.Done():
				t.Fatalf("%s wait outlived its settlement context", kind)
			}
			releasePermit, permitErr := service.gate.enter(principal.UserID)
			if permitErr == nil {
				releasePermit()
			}
			if err := held.Rollback(context.Background()); err != nil {
				t.Fatal(err)
			}

			released = true

			if uploadErr == nil || permitErr != nil {
				t.Fatalf("%s wait ended without an error or retained permit: %v permit %v", kind, uploadErr, permitErr)
			}
		})
	}
}
