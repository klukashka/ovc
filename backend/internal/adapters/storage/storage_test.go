package storage_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"testing"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/teststorage"
)

type contractStore interface {
	storage.Store
	IncompleteUploads(ctx context.Context, object string) ([]storage.MultipartUploadID, error)
}

func stores(t *testing.T) map[string]contractStore {
	t.Helper()
	return map[string]contractStore{"s3": teststorage.New(t).Store, "memory": storage.NewMemory()}
}

func patterned(size int, seed byte) []byte {
	block := sha256.Sum256([]byte{seed})
	data := make([]byte, 0, size+len(block))
	for len(data) < size {
		data = append(data, block[:]...)
		block = sha256.Sum256(block[:])
	}
	return data[:size]
}

func uploadParts(t *testing.T, store storage.Store, object string, parts [][]byte) (storage.MultipartUploadID, []storage.CompletedPart) {
	t.Helper()
	upload, err := store.CreateMultipartUpload(context.Background(), object, "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	completed := make([]storage.CompletedPart, 0, len(parts))
	for index, data := range parts {
		part, err := store.UploadPart(context.Background(), object, upload, storage.PartNumber(index+1), data)
		if err != nil {
			t.Fatal(err)
		}
		completed = append(completed, part)
	}
	return upload, completed
}

func incomplete(t *testing.T, store contractStore, object string) []storage.MultipartUploadID {
	t.Helper()
	uploads, err := store.IncompleteUploads(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	return uploads
}

func listedUpload(t *testing.T, store contractStore, object string) storage.MultipartUploadID {
	t.Helper()
	upload, _ := uploadParts(t, store, object, [][]byte{patterned(storage.MinimumPartSize, 4)})
	if listed := incomplete(t, store, object); len(listed) != 1 || listed[0] != upload {
		t.Fatalf("the started upload is listed as %v", listed)
	}
	return upload
}

func putPatterned(t *testing.T, store storage.Store, size int) (string, []byte) {
	t.Helper()
	object, data := key(t), patterned(size, 0x5a)
	if err := store.Put(context.Background(), object, data, "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	return object, data
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
			t.Run("a multipart upload of three parts reads back whole", func(t *testing.T) {
				object := key(t)
				parts := [][]byte{patterned(storage.MinimumPartSize, 1), patterned(storage.MinimumPartSize, 2), patterned(1000, 3)}
				upload, completed := uploadParts(t, store, object, parts)

				err := store.CompleteMultipartUpload(context.Background(), object, upload, completed)

				if err != nil {
					t.Fatalf("complete %v", err)
				}
				if !bytes.Equal(read(t, store, object), bytes.Join(parts, nil)) {
					t.Fatal("the object differs from its parts in order")
				}
				if remaining := incomplete(t, store, object); len(remaining) != 0 {
					t.Fatalf("incomplete uploads left %v", remaining)
				}
			})
			t.Run("an aborted multipart upload leaves no object and no incomplete upload", func(t *testing.T) {
				object := key(t)
				upload := listedUpload(t, store, object)

				first := store.AbortMultipartUpload(context.Background(), object, upload)
				second := store.AbortMultipartUpload(context.Background(), object, upload)

				_, openErr := store.Open(context.Background(), object)
				remaining := incomplete(t, store, object)
				if first != nil || second != nil || !errors.Is(openErr, storage.ErrObjectNotFound) || len(remaining) != 0 {
					t.Fatalf("aborts %v %v, open %v, incomplete uploads %v", first, second, openErr, remaining)
				}
			})
			t.Run("a part uploaded after abort is refused as not found", func(t *testing.T) {
				object := key(t)
				upload := listedUpload(t, store, object)
				if err := store.AbortMultipartUpload(context.Background(), object, upload); err != nil {
					t.Fatal(err)
				}

				_, err := store.UploadPart(context.Background(), object, upload, 2, patterned(1000, 5))

				if !errors.Is(err, storage.ErrUploadNotFound) {
					t.Fatalf("expected an unknown upload, got %v", err)
				}
			})
			t.Run("completing with a mismatched entity tag is an invalid part", func(t *testing.T) {
				object := key(t)
				upload, completed := uploadParts(t, store, object, [][]byte{patterned(storage.MinimumPartSize, 6), patterned(1000, 7)})
				completed[1].ETag = completed[0].ETag

				err := store.CompleteMultipartUpload(context.Background(), object, upload, completed)

				if !errors.Is(err, storage.ErrInvalidPart) {
					t.Fatalf("expected an invalid part, got %v", err)
				}
			})
			t.Run("a too small part before the last is an invalid part", func(t *testing.T) {
				object := key(t)
				upload, completed := uploadParts(t, store, object, [][]byte{patterned(1000, 8), patterned(1000, 9)})

				err := store.CompleteMultipartUpload(context.Background(), object, upload, completed)

				if !errors.Is(err, storage.ErrInvalidPart) {
					t.Fatalf("expected an invalid part, got %v", err)
				}
			})
			t.Run("an empty part list is an invalid part", func(t *testing.T) {
				object := key(t)
				upload := listedUpload(t, store, object)

				err := store.CompleteMultipartUpload(context.Background(), object, upload, nil)

				if !errors.Is(err, storage.ErrInvalidPart) {
					t.Fatalf("expected an invalid part, got %v", err)
				}
			})
			t.Run("completing twice succeeds", func(t *testing.T) {
				object := key(t)
				parts := [][]byte{patterned(1000, 10)}
				upload, completed := uploadParts(t, store, object, parts)
				if err := store.CompleteMultipartUpload(context.Background(), object, upload, completed); err != nil {
					t.Fatal(err)
				}

				err := store.CompleteMultipartUpload(context.Background(), object, upload, completed)

				if err != nil || !bytes.Equal(read(t, store, object), parts[0]) {
					t.Fatalf("second complete %v", err)
				}
			})
			t.Run("a ranged read of an empty object is refused as not satisfiable", func(t *testing.T) {
				object, _ := putPatterned(t, store, 0)

				_, err := storage.NewRangeReader(context.Background(), store, object).ReadAt(make([]byte, 10), 0)

				if !errors.Is(err, storage.ErrRangeNotSatisfiable) {
					t.Fatalf("expected not satisfiable, got %v", err)
				}
			})
			t.Run("a ranged read returns exactly the range", func(t *testing.T) {
				object, data := putPatterned(t, store, 4096)
				buffer := make([]byte, 700)

				count, err := storage.NewRangeReader(context.Background(), store, object).ReadAt(buffer, 1234)

				if err != nil || count != len(buffer) || !bytes.Equal(buffer, data[1234:1934]) {
					t.Fatalf("read %d bytes, %v", count, err)
				}
			})
			t.Run("a range overlapping the end returns the rest with end of file", func(t *testing.T) {
				object, data := putPatterned(t, store, 4096)
				buffer := make([]byte, 100)

				count, err := storage.NewRangeReader(context.Background(), store, object).ReadAt(buffer, 4046)

				if !errors.Is(err, io.EOF) || count != 50 || !bytes.Equal(buffer[:count], data[4046:]) {
					t.Fatalf("read %d bytes, %v", count, err)
				}
			})
			t.Run("a range past the end is refused", func(t *testing.T) {
				object, data := putPatterned(t, store, 4096)
				reader := storage.NewRangeReader(context.Background(), store, object)

				atEnd, atEndErr := reader.ReadAt(make([]byte, 10), int64(len(data)))
				beyond, beyondErr := reader.ReadAt(make([]byte, 10), int64(len(data))+500)

				if atEnd != 0 || beyond != 0 || !errors.Is(atEndErr, storage.ErrRangeNotSatisfiable) || !errors.Is(beyondErr, storage.ErrRangeNotSatisfiable) {
					t.Fatalf("at the end %d %v, beyond %d %v", atEnd, atEndErr, beyond, beyondErr)
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
