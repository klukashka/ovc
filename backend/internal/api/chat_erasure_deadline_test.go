package api_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/worker"
)

func (h *authHarness) erasureService(t *testing.T, store *repositories.Store, batch int32) *chat.Service {
	t.Helper()
	return h.erasureServiceWithin(t, store, batch, 200*time.Millisecond)
}

func (h *authHarness) erasureServiceWithin(t *testing.T, store *repositories.Store, batch int32, budget time.Duration) *chat.Service {
	t.Helper()
	settings := chat.DefaultErasureSettings()
	settings.Budget = budget
	settings.MessageBatch = batch
	return h.erasureServiceWith(t, store, settings)
}

func (h *authHarness) erasureServiceWith(t *testing.T, store *repositories.Store, settings chat.ErasureSettings) *chat.Service {
	t.Helper()
	return chat.New(chat.Dependencies{Store: store, Clock: h.clock, Random: randomPerTest(t), Addresses: h.files,
		Logger: slog.New(slog.NewJSONHandler(h.logs, nil)), Erasure: settings})
}

type erasureRun struct {
	processed int64
	err       error
}

func awaitErasure(t *testing.T, done <-chan erasureRun) erasureRun {
	t.Helper()
	timer := time.NewTimer(pauseDeadline)
	defer timer.Stop()
	select {
	case result := <-done:
		return result
	case <-timer.C:
		t.Fatal("erasure did not finish")
		return erasureRun{}
	}
}

func runErasure(service *chat.Service, now time.Time) <-chan erasureRun {
	done := make(chan erasureRun, 1)
	go func() {
		count, err := service.EraseChannels(context.Background(), now)
		done <- erasureRun{processed: count, err: err}
	}()
	return done
}

func TestErasureDeadlineCancelsDatabaseWaits(t *testing.T) {
	for _, table := range []string{"tutor_profiles", "channels"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, _ := h.squat(t, token, newcomerEmail, newcomerEmail)
			channel := h.directChannel(t, card.ID)
			h.sent(t, token, channel.ID, "Retained until retry")
			h.mustAnswer(t, h.removeStudent(t, token, card.ID), http.StatusOK)
			query, key := "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", channel.ID
			if table == "tutor_profiles" {
				query, key = "SELECT "+lockedRowPosition+" FROM tutor_profiles WHERE user_id = $1::uuid FOR UPDATE", tutor.String()
			}
			release := h.holdRow(t, query, key)
			service := h.erasureService(t, h.store, 500)

			running := runErasure(service, h.now(t))
			h.waitForLockWaiters(t, 1)
			result := awaitErasure(t, running)
			release()

			if !errors.Is(result.err, context.DeadlineExceeded) || result.processed != 0 || h.messageCount(t, channel.ID) != 1 || !h.directChannel(t, card.ID).ErasePending {
				t.Fatalf("blocked erasure ignored deadline or lost pending work: %+v", result)
			}
			if h.countLogs(`"msg":"channel.erased"`, channel.ID) != 0 {
				t.Fatal("incomplete erasure logged completion")
			}
		})
	}
	t.Run("candidate query", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		held, err := h.database.Pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		released := false
		t.Cleanup(func() {
			if !released {
				if err := held.Rollback(context.Background()); err != nil {
					t.Error(err)
				}
			}
		})
		if _, err := held.Exec(context.Background(), "LOCK TABLE channels IN ACCESS EXCLUSIVE MODE"); err != nil {
			t.Fatal(err)
		}
		service := h.erasureService(t, h.store, 500)

		result := awaitErasure(t, runErasure(service, authStart))
		released = true
		releaseErr := held.Commit(context.Background())

		if releaseErr != nil {
			t.Fatal(releaseErr)
		}
		if !errors.Is(result.err, context.DeadlineExceeded) || result.processed != 0 {
			t.Fatalf("candidate query ignored deadline: %+v", result)
		}
	})
	t.Run("pool acquisition before candidate query", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		acquisition, cancel := context.WithTimeout(context.Background(), pauseDeadline)
		defer cancel()
		releases := make([]func(), 0, h.database.Pool.Config().MaxConns)
		for range h.database.Pool.Config().MaxConns {
			held, err := h.database.Pool.Acquire(acquisition)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(held.Release)
			releases = append(releases, held.Release)
		}
		service := h.erasureService(t, h.store, 500)

		result := awaitErasure(t, runErasure(service, authStart))
		for _, release := range releases {
			release()
		}

		if !errors.Is(result.err, context.DeadlineExceeded) || result.processed != 0 {
			t.Fatalf("pool acquisition ignored deadline: %+v", result)
		}
	})
}

