package teststorage

import (
	"context"
	"testing"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
)

const (
	Bucket         = "ovc-test"
	prepareTimeout = 10 * time.Second
)

type Storage struct {
	Settings core.StorageSettings
	Store    *storage.S3
}

func New(tb testing.TB) *Storage {
	tb.Helper()
	settings, err := core.TestStorageFromEnvironment(Bucket)
	if err != nil {
		tb.Fatalf("test storage: %v", err)
	}
	store := storage.NewS3(settings)
	ctx, cancel := context.WithTimeout(context.Background(), prepareTimeout)
	defer cancel()
	if err := store.EnsureBucket(ctx); err != nil {
		tb.Fatalf("test storage: %s", core.ErrorKind(err))
	}
	return &Storage{Settings: settings, Store: store}
}
