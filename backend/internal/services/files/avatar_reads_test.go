package files_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

const avatarReadSlots = 32

func servedAvatar(t *testing.T) (*files.Service, domain.AttachmentID) {
	t.Helper()
	database := testdb.New(t)
	store := repositories.NewStore(database.Pool)
	shared := clock.NewShared(database.Pool)
	if _, err := shared.Set(context.Background(), raceStart); err != nil {
		t.Fatal(err)
	}
	user := seedUploader(t, store)
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service := files.New(files.Dependencies{Store: store, Objects: teststorage.New(t).Store, Clock: shared, Random: core.CryptoRandom(), Keys: keys, Logger: slog.New(slog.DiscardHandler)})
	data := avatarPNG(t)
	upload, err := service.UploadAvatar(context.Background(), user, "me.png", func() ([]byte, error) { return io.ReadAll(bytes.NewReader(data)) })
	if err != nil {
		t.Fatal(err)
	}
	err = store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		return service.BindAvatar(context.Background(), repos, user.UserID, &upload.ID, raceStart)
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, upload.ID
}

func openAvatar(t *testing.T, service *files.Service, id domain.AttachmentID) files.Avatar {
	t.Helper()
	avatar, err := service.Avatar(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := avatar.Body.Close(); err != nil {
			t.Error(err)
		}
	})
	return avatar
}

func TestAvatarReadsBeyondTheInstanceLimitAreRefused(t *testing.T) {
	service, id := servedAvatar(t)
	for range avatarReadSlots {
		openAvatar(t, service, id)
	}

	_, err := service.Avatar(context.Background(), id)

	if !errors.Is(err, files.ErrAvatarReadsBusy) {
		t.Fatalf("a read beyond the limit was %v", err)
	}
}

func TestClosingAnAvatarBodyFreesItsReadSlot(t *testing.T) {
	service, id := servedAvatar(t)
	var held []files.Avatar
	for range avatarReadSlots {
		avatar, err := service.Avatar(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, avatar)
	}
	for _, avatar := range held[1:] {
		t.Cleanup(func() {
			if err := avatar.Body.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	if err := held[0].Body.Close(); err != nil {
		t.Fatal(err)
	}

	admitted := openAvatar(t, service, id)

	served, err := io.ReadAll(admitted.Body)
	if err != nil || int64(len(served)) != admitted.SizeBytes {
		t.Fatalf("the streamed body had %d of %d bytes: %v", len(served), admitted.SizeBytes, err)
	}
}
