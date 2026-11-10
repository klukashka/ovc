package meeting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

type storageDouble struct {
	server   *httptest.Server
	requests atomic.Int64
	cut      atomic.Int64
	status   int
	body     []byte
	streamed int64
}

func newStorageServer(t *testing.T, status int, body []byte) *storageDouble {
	t.Helper()
	return startStorageServer(t, &storageDouble{status: status, body: body})
}

func startStorageServer(t *testing.T, served *storageDouble) *storageDouble {
	t.Helper()
	served.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		served.requests.Add(1)
		writer.WriteHeader(served.status)
		_, bodyErr := writer.Write(served.body)
		_, streamErr := io.CopyN(writer, zeros{}, served.streamed)
		if bodyErr != nil || streamErr != nil {
			served.cut.Add(1)
		}
	}))
	t.Cleanup(served.server.Close)
	return served
}

func presentationAt(origin string) *Presentation {
	presentation := NewPresentation(core.NewSecret(origin + "/ovc-test/library/key?X-Amz-Signature=0123abcd"))
	return &presentation
}

func receivingFake(t *testing.T, origin string, logger *slog.Logger) *SharedFake {
	t.Helper()
	provider, err := NewSharedFakeReceivingPresentations(t.Context(), testdb.New(t).Pool, sharedClock(), sharedRandom(7), logger, testLimits(), origin)
	requireNoErrors(t, err)
	return provider
}

func TestSharedFakeReceivesPresentationFromLocalStorage(t *testing.T) {
	document := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{'7'}, 4096)...)
	local := newStorageServer(t, http.StatusOK, document)
	provider := receivingFake(t, local.server.URL, testLogger())
	room := meetingInput()

	created, createErr := provider.CreateMeeting(t.Context(), room, presentationAt(local.server.URL))
	room.CreateTime = created
	repeated, repeatErr := provider.CreateMeeting(t.Context(), room, presentationAt(local.server.URL))
	again, againErr := provider.CreateMeeting(t.Context(), meetingInput(), presentationAt(local.server.URL))
	received, found, readErr := provider.ReceivedPresentation(t.Context(), room.ID)

	requireNoErrors(t, createErr, repeatErr, againErr, readErr)
	if !found || received.Size != int64(len(document)) || received.SHA256 != sha256.Sum256(document) || repeated != created || again != created {
		t.Fatalf("received %d bytes (found %v)", received.Size, found)
	}
	if local.requests.Load() != 1 {
		t.Fatalf("the fake fetched the presentation %d times", local.requests.Load())
	}
}

func TestSharedFakeRefusesPresentationOnAnotherHost(t *testing.T) {
	local := newStorageServer(t, http.StatusOK, []byte("%PDF-1.7\n"))
	foreign := newStorageServer(t, http.StatusOK, []byte("%PDF-1.7\n"))
	provider := receivingFake(t, local.server.URL, testLogger())
	unconfigured := receivingFake(t, "", testLogger())
	addresses := []*Presentation{
		presentationAt(foreign.server.URL),
		presentationAt(strings.Replace(local.server.URL, "http://", "https://", 1)),
		presentationAt(strings.Replace(local.server.URL, "://", "://user@", 1)),
	}

	refusals := make([]error, 0, len(addresses)+1)
	for _, presentation := range addresses {
		_, err := provider.CreateMeeting(t.Context(), meetingInput(), presentation)
		refusals = append(refusals, err)
	}
	_, unconfiguredErr := unconfigured.CreateMeeting(t.Context(), meetingInput(), presentationAt(local.server.URL))
	_, _, absentErr := provider.ReceivedPresentation(t.Context(), meetingInput().ID)

	for _, err := range append(refusals, unconfiguredErr) {
		requireFailure(t, err, FailureInvalidInput)
	}
	requireFailure(t, absentErr, FailureNotFound)
	if foreign.requests.Load() != 0 || local.requests.Load() != 0 {
		t.Fatal("a refused presentation was fetched")
	}
}

func TestSharedFakeRefusesAPresentationOriginOffLoopback(t *testing.T) {
	origins := []string{"https://storage.ovc.example", "http://10.1.2.3:58333", "http://127.0.0.1:58333/bucket", "http://user@127.0.0.1:58333", "ftp://127.0.0.1:58333", "http://127.0.0.1:58333?x"}

	refusals := make([]error, 0, len(origins))
	for _, origin := range origins {
		_, err := NewSharedFakeReceivingPresentations(t.Context(), testdb.New(t).Pool, sharedClock(), sharedRandom(8), testLogger(), testLimits(), origin)
		refusals = append(refusals, err)
	}

	for _, err := range refusals {
		requireFailure(t, err, FailureInvalidInput)
	}
}