func TestErasureResumesAfterDeadline(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, token, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	first := h.pendingUpload(t, token, channel.ID, "first.txt", 10)
	second := h.pendingUpload(t, token, channel.ID, "second.txt", 20)
	h.sent(t, token, channel.ID, "Committed batch", first)
	h.sent(t, token, channel.ID, "Rolled back batch", second)
	h.mustAnswer(t, h.removeStudent(t, token, card.ID), http.StatusOK)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM attachments WHERE id = $1::uuid FOR UPDATE", second)
	service := h.erasureService(t, h.store, 1)

	running := runErasure(service, h.now(t))
	h.waitForLockWaiters(t, 1)
	interrupted := awaitErasure(t, running)
	release()
	remaining, usage := h.messageCount(t, channel.ID), h.usage(t, tutor)
	pending, completedLogs := h.directChannel(t, card.ID).ErasePending, h.countLogs(`"msg":"channel.erased"`, channel.ID)
	secondCharges, secondReferences := h.chargesOf(t, second), h.referencesOf(t, second)
	queuedBefore := h.deletionQueued(t, second)
	resumed, resumeErr := service.EraseChannels(context.Background(), h.now(t))
	repeated, repeatErr := service.EraseChannels(context.Background(), h.now(t))

	if !errors.Is(interrupted.err, context.DeadlineExceeded) || interrupted.processed != 0 || remaining != 1 || usage != 20 || !pending || completedLogs != 0 || secondCharges != 1 || secondReferences != 1 || queuedBefore {
		t.Fatalf("unfinished batch did not roll back: %+v, remaining %d, usage %d, pending %t, logs %d, charges %d, references %d, queued %t", interrupted, remaining, usage, pending, completedLogs, secondCharges, secondReferences, queuedBefore)
	}
	if resumeErr != nil || repeatErr != nil || resumed != 1 || repeated != 0 || h.usage(t, tutor) != 0 || h.messageCount(t, channel.ID) != 0 || h.directChannel(t, card.ID).ErasePending {
		t.Fatalf("resume or repeated pass failed: %d/%d, %v/%v", resumed, repeated, resumeErr, repeatErr)
	}
	for _, file := range []string{first, second} {
		if h.chargesOf(t, file) != 0 || h.referencesOf(t, file) != 0 || h.countRows(t, "SELECT count(*) FROM outbox WHERE lane = 'storage_delete' AND payload->>'attachment' = $1", file) != 1 {
			t.Fatal("retry duplicated deletion or failed to release quota")
		}
	}
	if h.countLogs(`"msg":"channel.erased"`, channel.ID) != 1 {
		t.Fatal("completion was not recorded exactly once")
	}
}

