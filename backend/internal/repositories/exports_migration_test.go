package repositories_test

import (
	"testing"

	"ovc/backend/internal/testdb"
)

const (
	exportsMigration  = 45
	insertReadyExport = `INSERT INTO exports (id, user_id, state, language, timezone, requested_at, build_id, storage_key, build_started_at, built_at,
    size_bytes, expires_at, updated_at)
VALUES (md5('export')::uuid, md5('student')::uuid, 'ready', 'en', 'UTC', now(), md5('build')::uuid, 'exports/' || md5('build') || '.zip', now(),
    now(), 10, now() + interval '3 days', now())`
	insertPendingObjectDeletion = `INSERT INTO outbox (id, lane, priority, kind, payload, idempotency_key, status, next_attempt_at, created_at)
VALUES (md5('deletion')::uuid, 'storage_delete', 1, 'export_object', '{}', 'storage_delete:export:pending', 'pending', now(), now())`
	insertRunningIntentCleanup = `INSERT INTO outbox (id, lane, priority, kind, payload, idempotency_key, status, next_attempt_at, lease_token,
    lease_expires_at, created_at)
VALUES (md5('cleanup')::uuid, 'storage_delete', 1, 'export_intent', '{}', 'storage_delete:export:running', 'in_progress', now(), md5('lease')::uuid,
    now() + interval '1 minute', now())`
	insertExportIntent = `INSERT INTO upload_intents (attachment_id, storage_key, capacity_class, subject_kind, phase, created_at, updated_at)
VALUES (md5('other build')::uuid, 'exports/other.zip', 'export', 'export_build', 'opening', now(), now())`
)

func exportsDatabase(t *testing.T, fixtures ...string) *testdb.Database {
	t.Helper()
	database := testdb.NewMigratedTo(t, exportsMigration)
	for _, fixture := range append([]string{insertMarkerStudent}, fixtures...) {
		if _, err := database.Pool.Exec(t.Context(), fixture); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func TestExportsMigrationRollbackIsRefusedWhileArchivesNeedCleanup(t *testing.T) {
	t.Parallel()
	withObject := exportsDatabase(t, insertReadyExport)
	withIntent := exportsDatabase(t, insertExportIntent)
	withDeletion := exportsDatabase(t, insertPendingObjectDeletion)
	withCleanup := exportsDatabase(t, insertRunningIntentCleanup)

	objectErr := withObject.MigrateDownTo(t.Context(), exportsMigration-1)
	intentErr := withIntent.MigrateDownTo(t.Context(), exportsMigration-1)
	deletionErr := withDeletion.MigrateDownTo(t.Context(), exportsMigration-1)
	cleanupErr := withCleanup.MigrateDownTo(t.Context(), exportsMigration-1)

	if objectErr == nil || intentErr == nil || deletionErr == nil || cleanupErr == nil {
		t.Fatalf("the rollback ran with an export object (%v), an export intent (%v), a queued object deletion (%v) or a running intent cleanup (%v) left",
			objectErr, intentErr, deletionErr, cleanupErr)
	}
	if !scalar(t, withObject, `SELECT EXISTS (SELECT 1 FROM exports)`) || !scalar(t, withIntent, `SELECT EXISTS (SELECT 1 FROM upload_intents WHERE subject_kind = 'export_build')`) ||
		!scalar(t, withDeletion, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'exports')`) ||
		!scalar(t, withCleanup, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'exports')`) {
		t.Fatal("a refused rollback changed the tables")
	}
}

func TestExportsMigrationRollsBackWithoutArchives(t *testing.T) {
	t.Parallel()
	database := exportsDatabase(t)

	downErr := database.MigrateDownTo(t.Context(), exportsMigration-1)
	removed := downErr == nil && scalar(t, database, `SELECT NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name IN ('exports', 'export_limits'))`)
	againErr := database.MigrateUpTo(t.Context(), exportsMigration)

	if downErr != nil || !removed || againErr != nil {
		t.Fatalf("the rollback without archives failed: %v %v", downErr, againErr)
	}
}
