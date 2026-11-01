package startup_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

type shutdownUploadResult struct {
	status int
	err    error
}

func heldProcessComplete(t *testing.T) (string, <-chan struct{}, func()) {
	t.Helper()
	settings, err := core.TestStorageFromEnvironment(teststorage.Bucket)
	if err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(settings.Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	forward := httputil.NewSingleHostReverseProxy(target)
	reached, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	resume := func() { once.Do(func() { close(release) }) }
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Query().Get("uploadId") != "" {
			close(reached)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		forward.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	t.Cleanup(resume)
	return proxy.URL, reached, resume
}

func uploadShutdownRequest(t *testing.T, ctx context.Context, base, token, channel string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "drain.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "Upload shutdown fixture"); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/uploads?context=chat&scope=channel:"+channel, &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	return request
}

func TestUploadSettlementFinishesInsideProcessShutdownDrain(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.New(t)
	endpoint, reached, release := heldProcessComplete(t)
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	address := freeAddress(t)
	base := "http://" + address
	server, output := startServer(t, ctx, []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=1", "OVC_STORAGE_ENDPOINT=" + endpoint})
	waitForStatus(t, ctx, base+"/api/v1/health/ready")
	seed := call(t, ctx, http.MethodPost, base+"/__test__/accounts", `{"role":"tutor","name":"A tutor","email":"drain@example.org","language":"en","timezone":"UTC"}`, map[string]string{"Content-Type": "application/json"})
	var account seededAccount
	if err := decodeJSON(seed.body, &account); err != nil || seed.status != http.StatusOK {
		t.Fatal("could not seed upload account")
	}
	group := call(t, ctx, http.MethodPost, base+"/api/v1/groups", `{"name":"Drain group","colour":"moss","member_ids":[]}`, map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + account.AccessToken, "Idempotency-Key": core.NewRandomID()})
	if group.status != http.StatusCreated {
		t.Fatalf("group setup answered %d", group.status)
	}
	var channel string
	if err := database.Pool.QueryRow(ctx, "SELECT id FROM channels WHERE kind = 'group'").Scan(&channel); err != nil {
		t.Fatal(err)
	}
	request := uploadShutdownRequest(t, ctx, base, account.AccessToken, channel)
	done := make(chan shutdownUploadResult, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			done <- shutdownUploadResult{err: err}
			return
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		done <- shutdownUploadResult{status: response.StatusCode, err: errors.Join(readErr, response.Body.Close())}
	}()
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("upload did not reach Complete")
	}
	lock, err := database.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	var key string
	if err := database.Pool.QueryRow(ctx, "SELECT storage_key FROM upload_intents").Scan(&key); err != nil {
		t.Fatal(err)
	}
	settings, settingsErr := core.TestStorageFromEnvironment(teststorage.Bucket)
	if settingsErr != nil {
		t.Fatal(settingsErr)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := storage.NewS3(settings).Delete(cleanup, key); err != nil {
			t.Error(err)
		}
	})
	var attachment domain.AttachmentID
	if err := lock.QueryRow(ctx, "SELECT attachment_id FROM upload_intents FOR UPDATE").Scan(&attachment); err != nil {
		t.Fatal(err)
	}

	wall := clock.System{}
	started := wall.Now()
	signalErr := server.Process.Signal(syscall.SIGTERM)
	release()
	result := <-done
	waitErr := server.Wait()
	elapsed := wall.Now().Sub(started)

	if signalErr != nil || waitErr != nil || result.err != nil || result.status < 400 || elapsed >= 40*time.Second {
		t.Fatalf("shutdown: signal=%v exit=%v upload=%+v elapsed=%s", signalErr, waitErr, result, elapsed)
	}
	var retained bool
	if err := database.Pool.QueryRow(ctx, "SELECT completion_started AND NOT completion_confirmed FROM upload_intents WHERE attachment_id = $1", attachment).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if !retained || !strings.Contains(output.String(), `"reason":"shutdown"`) {
		t.Fatal("shutdown lost unresolved ownership or did not drain normally")
	}
}
