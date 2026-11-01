//go:build slice6_measurement

package api_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"ovc/backend/internal/adapters/clock"
	"testing"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/teststorage"
)

func newLifetimeMeasurementHarness(t *testing.T) (*authHarness, chan<- *downloadHold) {
	t.Helper()
	gates := make(chan *downloadHold, 1)
	h := newAuthHarnessStoring(t, flowProxies(), api.NewHandler, func(local *teststorage.Storage) storage.Store {
		target, err := url.Parse(local.Settings.Endpoint)
		if err != nil {
			t.Fatal("measurement storage endpoint invalid")
		}
		forward := httputil.NewSingleHostReverseProxy(target)
		forward.FlushInterval = -1
		forward.ErrorLog = slog.NewLogLogger(slog.DiscardHandler, slog.LevelError)
		forward.ModifyResponse = func(response *http.Response) error {
			if response.Request.Method == http.MethodGet && response.StatusCode == http.StatusOK {
				select {
				case held := <-gates:
					response.Body = &heldDownloadBody{ReadCloser: response.Body, ctx: response.Request.Context(), hold: held, remaining: 64 << 10}
				default:
				}
			}
			return nil
		}
		proxy := httptest.NewServer(forward)
		t.Cleanup(proxy.Close)
		settings := local.Settings
		settings.Endpoint = proxy.URL
		return storage.NewS3(settings)
	})
	return h, gates
}

func measurementPoolQuiescent(t *testing.T, h *authHarness) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for h.database.Pool.Stat().AcquiredConns() != 0 {
		select {
		case <-deadline.C:
			t.Fatal("measurement pool checkouts did not drain")
		case <-poll.C:
		}
	}
}

func measurementDownloadCycle(t *testing.T, h *authHarness, gates chan<- *downloadHold, address string, mode string) {
	t.Helper()
	if mode == "success" || mode == "refusal" {
		path := address
		expected := http.StatusOK
		if mode == "refusal" {
			parsed, err := url.Parse(address)
			if err != nil {
				t.Fatal("measurement address invalid")
			}
			parsed.RawQuery = "t=invalid"
			path, expected = parsed.String(), http.StatusNotFound
		}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, h.server.URL+path, nil)
		if err != nil {
			t.Fatal("measurement download request invalid")
		}
		response, err := h.server.Client().Do(request)
		if err != nil {
			t.Fatal("measurement download failed")
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil || response.StatusCode != expected {
			t.Fatal("measurement download did not finish with the expected outcome")
		}
		return
	}
	held := &downloadHold{reached: make(chan struct{}), ended: make(chan struct{}), release: make(chan struct{})}
	gates <- held
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	host := h.server
	if mode == "shutdown" {
		host = httptest.NewServer(h.server.Config.Handler)
	}
	var response *http.Response
	defer func() {
		cancel()
		held.resume()
		if response != nil {
			if err := response.Body.Close(); err != nil {
				t.Error("measurement response cleanup failed")
			}
		}
		if mode == "shutdown" {
			host.Close()
		}
	}()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL+address, nil)
	if err != nil {
		t.Fatal("measurement held request invalid")
	}
	response, err = host.Client().Do(request)
	if err != nil {
		t.Fatal("measurement held download did not open")
	}
	awaitDownloadCondition(t, held.reached)
	measurementResources(t, h, "held_download_peak")
	switch mode {
	case "cancel":
		cancel()
	case "shutdown":
		host.CloseClientConnections()
	case "disconnect":
	default:
		t.Fatal("measurement lifecycle mode invalid")
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal("measurement response close failed")
	}
	response = nil
	awaitDownloadCondition(t, held.ended)
}

func TestSlice6DownloadLifetimeMeasurement(t *testing.T) {
	h, gates := newLifetimeMeasurementHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	attachment := h.boundUpload(t, parties.studentToken, channel, "measurement.txt", plainText(2_000_000), 1)
	answer, address := h.addressOf(t, parties.studentToken, attachment, "")
	h.mustAnswer(t, answer, http.StatusOK)
	measurementResources(t, h, "baseline")

	for cycle := range measurementCount(100, 2) {
		for _, mode := range []string{"success", "refusal", "cancel", "disconnect", "shutdown"} {
			started := (clock.System{}).Now()
			measurementDownloadCycle(t, h, gates, address.URL, mode)
			measurementPoolQuiescent(t, h)
			t.Logf("event=measurement.lifecycle cycle=%d mode=%s duration_ns=%d", cycle, mode, time.Since(started).Nanoseconds())
			measurementResources(t, h, fmt.Sprintf("quiescent_%d_%s", cycle, mode))
		}
	}

	assertDownloadSlotsAvailable(t, h, attachment, address.URL)
	measurementResources(t, h, "complete")
}

