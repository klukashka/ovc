package startup_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"ovc/backend/cmd/internal/startup"
	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

var fakeRoomRoutes = []struct {
	method string
	path   string
	body   string
}{
	{http.MethodGet, "/__test__/fake-room", ""},
	{http.MethodGet, "/__test__/fake-room/room.js", ""},
	{http.MethodGet, "/__test__/fake-room/room.css", ""},
	{http.MethodPost, "/__test__/fake-room/state", `{"capability":"x"}`},
	{http.MethodPost, "/__test__/fake-room/enter", `{"capability":"x"}`},
	{http.MethodPost, "/__test__/fake-room/leave", `{"capability":"x"}`},
	{http.MethodPost, "/__test__/fake-provider/end", `{"capability":"x"}`},
	{http.MethodPost, "/__test__/fake-provider/availability", `{"available":true}`},
	{http.MethodPost, "/__test__/settings/embed", `{"allowed":false}`},
}

func TestFakeRoomRoutesExistOnlyWithTheFlag(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		flag   string
		clock  string
		status int
	}{
		"with the flag":    {flag: "1", clock: "test", status: http.StatusOK},
		"without the flag": {flag: "0", clock: "system", status: http.StatusNotFound},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			holdProcessSlotAfterParallel(t)
			database := testdb.New(t)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			address := freeAddress(t)
			server, output := startServer(t, ctx, []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + address, "OVC_TEST_CONTROL=" + testCase.flag, "OVC_CLOCK=" + testCase.clock})
			waitForStatus(t, ctx, "http://"+address+"/api/v1/health/ready")

			answers := make([]exchange, 0, len(fakeRoomRoutes))
			for _, route := range fakeRoomRoutes {
				answers = append(answers, call(t, ctx, route.method, "http://"+address+route.path, route.body, map[string]string{"Content-Type": "application/json"}))
			}
			signalErr := server.Process.Signal(syscall.SIGTERM)
			waitErr := server.Wait()

			for index, answer := range answers {
				if answer.status != testCase.status {
					t.Fatalf("%s answered %d %s, expected %d", fakeRoomRoutes[index].path, answer.status, answer.body, testCase.status)
				}
			}
			if signalErr != nil || waitErr != nil {
				t.Fatalf("signal %v, exit %v", signalErr, waitErr)
			}
			if unanswered := answeredElsewhere(output.String(), answers...); len(unanswered) != 0 {
				t.Fatalf("another process answered the requests %v", unanswered)
			}
		})
	}
}

func TestJoinProviderFollowsTheFlag(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	logger := slog.New(slog.DiscardHandler)
	shared, err := meeting.NewSharedFake(t.Context(), database.Pool, clock.System{}, core.CryptoRandom(), logger, meeting.DefaultFakeLimits())
	if err != nil {
		t.Fatal(err)
	}
	room := domain.Meeting{ID: domain.ProviderMeetingID{1}, Name: "OVC lesson"}
	room.CreateTime, err = shared.CreateMeeting(t.Context(), room, nil)
	if err != nil {
		t.Fatal(err)
	}
	participant := domain.Participant{ID: domain.ParticipantID{2}, Name: "Participant", Role: domain.ParticipantModerator}
	cases := map[string]struct {
		settings core.Settings
		prefix   string
	}{
		"without the flag":            {core.Settings{HTTPAddr: "127.0.0.1:18099"}, "ovc-shared-fake://join/"},
		"with the flag by default":    {core.Settings{TestControl: true, HTTPAddr: "127.0.0.1:18099"}, "http://127.0.0.1:18099/__test__/fake-room#"},
		"with the flag and an origin": {core.Settings{TestControl: true, HTTPAddr: "127.0.0.1:18099", FakeRoomOrigin: "https://127.0.0.1:18900"}, "https://127.0.0.1:18900/__test__/fake-room#"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			provider, err := startup.NewJoinProvider(testCase.settings, shared)
			if err != nil {
				t.Fatal(err)
			}

			link, err := provider.JoinLink(t.Context(), room, participant)

			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(link.Reveal(), testCase.prefix) {
				t.Fatalf("the link does not start with %s", testCase.prefix)
			}
		})
	}
}

