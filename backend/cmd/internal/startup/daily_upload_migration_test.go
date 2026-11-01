package startup_test

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
	"ovc/backend/migrations"
)

func TestDailyUploadMigrationBackfillsRetainedBytesAndKeepsActivation(t *testing.T) {
	t.Parallel()
	fixture := testdb.NewMigratedTo(t, 20)
	ctx := context.Background()
	owner := seedLegacyUploadOwner(t, fixture)
	student := domain.UserID{0x62}
	if _, err := fixture.Pool.Exec(ctx, insertInterimUser, student, "student", "active", "student-migration@example.org", &adminStart, adminStart); err != nil {
		t.Fatal(err)
	}
	_, err := fixture.Pool.Exec(ctx, `INSERT INTO attachments
 (id, uploader_id, quota_tutor_id, context, storage_key, filename, state, media_type, size_bytes, sha256, created_at, updated_at, expires_at)
 SELECT md5(n::text)::uuid, CASE WHEN n = 4 THEN $1::uuid ELSE $2::uuid END, NULL, 'chat', 'backfill/' || n, 'notes.txt',
 CASE WHEN n = 1 THEN 'deleted' ELSE 'pending' END, 'text/plain', n * 10, sha256('x'), $3, $3, CASE WHEN n = 1 THEN NULL ELSE $3::timestamptz + interval '24 hours' END
 FROM generate_series(1,4) n`, owner, student, adminStart)
	if err != nil {
		t.Fatal(err)
	}
	store := repositories.NewStore(fixture.Pool)
	if err := store.Repositories().TutorProfiles.Create(ctx, repositories.NewTutorProfile{UserID: owner, FileQuotaBytes: 1000, RecordingQuotaBytes: 1000, CreatedAt: adminStart}, tutorCodeSource(owner)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Pool.Exec(ctx, "UPDATE attachments SET quota_tutor_id = $1", owner); err != nil {
		t.Fatal(err)
	}
	provider := dailyMigrationProvider(t, fixture)
	var before time.Time
	if err := fixture.Pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&before); err != nil {
		t.Fatal(err)
	}

	_, migrationErr := provider.UpTo(ctx, 21)
	activation, activationErr := store.Repositories().DailyUploads.Activation(ctx)
	_, repeatedErr := provider.UpTo(ctx, 21)
	repeated, rereadErr := store.Repositories().DailyUploads.Activation(ctx)
	total, totalErr := store.Repositories().DailyUploads.Bytes(ctx, student, owner, adminStart.UTC().Truncate(24*time.Hour))
	_, updateErr := fixture.Pool.Exec(ctx, "UPDATE chat_upload_accounting_activation SET active_at = active_at + interval '1 day'")
	_, deleteErr := fixture.Pool.Exec(ctx, "DELETE FROM chat_upload_accounting_activation")
	_, truncateErr := fixture.Pool.Exec(ctx, "TRUNCATE chat_upload_accounting_activation")
	_, duplicateErr := fixture.Pool.Exec(ctx, "INSERT INTO chat_upload_accounting_activation (active_at) VALUES ($1)", activation)
	_, malformedErr := fixture.Pool.Exec(ctx, "INSERT INTO chat_upload_accounting_activation (singleton, active_at) VALUES (false, 'infinity')")

	if migrationErr != nil || activationErr != nil || repeatedErr != nil || rereadErr != nil || totalErr != nil {
		t.Fatalf("migration/read failed: %v %v %v %v %v", migrationErr, activationErr, repeatedErr, rereadErr, totalErr)
	}
	if !activation.After(before) || activation.Sub(before) > 24*time.Hour || !activation.Equal(repeated) || total != 60 {
		t.Fatalf("backfill or activation changed: activation %s repeated %s before %s bytes %d", activation, repeated, before, total)
	}
	if updateErr == nil || deleteErr == nil || truncateErr == nil || duplicateErr == nil || malformedErr == nil {
		t.Fatalf("activation accepted invalid mutation: update %v delete %v duplicate %v malformed %v", updateErr, deleteErr, duplicateErr, malformedErr)
	}
}

func TestDailyUploadMigrationRollsBackItsSchema(t *testing.T) {
	t.Parallel()
	fixture := testdb.NewMigratedTo(t, 20)
	provider := dailyMigrationProvider(t, fixture)
	ctx := context.Background()

	_, upErr := provider.UpTo(ctx, 21)
	_, downErr := provider.Down(ctx)
	var removed bool
	readErr := fixture.Pool.QueryRow(ctx, "SELECT to_regclass('daily_chat_upload_bytes') IS NULL AND to_regclass('chat_upload_accounting_activation') IS NULL").Scan(&removed)

	if upErr != nil || downErr != nil || readErr != nil || !removed {
		t.Fatalf("migration rollback: up %v down %v read %v removed %t", upErr, downErr, readErr, removed)
	}
}

func dailyMigrationProvider(t *testing.T, fixture *testdb.Database) *goose.Provider {
	t.Helper()
	database, err := sql.Open("pgx", fixture.URL.Reveal())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	files, err := fs.Sub(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, database, files)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
