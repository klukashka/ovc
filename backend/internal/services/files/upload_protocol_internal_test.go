package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
	"ovc/backend/internal/worker"
)

type recoveryDatabaseError interface {
	error
	SQLState() string
}

type uploadProtocolFixture struct {
	service  *Service
	database *testdb.Database
	objects  *storage.S3
	queue    *outbox.Service
	worker   *worker.Worker
	now      time.Time
}

func newUploadProtocolFixture(t *testing.T, objectsFor ...func(*teststorage.Storage) storage.Store) uploadProtocolFixture {
	t.Helper()
	return uploadProtocolFixtureWithDatabase(t, testdb.NewForLockContention(t), objectsFor...)
}

func uploadProtocolFixtureWithDatabase(t *testing.T, database *testdb.Database, objectsFor ...func(*teststorage.Storage) storage.Store) uploadProtocolFixture {
	t.Helper()
	local := teststorage.New(t)
	store := repositories.NewStore(database.Pool)
	logger := slog.New(slog.DiscardHandler)
	source := clock.NewShared(database.Pool)
	now := time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)
	if _, err := source.Set(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	var objects storage.Store = local.Store
	if len(objectsFor) != 0 {
		objects = objectsFor[0](local)
	}
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service := New(Dependencies{Store: store, Objects: objects, Keys: keys, Clock: source, Random: core.CryptoRandom(), Logger: logger})
	queue := outbox.New(store, core.CryptoRandom(), logger)
	background := worker.New(logger, source, queue, []worker.Lane{{Name: domain.LaneStorageDelete, Concurrency: 1, Perform: service.DeleteObject}}, nil, worker.DefaultTiming())
	return uploadProtocolFixture{service: service, database: database, objects: local.Store, queue: queue, worker: background, now: now}
}

func (h uploadProtocolFixture) intent(t *testing.T, id domain.AttachmentID, complete bool) openUpload {
	t.Helper()
	opened := openUpload{id: id, key: domain.ChatKeyPrefix + id.String(), created: h.now}
	upload, err := h.objects.CreateMultipartUpload(context.Background(), opened.key, objectContentType)
	if err != nil {
		t.Fatal(err)
	}
	opened.upload = upload
	err = h.service.deps.Store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		if err := repos.UploadIntents.Create(context.Background(), id, opened.key, domain.UploadCapacityChat, h.now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = h.service.deps.Store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		if _, err := repos.UploadIntents.Lock(context.Background(), id); err != nil {
			return err
		}
		if err := repos.UploadIntents.RecordMultipart(context.Background(), id, opened.key, string(upload), h.now); err != nil {
			return err
		}
		if complete {
			return repos.UploadIntents.AuthorizeComplete(context.Background(), id, opened.key, string(upload), h.now)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return opened
}

func (h uploadProtocolFixture) tick(t *testing.T, now time.Time) {
	t.Helper()
	if err := h.worker.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
}

func (h uploadProtocolFixture) read(t *testing.T, id domain.AttachmentID) repositories.UploadIntent {
	t.Helper()
	row, err := h.service.deps.Store.Repositories().UploadIntents.ByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (h uploadProtocolFixture) capacity(t *testing.T) int {
	t.Helper()
	var used int
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT COALESCE(sum(used), 0)::integer FROM upload_intent_capacity").Scan(&used); err != nil {
		t.Fatal(err)
	}
	return used
}

func TestUploadIntentCleanupGenerationsAreFinite(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	opened := h.intent(t, domain.AttachmentID{1}, true)
	if err := h.service.discardChatUpload(context.Background(), opened, false); err != nil {
		t.Fatal(err)
	}
	h.tick(t, h.now)
	initial := h.read(t, opened.id)

	for range 3 {
		if err := h.service.discardChatUpload(context.Background(), opened, true); err != nil {
			t.Fatal(err)
		}
		if _, err := h.service.RecoverUploads(context.Background(), h.now); err != nil {
			t.Fatal(err)
		}
	}
	initialGeneration := domain.UploadInitialCleanup
	repeated, lateErr := h.service.finishIntentDeletion(context.Background(), repositories.ClaimedOutboxRow{ID: *initial.CleanupOutboxID}, domain.AttachmentPayload{Key: opened.key, Generation: &initialGeneration}, initial, h.now)
	final := h.read(t, opened.id)
	h.tick(t, h.now)
	_, remaining := h.service.deps.Store.Repositories().UploadIntents.ByID(context.Background(), opened.id)
	var cycles int
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM outbox WHERE lane = 'storage_delete'").Scan(&cycles); err != nil {
		t.Fatal(err)
	}

	if initial.Phase != domain.UploadUncertain || initial.CleanupResult != domain.UploadCleanupDeleted || initial.CompletionConfirmed {
		t.Fatalf("initial cleanup falsely established termination: %+v", initial)
	}
	if lateErr != nil || repeated || final.CleanupResult != domain.UploadCleanupQueued || final.CleanupGeneration == nil || *final.CleanupGeneration != domain.UploadFinalCleanup || cycles != 2 {
		t.Fatalf("automatic cleanup is not finite: generation %+v cycles %d", final.CleanupGeneration, cycles)
	}
	if !errors.Is(remaining, repositories.ErrNotFound) || h.capacity(t) != 0 {
		t.Fatalf("confirmed final cleanup retained capacity: %v", remaining)
	}
}

func TestUploadIntentIdentityGuardsDeletion(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	opened := h.intent(t, domain.AttachmentID{2}, true)
	if err := h.objects.Put(context.Background(), opened.key, []byte("retained"), objectContentType); err != nil {
		t.Fatal(err)
	}
	if err := h.service.discardChatUpload(context.Background(), opened, false); err != nil {
		t.Fatal(err)
	}
	claims, err := h.queue.Claim(context.Background(), domain.LaneStorageDelete, 1, h.now)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim: %v", err)
	}
	var payload domain.AttachmentPayload
	if err := json.Unmarshal(claims[0].Row.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	payload.Key += "-other"
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	row := claims[0].Row
	row.Payload = encoded
	finalGeneration := domain.UploadFinalCleanup
	generationPayload := domain.AttachmentPayload{Attachment: opened.id.String(), Key: opened.key, Generation: &finalGeneration}
	generationEncoded, err := json.Marshal(generationPayload)
	if err != nil {
		t.Fatal(err)
	}
	generationRow := claims[0].Row
	generationRow.Payload = generationEncoded

	_, generationRefusal := h.service.DeleteObject(context.Background(), generationRow, h.now)
	_, refusal := h.service.DeleteObject(context.Background(), row, h.now)
	replacement := h.service.deps.Store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		if _, err := repos.UploadIntents.Lock(context.Background(), opened.id); err != nil {
			return err
		}
		return repos.UploadIntents.RecordMultipart(context.Background(), opened.id, opened.key, "different-upload", h.now)
	})
	object, objectErr := h.objects.Open(context.Background(), opened.key)
	if objectErr == nil {
		if err := object.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if !errors.Is(generationRefusal, domain.ErrPermanentEffect) || !errors.Is(refusal, domain.ErrPermanentEffect) || !errors.Is(replacement, repositories.ErrUploadTransition) || objectErr != nil {
		t.Fatalf("identity mutation was accepted: %v %v %v", refusal, replacement, objectErr)
	}
}

