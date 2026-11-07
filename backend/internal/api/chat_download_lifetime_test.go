package api_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

type downloadHold struct {
	mutex     sync.Mutex
	armed     bool
	reached   chan struct{}
	ended     chan struct{}
	release   chan struct{}
	once      sync.Once
	endedOnce sync.Once
}

func (h *downloadHold) arm() {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.armed = true
}

func (h *downloadHold) take() bool {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	armed := h.armed
	h.armed = false
	return armed
}

func (h *downloadHold) resume() {
	h.once.Do(func() { close(h.release) })
}

type heldDownloadBody struct {
	remaining int64
	io.ReadCloser
	ctx  context.Context
	hold *downloadHold
	once sync.Once
}

func (b *heldDownloadBody) Read(data []byte) (int, error) {
	if b.remaining > 0 {
		n, err := b.ReadCloser.Read(data[:min(int64(len(data)), b.remaining)])
		b.remaining -= int64(n)
		return n, err
	}
	b.once.Do(func() { close(b.hold.reached) })
	select {
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	case <-b.hold.release:
		return b.ReadCloser.Read(data)
	}
}

func (b *heldDownloadBody) Close() error {
	err := b.ReadCloser.Close()
	b.hold.endedOnce.Do(func() { close(b.hold.ended) })
	return err
}

var (
	shortStreamDeadline            = files.DownloadTiming{Floor: 200 * time.Millisecond}
	shortOpeningAndStreamDeadlines = files.DownloadTiming{Open: 200 * time.Millisecond, Floor: 200 * time.Millisecond}
)

func newDownloadLifetimeHarness(t *testing.T, prefix int64, timing files.DownloadTiming) (*authHarness, *downloadHold) {
	t.Helper()
	held := &downloadHold{reached: make(chan struct{}), ended: make(chan struct{}), release: make(chan struct{})}
	var objects storage.Store
	var service *files.Service
	h := newAuthHarnessStoring(t, flowProxies(), api.NewHandler, func(local *teststorage.Storage) storage.Store {
		target, err := url.Parse(local.Settings.Endpoint)
		if err != nil {
			t.Fatal(err)
		}
		forward := httputil.NewSingleHostReverseProxy(target)
		forward.FlushInterval = -1
		forward.ErrorLog = slog.NewLogLogger(slog.DiscardHandler, slog.LevelError)
		forward.ModifyResponse = func(response *http.Response) error {
			if response.Request.Method == http.MethodGet && response.StatusCode == http.StatusOK && held.take() {
				response.Body = &heldDownloadBody{ReadCloser: response.Body, ctx: response.Request.Context(), hold: held, remaining: prefix}
			}
			return nil
		}
		proxy := httptest.NewServer(forward)
		t.Cleanup(proxy.Close)
		t.Cleanup(held.resume)
		settings := local.Settings
		settings.Endpoint = proxy.URL
		objects = storage.NewS3(settings)
		return objects
	}, func(options *api.Options, shared *clock.Shared, logger *slog.Logger, database *testdb.Database) {
		keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
		if err != nil {
			t.Fatal(err)
		}
		service = files.New(files.Dependencies{
			Store: repositories.NewStore(database.Pool), Objects: objects, Clock: shared, Random: randomPerTest(t), Keys: keys, Logger: logger,
			Downloads: timing,
		})
		options.Files = service
	})
	h.files = service
	return h, held
}

func awaitDownloadCondition(t *testing.T, condition <-chan struct{}) {
	t.Helper()
	select {
	case <-condition:
	case <-time.After(pauseDeadline):
		t.Fatal("the download did not reach its lifecycle condition")
	}
}

func TestChatDownloadDeadlineCoversStorageBody(t *testing.T) {
	t.Parallel()
	h, held := newDownloadLifetimeHarness(t, 0, shortStreamDeadline)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	attachment := h.boundUpload(t, parties.studentToken, direct, "notes.txt", plainText(1024), 1)
	_, address := h.addressOf(t, parties.studentToken, attachment, "")
	held.arm()
	result := make(chan error, 1)

	go func() {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, h.server.URL+address.URL, nil)
		if err != nil {
			result <- err
			return
		}
		response, err := h.server.Client().Do(request)
		if err != nil {
			result <- err
			return
		}
		_, err = io.Copy(io.Discard, response.Body)
		result <- errors.Join(err, response.Body.Close())
	}()
	awaitDownloadCondition(t, held.reached)
	var failure error
	select {
	case failure = <-result:
	case <-time.After(pauseDeadline):
		t.Fatal("the download ignored its stream deadline")
	}
	awaitDownloadCondition(t, held.ended)
	held.resume()
	next := h.fetch(t, address.URL, nil)

	assertDownloadSlotsAvailable(t, h, attachment, address.URL)

	if failure == nil {
		t.Fatal("the interrupted response appeared complete")
	}
	h.mustAnswer(t, next, http.StatusOK)
	if len(next.body) != 1024 {
		t.Fatal("the next download did not return the real stored body")
	}
	if !strings.Contains(h.logs.String(), `"msg":"files.get.timed_out"`) {
		t.Fatal("the download did not record its safe timeout event")
	}
}

