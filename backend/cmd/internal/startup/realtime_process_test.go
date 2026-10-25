package startup_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

func dialProcessSocket(t *testing.T, ctx context.Context, address string, token string) *websocket.Conn {
	t.Helper()
	conn, answer, err := websocket.Dial(ctx, "ws://"+address+"/api/v1/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://ovc.example"}},
	})
	if answer != nil && answer.Body != nil {
		if closeErr := answer.Body.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := conn.CloseNow(); closeErr != nil {
			t.Logf("closing a test socket: %v", closeErr)
		}
	})
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"auth","payload":{"access_token":"`+token+`"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, ready, err := conn.Read(ctx); err != nil || !strings.Contains(string(ready), `"ready"`) {
		t.Fatalf("no ready frame: %s %v", ready, err)
	}
	return conn
}

func closeCodeOf(ctx context.Context, conn *websocket.Conn) websocket.StatusCode {
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

func TestRealtimeTestRoutesAreAbsentWithoutTestControl(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		flag   string
		status int
	}{
		"with the flag":    {flag: "1", status: http.StatusOK},
		"without the flag": {flag: "0", status: http.StatusNotFound},
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
			base := "http://" + address
			waitForStatus(t, ctx, base+"/api/v1/health/ready")
			signedIn := call(t, ctx, http.MethodPost, base+"/api/v1/auth/login", `{"email":"tutor@example.org","password":"correct horse battery"}`,
				map[string]string{"Content-Type": "application/json", "Origin": "https://ovc.example"})
			var token struct {
				AccessToken string `json:"access_token"`
			}
			if err := json.Unmarshal([]byte(signedIn.body), &token); err != nil {
				t.Fatal(err)
			}
			closing := dialProcessSocket(t, ctx, address, token.AccessToken)
			staying := dialProcessSocket(t, ctx, address, token.AccessToken)
			user := `"` + domain.UserID{1}.String() + `"`
			jsonBody := map[string]string{"Content-Type": "application/json"}

			present := call(t, ctx, http.MethodPost, base+"/__test__/presence", `{"user":`+user+`}`, jsonBody)
			closed := call(t, ctx, http.MethodPost, base+"/__test__/realtime/close", `{"user":`+user+`,"code":1012}`, jsonBody)
			refusedCode := call(t, ctx, http.MethodPost, base+"/__test__/realtime/close", `{"user":`+user+`,"code":4401}`, jsonBody)
			signalErr := server.Process.Signal(syscall.SIGTERM)
			shutdownCode := closeCodeOf(ctx, staying)
			waitErr := server.Wait()

			if present.status != testCase.status || closed.status != testCase.status || signalErr != nil || waitErr != nil {
				t.Fatalf("presence %d %s, close %d %s, signal %v, exit %v", present.status, present.body, closed.status, closed.body, signalErr, waitErr)
			}
			if testCase.flag == "1" {
				if strings.TrimSpace(present.body) != `{"online":true,"sockets":2}` || strings.TrimSpace(closed.body) != `{"closed":2}` ||
					refusedCode.status != http.StatusUnprocessableEntity {
					t.Fatalf("presence %s, close %s, a 4401 request %d", present.body, closed.body, refusedCode.status)
				}
				if code := closeCodeOf(ctx, closing); code != websocket.StatusCode(domain.CloseReconnect) || shutdownCode != websocket.StatusCode(domain.CloseReconnect) {
					t.Fatalf("the sockets closed with %d and %d", code, shutdownCode)
				}
				return
			}
			if shutdownCode != websocket.StatusCode(domain.CloseShutdown) {
				t.Fatalf("shutdown closed the socket with %d", shutdownCode)
			}
		})
	}
}
