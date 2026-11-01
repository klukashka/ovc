package startup_test

import (
	"context"
	"testing"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

func seedLegacyUploadOwner(t *testing.T, database *testdb.Database) domain.UserID {
	t.Helper()
	id := domain.UserID{0x61}
	if _, err := database.Pool.Exec(context.Background(), insertInterimUser, id, "tutor", "active", "migration@example.org", &adminStart, adminStart); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestUploadMigrationPreservesLegacyCleanupOwnership(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.NewMigratedTo(t, 19)
	owner := seedLegacyUploadOwner(t, database)
	_, err := database.Pool.Exec(context.Background(), `INSERT INTO attachments
(id, uploader_id, context, storage_key, filename, state, reserved_bytes, multipart_upload_id, created_at, updated_at)
SELECT md5(n::text)::uuid, $1, context, 'legacy/' || n, 'legacy.txt', state, 1, multipart, $2, $2
FROM (VALUES (1, 'chat', 'uploading', NULL), (2, 'chat', 'uploading', 'mpu-two'),
(3, 'chat', 'deleted', 'mpu-three'), (4, 'chat', 'deleted', NULL), (5, 'avatar', 'deleted', 'mpu-five'))
AS fixture(n, context, state, multipart)`, owner, adminStart)
	if err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, "api", []string{"OVC_DATABASE_URL=" + database.URL.Reveal()}, "migrate")

	if result.exitCode != 0 {
		t.Fatalf("migration failed: %s", result.stderr)
	}
	var valid bool
	err = database.Pool.QueryRow(context.Background(), `SELECT
(SELECT used = 3 FROM upload_intent_capacity) AND
(SELECT count(*) = 3 FROM upload_intents) AND
(SELECT count(*) = 5 FROM attachments) AND
(SELECT count(*) = 1 FROM upload_intents WHERE storage_key = 'legacy/1' AND phase = 'cleanup'
AND multipart_upload_id IS NULL AND NOT completion_started AND cleanup_requested) AND
(SELECT count(*) = 2 FROM upload_intents WHERE storage_key IN ('legacy/2', 'legacy/3') AND phase = 'uncertain'
AND multipart_upload_id IS NOT NULL AND completion_started AND NOT completion_confirmed AND cleanup_requested)`).Scan(&valid)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("legacy cleanup ownership or capacity was not preserved")
	}
}

func TestUploadMigrationRefusesLegacyCapacityOverflowAtomically(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.NewMigratedTo(t, 19)
	owner := seedLegacyUploadOwner(t, database)
	_, err := database.Pool.Exec(context.Background(), `INSERT INTO attachments
(id, uploader_id, context, storage_key, filename, state, reserved_bytes, created_at, updated_at)
SELECT md5(n::text)::uuid, $1, 'chat', 'legacy/' || n, 'legacy.txt', 'uploading', 1, $2, $2
FROM generate_series(1, 4097) AS n`, owner, adminStart)
	if err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, "api", []string{"OVC_DATABASE_URL=" + database.URL.Reveal()}, "migrate")

	if result.exitCode == 0 {
		t.Fatal("migration accepted more unresolved uploads than capacity")
	}
	var unchanged bool
	err = database.Pool.QueryRow(context.Background(), `SELECT to_regclass('upload_intents') IS NULL
AND to_regclass('upload_intent_capacity') IS NULL AND to_regclass('upload_cleanup_receipts') IS NULL
AND (SELECT count(*) = 4097 FROM attachments)
AND NOT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id = 20 AND is_applied)`).Scan(&unchanged)
	if err != nil {
		t.Fatal(err)
	}
	if !unchanged {
		t.Fatal("failed adoption changed the legacy database")
	}
}
