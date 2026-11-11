package api_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

const (
	stalledDownloadBytes      = 12 << 20
	starvedClientReceiveBytes = 4 << 10
)

var (
	stallingDownloadTiming   = api.DownloadTiming{StallWindow: 300 * time.Millisecond, StallBytes: 16 << 10}
	errUnexpectedConnection  = errors.New("the dialled connection is not a TCP connection")
	errUnexpectedHTTPAdapter = errors.New("the test server client has no HTTP transport")
)

func newDownloadStallHarness(t *testing.T, timing api.DownloadTiming) *authHarness {
	t.Helper()
	return newAuthHarnessStoring(t, flowProxies(), api.NewHandler, func(local *teststorage.Storage) storage.Store { return local.Store },
		func(options *api.Options, _ *clock.Shared, _ *slog.Logger, _ *testdb.Database) {
			options.Downloads = timing
		})
}

type starvedDownload struct {
	response *http.Response
}

func (h *authHarness) startStarvedDownload(t *testing.T, address string, headers map[string]string) starvedDownload {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, h.server.URL+address, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := starvedClient(t, h).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	return starvedDownload{response: response}
}

func (d starvedDownload) awaitAbort(t *testing.T, h *authHarness, event string) error {
	t.Helper()
	h.awaitLogs(t, 1, `"msg":"`+event+`"`)
	_, err := io.Copy(io.Discard, d.response.Body)
	return err
}

func starvedClient(t *testing.T, h *authHarness) *http.Client {
	t.Helper()
	base, ok := h.server.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal(errUnexpectedHTTPAdapter)
	}
	transport := base.Clone()
	transport.DisableCompression = true
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		connection, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		tcp, isTCP := connection.(*net.TCPConn)
		if !isTCP {
			return nil, errors.Join(errUnexpectedConnection, connection.Close())
		}
		if err := tcp.SetReadBuffer(starvedClientReceiveBytes); err != nil {
			return nil, errors.Join(err, connection.Close())
		}
		return tcp, nil
	}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport}
}

func TestADownloadThatStopsReadingIsAbortedAndReleasesItsSlot(t *testing.T) {
	t.Parallel()
	h := newDownloadStallHarness(t, stallingDownloadTiming)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	attachment := h.boundUpload(t, parties.studentToken, direct, "notes.txt", plainText(stalledDownloadBytes), 1)
	_, address := h.addressOf(t, parties.studentToken, attachment, "")

	starved := h.startStarvedDownload(t, address.URL, nil)
	aborted := starved.awaitAbort(t, h, "files.get.stalled")

	if starved.response.StatusCode != http.StatusOK || aborted == nil {
		t.Fatalf("the starved download answered %d and ended with %v", starved.response.StatusCode, aborted)
	}
	if h.countLogs(`"msg":"files.get.timed_out"`) != 0 {
		t.Fatal("the download reached its stream deadline instead of the stall rule")
	}
	assertDownloadSlotsAvailable(t, h, attachment, address.URL)
}

func TestLongDownloadsFromOneClientAddressAreCapped(t *testing.T) {
	t.Parallel()
	h := newDownloadStallHarness(t, api.DownloadTiming{PerAddress: 1, LongAfter: 2 * time.Second})
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	large := h.boundUpload(t, parties.studentToken, direct, "notes.txt", plainText(stalledDownloadBytes), 1)
	small := h.boundUpload(t, parties.studentToken, direct, "summary.txt", plainText(64), 2)
	_, largeAddress := h.addressOf(t, parties.studentToken, large, "")
	_, smallAddress := h.addressOf(t, parties.studentToken, small, "")

	quickBeforeLong := h.fetch(t, smallAddress.URL, nil)
	first := h.startStarvedDownload(t, largeAddress.URL, nil)
	second := h.startStarvedDownload(t, largeAddress.URL, nil)
	h.awaitLogs(t, 1, `"msg":"files.get.address_busy"`)
	h.awaitLogs(t, 1, `"msg":"files.get.long_running"`)
	refusedQuick := h.fetch(t, smallAddress.URL, nil)
	h.startStarvedDownload(t, largeAddress.URL, map[string]string{"X-Forwarded-For": homeAddress})
	h.awaitLogs(t, 2, `"msg":"files.get.long_running"`)
	closed := errors.Join(first.response.Body.Close(), second.response.Body.Close())
	h.waitFor(t, "the first address to be admitted again", func() bool {
		return h.fetch(t, smallAddress.URL, nil).status == http.StatusOK
	})

	if quickBeforeLong.status != http.StatusOK || quickBeforeLong.body != string(plainText(64)) {
		t.Fatalf("a quick download answered %s", describe(quickBeforeLong))
	}
	if first.response.StatusCode != http.StatusOK || second.response.StatusCode != http.StatusOK || closed != nil {
		t.Fatalf("the two long downloads answered %d and %d; closing them: %v", first.response.StatusCode, second.response.StatusCode, closed)
	}
	if refusedQuick.status != http.StatusServiceUnavailable || refusedQuick.headers.Get("Retry-After") != "1" {
		t.Fatalf("a new download from the full address answered %s", describe(refusedQuick))
	}
	if h.countLogs(`"msg":"files.get.long_running"`) != 2 {
		t.Fatal("the long download from another address was not counted on its own")
	}
}

func TestDownloadsInFlightFromOneClientAddressAreCapped(t *testing.T) {
	t.Parallel()
	h := newDownloadStallHarness(t, api.DownloadTiming{ActivePerAddress: 1})
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	large := h.boundUpload(t, parties.studentToken, direct, "notes.txt", plainText(stalledDownloadBytes), 1)
	small := h.boundUpload(t, parties.studentToken, direct, "summary.txt", plainText(64), 2)
	_, largeAddress := h.addressOf(t, parties.studentToken, large, "")
	_, smallAddress := h.addressOf(t, parties.studentToken, small, "")

	forged := h.fetch(t, "/api/v1/files/"+small+"?t=forged", nil)
	held := h.startStarvedDownload(t, largeAddress.URL, nil)
	sameAddress := h.fetch(t, smallAddress.URL, nil)
	otherAddress := h.fetch(t, smallAddress.URL, map[string]string{"X-Forwarded-For": homeAddress})
	closed := held.response.Body.Close()
	h.waitFor(t, "the address to be admitted again", func() bool {
		return h.fetch(t, smallAddress.URL, nil).status == http.StatusOK
	})

	if forged.status != http.StatusNotFound || held.response.StatusCode != http.StatusOK {
		t.Fatalf("a refused download kept its place: the forged one answered %s and the next one %d", describe(forged), held.response.StatusCode)
	}
	if sameAddress.status != http.StatusServiceUnavailable || sameAddress.headers.Get("Retry-After") != "1" {
		t.Fatalf("a second download in flight from one address answered %s", describe(sameAddress))
	}
	if otherAddress.status != http.StatusOK || otherAddress.body != string(plainText(64)) || closed != nil {
		t.Fatalf("a download from another address answered %s; closing the held one: %v", describe(otherAddress), closed)
	}
	if h.countLogs(`"msg":"files.get.address_busy"`) < 1 {
		t.Fatal("the refused download was not recorded")
	}
}