func TestUploadIntentSurvivesDeadOutboxRetention(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	opened := h.intent(t, domain.AttachmentID{3}, true)
	if err := h.service.discardChatUpload(context.Background(), opened, false); err != nil {
		t.Fatal(err)
	}
	initial := h.read(t, opened.id)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE outbox SET status = 'in_progress', attempts = 8, lease_token = $1, lease_expires_at = $2 WHERE id = $3", domain.LeaseToken{8}, h.now.Add(-time.Second), *initial.CleanupOutboxID); err != nil {
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
	if _, err := h.service.RecoverUploads(context.Background(), h.now.Add(31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	result := h.read(t, opened.id)
	_, missing := h.service.deps.Store.Repositories().Outbox.ByID(context.Background(), *initial.CleanupOutboxID)
	cycle, err := h.service.RetryUploadCleanup(context.Background(), opened.id, domain.UploadAttemptID{1})

	if !errors.Is(missing, repositories.ErrNotFound) || result.CleanupResult != domain.UploadCleanupFailed || result.CleanupOutboxID == nil || *result.CleanupOutboxID != *initial.CleanupOutboxID || err != nil || cycle.Status != domain.OutboxPending {
		t.Fatalf("retention lost failure or reset attempts: %v %+v %v", missing, result, err)
	}
}

func (h uploadProtocolFixture) intentSnapshot(t *testing.T, id domain.AttachmentID) string {
	t.Helper()
	encoded, err := json.Marshal(h.read(t, id))
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func (h uploadProtocolFixture) outboxCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM outbox").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestUploadRecoveryBoundsCandidateAndPassDeadlines(t *testing.T) {
	h := newUploadProtocolFixture(t)
	h.service.deps.Uploads.Candidate = 50 * time.Millisecond
	h.service.deps.Uploads.Recovery = 200 * time.Millisecond
	first := h.intent(t, domain.AttachmentID{1}, false)
	later := h.intent(t, domain.AttachmentID{2}, false)
	beforeFirst := h.intentSnapshot(t, first.id)
	beforeLater := h.intentSnapshot(t, later.id)
	held, err := h.database.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := held.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	if _, err := held.Exec(context.Background(), "SELECT attachment_id FROM upload_intents ORDER BY attachment_id FOR UPDATE"); err != nil {
		t.Fatal(err)
	}

	candidateStarted := clock.System{}.Now()
	candidateCount, candidateErr := h.service.RecoverUploads(context.Background(), h.now.Add(time.Hour))
	candidateElapsed := time.Since(candidateStarted)
	candidateCursor := h.service.recoveryCursor
	candidateFirst := h.intentSnapshot(t, first.id)
	candidateLater := h.intentSnapshot(t, later.id)
	candidateOutbox := h.outboxCount(t)
	h.service.deps.Uploads.Candidate = domain.UploadRecoveryCandidateBudget
	h.service.deps.Uploads.Recovery = 75 * time.Millisecond
	passStarted := clock.System{}.Now()
	passCount, passErr := h.service.RecoverUploads(context.Background(), h.now.Add(time.Hour))
	passElapsed := time.Since(passStarted)
	passCursor := h.service.recoveryCursor
	passFirst := h.intentSnapshot(t, first.id)
	passLater := h.intentSnapshot(t, later.id)
	passOutbox := h.outboxCount(t)

	if !errors.Is(candidateErr, context.DeadlineExceeded) || candidateCount != 0 || candidateElapsed > time.Second {
		t.Fatalf("candidate deadlines failed: count %d elapsed %s error %v", candidateCount, candidateElapsed, candidateErr)
	}
	if candidateCursor != (domain.AttachmentID{}) {
		t.Fatal("candidate deadlines did not allow the whole locked batch to be examined")
	}
	if beforeFirst != candidateFirst || beforeLater != candidateLater || candidateOutbox != 0 {
		t.Fatal("timed-out candidates changed durable state")
	}
	if !errors.Is(passErr, context.DeadlineExceeded) || passCount != 0 || passElapsed > time.Second {
		t.Fatalf("pass deadline failed: count %d elapsed %s error %v", passCount, passElapsed, passErr)
	}
	if passCursor != (domain.AttachmentID{}) && passCursor != first.id {
		t.Fatalf("pass retained an unvisited cursor: %s", passCursor)
	}
	if beforeFirst != passFirst || beforeLater != passLater || passOutbox != 0 {
		t.Fatal("expired pass changed durable state")
	}
}

func TestUploadRecoveryContinuesAfterOneCandidateFails(t *testing.T) {
	h := newUploadProtocolFixture(t)
	first := h.intent(t, domain.AttachmentID{1}, false)
	later := h.intent(t, domain.AttachmentID{2}, false)
	beforeFirst := h.intentSnapshot(t, first.id)
	statement := fmt.Sprintf(`CREATE FUNCTION fail_one_upload_recovery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'test upload recovery failure'; END $$;
CREATE TRIGGER fail_one_upload_recovery BEFORE UPDATE OF cleanup_requested ON upload_intents
FOR EACH ROW WHEN (OLD.attachment_id = '%s'::uuid AND NEW.cleanup_requested)
EXECUTE FUNCTION fail_one_upload_recovery()`, first.id)
	if _, err := h.database.Pool.Exec(context.Background(), statement); err != nil {
		t.Fatal(err)
	}

	processed, recoveryErr := h.service.RecoverUploads(context.Background(), h.now.Add(time.Hour))
	afterFirst := h.intentSnapshot(t, first.id)
	row := h.read(t, later.id)

	var databaseError recoveryDatabaseError
	if !errors.As(recoveryErr, &databaseError) || databaseError.SQLState() != "P0001" {
		t.Fatalf("expected the first candidate's database error, got %v", recoveryErr)
	}
	if processed != 1 || beforeFirst != afterFirst || row.CleanupOutboxID == nil ||
		row.CleanupResult != domain.UploadCleanupQueued || !row.CleanupRequested || h.outboxCount(t) != 1 {
		t.Fatal("one candidate failure prevented later durable cleanup or changed the failed candidate")
	}
	if h.service.recoveryCursor != (domain.AttachmentID{}) {
		t.Fatal("recovery did not wrap after its final candidate")
	}
}

func TestUploadIntentCapacityIsAtomic(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	if _, err := h.database.Pool.Exec(context.Background(), `INSERT INTO upload_intents (attachment_id, storage_key, capacity_class, phase, cleanup_requested, created_at, updated_at)
 SELECT md5(n::text)::uuid, 'chat/capacity/' || n, 'chat', 'cleanup', true, $1, $1 FROM generate_series(1,4095) AS n`, h.now); err != nil {
		t.Fatal(err)
	}
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE upload_intent_capacity SET used = 4095 WHERE capacity_class = 'chat'"); err != nil {
		t.Fatal(err)
	}
	type admissionResult struct {
		id  domain.AttachmentID
		err error
	}
	finished := make(chan admissionResult, 16)
	start := make(chan struct{})
	for index := byte(1); index <= 16; index++ {
		id := domain.AttachmentID{index}
		go func() {
			<-start
			err := h.service.deps.Store.WithTx(context.Background(), func(repos repositories.Repositories) error {
				return repos.UploadIntents.Create(context.Background(), id, domain.ChatKeyPrefix+id.String(), domain.UploadCapacityChat, h.now)
			})
			finished <- admissionResult{id: id, err: err}
		}()
	}

	close(start)
	accepted, refused := 0, 0
	var winner domain.AttachmentID
	for range 16 {
		result := <-finished
		if result.err == nil {
			accepted++
			winner = result.id
		} else if errors.Is(result.err, repositories.ErrUploadCapacity) {
			refused++
		} else {
			t.Fatal(result.err)
		}
	}
	capacity := h.capacity(t)
	var count int
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM upload_intents").Scan(&count); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := h.service.deps.Store.WithTx(context.Background(), func(repos repositories.Repositories) error {
			return repos.UploadIntents.Remove(context.Background(), winner, domain.ChatKeyPrefix+winner.String(), h.now)
		}); err != nil {
			t.Fatal(err)
		}
	}

	if accepted != 1 || refused != 15 || capacity != 4096 || count != capacity || h.capacity(t) != 4095 {
		t.Fatalf("capacity raced: accepted %d refused %d used %d rows %d final %d", accepted, refused, capacity, count, h.capacity(t))
	}
}

