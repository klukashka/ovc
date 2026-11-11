package files_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

const (
	insertExpiryTutorProfile = `INSERT INTO tutor_profiles (user_id, code, file_quota_bytes, recording_quota_bytes, created_at, updated_at)
VALUES ($1::uuid, 'ABCD2345', 1073741824, 1073741824, $2::timestamptz, $2::timestamptz)`
	insertExpiringLibraryFile = `INSERT INTO attachments (id, uploader_id, context, quota_tutor_id, storage_key, filename, media_type, size_bytes, sha256,
    state, expires_at, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, 'library', $2::uuid, 'library/' || $1, 'notes.pdf', 'application/pdf', 1024, sha256('x'), 'pending',
    $3::timestamptz, $3::timestamptz - interval '1 day', $3::timestamptz - interval '1 day')`
	bindHeldLibraryFile   = `UPDATE attachments SET state = 'bound', expires_at = NULL, updated_at = $2::timestamptz WHERE id = $1::uuid AND state = 'pending'`
	chargeHeldLibraryFile = `INSERT INTO quota_charges (tutor_id, attachment_id, size_bytes, created_at) VALUES ($2::uuid, $1::uuid, 1024, $3::timestamptz)`
	boundFileIsIntact     = `SELECT
    (SELECT state = 'bound' AND expires_at IS NULL FROM attachments WHERE id = $1::uuid) AND
    (SELECT count(*) = 1 FROM quota_charges WHERE attachment_id = $1::uuid) AND
    (SELECT count(*) = 0 FROM outbox WHERE kind = 'storage_delete' AND strpos(idempotency_key, $1::uuid::text) > 0)`
)

func TestABindCommittingAtTheExpiryBoundaryKeepsItsFile(t *testing.T) {
	database := testdb.New(t)
	store := repositories.NewStore(database.Pool)
	shared := clock.NewShared(database.Pool)
	if _, err := shared.Set(context.Background(), raceStart); err != nil {
		t.Fatal(err)
	}
	user := seedUploader(t, store)
	if _, err := database.Pool.Exec(context.Background(), insertExpiryTutorProfile, user.UserID.String(), raceStart); err != nil {
		t.Fatal(err)
	}
	attachment := domain.AttachmentID{9}
	if _, err := database.Pool.Exec(context.Background(), insertExpiringLibraryFile, attachment.String(), user.UserID.String(), raceStart); err != nil {
		t.Fatal(err)
	}
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service := files.New(files.Dependencies{
		Store: store, Objects: teststorage.New(t).Store, Clock: shared, Random: core.CryptoRandom(), Keys: keys, Logger: slog.New(slog.DiscardHandler),
	})
	binding, err := database.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	committed := false
	t.Cleanup(func() {
		if committed {
			return
		}
		if err := binding.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if _, err := binding.Exec(context.Background(), bindHeldLibraryFile, attachment.String(), raceStart.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := binding.Exec(context.Background(), chargeHeldLibraryFile, attachment.String(), user.UserID.String(), raceStart.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}

	expiredDuringBind, expireErr := service.ExpirePending(context.Background(), raceStart.Add(time.Second))
	commitErr := binding.Commit(context.Background())
	committed = commitErr == nil
	expiredAfterBind, laterErr := service.ExpirePending(context.Background(), raceStart.Add(time.Minute))

	if expireErr != nil || commitErr != nil || laterErr != nil {
		t.Fatalf("the expiry or the bind failed: %v %v %v", expireErr, commitErr, laterErr)
	}
	if expiredDuringBind != 0 || expiredAfterBind != 0 {
		t.Fatalf("the expiry claimed the file being bound: %d during the bind, %d after it", expiredDuringBind, expiredAfterBind)
	}
	var intact bool
	if err := database.Pool.QueryRow(context.Background(), boundFileIsIntact, attachment.String()).Scan(&intact); err != nil {
		t.Fatal(err)
	}
	if !intact {
		t.Fatal("the bound file lost its state or its charge, or its object deletion was queued")
	}
}
