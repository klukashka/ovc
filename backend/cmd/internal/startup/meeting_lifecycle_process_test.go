package startup_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/testdb"
)

const absentLessonPath = "/api/v1/lessons/00000000-0000-4000-8000-000000000001"

func stopServer(t *testing.T, server *exec.Cmd) {
	t.Helper()
	if err := server.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := server.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestMeetingLifecycleProcess(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	address := freeAddress(t)
	base := "http://" + address
	guarded, guardedOutput := startServer(t, ctx, []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=1", "OVC_CLOCK=test", "OVC_LOG_LEVEL=debug"})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("server output:\n%s", guardedOutput.String())
		}
	})
	waitForStatus(t, ctx, base+"/api/v1/health/ready")
	jsonHeaders := map[string]string{"Content-Type": "application/json", "Origin": "http://" + address}
	current := clock.System{}.Now().UTC().Format(time.RFC3339)
	if set := call(t, ctx, http.MethodPost, base+"/__test__/clock", `{"set":"`+current+`"}`, jsonHeaders); set.status != http.StatusOK {
		t.Fatalf("setting the clock answered %d %s", set.status, set.body)
	}
	seeded := call(t, ctx, http.MethodPost, base+"/__test__/accounts", `{"role":"tutor","name":"A language tutor","email":"tutor@example.org","language":"en","timezone":"UTC"}`, jsonHeaders)
	var account struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(seeded.body), &account); err != nil || account.AccessToken == "" {
		t.Fatalf("seeding answered %d %s", seeded.status, seeded.body)
	}
	signedIn := map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + account.AccessToken, "Origin": "http://" + address}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO outbox (id, lane, priority, kind, payload, idempotency_key, status, next_attempt_at, created_at)
VALUES (gen_random_uuid(), 'provider_call', 1, 'meeting_ended_hint', '{"lesson_id":"00000000-0000-4000-8000-000000000001","meeting_id":"00000000-0000-4000-8000-000000000001","check":0}', 'lifecycle-process', 'pending', now() - interval '1 hour', now() - interval '1 hour')`); err != nil {
		t.Fatal(err)
	}

	guardedJoin := call(t, ctx, http.MethodPost, base+absentLessonPath+"/join", "", signedIn)
	guardedHint := call(t, ctx, http.MethodGet, base+"/api/v1/integrations/bbb/not-a-key/ended", "", nil)
	tick := call(t, ctx, http.MethodPost, base+"/__test__/tick", `{}`, jsonHeaders)
	stopServer(t, guarded)
	plain, plainOutput := startServer(t, ctx, []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=0", "OVC_CLOCK=system"})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("plain server output:\n%s", plainOutput.String())
		}
	})
	waitForStatus(t, ctx, base+"/api/v1/health/ready")
	plainJoin := call(t, ctx, http.MethodPost, base+absentLessonPath+"/join", "", signedIn)
	plainEnd := call(t, ctx, http.MethodPost, base+absentLessonPath+"/end", "", signedIn)
	plainRestart := call(t, ctx, http.MethodPost, base+absentLessonPath+"/restart-room", "", signedIn)
	plainLesson := call(t, ctx, http.MethodGet, base+absentLessonPath, "", signedIn)
	plainHint := call(t, ctx, http.MethodGet, base+"/api/v1/integrations/bbb/not-a-key/ended", "", nil)
	stopServer(t, plain)

	if guardedJoin.status != http.StatusNotFound || guardedHint.status != http.StatusOK || tick.status != http.StatusOK {
		t.Fatalf("guarded deployment: join %d %s, hint %d, tick %d", guardedJoin.status, guardedJoin.body, guardedHint.status, tick.status)
	}
	var done bool
	if err := database.Pool.QueryRow(ctx, "SELECT status = 'done' AND done_reason = 'obsolete' FROM outbox WHERE idempotency_key = 'lifecycle-process'").Scan(&done); err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("the production provider lane did not run the ended hint effect")
	}
	for name, result := range map[string]exchange{"join": plainJoin, "end": plainEnd, "restart": plainRestart} {
		if result.status != http.StatusServiceUnavailable || !strings.Contains(result.body, "service_unavailable") {
			t.Fatalf("without test control %s answered %d %s", name, result.status, result.body)
		}
	}
	if plainLesson.status != http.StatusNotFound || plainHint.status != http.StatusOK {
		t.Fatalf("without test control the lesson answered %d and the hint %d", plainLesson.status, plainHint.status)
	}
}