func TestErasureDeadlineDoesNotStopWorkerScheduling(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, token, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	h.sent(t, token, channel.ID, "Retried on the next scheduled pass")
	h.mustAnswer(t, h.removeStudent(t, token, card.ID), http.StatusOK)
	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM channels WHERE id = $1::uuid FOR UPDATE", channel.ID)
	logger := slog.New(slog.NewJSONHandler(h.logs, nil))
	blockedSettings := chat.DefaultErasureSettings()
	blockedSettings.Budget, blockedSettings.MessageBatch = 200*time.Millisecond, 500
	blockedSettings.Wall = func() time.Time { return authStart }
	bounded := h.erasureServiceWith(t, h.store, blockedSettings)
	recovering := h.erasureServiceWithin(t, h.store, 500, chat.DefaultErasureSettings().Budget)
	passes := []func(context.Context, time.Time) (int64, error){bounded.EraseChannels, recovering.EraseChannels}
	var started atomic.Int32
	erase := func(ctx context.Context, now time.Time) (int64, error) {
		return passes[min(int(started.Add(1)), len(passes))-1](ctx, now)
	}
	jobs := []worker.Job{{Name: domain.JobEraseChannels, Every: time.Minute, Run: erase},
		{Name: domain.JobSessionsCleanup, Every: time.Hour, Run: accounts.CleanupSessions(h.store)}}
	background := worker.New(logger, h.clock, outbox.New(h.store, randomPerTest(t), logger), nil, jobs, worker.DefaultTiming())

	firstErr := background.Tick(context.Background(), authStart)
	release()
	pending := h.directChannel(t, card.ID).ErasePending
	secondErr := background.Tick(context.Background(), authStart.Add(time.Minute))

	if firstErr != nil || secondErr != nil || !pending || h.directChannel(t, card.ID).ErasePending {
		t.Fatalf("scheduler did not recover: %v/%v", firstErr, secondErr)
	}
	if h.countLogs(`"msg":"job.run"`, `"job":"chat.erase_channels"`, `"result":"failed"`, `"kind":"timeout"`) != 1 ||
		h.countLogs(`"msg":"job.run"`, `"job":"chat.erase_channels"`, `"result":"ok"`) != 1 ||
		h.countLogs(`"msg":"job.run"`, `"job":"sessions.cleanup"`, `"result":"ok"`) != 1 {
		t.Fatal("scheduler omitted failed, later-job or recovered diagnostics")
	}
}

func TestErasureDrainsMoreThanOneCandidatePage(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, sender := h.squat(t, token, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID)
	h.seedMessages(t, channel.ID, sender, 501)
	h.mustAnswer(t, h.removeStudent(t, token, card.ID), http.StatusOK)
	statement := `WITH cards AS (
 INSERT INTO relationships (id, tutor_id, state, discard_reason, display_name, discarded_at, created_at, updated_at)
 SELECT ('10000000-0000-4000-8000-' || lpad(number::text, 12, '0'))::uuid, $1::uuid, 'discarded', 'removed', 'Discarded card', $2, $2, $2
 FROM generate_series(1, 100) AS number RETURNING id, tutor_id
 ) INSERT INTO channels (id, kind, tutor_id, relationship_id, last_activity_at, discarded_at, erase_pending_since, created_at, updated_at)
 SELECT id, 'direct', tutor_id, id, $2, $2, $2, $2, $2 FROM cards`
	if _, err := h.database.Pool.Exec(context.Background(), statement, tutor, authStart.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	service := chat.New(chat.Dependencies{Store: h.store, Clock: h.clock, Random: randomPerTest(t), Addresses: h.files,
		Logger: slog.New(slog.NewJSONHandler(h.logs, nil)), Erasure: chat.DefaultErasureSettings()})

	first, firstErr := service.EraseChannels(context.Background(), authStart)
	pendingAfterFirst := h.countRows(t, "SELECT count(*) FROM channels WHERE tutor_id = $1::uuid AND erase_pending_since IS NOT NULL", tutor.String())
	second, secondErr := service.EraseChannels(context.Background(), authStart.Add(time.Minute))

	if firstErr != nil || secondErr != nil || first != 100 || second != 1 || pendingAfterFirst != 1 {
		t.Fatalf("candidate pages did not drain: %d/%d, pending %d, %v/%v", first, second, pendingAfterFirst, firstErr, secondErr)
	}
	if h.messageCount(t, channel.ID) != 0 || h.countRows(t, "SELECT count(*) FROM channels WHERE tutor_id = $1::uuid AND erase_pending_since IS NOT NULL", tutor.String()) != 0 || h.countLogs(`"msg":"channel.erased"`) != 101 {
		t.Fatal("multi-batch channel or candidate backlog did not complete")
	}
}
