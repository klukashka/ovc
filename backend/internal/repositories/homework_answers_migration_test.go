package repositories_test

import (
	"testing"

	"ovc/backend/internal/testdb"
)

const (
	homeworkAnswersMigration = 40
	insertAnswerReference    = `INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, question_id, created_at)
VALUES (md5('answer')::uuid, 'answer', md5('submission')::uuid, md5('question')::uuid, now())`
)

func answersDatabase(t *testing.T, fixtures ...string) *testdb.Database {
	t.Helper()
	database := testdb.NewMigratedTo(t, homeworkAnswersMigration)
	for _, fixture := range fixtures {
		if _, err := database.Pool.Exec(t.Context(), fixture); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func TestHomeworkAnswersMigrateAndRollBackOnlyWithoutAnswers(t *testing.T) {
	t.Parallel()
	t.Run("a rollback with an answer reference is refused", func(t *testing.T) {
		t.Parallel()
		database := answersDatabase(t, insertLibraryOwner, insertAnswerFile, insertAnswerReference)

		err := database.MigrateDownTo(t.Context(), homeworkAnswersMigration-1)

		if err == nil {
			t.Fatal("a rollback with an answer reference was not refused")
		}
	})
	t.Run("an empty rollback removes the answers table", func(t *testing.T) {
		t.Parallel()
		database := answersDatabase(t)

		downErr := database.MigrateDownTo(t.Context(), homeworkAnswersMigration-1)
		removed := downErr == nil && scalar(t, database, `SELECT to_regclass('answers') IS NULL`)
		againErr := database.MigrateUpTo(t.Context(), homeworkAnswersMigration)

		if downErr != nil || !removed || againErr != nil {
			t.Fatalf("the empty rollback did not remove the answers table: %v %v", downErr, againErr)
		}
	})
}