func assertDownloadSlotsAvailable(t *testing.T, h *authHarness, attachment, address string) {
	t.Helper()
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatal("the download address did not parse")
	}
	id, valid := domain.ParseUUID[domain.AttachmentID](attachment)
	if !valid {
		t.Fatal("the attachment identifier did not parse")
	}
	var opened []files.ObjectBody
	defer func() {
		for _, body := range opened {
			if err := body.Close(); err != nil {
				t.Error("closing the admitted download failed")
			}
		}
	}()
	deadline := clock.System{}.Now().Add(pauseDeadline)
	for len(opened) < 64 {
		download, err := h.files.Download(t.Context(), id, parsed.Query().Get("t"))
		switch {
		case err == nil:
			opened = append(opened, download.Body)
		case errors.Is(err, files.ErrDownloadsBusy):
			t.Fatalf("a released download slot was unavailable with %d downloads admitted", len(opened))
		case !errors.Is(err, context.DeadlineExceeded):
			t.Fatal("a download failed before reaching its opening deadline")
		case clock.System{}.Now().After(deadline):
			t.Fatal("admitted downloads kept reaching their opening deadline")
		}
	}
	_, err = h.files.Download(t.Context(), id, parsed.Query().Get("t"))
	if !errors.Is(err, files.ErrDownloadsBusy) {
		t.Fatal("the download admission limit changed")
	}
}

func TestChatDownloadCancellationReleasesStorageBody(t *testing.T) {
	t.Parallel()
	h, held := newDownloadLifetimeHarness(t, 0, shortStreamDeadline)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	attachment := h.boundUpload(t, parties.studentToken, direct, "notes.txt", plainText(1024), 1)
	_, address := h.addressOf(t, parties.studentToken, attachment, "")
	parsed, err := url.Parse(address.URL)
	if err != nil {
		t.Fatal("the download address did not parse")
	}
	id, valid := domain.ParseUUID[domain.AttachmentID](attachment)
	if !valid {
		t.Fatal("the attachment identifier did not parse")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	held.arm()

	download, err := h.files.Download(ctx, id, parsed.Query().Get("t"))
	if err != nil {
		t.Fatal("the storage response did not open")
	}
	awaitDownloadCondition(t, held.reached)
	cancel()
	_, readErr := io.Copy(io.Discard, download.Body)
	closeErr := download.Body.Close()
	againErr := download.Body.Close()
	awaitDownloadCondition(t, held.ended)
	held.resume()
	assertDownloadSlotsAvailable(t, h, attachment, address.URL)

	if !errors.Is(readErr, context.Canceled) || closeErr != nil || againErr != nil {
		t.Fatal("cancellation or repeated close did not release the download")
	}
}

func TestChatDownloadOpeningDeadlineIncludesDatabaseWaits(t *testing.T) {
	t.Parallel()
	h, _ := newDownloadLifetimeHarness(t, 0, shortOpeningAndStreamDeadlines)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	attachment := h.boundUpload(t, parties.studentToken, direct, "notes.txt", plainText(1024), 1)
	_, address := h.addressOf(t, parties.studentToken, attachment, "")
	parsed, err := url.Parse(address.URL)
	if err != nil {
		t.Fatal("the download address did not parse")
	}
	id, valid := domain.ParseUUID[domain.AttachmentID](attachment)
	if !valid {
		t.Fatal("the attachment identifier did not parse")
	}
	held, err := h.database.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			if err := held.Rollback(context.Background()); err != nil {
				t.Error(err)
			}
		}
	})
	if _, err := held.Exec(t.Context(), "LOCK TABLE attachments IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}

	_, downloadErr := h.files.Download(t.Context(), id, parsed.Query().Get("t"))
	if err := held.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	released = true
	assertDownloadSlotsAvailable(t, h, attachment, address.URL)

	if !errors.Is(downloadErr, context.DeadlineExceeded) {
		t.Fatal("opening did not report the deadline")
	}
}

