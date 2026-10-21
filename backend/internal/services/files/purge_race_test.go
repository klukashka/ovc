package files_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

var raceStart = time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)

type interruptedStore struct {
	storage.Store
	beforePut func()
	lastKey   string
}

func (s *interruptedStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	s.beforePut()
	s.lastKey = key
	return s.Store.Put(ctx, key, data, contentType)
}

func seedUploader(t *testing.T, store *repositories.Store) domain.UserID {
	t.Helper()
	user := domain.UserID{7}
	hasher, err := core.NewPasswordHasher(core.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}, core.CryptoRandom(), 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := hasher.Hash(context.Background(), "uploader horse battery")
	if err != nil {
		t.Fatal(err)
	}
	err = store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: user, Role: domain.RoleTutor, State: domain.AccountUnverified, Email: "uploader@example.org",
		PasswordHash: encoded, Name: "A language tutor", Language: domain.LanguageEnglish,
		Timezone: "UTC", Theme: domain.ThemeSystem, ConsentVersion: "2026-10", ConsentAt: raceStart, CreatedAt: raceStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func avatarPNG(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestAnUploadPurgedBeforeItsObjectLandsQueuesTheObjectsDeletion(t *testing.T) {
	database := testdb.New(t)
	store := repositories.NewStore(database.Pool)
	shared := clock.NewShared(database.Pool)
	if _, err := shared.Set(context.Background(), raceStart); err != nil {
		t.Fatal(err)
	}
	user := seedUploader(t, store)
	objects := &interruptedStore{Store: teststorage.New(t).Store}
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service := files.New(files.Dependencies{Store: store, Objects: objects, Clock: shared, Random: core.CryptoRandom(), Keys: keys, Logger: slog.New(slog.DiscardHandler)})
	objects.beforePut = func() {
		err := store.WithTx(context.Background(), func(repos repositories.Repositories) error {
			if err := service.PurgeUploadsOf(context.Background(), repos, user, raceStart); err != nil {
				return err
			}
			_, err := repos.Users.Delete(context.Background(), user)
			return err
		})
		if err != nil {
			t.Error(err)
		}
	}
	data := avatarPNG(t)

	_, err = service.UploadAvatar(context.Background(), user, "me.png", func() ([]byte, error) { return io.ReadAll(bytes.NewReader(data)) })

	if !errors.Is(err, files.ErrPurgedDuringUpload) {
		t.Fatalf("expected the purge to be reported, got %v", err)
	}
	var row repositories.ClaimedOutboxRow
	err = database.Pool.QueryRow(context.Background(), "SELECT kind, payload, idempotency_key FROM outbox WHERE strpos(idempotency_key, $1) > 0", ":after-purge").
		Scan(&row.Kind, &row.Payload, &row.IdempotencyKey)
	if err != nil {
		t.Fatalf("no after-purge deletion was queued: %v", err)
	}
	reason, deleteErr := service.DeleteObject(context.Background(), row, raceStart)
	_, openErr := objects.Open(context.Background(), objects.lastKey)
	if deleteErr != nil || reason != domain.DoneSent || !errors.Is(openErr, storage.ErrObjectNotFound) {
		t.Fatalf("deletion %s %v; the object is still readable: %v", reason, deleteErr, openErr)
	}
}