func TestSharedFakeCreatesWithoutAnUnfetchablePresentation(t *testing.T) {
	cases := map[string]*storageDouble{
		"a refused read":     newStorageServer(t, http.StatusForbidden, []byte("<Error/>")),
		"a redirect":         newStorageServer(t, http.StatusFound, nil),
		"an oversized file":  startStorageServer(t, &storageDouble{status: http.StatusOK, streamed: fakePresentationMaxBytes + 1}),
		"a successful empty": newStorageServer(t, http.StatusOK, nil),
	}
	for name, served := range cases {
		t.Run(name, func(t *testing.T) {
			var records bytes.Buffer
			provider := receivingFake(t, served.server.URL, slog.New(slog.NewJSONHandler(&records, &slog.HandlerOptions{Level: slog.LevelDebug})))

			created, createErr := provider.CreateMeeting(t.Context(), meetingInput(), presentationAt(served.server.URL))
			received, found, readErr := provider.ReceivedPresentation(t.Context(), meetingInput().ID)

			requireNoErrors(t, createErr, readErr)
			if created <= 0 {
				t.Fatal("the meeting was not created")
			}
			unfetched := strings.Contains(records.String(), `"msg":"meeting.provider.presentation_unfetched"`)
			if served.status == http.StatusOK && served.streamed == 0 {
				if !found || received.Size != 0 || unfetched {
					t.Fatal("an empty file was not recorded")
				}
				return
			}
			if found || !unfetched || strings.Contains(records.String(), "X-Amz") || strings.Contains(records.String(), served.server.URL) {
				t.Fatal("an unfetchable presentation was recorded or reported unsafely")
			}
		})
	}
}

func TestSharedFakePresentationFetchEndsWithTheCall(t *testing.T) {
	arrived := make(chan struct{}, 1)
	holding := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		arrived <- struct{}{}
		<-request.Context().Done()
	}))
	t.Cleanup(holding.Close)
	provider := receivingFake(t, holding.URL, testLogger())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)

	go func() {
		_, err := provider.CreateMeeting(ctx, meetingInput(), presentationAt(holding.URL))
		result <- err
	}()
	awaitSignal(t, arrived, "the presentation request")
	cancel()
	err := awaitResult(t, result)
	_, _, absentErr := provider.ReceivedPresentation(t.Context(), meetingInput().ID)

	requireFailure(t, err, context.Canceled)
	requireFailure(t, absentErr, FailureNotFound)
}

func TestInMemoryFakeRefusesPresentations(t *testing.T) {
	provider := newTestFake(t, testLimits())

	_, err := provider.CreateMeeting(t.Context(), meetingInput(), presentationAt("http://127.0.0.1:1"))
	_, inspectErr := provider.InspectMeeting(t.Context(), domain.ProviderMeetingID{1})

	requireFailure(t, err, FailureInvalidInput)
	requireNoErrors(t, inspectErr)
}

type zeros struct{}

func (zeros) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

const fakePresentationMigration = 37

func TestSharedFakePresentationMigration(t *testing.T) {
	database := testdb.NewMigratedTo(t, fakePresentationMigration-1)
	_, roomErr := database.Pool.Exec(t.Context(), `INSERT INTO fake_meeting_rooms (id, create_time, name) VALUES ('00000000-0000-4000-8000-000000000001', 1, 'OVC lesson')`)
	requireNoErrors(t, roomErr)

	upErr := database.MigrateUpTo(t.Context(), fakePresentationMigration)
	var unrecorded bool
	existingErr := database.Pool.QueryRow(t.Context(), `SELECT presentation_size IS NULL AND presentation_sha256 IS NULL FROM fake_meeting_rooms`).Scan(&unrecorded)
	_, sizeOnlyErr := database.Pool.Exec(t.Context(), `INSERT INTO fake_meeting_rooms (id, create_time, name, presentation_size) VALUES ('00000000-0000-4000-8000-000000000002', 2, 'OVC lesson', 10)`)
	_, shortDigestErr := database.Pool.Exec(t.Context(), `INSERT INTO fake_meeting_rooms (id, create_time, name, presentation_size, presentation_sha256) VALUES ('00000000-0000-4000-8000-000000000003', 3, 'OVC lesson', 10, '\x00')`)
	_, negativeErr := database.Pool.Exec(t.Context(), `INSERT INTO fake_meeting_rooms (id, create_time, name, presentation_size, presentation_sha256) VALUES ('00000000-0000-4000-8000-000000000004', 4, 'OVC lesson', -1, sha256('x'))`)
	_, recordedErr := database.Pool.Exec(t.Context(), `INSERT INTO fake_meeting_rooms (id, create_time, name, presentation_size, presentation_sha256) VALUES ('00000000-0000-4000-8000-000000000005', 5, 'OVC lesson', 1, sha256('x'))`)
	downErr := database.MigrateDownTo(t.Context(), fakePresentationMigration-1)
	var kept int
	keptErr := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM fake_meeting_rooms`).Scan(&kept)

	requireNoErrors(t, upErr, existingErr, recordedErr, downErr, keptErr)
	if !unrecorded || sizeOnlyErr == nil || shortDigestErr == nil || negativeErr == nil || kept != 2 {
		t.Fatalf("existing unrecorded %v, size only %v, short digest %v, negative %v, kept %d", unrecorded, sizeOnlyErr, shortDigestErr, negativeErr, kept)
	}
}
