package startup_test

import (
	"context"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

func TestProductionWorkerRecoversAnUploadWithoutAnAttachment(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id := domain.AttachmentID{0x65}
	_, err := database.Pool.Exec(ctx, `INSERT INTO upload_intents
 (attachment_id, storage_key, multipart_upload_id, phase, completion_started, cleanup_requested, created_at, updated_at)
 VALUES ($1, 'chat/recovery-process-fixture', 'absent-multipart-fixture', 'uncertain', true, true, $2, $2)`, id, adminStart)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, "UPDATE upload_intent_capacity SET used = 1"); err != nil {
		t.Fatal(err)
	}
	address := freeAddress(t)
	base := "http://" + address
	server, output := startServer(t, ctx, []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=1", "OVC_CLOCK=test"})
	waitForStatus(t, ctx, base+"/api/v1/health/ready")
	headers := map[string]string{"Content-Type": "application/json"}
	setClock := call(t, ctx, http.MethodPost, base+"/__test__/clock", `{"set":"2030-01-01T00:00:00Z"}`, headers)
	if setClock.status != http.StatusOK {
		t.Fatalf("clock setup: %d", setClock.status)
	}

	tick := call(t, ctx, http.MethodPost, base+"/__test__/tick", `{}`, headers)
	signalErr := server.Process.Signal(syscall.SIGTERM)
	waitErr := server.Wait()

	if tick.status != http.StatusOK || signalErr != nil || waitErr != nil {
		t.Fatalf("tick %d, signal %v, exit %v: %s", tick.status, signalErr, waitErr, output.String())
	}
	var recovered bool
	err = database.Pool.QueryRow(ctx, `SELECT COALESCE(cleanup_generation = 0 AND cleanup_result = 'deleted'
 AND phase = 'uncertain' AND completion_started AND NOT completion_confirmed
 AND (SELECT used = 1 FROM upload_intent_capacity)
 AND EXISTS (SELECT 1 FROM outbox WHERE id = upload_intents.cleanup_outbox_id AND status = 'done')
 AND EXISTS (SELECT 1 FROM job_runs WHERE job = 'uploads.recover' AND last_result = 'ok'), false)
 FROM upload_intents WHERE attachment_id = $1`, id).Scan(&recovered)
	if err != nil {
		t.Fatal(err)
	}
	if !recovered || !strings.Contains(output.String(), "uploads.recover") {
		t.Fatalf("production worker did not recover retained ownership: %s", output.String())
	}
}