func TestChatDownloadFinalProbeHonorsCancellationAndConcurrentClose(t *testing.T) {
	t.Parallel()
	h, _ := newDownloadLifetimeHarness(t, 0, shortStreamDeadline)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	attachment := h.boundUpload(t, parties.studentToken, direct, "notes.txt", plainText(1024), 1)
	_, address := h.addressOf(t, parties.studentToken, attachment, "")
	parsed, err := url.Parse(address.URL)
	if err != nil {
		t.Fatal("the download address did not parse")
	}
	id, valid := domain.ParseUUID[domain.AttachmentID](attachment)
	if !valid {
		t.Fatal("the attachment identifier did not parse")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	download, err := h.files.Download(ctx, id, parsed.Query().Get("t"))
	if err != nil {
		t.Fatal("the storage response did not open")
	}
	if _, err := io.ReadFull(download.Body, make([]byte, 1024)); err != nil {
		t.Fatal("the real stored body could not be read")
	}

	cancel()
	_, probeErr := download.Body.Overruns()
	closed := make(chan error, 2)
	for range 2 {
		go func() { closed <- download.Body.Close() }()
	}
	firstClose, secondClose := <-closed, <-closed
	assertDownloadSlotsAvailable(t, h, attachment, address.URL)

	if !errors.Is(probeErr, context.Canceled) || firstClose != nil || secondClose != nil {
		t.Fatal("the final probe or concurrent close ignored cancellation")
	}
}

func TestChatDownloadDeadlineCancelsBlockedFinalProbe(t *testing.T) {
	t.Parallel()
	h, held := newDownloadLifetimeHarness(t, 1024, shortStreamDeadline)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	attachment := h.boundUpload(t, parties.studentToken, direct, "notes.txt", plainText(1025), 1)
	h.exec(t, "UPDATE attachments SET size_bytes = 1024 WHERE id = $1::uuid", attachment)
	_, address := h.addressOf(t, parties.studentToken, attachment, "")
	parsed, err := url.Parse(address.URL)
	if err != nil {
		t.Fatal("the download address did not parse")
	}
	id, valid := domain.ParseUUID[domain.AttachmentID](attachment)
	if !valid {
		t.Fatal("the attachment identifier did not parse")
	}
	held.arm()
	download, err := h.files.Download(t.Context(), id, parsed.Query().Get("t"))
	if err != nil {
		t.Fatal("the storage response did not open")
	}
	if _, err := io.ReadFull(download.Body, make([]byte, 1024)); err != nil {
		t.Fatal("the recorded body could not be read")
	}
	awaitDownloadCondition(t, held.reached)
	probed := make(chan error, 1)

	go func() {
		_, err := download.Body.Overruns()
		probed <- err
	}()
	var probeErr error
	select {
	case probeErr = <-probed:
	case <-time.After(pauseDeadline):
		t.Fatal("the final probe ignored its deadline")
	}
	closeErr := download.Body.Close()
	awaitDownloadCondition(t, held.ended)
	held.resume()
	assertDownloadSlotsAvailable(t, h, attachment, address.URL)

	if !errors.Is(probeErr, context.DeadlineExceeded) || closeErr != nil {
		t.Fatal("the final probe did not terminate with its deadline")
	}
}

func TestChatDownloadCloseRacesItsDeadline(t *testing.T) {
	t.Parallel()
	h, _ := newDownloadLifetimeHarness(t, 0, shortStreamDeadline)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	attachment := h.boundUpload(t, parties.studentToken, direct, "notes.txt", plainText(1024), 1)
	_, address := h.addressOf(t, parties.studentToken, attachment, "")
	parsed, err := url.Parse(address.URL)
	if err != nil {
		t.Fatal("the download address did not parse")
	}
	id, valid := domain.ParseUUID[domain.AttachmentID](attachment)
	if !valid {
		t.Fatal("the attachment identifier did not parse")
	}
	var failures []error

	for range 8 {
		download, err := h.files.Download(t.Context(), id, parsed.Query().Get("t"))
		if err != nil {
			t.Fatal("the storage response did not open")
		}
		closed := make(chan error, 1)
		timer := time.AfterFunc(time.Until(download.Deadline), func() { closed <- download.Body.Close() })
		select {
		case err := <-closed:
			failures = append(failures, err, download.Body.Close())
		case <-time.After(pauseDeadline):
			timer.Stop()
			t.Fatal("closing at the deadline did not finish")
		}
	}
	assertDownloadSlotsAvailable(t, h, attachment, address.URL)

	if errors.Join(failures...) != nil {
		t.Fatal("closing concurrently with the deadline failed")
	}
}
