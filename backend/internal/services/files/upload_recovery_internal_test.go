package files

import (
	"context"
	"errors"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func TestUploadRecoveryRetainsCursorAcrossBudgetAndWraps(t *testing.T) {
	h := newUploadProtocolFixture(t)
	h.service.deps.Uploads.Candidate = 50 * time.Millisecond
	h.service.deps.Uploads.Recovery = 75 * time.Millisecond
	first := h.intent(t, domain.AttachmentID{1}, false)
	second := h.intent(t, domain.AttachmentID{2}, false)
	later := h.intent(t, domain.AttachmentID{3}, false)
	held, err := h.database.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := held.Exec(context.Background(), "SELECT attachment_id FROM upload_intents WHERE attachment_id IN ($1,$2) ORDER BY attachment_id FOR UPDATE", first.id, second.id); err != nil {
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

	firstCount, firstErr := h.service.RecoverUploads(context.Background(), h.now.Add(time.Hour))
	retained := h.service.recoveryCursor
	nextCount, nextErr := h.service.RecoverUploads(context.Background(), h.now.Add(time.Hour))
	laterRow := h.read(t, later.id)
	wrapped := h.service.recoveryCursor
	if err := held.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	released = true
	h.service.deps.Uploads.Recovery = domain.UploadRecoveryBudget
	h.service.deps.Uploads.Candidate = domain.UploadRecoveryCandidateBudget
	wrappedCount, wrappedErr := h.service.RecoverUploads(context.Background(), h.now.Add(time.Hour))
	firstRow := h.read(t, first.id)
	secondRow := h.read(t, second.id)

	if firstCount != 0 || firstErr == nil || retained != second.id || nextCount != 1 || nextErr != nil || laterRow.CleanupOutboxID == nil || wrapped != (domain.AttachmentID{}) || wrappedCount != 3 || wrappedErr != nil || firstRow.CleanupOutboxID == nil || secondRow.CleanupOutboxID == nil {
		t.Fatalf("recovery lost cursor or wrap: first %d %v cursor %s next %d %v wrap %s final %d %v", firstCount, firstErr, retained, nextCount, nextErr, wrapped, wrappedCount, wrappedErr)
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
