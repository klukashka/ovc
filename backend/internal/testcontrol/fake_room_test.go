package testcontrol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/testcontrol"
	"ovc/backend/internal/testdb"
)

const (
	fakeRoomOrigin    = "http://127.0.0.1:18099"
	framingOrigin     = "https://127.0.0.1:4399"
	hostileName       = `<img src=x onerror="window.hostile=1">Анна`
	tutorDisplayName  = "Moderator display sentinel"
	neutralRoomName   = "OVC lesson"
	jsonContentType   = "application/json"
	expectedRoomCSP   = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors " + framingOrigin
	statePath         = meeting.FakeRoomPath + "/state"
	enterPath         = meeting.FakeRoomPath + "/enter"
	leavePath         = meeting.FakeRoomPath + "/leave"
	endPath           = "/__test__/fake-provider/end"
	availabilityPath  = "/__test__/fake-provider/availability"
	embedPath         = "/__test__/settings/embed"
	maximumProbeBytes = 8 * 1024
)

type safeLog struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (l *safeLog) Write(data []byte) (int, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.buffer.Write(data)
}

func (l *safeLog) String() string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.buffer.String()
}

type roomFixture struct {
	server   *httptest.Server
	database *testdb.Database
	shared   *meeting.SharedFake
	links    *meeting.NavigableFake
	embed    *core.Switch
	log      *safeLog
}

