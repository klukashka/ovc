package storage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/teststorage"
)

func stores(t *testing.T) map[string]storage.Store {
	t.Helper()
	return map[string]storage.Store{"s3": teststorage.New(t).Store, "memory": storage.NewMemory()}
}

func key(t *testing.T) string {
	t.Helper()
	return "contract/" + core.NewRandomID()
}

func read(t *testing.T, store storage.Store, name string) []byte {
	t.Helper()
	body, err := store.Open(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := body.Close(); err != nil {
			t.Error(err)
		}
	}()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestStorageContract(t *testing.T) {
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			t.Run("put then open returns the bytes", func(t *testing.T) {
				object := key(t)
				data := []byte("\x89PNG avatar bytes")

				err := store.Put(context.Background(), object, data, "image/png")

				if err != nil || !bytes.Equal(read(t, store, object), data) {
					t.Fatalf("put %v", err)
				}
			})
			t.Run("a missing key is not found", func(t *testing.T) {
				object := key(t)

				_, err := store.Open(context.Background(), object)

				if !errors.Is(err, storage.ErrObjectNotFound) {
					t.Fatalf("expected not found, got %v", err)
				}
			})
			t.Run("delete removes the object and repeats safely", func(t *testing.T) {
				object := key(t)
				if err := store.Put(context.Background(), object, []byte("bytes"), "image/jpeg"); err != nil {
					t.Fatal(err)
				}

				first := store.Delete(context.Background(), object)
				second := store.Delete(context.Background(), object)

				_, openErr := store.Open(context.Background(), object)
				if first != nil || second != nil || !errors.Is(openErr, storage.ErrObjectNotFound) {
					t.Fatalf("deletes %v %v, open %v", first, second, openErr)
				}
			})
			t.Run("ping reaches the bucket", func(t *testing.T) {
				err := store.Ping(context.Background())

				if err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestS3PingFailsForAMissingBucket(t *testing.T) {
	settings := teststorage.New(t).Settings
	settings.Bucket = "ovc-missing-" + core.NewRandomID()[:12]

	err := storage.NewS3(settings).Ping(context.Background())

	if err == nil {
		t.Fatal("a missing bucket answered the ping")
	}
}
