package startup_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

func seedAdminUploadIntent(t *testing.T, f *adminFixture) domain.AttachmentID {
	t.Helper()
	id := domain.AttachmentID{0x71}
	_, err := f.database.Pool.Exec(context.Background(), `INSERT INTO upload_intents
(attachment_id, storage_key, capacity_class, multipart_upload_id, phase, completion_started, cleanup_requested, created_at, updated_at)
VALUES ($1, 'chat/private-storage-key', 'chat', 'private-multipart-id', 'uncertain', true, true, $2, $2)`, id, adminStart)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.Pool.Exec(context.Background(), "UPDATE upload_intent_capacity SET used = 1 WHERE capacity_class = 'chat'"); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestOvcAdminUploadCleanupRetryKeepsItsAttemptIdentity(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	id := seedAdminUploadIntent(t, f)
	attempt := domain.UploadAttemptID{0x72}
	other := domain.UploadAttemptID{0x73}

	first := f.admin(t, "uploads", "retry-cleanup", id.String(), "--attempt", attempt.String())
	replay := f.admin(t, "uploads", "retry-cleanup", id.String(), "--attempt", attempt.String())
	competing := f.admin(t, "uploads", "retry-cleanup", id.String(), "--attempt", other.String())
	inspection := f.admin(t, "uploads", "inspect", id.String())
	missingAttempt := f.admin(t, "uploads", "retry-cleanup", id.String())

	if first.exitCode != 0 || replay.exitCode != 0 || first.stdout != replay.stdout {
		t.Fatalf("retry identity changed: first=%+v replay=%+v", first, replay)
	}
	if competing.exitCode != 1 || competing.stdout != first.stdout {
		t.Fatalf("concurrent cycle was not refused with its identity: %+v", competing)
	}
	if inspection.exitCode != 0 || !strings.Contains(inspection.stdout, "receipts=1") || !strings.Contains(inspection.stdout, attempt.String()) {
		t.Fatalf("inspection lost receipt: %+v", inspection)
	}
	if missingAttempt.exitCode != 2 {
		t.Fatalf("missing attempt was accepted: %+v", missingAttempt)
	}
	for _, result := range []outcome{first, replay, competing, inspection, missingAttempt} {
		for _, forbidden := range []string{"private-storage-key", "private-multipart-id"} {
			if strings.Contains(result.stdout+result.stderr, forbidden) {
				t.Fatal("operator output disclosed a storage identifier")
			}
		}
		if !strings.Contains(result.stderr, "admin.command") || !strings.Contains(result.stderr, id.String()) {
			t.Fatalf("operator command was not audited: %s", result.stderr)
		}
	}
	var receipts, queued int
	if err := f.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM upload_cleanup_receipts").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := f.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM outbox WHERE lane = 'storage_delete'").Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || queued != 1 {
		t.Fatalf("duplicate operator work: receipts=%d queued=%d", receipts, queued)
	}
}

func TestOvcAdminUploadInspectionUsesBoundedKeysetPages(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	id := seedAdminUploadIntent(t, f)
	for index := byte(1); index <= 101; index++ {
		attempt := domain.UploadAttemptID{index}
		outbox := domain.OutboxID{index}
		_, err := f.database.Pool.Exec(context.Background(), `INSERT INTO upload_cleanup_receipts
(attachment_id, attempt_id, outbox_id, status, created_at, updated_at) VALUES ($1, $2, $3, 'dead', $4, $4)`, id, attempt, outbox, adminStart)
		if err != nil {
			t.Fatal(err)
		}
	}
	cursor := domain.UploadAttemptID{100}

	first := f.admin(t, "uploads", "inspect", id.String())
	last := f.admin(t, "uploads", "inspect", id.String(), "--after", cursor.String())

	if first.exitCode != 0 || strings.Count(first.stdout, "receipt\t") != 100 || !strings.Contains(first.stdout, "next\t"+cursor.String()) || !strings.Contains(first.stdout, "receipts=101") {
		t.Fatalf("first inspection page is not bounded: %+v", first)
	}
	wanted := domain.UploadAttemptID{101}
	if last.exitCode != 0 || strings.Count(last.stdout, "receipt\t") != 1 || !strings.Contains(last.stdout, fmt.Sprintf("receipt\t%s\t", wanted)) || strings.Contains(last.stdout, "next\t") {
		t.Fatalf("last inspection page is wrong: %+v", last)
	}
}