func newRoomFixture(t *testing.T) *roomFixture {
	t.Helper()
	database := testdb.New(t)
	log := &safeLog{}
	logger := slog.New(slog.NewJSONHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	source := clock.NewShared(database.Pool)
	if _, err := source.Set(t.Context(), time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	shared, err := meeting.NewSharedFake(t.Context(), database.Pool, source, core.CryptoRandom(), logger, meeting.DefaultFakeLimits())
	if err != nil {
		t.Fatal(err)
	}
	links, err := meeting.NewNavigableFake(shared, fakeRoomOrigin)
	if err != nil {
		t.Fatal(err)
	}
	embed := &core.Switch{}
	routes := testcontrol.New(testcontrol.Dependencies{Logger: logger, Room: shared, Embed: embed, FramingOrigin: framingOrigin})
	handler, err := api.NewHandler(api.Options{
		Logger: logger, Now: clock.System{}.Now, NewRequestID: core.NewRandomID,
		Readiness: func(context.Context) error { return nil }, Mount: routes.Mount,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &roomFixture{server: server, database: database, shared: shared, links: links, embed: embed, log: log}
}

func (f *roomFixture) room(t *testing.T, id byte) domain.Meeting {
	t.Helper()
	requested := domain.Meeting{ID: domain.ProviderMeetingID{id}, Name: neutralRoomName}
	created, err := f.shared.CreateMeeting(t.Context(), requested)
	if err != nil {
		t.Fatal(err)
	}
	requested.CreateTime = created
	return requested
}

func (f *roomFixture) capability(t *testing.T, room domain.Meeting, participant domain.Participant) string {
	t.Helper()
	link, err := f.links.JoinLink(t.Context(), room, participant)
	if err != nil {
		t.Fatal(err)
	}
	prefix := fakeRoomOrigin + meeting.FakeRoomPath + "#"
	if !strings.HasPrefix(link.Reveal(), prefix) {
		t.Fatal("the join link does not open the configured room origin")
	}
	return strings.TrimPrefix(link.Reveal(), prefix)
}

func (f *roomFixture) participants(t *testing.T) int64 {
	t.Helper()
	var count int64
	if err := f.database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM fake_meeting_participants`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (f *roomFixture) presentParticipants(t *testing.T) int64 {
	t.Helper()
	var count int64
	if err := f.database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM fake_meeting_participants WHERE present`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func moderator(id byte) domain.Participant {
	return domain.Participant{ID: domain.ParticipantID{id}, Name: tutorDisplayName, Role: domain.ParticipantModerator}
}

func viewer(id byte, name string) domain.Participant {
	return domain.Participant{ID: domain.ParticipantID{id}, Name: name, Role: domain.ParticipantViewer}
}

func roomBody(t *testing.T, capability string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]string{"capability": capability})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func (f *roomFixture) exchange(t *testing.T, path string, capability string) map[string]string {
	t.Helper()
	status, body := post(t, f.server, path, jsonContentType, "", roomBody(t, capability))
	if status != http.StatusOK {
		t.Fatalf("%s answered %d %s", path, status, body)
	}
	var view map[string]string
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

type fetched struct {
	status int
	header http.Header
}

func get(t *testing.T, server *httptest.Server, path string, host string) (fetched, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		request.Host = host
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	text, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	return fetched{status: response.StatusCode, header: response.Header}, string(text)
}

func expectView(t *testing.T, got map[string]string, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("the room answered %v, expected %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("the room answered %v, expected %v", got, want)
		}
	}
}

func entered(name string, role domain.ParticipantRole) map[string]string {
	return map[string]string{"status": "entered", "room_name": neutralRoomName, "participant_name": name, "role": string(role)}
}

func absent() map[string]string {
	return map[string]string{"status": "absent"}
}

func TestFakeRoomContract(t *testing.T) {
	t.Run("the page and its assets carry the room headers", testRoomHeaders)
	t.Run("each capability shows only its own participant", testRoomIdentity)
	t.Run("a hostile name stays data", testRoomHostileName)
	t.Run("a changed capability is refused without any change", testRoomChangedCapability)
	t.Run("a stale generation is refused and its name released", testRoomStaleGeneration)
	t.Run("leaving and entering change only presence", testRoomPresence)
	t.Run("a rejoin keeps the role and shows the current name", testRoomRejoinName)
	t.Run("the availability control refuses every room operation", testRoomAvailability)
	t.Run("requests are bounded and typed", testRoomRequestBounds)
	t.Run("the embed override applies until the next override", testRoomEmbedOverride)
	t.Run("no recording control or callback bypass exists", testRoomNoRecordings)
	t.Run("credentials and names stay out of diagnostics", testRoomDiagnostics)
	t.Run("a cancelled request is no failure in diagnostics", testRoomCancelled)
	t.Run("a cross-origin preflight is not allowed", testRoomPreflight)
}

func testRoomHeaders(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	want := map[string]string{
		meeting.FakeRoomPath:               "text/html; charset=utf-8",
		meeting.FakeRoomPath + "/room.js":  "text/javascript; charset=utf-8",
		meeting.FakeRoomPath + "/room.css": "text/css; charset=utf-8",
	}

	for path, contentType := range want {
		response, body := get(t, f.server, path, "")

		if response.status != http.StatusOK || response.header.Get("Content-Type") != contentType || body == "" {
			t.Fatalf("%s answered %d %s", path, response.status, response.header.Get("Content-Type"))
		}
		if response.header.Get("Cache-Control") != "no-store" || response.header.Get("Referrer-Policy") != "no-referrer" ||
			response.header.Get("X-Content-Type-Options") != "nosniff" || response.header.Get("Content-Security-Policy") != expectedRoomCSP {
			t.Fatalf("%s carries the headers %v", path, response.header)
		}
		if response.header.Get("Access-Control-Allow-Origin") != "" || response.header.Get("X-Frame-Options") != "" {
			t.Fatalf("%s carries the headers %v", path, response.header)
		}
	}
}

func testRoomIdentity(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	room := f.room(t, 1)
	tutor := f.capability(t, room, moderator(1))
	student := f.capability(t, room, viewer(2, "Student display sentinel"))

	tutorView := f.exchange(t, enterPath, tutor)
	studentView := f.exchange(t, enterPath, student)

	expectView(t, tutorView, entered(tutorDisplayName, domain.ParticipantModerator))
	expectView(t, studentView, entered("Student display sentinel", domain.ParticipantViewer))
}

func testRoomHostileName(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	capability := f.capability(t, f.room(t, 1), viewer(2, hostileName))

	view := f.exchange(t, enterPath, capability)
	_, page := get(t, f.server, meeting.FakeRoomPath, "")
	_, script := get(t, f.server, meeting.FakeRoomPath+"/room.js", "")

	expectView(t, view, entered(hostileName, domain.ParticipantViewer))
	if strings.Contains(page, "onerror") || strings.Contains(script, "innerHTML") || strings.Contains(script, "insertAdjacentHTML") {
		t.Fatal("the page may render a name as markup")
	}
}

func testRoomChangedCapability(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	capability := f.capability(t, f.room(t, 1), moderator(1))
	parts := strings.Split(capability, "/")
	flippedNonce := strings.Repeat("0", 64)
	if parts[2] == flippedNonce {
		flippedNonce = strings.Repeat("1", 64)
	}
	sequence, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	changed := map[string]string{
		"nothing":           "",
		"another nonce":     parts[0] + "/" + parts[1] + "/" + flippedNonce,
		"another sequence":  parts[0] + "/" + strconv.FormatInt(sequence+1, 10) + "/" + parts[2],
		"another namespace": strings.Repeat("a", 64) + "/" + parts[1] + "/" + parts[2],
		"a padded sequence": parts[0] + "/0" + parts[1] + "/" + parts[2],
		"a suffix":          capability + "/x",
		"a full address":    "ovc-shared-fake://join/" + capability,
		"a caller URL":      "https://attacker.example/" + capability,
		"upper case":        strings.ToUpper(capability),
	}

	for name, attempt := range changed {
		view := f.exchange(t, enterPath, attempt)
		ended := f.exchange(t, endPath, attempt)

		expectView(t, view, absent())
		expectView(t, ended, absent())
		if f.presentParticipants(t) != 0 || f.participants(t) != 1 {
			t.Fatalf("%s changed the room", name)
		}
	}
}

func testRoomStaleGeneration(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	first := f.room(t, 1)
	stale := f.capability(t, first, viewer(2, "Released name sentinel"))
	f.exchange(t, enterPath, stale)
	if err := f.shared.EndMeeting(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	replacement := f.room(t, 1)
	current := f.capability(t, replacement, viewer(2, "Current name"))

	staleEntry := f.exchange(t, enterPath, stale)
	staleState := f.exchange(t, statePath, stale)
	currentState := f.exchange(t, statePath, current)

	expectView(t, staleEntry, absent())
	expectView(t, staleState, absent())
	if currentState["status"] != "left" || currentState["participant_name"] != "Current name" {
		t.Fatalf("the replacement answered %v", currentState)
	}
	var released int64
	if err := f.database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM fake_meeting_participants WHERE display_name = 'Released name sentinel'`).Scan(&released); err != nil {
		t.Fatal(err)
	}
	if released != 0 {
		t.Fatal("ending the meeting kept the display name")
	}
}

func testRoomPresence(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	room := f.room(t, 1)
	capability := f.capability(t, room, moderator(1))

	before := f.exchange(t, statePath, capability)
	inside := f.exchange(t, enterPath, capability)
	again := f.exchange(t, enterPath, capability)
	info, err := f.shared.MeetingInfo(t.Context(), room)
	if err != nil {
		t.Fatal(err)
	}
	outside := f.exchange(t, leavePath, capability)
	after := f.exchange(t, statePath, capability)
	ended := f.exchange(t, endPath, capability)
	gone := f.exchange(t, statePath, capability)

	if before["status"] != "left" || inside["status"] != "entered" || again["status"] != "entered" || outside["status"] != "left" || after["status"] != "left" {
		t.Fatalf("presence went %v %v %v %v %v", before, inside, again, outside, after)
	}
	if info.Status != domain.MeetingRunning || info.Participants != 1 {
		t.Fatalf("the provider saw %+v", info)
	}
	expectView(t, ended, absent())
	expectView(t, gone, absent())
}

func testRoomRejoinName(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	room := f.room(t, 1)
	first := f.capability(t, room, viewer(2, "Earlier name"))
	second := f.capability(t, room, viewer(2, "Later name"))

	firstView := f.exchange(t, enterPath, first)
	secondView := f.exchange(t, enterPath, second)
	_, refused := f.links.JoinLink(t.Context(), room, domain.Participant{ID: domain.ParticipantID{2}, Name: "Later name", Role: domain.ParticipantModerator})

	expectView(t, firstView, entered("Later name", domain.ParticipantViewer))
	expectView(t, secondView, entered("Later name", domain.ParticipantViewer))
	if !errors.Is(refused, meeting.FailureConflict) {
		t.Fatalf("a changed role answered %v", refused)
	}
}

func testRoomAvailability(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	capability := f.capability(t, f.room(t, 1), moderator(1))

	downStatus, down := post(t, f.server, availabilityPath, jsonContentType, "", `{"available":false}`)
	view := f.exchange(t, enterPath, capability)
	end := f.exchange(t, endPath, capability)
	upStatus, up := post(t, f.server, availabilityPath, jsonContentType, "", `{"available":true}`)
	recovered := f.exchange(t, enterPath, capability)

	if downStatus != http.StatusOK || down != `{"available":false}` || upStatus != http.StatusOK || up != `{"available":true}` {
		t.Fatalf("availability answered %d %s and %d %s", downStatus, down, upStatus, up)
	}
	expectView(t, view, map[string]string{"status": "unavailable"})
	expectView(t, end, map[string]string{"status": "unavailable"})
	expectView(t, recovered, entered(tutorDisplayName, domain.ParticipantModerator))
}

func testRoomRequestBounds(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	capability := f.capability(t, f.room(t, 1), moderator(1))
	cases := []struct {
		name        string
		path        string
		contentType string
		host        string
		body        string
		status      int
	}{
		{"a form post", enterPath, "text/plain", "", roomBody(t, capability), http.StatusUnsupportedMediaType},
		{"an unknown field", enterPath, jsonContentType, "", `{"capability":"x","url":"https://attacker.example"}`, http.StatusBadRequest},
		{"trailing data", enterPath, jsonContentType, "", `{"capability":"x"}{}`, http.StatusBadRequest},
		{"an oversized body", enterPath, jsonContentType, "", `{"capability":"` + strings.Repeat("a", maximumProbeBytes) + `"}`, http.StatusRequestEntityTooLarge},
		{"a foreign host", enterPath, jsonContentType, "attacker.example", roomBody(t, capability), http.StatusNotFound},
		{"an unset availability", availabilityPath, jsonContentType, "", `{}`, http.StatusUnprocessableEntity},
		{"an unset embed override", embedPath, jsonContentType, "", `{}`, http.StatusUnprocessableEntity},
		{"a foreign host on the end control", endPath, jsonContentType, "attacker.example", roomBody(t, capability), http.StatusNotFound},
	}

	for _, testCase := range cases {
		status, body := post(t, f.server, testCase.path, testCase.contentType, testCase.host, testCase.body)

		if status != testCase.status {
			t.Fatalf("%s answered %d %s, expected %d", testCase.name, status, body, testCase.status)
		}
	}
	for _, path := range []string{meeting.FakeRoomPath, meeting.FakeRoomPath + "/room.js", meeting.FakeRoomPath + "/room.css"} {
		page, _ := get(t, f.server, path, "attacker.example")
		if page.status != http.StatusNotFound {
			t.Fatalf("a foreign host read %s with %d", path, page.status)
		}
	}
	for _, path := range []string{statePath, enterPath, leavePath, endPath, availabilityPath, embedPath} {
		foreign, foreignBody := post(t, f.server, path, jsonContentType, "attacker.example", `{}`)
		plain, plainBody := post(t, f.server, path, "text/plain", "", `{}`)
		if foreign != http.StatusNotFound || plain != http.StatusUnsupportedMediaType {
			t.Fatalf("%s answered a foreign host %d %s and plain text %d %s", path, foreign, foreignBody, plain, plainBody)
		}
	}
	if f.presentParticipants(t) != 0 {
		t.Fatal("a refused request entered the room")
	}
}

func testRoomEmbedOverride(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	initially := f.embed.On()

	onStatus, on := post(t, f.server, embedPath, jsonContentType, "", `{"allowed":true}`)
	whileOn := f.embed.On()
	offStatus, off := post(t, f.server, embedPath, jsonContentType, "", `{"allowed":false}`)
	afterwards := f.embed.On()

	if initially || !whileOn || afterwards || onStatus != http.StatusOK || on != `{"allowed":true}` || offStatus != http.StatusOK || off != `{"allowed":false}` {
		t.Fatalf("the override went %v %v %v, answered %d %s, %d %s", initially, whileOn, afterwards, onStatus, on, offStatus, off)
	}
}

func testRoomNoRecordings(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	capability := f.capability(t, f.room(t, 1), moderator(1))
	lesson := "00000000-0000-4000-8000-000000000001"

	view := f.exchange(t, enterPath, capability)
	recordingStatus, _ := post(t, f.server, "/__test__/fake-provider/lessons/"+lesson+"/recording", jsonContentType, "", `{}`)
	readyStatus, _ := post(t, f.server, "/__test__/fake-provider/lessons/"+lesson+"/ready", jsonContentType, "", `{}`)
	_, page := get(t, f.server, meeting.FakeRoomPath, "")
	_, script := get(t, f.server, meeting.FakeRoomPath+"/room.js", "")

	expectView(t, view, entered(tutorDisplayName, domain.ParticipantModerator))
	if recordingStatus != http.StatusNotFound || readyStatus != http.StatusNotFound {
		t.Fatalf("recording controls answered %d and %d", recordingStatus, readyStatus)
	}
	if strings.Contains(strings.ToLower(page+script), "record") {
		t.Fatal("the room mentions recordings")
	}
}

func testRoomDiagnostics(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	capability := f.capability(t, f.room(t, 1), viewer(2, "Diagnostics name sentinel"))
	nonce := capability[strings.LastIndex(capability, "/")+1:]

	f.exchange(t, enterPath, capability)
	f.exchange(t, statePath, capability)
	f.exchange(t, leavePath, capability)
	f.exchange(t, enterPath, "not a capability")
	f.exchange(t, endPath, capability)

	logged := f.log.String()
	if logged == "" || strings.Contains(logged, nonce) || strings.Contains(logged, "Diagnostics name sentinel") || strings.Contains(logged, meeting.FakeRoomPath+"#") {
		t.Fatalf("the diagnostics hold a credential or a name:\n%s", logged)
	}
	if !strings.Contains(logged, `"route":"POST `+enterPath+`"`) || !strings.Contains(logged, `"operation":"room_view"`) {
		t.Fatalf("the diagnostics miss the room's events:\n%s", logged)
	}
}

func testRoomPreflight(t *testing.T) {
	t.Parallel()
	f := newRoomFixture(t)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodOptions, f.server.URL+enterPath, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "content-type")

	answer, err := f.server.Client().Do(request)

	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := answer.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	if answer.StatusCode == http.StatusOK || answer.Header.Get("Access-Control-Allow-Origin") != "" || answer.Header.Get("Access-Control-Allow-Headers") != "" {
		t.Fatalf("the preflight answered %d with %v", answer.StatusCode, answer.Header)
	}
}

func testRoomCancelled(t *testing.T) {
	t.Parallel()
	fixture := newRoomFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, statePath, strings.NewReader(roomBody(t, "x")))
	request.Host = "127.0.0.1"
	request.Header.Set("Content-Type", jsonContentType)

	fixture.server.Config.Handler.ServeHTTP(httptest.NewRecorder(), request)

	diagnostics := fixture.log.String()
	if !strings.Contains(diagnostics, `"kind":"cancelled"`) {
		t.Fatalf("the provider did not report the cancellation: %s", diagnostics)
	}
	if strings.Contains(diagnostics, "testcontrol.failed") || strings.Contains(diagnostics, `"level":"ERROR"`) {
		t.Fatalf("a cancelled request was reported as a failure: %s", diagnostics)
	}
}

func TestNavigableFakeOrigin(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	shared, err := meeting.NewSharedFake(t.Context(), database.Pool, clock.NewShared(database.Pool), core.CryptoRandom(), slog.New(slog.DiscardHandler), meeting.DefaultFakeLimits())
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"http://127.0.0.1:8000":        true,
		"https://127.0.0.1:8000":       true,
		"http://[::1]:8000":            true,
		"http://127.0.0.1":             false,
		"http://127.0.0.1:0":           false,
		"http://localhost:8000":        false,
		"http://192.0.2.1:8000":        false,
		"https://attacker.example:443": false,
		"http://127.0.0.1:8000/room":   false,
		"http://127.0.0.1:8000?x=1":    false,
		"http://user@127.0.0.1:8000":   false,
		"ftp://127.0.0.1:8000":         false,
		"http://127.0.0.1:8000?":       false,
		"http://127.0.0.1:8000#":       false,
		"http://127.0.0.1:99999":       false,
		"http://127.0.0.2:8000":        false,
		"http://[::1%25en0]:8000":      false,
		"":                             false,
	}

	for origin, accepted := range cases {
		_, err := meeting.NewNavigableFake(shared, origin)

		if (err == nil) != accepted {
			t.Fatalf("the origin %q answered %v, expected accepted %v", origin, err, accepted)
		}
	}
}