func roomStatus(t *testing.T, ctx context.Context, base string, path string, capability string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]string{"capability": capability})
	if err != nil {
		t.Fatal(err)
	}
	answer := call(t, ctx, http.MethodPost, base+path, string(encoded), map[string]string{"Content-Type": "application/json"})
	if answer.status != http.StatusOK {
		t.Fatalf("%s answered %d %s", path, answer.status, answer.body)
	}
	var view struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(answer.body), &view); err != nil {
		t.Fatal(err)
	}
	return view.Status
}

func TestFakeRoomAcrossProcesses(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	holdProcessSlotAfterParallel(t)
	database := testdb.New(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	source := clock.NewShared(database.Pool)
	if _, err := source.Set(ctx, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	provider, err := meeting.NewSharedFake(ctx, database.Pool, source, core.CryptoRandom(), logger, meeting.DefaultFakeLimits())
	if err != nil {
		t.Fatal(err)
	}
	firstAddress, secondAddress := freeAddress(t), freeAddress(t)
	first, second := "http://"+firstAddress, "http://"+secondAddress
	links, err := meeting.NewNavigableFake(provider, first)
	if err != nil {
		t.Fatal(err)
	}
	firstServer, firstOutput := startServer(t, ctx, []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + firstAddress, "OVC_TEST_CONTROL=1", "OVC_CLOCK=test"})
	secondServer, secondOutput := startServer(t, ctx, []string{"OVC_DATABASE_URL=" + database.URL.Reveal(), "OVC_HTTP_ADDR=" + secondAddress, "OVC_TEST_CONTROL=1", "OVC_CLOCK=test"})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("first server:\n%s\nsecond server:\n%s", firstOutput.String(), secondOutput.String())
		}
	})
	waitForStatus(t, ctx, first+"/api/v1/health/ready")
	waitForStatus(t, ctx, second+"/api/v1/health/ready")
	room := domain.Meeting{ID: domain.ProviderMeetingID{1}, Name: "OVC lesson"}
	room.CreateTime, err = provider.CreateMeeting(ctx, room, nil)
	if err != nil {
		t.Fatal(err)
	}
	link, err := links.JoinLink(ctx, room, domain.Participant{ID: domain.ParticipantID{2}, Name: "A student", Role: domain.ParticipantViewer})
	if err != nil {
		t.Fatal(err)
	}
	_, capability, found := strings.Cut(link.Reveal(), "#")
	if !found {
		t.Fatal("the join link has no fragment")
	}

	entered := roomStatus(t, ctx, first, "/__test__/fake-room/enter", capability)
	seenElsewhere := roomStatus(t, ctx, second, "/__test__/fake-room/state", capability)
	ended := roomStatus(t, ctx, second, "/__test__/fake-provider/end", capability)
	goneOnFirst := roomStatus(t, ctx, first, "/__test__/fake-room/state", capability)
	info, infoErr := provider.MeetingInfo(ctx, room)
	firstSignal, secondSignal := firstServer.Process.Signal(syscall.SIGTERM), secondServer.Process.Signal(syscall.SIGTERM)
	firstExit, secondExit := firstServer.Wait(), secondServer.Wait()

	if entered != "entered" || seenElsewhere != "entered" || ended != "absent" || goneOnFirst != "absent" {
		t.Fatalf("across processes the room went %s, %s, %s, %s", entered, seenElsewhere, ended, goneOnFirst)
	}
	if infoErr != nil || info.Status != domain.MeetingNotFound {
		t.Fatalf("the provider answered %+v, %v", info, infoErr)
	}
	if firstSignal != nil || secondSignal != nil || firstExit != nil || secondExit != nil {
		t.Fatalf("stopping the servers: %v %v %v %v", firstSignal, secondSignal, firstExit, secondExit)
	}
}
