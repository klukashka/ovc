package repositories_test

import (
	"testing"

	"ovc/backend/internal/testdb"
)

const (
	capacityClassesMigration = 33
	insertLegacyChatIntents  = `INSERT INTO upload_intents (attachment_id, storage_key, phase, cleanup_requested, created_at, updated_at)
SELECT md5(n::text)::uuid, 'chat/legacy/' || n, 'cleanup', true, now(), now() FROM generate_series(1, 2) AS n`
	insertLibraryIntent = `INSERT INTO upload_intents (attachment_id, storage_key, capacity_class, phase, cleanup_requested, created_at, updated_at)
VALUES (md5('library')::uuid, 'library/pending', 'library', 'cleanup', true, now(), now())`
	insertLibraryOwner = `INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme,
    consent_version, consent_at, unverified_since, created_at, updated_at)
VALUES (md5('owner')::uuid, 'tutor', 'active', 'owner@example.org', now(), '$argon2id$fixture', 'Owner', 'en', 'UTC', 'system', 'v1',
    now(), now(), now(), now())`
	insertLibraryFile = `INSERT INTO attachments (id, uploader_id, context, storage_key, filename, media_type, size_bytes, sha256, state,
    expires_at, created_at, updated_at)
VALUES (md5('file')::uuid, md5('owner')::uuid, 'library', 'library/file', 'notes.pdf', 'application/pdf', 1, sha256('x'), 'pending',
    now() + interval '1 day', now(), now())`
)

func scalar(t *testing.T, database *testdb.Database, query string) bool {
	t.Helper()
	var result bool
	if err := database.Pool.QueryRow(t.Context(), query).Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func legacyCapacityDatabase(t *testing.T) *testdb.Database {
	t.Helper()
	database := testdb.NewMigratedTo(t, capacityClassesMigration-1)
	if _, err := database.Pool.Exec(t.Context(), insertLegacyChatIntents); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(t.Context(), "UPDATE upload_intent_capacity SET used = 2"); err != nil {
		t.Fatal(err)
	}
	return database
}

func classifiedCapacityDatabase(t *testing.T, fixtures ...string) *testdb.Database {
	t.Helper()
	database := legacyCapacityDatabase(t)
	if err := database.MigrateUpTo(t.Context(), capacityClassesMigration); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		if _, err := database.Pool.Exec(t.Context(), fixture); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func TestUploadCapacityClassesMigrateAndRollBackOnlyWithoutLibraryWork(t *testing.T) {
	t.Parallel()
	t.Run("the migration splits the capacity into its classes", func(t *testing.T) {
		t.Parallel()
		database := legacyCapacityDatabase(t)

		err := database.MigrateUpTo(t.Context(), capacityClassesMigration)

		if err != nil || !scalar(t, database, `SELECT
(SELECT used = 2 FROM upload_intent_capacity WHERE capacity_class = 'chat') AND
(SELECT used = 0 FROM upload_intent_capacity WHERE capacity_class = 'library') AND
(SELECT count(*) = 2 FROM upload_intent_capacity) AND
(SELECT count(*) = 2 FROM upload_intents WHERE capacity_class = 'chat')`) {
			t.Fatalf("the migration did not split the capacity into its classes: %v", err)
		}
	})
	t.Run("a rollback with a library intent is refused", func(t *testing.T) {
		t.Parallel()
		database := classifiedCapacityDatabase(t, insertLibraryIntent)

		err := database.MigrateDownTo(t.Context(), capacityClassesMigration-1)

		if err == nil {
			t.Fatal("a rollback with a library intent was not refused")
		}
	})
	t.Run("a rollback with a library file is refused", func(t *testing.T) {
		t.Parallel()
		database := classifiedCapacityDatabase(t, insertLibraryOwner, insertLibraryFile)

		err := database.MigrateDownTo(t.Context(), capacityClassesMigration-1)

		if err == nil {
			t.Fatal("a rollback with a library file was not refused")
		}
	})
	t.Run("an empty rollback restores the single capacity row", func(t *testing.T) {
		t.Parallel()
		database := classifiedCapacityDatabase(t)

		downErr := database.MigrateDownTo(t.Context(), capacityClassesMigration-1)
		restored := downErr == nil && scalar(t, database, `SELECT (SELECT used = 2 FROM upload_intent_capacity WHERE singleton) AND
(SELECT count(*) = 1 FROM upload_intent_capacity) AND (SELECT count(*) = 2 FROM upload_intents)`)
		againErr := database.MigrateUpTo(t.Context(), capacityClassesMigration)

		if downErr != nil || !restored || againErr != nil {
			t.Fatalf("the empty rollback did not restore the single capacity row: %v %v", downErr, againErr)
		}
	})
}
