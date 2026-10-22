package files_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
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

const pauseDeadline = 10 * time.Second

type pausedWrite struct {
	reached chan struct{}
	resume  chan struct{}
	once    sync.Once
	key     string
}

func (p *pausedWrite) Resume() {
	p.once.Do(func() { close(p.resume) })
}

func storageHoldingTheFirstWrite(t *testing.T, local *teststorage.Storage) (storage.Store, *pausedWrite) {
	t.Helper()
	target, err := url.Parse(local.Settings.Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	forward := httputil.NewSingleHostReverseProxy(target)
	paused := &pausedWrite{reached: make(chan struct{}), resume: make(chan struct{})}
	var first sync.Once
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			first.Do(func() {
				paused.key = strings.TrimPrefix(r.URL.Path, "/"+local.Settings.Bucket+"/")
				close(paused.reached)
				<-paused.resume
			})
		}
		forward.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	t.Cleanup(paused.Resume)
	settings := local.Settings
	settings.Endpoint = proxy.URL
	return storage.NewS3(settings), paused
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
	local := teststorage.New(t)
	objects, pause := storageHoldingTheFirstWrite(t, local)
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service := files.New(files.Dependencies{Store: store, Objects: objects, Clock: shared, Random: core.CryptoRandom(), Keys: keys, Logger: slog.New(slog.DiscardHandler)})
	data := avatarPNG(t)
	uploaded := make(chan error, 1)
	go func() {
		_, uploadErr := service.UploadAvatar(context.Background(), user, "me.png", func() ([]byte, error) { return io.ReadAll(bytes.NewReader(data)) })
		uploaded <- uploadErr
	}()
	deadline := time.NewTimer(pauseDeadline)
	defer deadline.Stop()
	select {
	case <-pause.reached:
	case <-deadline.C:
		t.Fatal("the upload never reached its object write")
	}

	err = store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		if err := service.PurgeUploadsOf(context.Background(), repos, user, raceStart); err != nil {
			return err
		}
		_, err := repos.Users.Delete(context.Background(), user)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	pause.Resume()
	select {
	case err = <-uploaded:
	case <-deadline.C:
		t.Fatal("the upload did not finish after its write resumed")
	}

	if !errors.Is(err, files.ErrPurgedDuringUpload) {
		t.Fatalf("expected the purge to be reported, got %v", err)
	}
	var row repositories.ClaimedOutboxRow
	err = database.Pool.QueryRow(context.Background(), "SELECT kind, payload, idempotency_key FROM outbox WHERE strpos(idempotency_key, $1) > 0", ":after-purge").
		Scan(&row.Kind, &row.Payload, &row.IdempotencyKey)
	if err != nil {
		t.Fatalf("no after-purge deletion was queued: %v", err)
	}
	landed, landedErr := local.Store.Open(context.Background(), pause.key)
	if landedErr != nil {
		t.Fatalf("the paused write never reached storage: %v", landedErr)
	}
	if err := landed.Close(); err != nil {
		t.Fatal(err)
	}
	reason, deleteErr := service.DeleteObject(context.Background(), row, raceStart)
	_, openErr := local.Store.Open(context.Background(), pause.key)
	if deleteErr != nil || reason != domain.DoneSent || !errors.Is(openErr, storage.ErrObjectNotFound) {
		t.Fatalf("deletion %s %v; the object is still readable: %v", reason, deleteErr, openErr)
	}
}

func TestAPurgeAfterADeadDeletionStillDeletesTheObject(t *testing.T) {
	database := testdb.New(t)
	store := repositories.NewStore(database.Pool)
	shared := clock.NewShared(database.Pool)
	if _, err := shared.Set(context.Background(), raceStart); err != nil {
		t.Fatal(err)
	}
	user := seedUploader(t, store)
	objects := teststorage.New(t).Store
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service := files.New(files.Dependencies{Store: store, Objects: objects, Clock: shared, Random: core.CryptoRandom(), Keys: keys, Logger: slog.New(slog.DiscardHandler)})
	data := avatarPNG(t)
	upload, err := service.UploadAvatar(context.Background(), user, "me.png", func() ([]byte, error) { return io.ReadAll(bytes.NewReader(data)) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExpirePending(context.Background(), raceStart.Add(25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	firstKey := string(domain.KindStorageDelete) + ":" + upload.ID.String()
	if _, err := database.Pool.Exec(context.Background(), "UPDATE outbox SET status = 'dead', done_at = $2 WHERE idempotency_key = $1", firstKey, raceStart.Add(25*time.Hour)); err != nil {
		t.Fatal(err)
	}

	err = store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		return service.PurgeUploadsOf(context.Background(), repos, user, raceStart.Add(26*time.Hour))
	})

	if err != nil {
		t.Fatal(err)
	}
	var row repositories.ClaimedOutboxRow
	err = database.Pool.QueryRow(context.Background(), "SELECT kind, payload, idempotency_key FROM outbox WHERE idempotency_key = $1", firstKey+":purge").
		Scan(&row.Kind, &row.Payload, &row.IdempotencyKey)
	if err != nil {
		t.Fatalf("the purge queued no deletion of its own: %v", err)
	}
	var payload domain.AttachmentPayload
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	reason, deleteErr := service.DeleteObject(context.Background(), row, raceStart.Add(26*time.Hour))
	_, openErr := objects.Open(context.Background(), payload.Key)
	if deleteErr != nil || reason != domain.DoneSent || !errors.Is(openErr, storage.ErrObjectNotFound) {
		t.Fatalf("deletion %s %v; the object is still readable: %v", reason, deleteErr, openErr)
	}
}