type measurementUploadActor struct {
	user    domain.UserID
	token   string
	channel string
}

func measurementUploadCleanup(t *testing.T, h *authHarness, actor measurementUploadActor, captured []storedUpload) {
	t.Helper()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(25 * time.Millisecond)
	defer poll.Stop()
	for {
		h.tick(t)
		remaining := h.countRows(t, "SELECT count(*) FROM attachments WHERE uploader_id=$1::uuid AND state IN ('uploading','pending','bound')", actor.user.String())
		intents := h.countRows(t, "SELECT count(*) FROM upload_intents WHERE storage_key LIKE $1", "chat/%")
		var capacity, receipts int64
		if err := h.database.Pool.QueryRow(t.Context(), "SELECT (SELECT used FROM upload_intent_capacity), (SELECT count(*) FROM upload_cleanup_receipts)").Scan(&capacity, &receipts); err != nil {
			t.Fatal("measurement upload resource observation failed")
		}
		if remaining == 0 && intents == 0 && capacity == 0 && receipts == 0 {
			for _, upload := range append(captured, h.uploadsBy(t, actor.user)...) {
				if len(h.incompleteUploads(t, upload.Key)) != 0 {
					t.Fatal("measurement upload retained multipart state")
				}
				if _, found := h.storedObject(t, upload.Key); found {
					t.Fatal("measurement upload retained an object")
				}
			}
			measurementPoolQuiescent(t, h)
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("measurement upload cleanup deadline exceeded")
		case <-poll.C:
		}
	}
}

func TestSlice6UploadLifetimeMeasurement(t *testing.T) {
	h := newUploadHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	actors := make([]measurementUploadActor, 0, 10)
	for index := range 10 {
		email := fmt.Sprintf("measurement-upload-%d@example.org", index)
		user, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, byte(index+2), email)
		card := h.linkedSignedIn(t, tutor, token, "Synthetic student", email)
		actors = append(actors, measurementUploadActor{user: user, token: token, channel: h.directChannel(t, card.ID).ID})
	}
	measurementResources(t, h, "baseline")

	for cycle := range measurementCount(100, 2) {
		actor := actors[cycle%len(actors)]
		for _, mode := range []string{"success", "refusal", "disconnect"} {
			started := (clock.System{}).Now()
			var captured []storedUpload
			var disconnected *storedUpload
			switch mode {
			case "success":
				answer := h.chatUpload(t, actor.token, actor.channel, "measurement.txt", plainText(1024))
				h.mustAnswer(t, answer, http.StatusCreated)
				captured = h.uploadsBy(t, actor.user)
				if len(captured) == 0 {
					t.Fatal("measurement successful upload has no captured storage key")
				}
				message := h.sent(t, actor.token, actor.channel, "Synthetic upload", uploadedID(t, answer))
				h.mustAnswer(t, h.deleteMessage(t, actor.token, message.ID), http.StatusNoContent)
			case "refusal":
				answer := h.chatUpload(t, actor.token, actor.channel, "measurement.exe", plainText(1024))
				h.mustAnswer(t, answer, http.StatusUnsupportedMediaType)
			case "disconnect":
				upload := h.startChatUpload(t, actor.token, actor.channel)
				upload.begin(t, "measurement.txt", plainText(1024))
				active := h.awaitUploading(t, actor.user)
				captured = append(captured, active)
				disconnected = &active
				measurementResources(t, h, "upload_in_flight")
				if err := upload.pipe.CloseWithError(context.Canceled); err != nil {
					t.Fatal("measurement upload disconnection failed")
				}
				outcome := upload.outcome(t)
				if outcome.err == nil && outcome.result.status != http.StatusBadRequest {
					t.Fatal("measurement disconnected upload appeared successful")
				}
			}
			measurementUploadCleanup(t, h, actor, captured)
			if disconnected != nil {
				h.assertLeftNothing(t, *disconnected)
			}
			t.Logf("event=measurement.upload_lifecycle cycle=%d mode=%s duration_ns=%d", cycle, mode, time.Since(started).Nanoseconds())
			measurementResources(t, h, fmt.Sprintf("upload_quiescent_%d_%s", cycle, mode))
		}
	}

	var completedBytes int64
	if err := h.database.Pool.QueryRow(t.Context(), "SELECT coalesce(sum(completed_bytes), 0)::bigint FROM daily_chat_upload_bytes").Scan(&completedBytes); err != nil {
		t.Fatal("measurement daily accounting total unavailable")
	}
	if completedBytes != int64(measurementCount(100, 2)*1024) {
		t.Fatal("measurement successful uploads lost durable daily accounting")
	}
	if h.usage(t, domain.UserID{1}) != 0 {
		t.Fatal("measurement upload cycles retained charged quota")
	}
	measurementResources(t, h, "complete")
}
