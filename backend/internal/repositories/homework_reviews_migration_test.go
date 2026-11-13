package repositories_test

import (
	"testing"

	"ovc/backend/internal/testdb"
)

const (
	homeworkReviewsMigration  = 41
	insertReviewItemReference = `INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, question_id, created_at)
VALUES (md5('answer')::uuid, 'review_item', md5('revision')::uuid, md5('question')::uuid, now())`
)

func reviewsDatabase(t *testing.T, fixtures ...string) *testdb.Database {
	t.Helper()
	database := testdb.NewMigratedTo(t, homeworkReviewsMigration)
	for _, fixture := range fixtures {
		if _, err := database.Pool.Exec(t.Context(), fixture); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func TestHomeworkReviewsMigrateAndRollBackOnlyWithoutReviews(t *testing.T) {
	t.Parallel()
	t.Run("a rollback with a corrected file reference is refused", func(t *testing.T) {
		t.Parallel()
		database := reviewsDatabase(t, insertLibraryOwner, insertAnswerFile, insertReviewItemReference)

		err := database.MigrateDownTo(t.Context(), homeworkReviewsMigration-1)

		if err == nil {
			t.Fatal("a rollback with a corrected file reference was not refused")
		}
	})
	t.Run("an empty rollback removes the review revisions", func(t *testing.T) {
		t.Parallel()
		database := reviewsDatabase(t)

		downErr := database.MigrateDownTo(t.Context(), homeworkReviewsMigration-1)
		removed := downErr == nil && scalar(t, database, `SELECT to_regclass('review_revisions') IS NULL`) &&
			scalar(t, database, `SELECT NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'submissions' AND column_name = 'published_revision_id')`)
		againErr := database.MigrateUpTo(t.Context(), homeworkReviewsMigration)

		if downErr != nil || !removed || againErr != nil {
			t.Fatalf("the empty rollback did not remove the review revisions: %v %v", downErr, againErr)
		}
	})
}
