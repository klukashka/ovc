package startup_test

import (
	"context"
	"strings"
	"testing"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/teststorage"
)

func TestEnsureBucketCreatesTheBucketWithItsIncompleteUploadRule(t *testing.T) {
	t.Parallel()
	bucket := "ovc-ensure-" + core.NewRandomID()[:12]
	settings, err := core.TestStorageFromEnvironment(teststorage.Bucket)
	if err != nil {
		t.Fatal(err)
	}
	settings.Bucket = bucket
	t.Cleanup(func() {
		if err := storage.NewS3(settings).DeleteEmptyBucket(context.Background()); err != nil {
			t.Errorf("the test bucket could not be removed: %s", core.ErrorKind(err))
		}
	})

	created := runBinary(t, "api", []string{"OVC_STORAGE_BUCKET=" + bucket}, "ensure-bucket")
	again := runBinary(t, "api", []string{"OVC_STORAGE_BUCKET=" + bucket}, "ensure-bucket")
	refused := runBinary(t, "api", []string{"OVC_STORAGE_BUCKET=Not_A_Bucket"}, "ensure-bucket")
	days, found, err := storage.NewS3(settings).IncompleteUploadDays(context.Background())

	if created.exitCode != 0 || created.stdout != "ok\n" || again.exitCode != 0 || again.stdout != "ok\n" {
		t.Fatalf("ensure-bucket answered %d %q %q, then %d %q", created.exitCode, created.stdout, created.stderr, again.exitCode, again.stdout)
	}
	if refused.exitCode != 1 || !strings.HasPrefix(refused.stderr, "configuration: OVC_STORAGE_BUCKET: ") {
		t.Fatalf("an invalid bucket answered %d %q", refused.exitCode, refused.stderr)
	}
	if err != nil || !found || days != 1 {
		t.Fatalf("the bucket's incomplete upload rule is %d days, found %t: %v", days, found, err)
	}
}
