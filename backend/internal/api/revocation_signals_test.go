package api_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

type signalWatch struct {
	mu        sync.Mutex
	rechecked []domain.UserID
	connected bool
	changed   chan struct{}
}

func (w *signalWatch) record(update func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	update()
	close(w.changed)
	w.changed = make(chan struct{})
}

func (w *signalWatch) ListenerConnected(listener.Generation) {
	w.record(func() { w.connected = true })
}

func (w *signalWatch) ListenerLost(listener.Generation, listener.LossKind) {
	w.record(func() { w.connected = false })
}

func (w *signalWatch) Signal(signal domain.Signal) {
	if recheck, ok := signal.(domain.RecheckSignal); ok {
		w.record(func() { w.rechecked = append(w.rechecked, recheck.Users...) })
	}
}

func (w *signalWatch) waitUntil(t *testing.T, what string, found func() bool) {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for {
		w.mu.Lock()
		done, changed := found(), w.changed
		w.mu.Unlock()
		if done {
			return
		}
		select {
		case <-changed:
		case <-deadline.C:
			t.Fatalf("no %s within 15 seconds", what)
		}
	}
}

func (w *signalWatch) awaitRecheckOf(t *testing.T, user domain.UserID) {
	t.Helper()
	w.waitUntil(t, "recheck signal for the account", func() bool { return slices.Contains(w.rechecked, user) })
}

func (h *authHarness) watchSignals(t *testing.T) *signalWatch {
	t.Helper()
	watch := &signalWatch{changed: make(chan struct{})}
	instance, err := core.NewID[domain.InstanceID](core.CryptoRandom())
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := listener.New(listener.Dependencies{
		DatabaseURL: h.database.URL, Instance: instance, Heartbeats: listener.ConnectionHeartbeats{}, Consumer: watch,
		Logger: slog.New(slog.NewJSONHandler(h.logs, nil)), Timing: listener.DefaultTiming(),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		adapter.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	watch.waitUntil(t, "listening connection", func() bool { return watch.connected })
	return watch
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

type revocationPath struct {
	name    string
	arrange func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T))
}

func (h *authHarness) mustAnswer(t *testing.T, result response, status int) {
	t.Helper()
	if result.status != status {
		t.Fatalf("answered %d %s, want %d", result.status, result.body, status)
	}
}

func revocationPaths() []revocationPath {
	return []revocationPath{
		{"sign-out", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			user := h.seedUser(t, 1, tutorEmail, accountPassword, currentHash)
			token := accessToken(t, h.login(t, tutorEmail, accountPassword))
			return user, func(t *testing.T) {
				h.mustAnswer(t, h.send(t, http.MethodPost, "/api/v1/auth/logout", "", bearer(token)), http.StatusNoContent)
			}
		}},
		{"sign-out everywhere", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			user := h.seedUser(t, 1, tutorEmail, accountPassword, currentHash)
			token := accessToken(t, h.login(t, tutorEmail, accountPassword))
			return user, func(t *testing.T) {
				h.mustAnswer(t, h.send(t, http.MethodPost, "/api/v1/auth/logout-all", "", bearer(token)), http.StatusNoContent)
			}
		}},
		{"refresh-token reuse", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			user := h.seedUser(t, 1, tutorEmail, accountPassword, currentHash)
			stale := refreshValue(t, h.login(t, tutorEmail, accountPassword))
			refreshValue(t, h.refresh(t, stale))
			return user, func(t *testing.T) {
				h.mustAnswer(t, h.refresh(t, stale), http.StatusUnauthorized)
			}
		}},
		{"same-browser confirmation", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			h.register(t, tutorSignUp(newcomerEmail))
			user := h.userID(t, newcomerEmail)
			here := h.login(t, newcomerEmail, newcomerPassword)
			h.login(t, newcomerEmail, newcomerPassword)
			h.tick(t)
			token := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
			return user, func(t *testing.T) {
				h.mustAnswer(t, h.verify(t, fmt.Sprintf(`{"token":%q}`, token), accessToken(t, here)), http.StatusOK)
			}
		}},
		{"reclaim", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			_, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
			h.login(t, newcomerEmail, newcomerPassword)
			return student, func(t *testing.T) {
				h.reclaimed(t, newcomerEmail)
			}
		}},
		{"password reset", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			user := h.seedUser(t, 1, tutorEmail, accountPassword, currentHash)
			h.login(t, tutorEmail, accountPassword)
			token := h.resetToken(t, tutorEmail)
			return user, func(t *testing.T) {
				h.mustAnswer(t, h.reset(t, token), http.StatusOK)
			}
		}},
		{"password change", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			user := h.seedUser(t, 1, tutorEmail, accountPassword, currentHash)
			here := accessToken(t, h.login(t, tutorEmail, accountPassword))
			h.login(t, tutorEmail, accountPassword)
			return user, func(t *testing.T) {
				changed := h.postJSON(t, "/api/v1/auth/password/change",
					fmt.Sprintf(`{"current_password":%q,"new_password":"a brand new password"}`, accountPassword), bearer(here))
				h.mustAnswer(t, changed, http.StatusNoContent)
			}
		}},
		{"deletion", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			user := h.seedUser(t, 1, tutorEmail, accountPassword, currentHash)
			token := accessToken(t, h.login(t, tutorEmail, accountPassword))
			return user, func(t *testing.T) {
				h.mustAnswer(t, h.deleteMe(t, token, accountPassword), http.StatusNoContent)
			}
		}},
		{"operator block", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			user := h.seedUser(t, 1, tutorEmail, accountPassword, currentHash)
			return user, func(t *testing.T) {
				if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
					t.Fatal(err)
				}
			}
		}},
		{"7-day cleanup", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			h.register(t, tutorSignUp(newcomerEmail))
			user := h.userID(t, newcomerEmail)
			return user, func(t *testing.T) {
				h.at(t, authStart.Add(7*24*time.Hour))
				h.tick(t)
			}
		}},
		{"immediate purge", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			h.register(t, tutorSignUp(newcomerEmail))
			user := h.userID(t, newcomerEmail)
			token := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
			return user, func(t *testing.T) {
				h.mustAnswer(t, h.deleteMe(t, token, newcomerPassword), http.StatusNoContent)
			}
		}},
		{"30-day purge", func(t *testing.T, h *authHarness) (domain.UserID, func(t *testing.T)) {
			user := h.seedUser(t, 1, tutorEmail, accountPassword, currentHash)
			token := accessToken(t, h.login(t, tutorEmail, accountPassword))
			h.mustAnswer(t, h.deleteMe(t, token, accountPassword), http.StatusNoContent)
			return user, func(t *testing.T) {
				h.at(t, authStart.Add(30*24*time.Hour))
				h.tick(t)
			}
		}},
	}
}

func TestEveryRevocationPathEmitsARecheckSignal(t *testing.T) {
	t.Parallel()
	for _, path := range revocationPaths() {
		t.Run(path.name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			user, revoke := path.arrange(t, h)
			watch := h.watchSignals(t)

			revoke(t)

			watch.awaitRecheckOf(t, user)
		})
	}
}