func TestOperatorUploadRetryIsIdempotentAndExclusive(t *testing.T) {
	t.Parallel()
	h := newUploadProtocolFixture(t)
	opened := h.intent(t, domain.AttachmentID{12}, true)
	if err := h.service.discardChatUpload(context.Background(), opened, false); err != nil {
		t.Fatal(err)
	}
	h.tick(t, h.now)
	attempt := domain.UploadAttemptID{1}
	type retryResult struct {
		cycle UploadCleanupCycle
		err   error
	}
	finished := make(chan retryResult, 8)
	start := make(chan struct{})
	for range 8 {
		go func() {
			<-start
			cycle, err := h.service.RetryUploadCleanup(context.Background(), opened.id, attempt)
			finished <- retryResult{cycle: cycle, err: err}
		}()
	}

	close(start)
	var first UploadCleanupCycle
	for range 8 {
		result := <-finished
		if result.err != nil {
			t.Fatal(result.err)
		}
		if first.Outbox == (domain.OutboxID{}) {
			first = result.cycle
		}
		if result.cycle.Outbox != first.Outbox {
			t.Fatal("a repeated UUID created more than one cycle")
		}
	}
	refused, activeErr := h.service.RetryUploadCleanup(context.Background(), opened.id, domain.UploadAttemptID{2})
	if err := h.service.discardChatUpload(context.Background(), opened, true); err != nil {
		t.Fatal(err)
	}
	deferred := h.read(t, opened.id)
	h.tick(t, h.now)
	if _, err := h.service.RecoverUploads(context.Background(), h.now); err != nil {
		t.Fatal(err)
	}
	final := h.read(t, opened.id)
	replay, replayErr := h.service.RetryUploadCleanup(context.Background(), opened.id, attempt)

	if !errors.Is(activeErr, ErrUploadCleanupActive) || refused.Outbox != first.Outbox || !deferred.FinalCleanupRequired || deferred.CleanupGeneration == nil || *deferred.CleanupGeneration != domain.UploadInitialCleanup {
		t.Fatalf("manual work overlapped automatic cleanup: %v %+v", activeErr, deferred)
	}
	if final.CleanupGeneration == nil || *final.CleanupGeneration != domain.UploadFinalCleanup || final.FinalCleanupRequired || replayErr != nil || replay.Outbox != first.Outbox || replay.Status != domain.OutboxDone {
		t.Fatalf("deferred generation or receipt was lost: %+v %+v %v", final, replay, replayErr)
	}
}
