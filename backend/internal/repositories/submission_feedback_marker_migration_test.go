package repositories_test

import (
	"testing"

	"ovc/backend/internal/testdb"
)

const (
	feedbackMarkerMigration = 43
	insertMarkerStudent     = `INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme,
    consent_version, consent_at, unverified_since, created_at, updated_at)
VALUES (md5('student')::uuid, 'student', 'active', 'student@example.org', now(), '$argon2id$fixture', 'Student', 'en', 'UTC', 'system', 'v1',
    now(), now(), now(), now())`
	insertMarkerRelationship = `INSERT INTO relationships (id, tutor_id, student_id, state, display_name, colour, linked_at, created_at, updated_at)
VALUES (md5('relationship')::uuid, md5('owner')::uuid, md5('student')::uuid, 'active', 'Student', 'clay', now(), now(), now())`
	insertMarkerTemplate = `INSERT INTO templates (id, tutor_id, title, normalised_title, blocks, schema_version, version, question_count, max_points,
    ready, blocks_bytes, created_at, updated_at)
VALUES (md5('template')::uuid, md5('owner')::uuid, 'Work', 'work', '[]', 1, 1, 0, 0, true, 2, now(), now())`
	insertMarkerIssues = `INSERT INTO issues (id, tutor_id, template_id, relationship_id, title, snapshot, snapshot_bytes, max_points,
    has_manual_questions, answer_policy, status, version, issued_at, updated_at)
SELECT md5('issue' || n)::uuid, md5('owner')::uuid, md5('template')::uuid, md5('relationship')::uuid, 'Work', '[]', 2, 0, false, 'never',
    'active', 1, now(), now()
FROM generate_series(1, 2) AS n`
	insertMarkerSubmissions = `INSERT INTO submissions (id, issue_id, tutor_id, relationship_id, student_id, status, issued_at, version, created_at, updated_at)
SELECT md5('submission' || n)::uuid, md5('issue' || n)::uuid, md5('owner')::uuid, md5('relationship')::uuid, md5('student')::uuid,
    CASE WHEN n = 1 THEN 'checked' ELSE 'submitted' END, now(), 2 + n, now(), now()
FROM generate_series(1, 2) AS n`
	insertMarkerRevision = `INSERT INTO review_revisions (id, submission_id, tutor_id, revision, state, action, feedback, items, version, published_at,
    created_at, updated_at)
VALUES (md5('revision')::uuid, md5('submission1')::uuid, md5('owner')::uuid, 1, 'published', 'check', 'Good', '[]', 1, now(), now(), now())`
	linkMarkerRevision = `UPDATE submissions SET published_revision_id = md5('revision')::uuid WHERE id = md5('submission1')::uuid`
)

func markerDatabase(t *testing.T) *testdb.Database {
	t.Helper()
	database := testdb.NewMigratedTo(t, feedbackMarkerMigration-1)
	for _, fixture := range []string{
		insertLibraryOwner, insertLibraryTutor, insertMarkerStudent, insertMarkerRelationship, insertMarkerTemplate, insertMarkerIssues,
		insertMarkerSubmissions, insertMarkerRevision, linkMarkerRevision,
	} {
		if _, err := database.Pool.Exec(t.Context(), fixture); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func TestSubmissionFeedbackMarkerMigrationBackfills(t *testing.T) {
	t.Parallel()
	database := markerDatabase(t)

	upErr := database.MigrateUpTo(t.Context(), feedbackMarkerMigration)
	reviewed := upErr == nil && scalar(t, database,
		`SELECT feedback_version = 3 AND feedback_seen_version = 3 FROM submissions WHERE id = md5('submission1')::uuid`)
	unreviewed := upErr == nil && scalar(t, database,
		`SELECT feedback_version IS NULL AND feedback_seen_version IS NULL FROM submissions WHERE id = md5('submission2')::uuid`)
	downErr := database.MigrateDownTo(t.Context(), feedbackMarkerMigration-1)
	removed := downErr == nil && scalar(t, database,
		`SELECT NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'submissions' AND column_name LIKE 'feedback%')`)
	againErr := database.MigrateUpTo(t.Context(), feedbackMarkerMigration)

	if upErr != nil || !reviewed || !unreviewed {
		t.Fatalf("the backfill set the reviewed submission %v and left the unreviewed one null %v: %v", reviewed, unreviewed, upErr)
	}
	if downErr != nil || !removed || againErr != nil {
		t.Fatalf("the rollback did not remove the marker columns: %v %v", downErr, againErr)
	}
}
