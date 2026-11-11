package repositories_test

import (
	"testing"

	"ovc/backend/internal/testdb"
)

const (
	homeworkUploadsMigration = 39
	insertHomeworkIntent     = `INSERT INTO upload_intents (attachment_id, storage_key, capacity_class, phase, cleanup_requested, created_at, updated_at)
VALUES (md5('homework')::uuid, 'answers/pending', 'homework', 'cleanup', true, now(), now())`
	insertAnswerFile = `INSERT INTO attachments (id, uploader_id, context, storage_key, filename, media_type, size_bytes, sha256, state,
    expires_at, created_at, updated_at)
VALUES (md5('answer')::uuid, md5('owner')::uuid, 'answer', 'answers/file', 'page.pdf', 'application/pdf', 1, sha256('x'), 'pending',
    now() + interval '1 day', now(), now())`
)

func homeworkCapacityDatabase(t *testing.T, fixtures ...string) *testdb.Database {
	t.Helper()
	database := testdb.NewMigratedTo(t, homeworkUploadsMigration)
	for _, fixture := range fixtures {
		if _, err := database.Pool.Exec(t.Context(), fixture); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func TestHomeworkUploadCapacityMigratesAndRollsBackOnlyWithoutHomeworkWork(t *testing.T) {
	t.Parallel()
	t.Run("the migration adds the homework class at zero", func(t *testing.T) {
		t.Parallel()
		database := homeworkCapacityDatabase(t)

		added := scalar(t, database, `SELECT (SELECT used = 0 FROM upload_intent_capacity WHERE capacity_class = 'homework') AND
(SELECT count(*) = 3 FROM upload_intent_capacity)`)

		if !added {
			t.Fatal("the migration did not add the homework capacity class")
		}
	})
	t.Run("the homework class is bounded to 1,024", func(t *testing.T) {
		t.Parallel()
		database := homeworkCapacityDatabase(t)

		_, err := database.Pool.Exec(t.Context(), "UPDATE upload_intent_capacity SET used = 1025 WHERE capacity_class = 'homework'")

		if err == nil {
			t.Fatal("the homework class accepted more than 1,024 intents")
		}
	})
	t.Run("a rollback with a homework intent is refused", func(t *testing.T) {
		t.Parallel()
		database := homeworkCapacityDatabase(t, insertHomeworkIntent)

		err := database.MigrateDownTo(t.Context(), homeworkUploadsMigration-1)

		if err == nil {
			t.Fatal("a rollback with a homework intent was not refused")
		}
	})
	t.Run("a rollback with an answer file is refused", func(t *testing.T) {
		t.Parallel()
		database := homeworkCapacityDatabase(t, insertLibraryOwner, insertAnswerFile)

		err := database.MigrateDownTo(t.Context(), homeworkUploadsMigration-1)

		if err == nil {
			t.Fatal("a rollback with an answer file was not refused")
		}
	})
	t.Run("an empty rollback removes the homework class", func(t *testing.T) {
		t.Parallel()
		database := homeworkCapacityDatabase(t)

		downErr := database.MigrateDownTo(t.Context(), homeworkUploadsMigration-1)
		removed := downErr == nil && scalar(t, database, `SELECT (SELECT count(*) = 2 FROM upload_intent_capacity) AND
to_regclass('daily_answer_upload_bytes') IS NULL`)
		againErr := database.MigrateUpTo(t.Context(), homeworkUploadsMigration)

		if downErr != nil || !removed || againErr != nil {
			t.Fatalf("the empty rollback did not remove the homework class: %v %v", downErr, againErr)
		}
	})
}
