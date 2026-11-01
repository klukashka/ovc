package files

import (
	"context"
	"errors"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func TestUploadRecoveryRetainsCursorAfterCancellationAndWraps(t *testing.T) {
	h := newUploadProtocolFixture(t)
	first := h.intent(t, domain.AttachmentID{1}, false)
	second := h.intent(t, domain.AttachmentID{2}, false)
	later := h.intent(t, domain.AttachmentID{3}, false)
	beforeFirst := h.intentSnapshot(t, first.id)
	beforeSecond := h.intentSnapshot(t, second.id)
	beforeLater := h.intentSnapshot(t, later.id)
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
	if _, err := held.Exec(context.Background(), "SELECT attachment_id FROM upload_intents WHERE attachment_id = $1 FOR UPDATE", first.id); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	type recoveryResult struct {
		count int64
		err   error
	}
	finished := make(chan recoveryResult, 1)
	joined := false
	defer func() {
		cancel()
		if !joined {
			<-finished
		}
	}()
	observing, stopObserving := context.WithTimeout(context.Background(), domain.UploadRecoveryCandidateBudget/2)
	defer stopObserving()

	go func() {
		count, err := h.service.RecoverUploads(ctx, h.now.Add(time.Hour))
		finished <- recoveryResult{count: count, err: err}
	}()
	h.awaitDatabaseLock(t, observing)
	cancel()
	firstResult := <-finished
	joined = true
	retained := h.service.recoveryCursor
	afterFirst := h.intentSnapshot(t, first.id)
	afterSecond := h.intentSnapshot(t, second.id)
	afterLater := h.intentSnapshot(t, later.id)
	canceledOutbox := h.outboxCount(t)
	if err := held.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	released = true
	nextCount, nextErr := h.service.RecoverUploads(context.Background(), h.now.Add(time.Hour))
	skippedFirst := h.intentSnapshot(t, first.id)
	secondRow := h.read(t, second.id)
	laterRow := h.read(t, later.id)
	nextSecond := h.intentSnapshot(t, second.id)
	nextLater := h.intentSnapshot(t, later.id)
	nextOutbox := h.outboxCount(t)
	wrapped := h.service.recoveryCursor
	wrappedCount, wrappedErr := h.service.RecoverUploads(context.Background(), h.now.Add(time.Hour))
	firstRow := h.read(t, first.id)
	finalSecond := h.intentSnapshot(t, second.id)
	finalLater := h.intentSnapshot(t, later.id)

	if firstResult.count != 0 || !errors.Is(firstResult.err, context.Canceled) || retained != first.id {
		t.Fatalf("cancellation lost cursor: count %d error %v cursor %s", firstResult.count, firstResult.err, retained)
	}
	if beforeFirst != afterFirst || beforeSecond != afterSecond || beforeLater != afterLater || canceledOutbox != 0 {
		t.Fatal("canceled pass changed durable state")
	}
	if nextCount != 2 || nextErr != nil || skippedFirst != beforeFirst || nextOutbox != 2 || wrapped != (domain.AttachmentID{}) {
		t.Fatalf("continuation did not process exactly the remaining two candidates and wrap: count %d error %v cursor %s", nextCount, nextErr, wrapped)
	}
	if secondRow.CleanupOutboxID == nil || laterRow.CleanupOutboxID == nil ||
		!secondRow.CleanupRequested || !laterRow.CleanupRequested ||
		secondRow.CleanupResult != domain.UploadCleanupQueued || laterRow.CleanupResult != domain.UploadCleanupQueued {
		t.Fatal("continuation lost durable cleanup ownership")
	}
	if wrappedCount != 3 || wrappedErr != nil || h.service.recoveryCursor != (domain.AttachmentID{}) ||
		firstRow.CleanupOutboxID == nil || !firstRow.CleanupRequested || firstRow.CleanupResult != domain.UploadCleanupQueued ||
		h.outboxCount(t) != 3 || finalSecond != nextSecond || finalLater != nextLater {
		t.Fatalf("wrapped pass lost cleanup ownership: count %d error %v", wrappedCount, wrappedErr)
	}
}

func TestOperatorUploadRetryRetainsOldReceiptsAfterNewCycleAndRetention(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	opened := h.intent(t, domain.AttachmentID{14}, true)
	if err := h.service.discardChatUpload(context.Background(), opened, false); err != nil {
		t.Fatal(err)
	}
	h.tick(t, h.now)
	first, err := h.service.RetryUploadCleanup(context.Background(), opened.id, domain.UploadAttemptID{1})
	if err != nil {
		t.Fatal(err)
	}
	h.tick(t, h.now)
	if _, err := h.service.RecoverUploads(context.Background(), h.now); err != nil {
		t.Fatal(err)
	}

	second, secondErr := h.service.RetryUploadCleanup(context.Background(), opened.id, domain.UploadAttemptID{2})
	if secondErr != nil {
		t.Fatal(secondErr)
	}
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE outbox SET status = 'in_progress', attempts = 8, lease_token = $1, lease_expires_at = $2 WHERE id = $3", domain.LeaseToken{8}, h.now.Add(-time.Second), second.Outbox); err != nil {
		t.Fatal(err)
	}
	if err := h.queue.ReclaimExpired(context.Background(), h.now); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.RecoverUploads(context.Background(), h.now); err != nil {
		t.Fatal(err)
	}
	if _, err := h.queue.Cleanup(context.Background(), h.now.Add(31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	replay, replayErr := h.service.RetryUploadCleanup(context.Background(), opened.id, first.Attempt)
	deadReplay, deadErr := h.service.RetryUploadCleanup(context.Background(), opened.id, second.Attempt)
	_, retained := h.service.deps.Store.Repositories().Outbox.ByID(context.Background(), first.Outbox)
	var count int
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM upload_cleanup_receipts WHERE attachment_id = $1", opened.id).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if deadErr != nil || deadReplay.Status != domain.OutboxDead || deadReplay.Outbox != second.Outbox || secondErr != nil || second.Outbox == first.Outbox || replayErr != nil || replay.Outbox != first.Outbox || replay.Status != domain.OutboxDone || !errors.Is(retained, repositories.ErrNotFound) || count != 2 {
		t.Fatalf("receipt replay changed cycle: second %+v %v replay %+v %v retained %v count %d", second, secondErr, replay, replayErr, retained, count)
	}
}
