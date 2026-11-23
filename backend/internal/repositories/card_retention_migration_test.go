package repositories_test

import (
	"testing"

	"ovc/backend/internal/testdb"
)

const (
	cardRetentionMigration = 44
	insertRetentionCards   = `INSERT INTO relationships (id, tutor_id, state, display_name, email, created_at, updated_at)
VALUES (md5('with-email')::uuid, md5('owner')::uuid, 'invited', 'With email', 'card@example.org', now() - interval '2 days', now() - interval '1 day'),
    (md5('without-email')::uuid, md5('owner')::uuid, 'invited', 'Without email', NULL, now(), now())`
	insertRetentionInvitation = `INSERT INTO invitations (id, relationship_id, token_hash, expires_at, created_at)
VALUES (md5('invitation')::uuid, md5('with-email')::uuid, decode(md5('token') || md5('token'), 'hex'), now() + interval '14 days', now())`
	eraseRetentionCard = `UPDATE relationships SET email = NULL, email_set_at = NULL, email_erased_at = now() WHERE id = md5('with-email')::uuid;
UPDATE invitations SET revoked_at = now(), revoke_reason = 'email_erased' WHERE id = md5('invitation')::uuid;
UPDATE relationships SET state = 'discarded', discard_reason = 'withdrawn', discarded_at = now(), display_name = NULL
WHERE id = md5('without-email')::uuid`
)

func retentionDatabase(t *testing.T) *testdb.Database {
	t.Helper()
	database := testdb.NewMigratedTo(t, cardRetentionMigration-1)
	for _, fixture := range []string{insertLibraryOwner, insertLibraryTutor, insertRetentionCards, insertRetentionInvitation} {
		if _, err := database.Pool.Exec(t.Context(), fixture); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func refused(t *testing.T, database *testdb.Database, statement string) bool {
	t.Helper()
	_, err := database.Pool.Exec(t.Context(), statement)
	return err != nil
}

func TestCardRetentionMigrationBackfillsAndConstrains(t *testing.T) {
	t.Parallel()
	database := retentionDatabase(t)

	upErr := database.MigrateUpTo(t.Context(), cardRetentionMigration)
	backfilled := upErr == nil && scalar(t, database,
		`SELECT email_set_at = updated_at FROM relationships WHERE id = md5('with-email')::uuid`)
	untouched := upErr == nil && scalar(t, database,
		`SELECT email_set_at IS NULL AND email_erased_at IS NULL FROM relationships WHERE id = md5('without-email')::uuid`)
	nullName := refused(t, database, `UPDATE relationships SET display_name = NULL WHERE id = md5('with-email')::uuid`)
	setAtWithoutEmail := refused(t, database, `UPDATE relationships SET email_set_at = now() WHERE id = md5('without-email')::uuid`)
	emailWithoutSetAt := refused(t, database, `UPDATE relationships SET email_set_at = NULL WHERE id = md5('with-email')::uuid`)
	markerWithEmail := refused(t, database, `UPDATE relationships SET email_erased_at = now() WHERE id = md5('with-email')::uuid`)
	markerOutsideInvited := refused(t, database,
		`UPDATE relationships SET state = 'discarded', discard_reason = 'withdrawn', discarded_at = now(), email_erased_at = now() WHERE id = md5('without-email')::uuid`)

	if upErr != nil || !backfilled || !untouched {
		t.Fatalf("the migration did not backfill email_set_at from the last update: %v", upErr)
	}
	if !nullName || !setAtWithoutEmail || !emailWithoutSetAt || !markerWithEmail || !markerOutsideInvited {
		t.Fatalf("a constraint accepted an invalid row: name %t, set without email %t, email without set %t, marker with email %t, marker outside invited %t",
			nullName, setAtWithoutEmail, emailWithoutSetAt, markerWithEmail, markerOutsideInvited)
	}
}

func TestCardRetentionMigrationRollsBackErasedRows(t *testing.T) {
	t.Parallel()
	database := retentionDatabase(t)
	if err := database.MigrateUpTo(t.Context(), cardRetentionMigration); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(t.Context(), eraseRetentionCard); err != nil {
		t.Fatal(err)
	}

	downErr := database.MigrateDownTo(t.Context(), cardRetentionMigration-1)
	restored := downErr == nil && scalar(t, database,
		`SELECT display_name = 'removed' FROM relationships WHERE id = md5('without-email')::uuid`)
	reason := downErr == nil && scalar(t, database,
		`SELECT revoke_reason = 'regenerated' FROM invitations WHERE id = md5('invitation')::uuid`)
	removed := downErr == nil && scalar(t, database,
		`SELECT NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'relationships' AND column_name IN ('email_set_at', 'email_erased_at'))`)
	againErr := database.MigrateUpTo(t.Context(), cardRetentionMigration)

	if downErr != nil || !restored || !reason || !removed || againErr != nil {
		t.Fatalf("the rollback after an erasure failed: down %v, name %t, reason %t, columns removed %t, up again %v", downErr, restored, reason, removed, againErr)
	}
}
