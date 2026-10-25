package startup_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

func TestUnsubscribeTokenRouteIsAbsentWithoutTestControl(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		flag        string
		mintStatus  int
		previewBody string
	}{
		"with the flag":    {flag: "1", mintStatus: http.StatusOK, previewBody: `{"group":"lesson_reminders"}`},
		"without the flag": {flag: "0", mintStatus: http.StatusNotFound},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			database := testdb.New(t)
			seedTutor(t, database, "correct horse battery")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			address := freeAddress(t)
			env := []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=" + testCase.flag}
			server, output := startServer(t, ctx, env)
			t.Cleanup(func() {
				if t.Failed() {
					t.Logf("server output:\n%s", output.String())
				}
			})
			waitForStatus(t, ctx, "http://"+address+"/api/v1/health/ready")
			request := `{"user":"` + domain.UserID{1}.String() + `","group":"lesson_reminders"}`

			minted := call(t, ctx, http.MethodPost, "http://"+address+"/__test__/unsubscribe-token", request, map[string]string{"Content-Type": "application/json"})
			var token struct {
				Token string `json:"token"`
			}
			preview := exchange{}
			if minted.status == http.StatusOK {
				if err := json.Unmarshal([]byte(minted.body), &token); err != nil {
					t.Fatal(err)
				}
				preview = call(t, ctx, http.MethodGet, "http://"+address+"/api/v1/unsubscribe/preview?token="+url.QueryEscape(token.Token), "", nil)
			}
			signalErr := server.Process.Signal(syscall.SIGTERM)
			waitErr := server.Wait()

			if minted.status != testCase.mintStatus || signalErr != nil || waitErr != nil {
				t.Fatalf("minting answered %d %s (want %d), signal %v, exit %v", minted.status, minted.body, testCase.mintStatus, signalErr, waitErr)
			}
			if testCase.previewBody != "" && (preview.status != http.StatusOK || strings.TrimSpace(preview.body) != testCase.previewBody) {
				t.Fatalf("the minted token previewed as %d %s", preview.status, preview.body)
			}
		})
	}
}
