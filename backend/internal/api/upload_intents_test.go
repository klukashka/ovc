package api_test

import (
	"context"
	"fmt"
	"net/http"
	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/services/files"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func TestUploadIntentCapacityRefusesWithoutReadingTheBody(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	if _, err := h.database.Pool.Exec(context.Background(), `INSERT INTO upload_intents (attachment_id, storage_key, capacity_class, phase, cleanup_requested, created_at, updated_at)
 SELECT md5(n::text)::uuid, 'chat/capacity/' || n, 'chat', 'cleanup', true, $1, $1 FROM generate_series(1,4096) AS n`, authStart); err != nil {
		t.Fatal(err)
	}
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE upload_intent_capacity SET used = 4096 WHERE capacity_class = 'chat'"); err != nil {
		t.Fatal(err)
	}
	request := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer %s\r\nContent-Type: multipart/form-data; boundary=waiting\r\nContent-Length: 1000\r\n\r\n", chatScope(channel), parties.studentToken)

	result := h.rawRequest(t, request)
	uploads := h.uploadsBy(t, parties.student)

	if result.status != http.StatusServiceUnavailable || result.headers.Get("Connection") != "close" || len(uploads) != 0 {
		t.Fatalf("capacity consumed an unread upload: %s count %d", describe(result), len(uploads))
	}
}

func TestUploadCompleteRequiresConfirmedIntentTransition(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	held.holdNext(isPart("1"))
	upload := h.startChatUpload(t, parties.studentToken, channel)
	upload.begin(t, "revoked.txt", plainText(1024))
	written := upload.completeInBackground(nil)
	held.awaitReached(t)
	row := h.onlyUploadBy(t, parties.student)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE upload_intents SET cleanup_requested = true, phase = 'cleanup' WHERE attachment_id = $1::uuid", row.ID); err != nil {
		t.Fatal(err)
	}

	held.resume()
	result := upload.answer(t)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	_, present := h.storedObject(t, row.Key)
	var started bool
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT completion_started FROM upload_intents WHERE attachment_id = $1::uuid", row.ID).Scan(&started); err != nil {
		t.Fatal(err)
	}

	_, completes := held.multipartWrites()
	if completes != 0 {
		t.Fatalf("revoked authorization sent %d Complete requests", completes)
	}
	if result.status == http.StatusCreated || present || started {
		t.Fatalf("revoked transition completed an object: %s present %t started %t", describe(result), present, started)
	}
	h.assertLeftNothing(t, row)
}

func TestUploadExpiryDoesNotFenceRemoteCompletion(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	held.holdNext(func(r *http.Request) bool { return r.Method == http.MethodPost && r.URL.Query().Get("uploadId") != "" })
	upload := h.startChatUpload(t, parties.studentToken, channel)
	upload.begin(t, "late.txt", plainText(1024))
	written := upload.completeInBackground(nil)
	held.awaitReached(t)
	row := h.onlyUploadBy(t, parties.student)
	later := authStart.Add(domain.UploadDeadlineCeiling + domain.UploadAgeAfterDeadline)
	if _, err := h.files.ExpirePending(context.Background(), later); err != nil {
		t.Fatal(err)
	}

	held.resume()
	result := upload.answer(t)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	state := h.uploadState(t, row.ID)
	h.tick(t)
	_, present := h.storedObject(t, row.Key)

	if result.status == http.StatusCreated || state == string(domain.AttachmentPending) || state == string(domain.AttachmentBound) || present {
		t.Fatalf("expiry permitted late publication: %s state %s stored %t", describe(result), state, present)
	}
}

func TestChatUploadCleanupSurvivesPurgeAndDatabaseTimeout(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t, files.UploadTiming{Settlement: 200 * time.Millisecond})
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	held.holdNext(func(r *http.Request) bool { return r.Method == http.MethodPost && r.URL.Query().Get("uploadId") != "" })
	upload := h.startChatUpload(t, parties.studentToken, channel)
	upload.begin(t, "late completion.txt", plainText(1024))
	written := upload.completeInBackground(nil)
	held.awaitReached(t)
	row := h.onlyUploadBy(t, parties.student)
	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
	h.at(t, authStart.Add(domain.DeletedAccountRetention+time.Hour))
	if _, err := h.purger.PurgeDeleted(context.Background(), authStart.Add(domain.DeletedAccountRetention+time.Hour)); err != nil {
		t.Fatal(err)
	}
	locked, err := h.database.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := locked.Exec(context.Background(), "SELECT attachment_id FROM upload_intents WHERE attachment_id = $1::uuid FOR UPDATE", row.ID); err != nil {
		t.Fatal(err)
	}

	started := clock.System{}.Now()
	held.resume()
	result := upload.answer(t)
	elapsed := time.Since(started)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	_, beforeCleanup := h.storedObject(t, row.Key)
	if err := locked.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.files.RecoverUploads(context.Background(), authStart); err != nil {
		t.Fatal(err)
	}
	h.tick(t)
	_, afterCleanup := h.storedObject(t, row.Key)
	var phase string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT phase FROM upload_intents WHERE attachment_id = $1::uuid", row.ID).Scan(&phase); err != nil {
		t.Fatal(err)
	}

	if result.status == http.StatusCreated || elapsed > 2*time.Second || !beforeCleanup || afterCleanup || phase != string(domain.UploadUncertain) {
		t.Fatalf("purge lost durable cleanup: %s elapsed %s before %t after %t phase %s", describe(result), elapsed, beforeCleanup, afterCleanup, phase)
	}
}

func TestUploadCleanupAcceptsLateCreateIdentifier(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	held.holdNext(func(r *http.Request) bool { return r.Method == http.MethodPost && r.URL.Query().Has("uploads") })
	upload := h.startChatUpload(t, parties.studentToken, channel)
	upload.begin(t, "late create.txt", plainText(1024))
	written := upload.completeInBackground(nil)
	held.awaitReached(t)
	row := h.onlyUploadBy(t, parties.student)
	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
	h.at(t, authStart.Add(domain.DeletedAccountRetention+time.Hour))
	if _, err := h.purger.PurgeDeleted(context.Background(), authStart.Add(domain.DeletedAccountRetention+time.Hour)); err != nil {
		t.Fatal(err)
	}

	held.resume()
	result := upload.answer(t)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	var recorded string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT multipart_upload_id FROM upload_intents WHERE attachment_id = $1::uuid", row.ID).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	h.tick(t)
	remaining := h.countRows(t, "SELECT count(*) FROM upload_intents WHERE attachment_id = $1::uuid", row.ID)
	incomplete := h.incompleteUploads(t, row.Key)

	if result.status == http.StatusCreated || recorded == "" || remaining != 0 || len(incomplete) != 0 {
		t.Fatalf("late Create identifier lost: %s remaining %d incomplete %d", describe(result), remaining, len(incomplete))
	}
}

func TestUploadCleanupBeforeCreateReturns(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	held.holdNext(func(r *http.Request) bool { return r.Method == http.MethodPost && r.URL.Query().Has("uploads") })
	upload := h.startChatUpload(t, parties.studentToken, channel)
	upload.begin(t, "late create.txt", plainText(1024))
	written := upload.completeInBackground(nil)
	held.awaitReached(t)
	row := h.onlyUploadBy(t, parties.student)
	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)
	h.at(t, authStart.Add(domain.DeletedAccountRetention+time.Hour))
	if _, err := h.purger.PurgeDeleted(context.Background(), authStart.Add(domain.DeletedAccountRetention+time.Hour)); err != nil {
		t.Fatal(err)
	}
	h.tick(t)
	before := h.countRows(t, "SELECT count(*) FROM upload_intents WHERE attachment_id = $1::uuid", row.ID)

	held.resume()
	result := upload.answer(t)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	incomplete := h.incompleteUploads(t, row.Key)
	after := h.countRows(t, "SELECT count(*) FROM upload_intents WHERE attachment_id = $1::uuid", row.ID)

	if before != 0 || after != 0 || result.status == http.StatusCreated || len(incomplete) != 0 {
		t.Fatalf("late Create recreated cleanup: %s before %d after %d incomplete %d", describe(result), before, after, len(incomplete))
	}
}

func TestUploadCreateIdentifierPersistenceHasOneSettlementBudget(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t, files.UploadTiming{Settlement: 200 * time.Millisecond})
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	held.holdNext(func(r *http.Request) bool { return r.Method == http.MethodPost && r.URL.Query().Has("uploads") })
	upload := h.startChatUpload(t, parties.studentToken, channel)
	upload.begin(t, "unrecorded.txt", plainText(1024))
	written := upload.completeInBackground(nil)
	held.awaitReached(t)
	row := h.onlyUploadBy(t, parties.student)
	locked, err := h.database.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := locked.Exec(context.Background(), "SELECT attachment_id FROM upload_intents WHERE attachment_id = $1::uuid FOR UPDATE", row.ID); err != nil {
		t.Fatal(err)
	}

	started := clock.System{}.Now()
	held.resume()
	result := upload.answer(t)
	elapsed := time.Since(started)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if err := locked.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, objectExists := h.storedObject(t, row.Key)
	var startedComplete bool
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT completion_started FROM upload_intents WHERE attachment_id = $1::uuid", row.ID).Scan(&startedComplete); err != nil {
		t.Fatal(err)
	}
	incomplete := h.incompleteUploads(t, row.Key)
	for _, id := range incomplete {
		if err := h.objects.AbortMultipartUpload(context.Background(), row.Key, id); err != nil {
			t.Fatal(err)
		}
	}

	parts, completes := held.multipartWrites()
	if parts != 0 || completes != 0 {
		t.Fatalf("unconfirmed identifier sent %d parts and %d Complete requests", parts, completes)
	}
	if result.status == http.StatusCreated || elapsed > 2*time.Second || objectExists || startedComplete || len(incomplete) != 1 {
		t.Fatalf("unconfirmed identifier allowed storage writes: %s elapsed %s object %t complete %t incomplete %d", describe(result), elapsed, objectExists, startedComplete, len(incomplete))
	}
	if h.countLogs(`"msg":"upload.settlement.deadline"`) != 1 {
		t.Fatal("settlement deadline was not recorded once")
	}
}
