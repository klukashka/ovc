package startup_test

import (
	"context"
	"net/http"
	"syscall"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

func TestUnsubscribeTokenRouteIsGone(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	seedTutor(t, database, "correct horse battery")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	address := freeAddress(t)
	env := []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=1"}
	server, output := startServer(t, ctx, env)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("server output:\n%s", output.String())
		}
	})
	waitForStatus(t, ctx, "http://"+address+"/api/v1/health/ready")
	request := `{"user":"` + domain.UserID{1}.String() + `","group":"lesson_reminders"}`

	minted := call(t, ctx, http.MethodPost, "http://"+address+"/__test__/unsubscribe-token", request, map[string]string{"Content-Type": "application/json"})
	mail := call(t, ctx, http.MethodPost, "http://"+address+"/__test__/mail", `{"to":"tutor@example.org"}`, map[string]string{"Content-Type": "application/json"})
	signalErr := server.Process.Signal(syscall.SIGTERM)
	waitErr := server.Wait()

	if minted.status != http.StatusNotFound || signalErr != nil || waitErr != nil {
		t.Fatalf("the removed route answered %d %s, signal %v, exit %v", minted.status, minted.body, signalErr, waitErr)
	}
	if mail.status != http.StatusOK {
		t.Fatalf("the test control surface is not mounted: the mail route answered %d %s", mail.status, mail.body)
	}
}
