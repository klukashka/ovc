package api_test

import (
	"context"
	"crypto/sha256"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/meetings"
)

const (
	presentationOffered     = `"msg":"meeting.presentation.offered"`
	presentationUnavailable = `"level":"WARN","msg":"meeting.presentation.unavailable"`
	shortProviderCall       = time.Second
)

var signedQueryMarkers = []string{"X-Amz-Signature", "X-Amz-Credential", "X-Amz-Algorithm", "X-Amz-Date", "X-Amz-Expires", "X-Amz-SignedHeaders"}

var storedRowQueries = []string{
	"SELECT count(id) FROM outbox WHERE strpos(outbox::text, $1) > 0",
	"SELECT count(id) FROM meetings WHERE strpos(meetings::text, $1) > 0",
	"SELECT count(provider_meeting_id) FROM meeting_recovery WHERE strpos(meeting_recovery::text, $1) > 0",
	"SELECT count(outbox_id) FROM meeting_recovery_cycles WHERE strpos(meeting_recovery_cycles::text, $1) > 0",
	"SELECT count(id) FROM fake_meeting_rooms WHERE strpos(fake_meeting_rooms::text, $1) > 0",
}

type presentedLesson struct {
	lesson       string
	presentation []byte
	attachment   string
	storageKey   string
}

func (h *authHarness) lessonWithMaterials(t *testing.T, parties visibilityParties, files map[string][]byte, order []string) (string, map[string]libraryItemBody) {
	t.Helper()
	lesson := oneToOneLesson(t, h, parties)
	materials := make(map[string]libraryItemBody, len(order))
	ids := make([]string, 0, len(order))
	for _, filename := range order {
		upload := h.libraryUpload(t, parties.tutorToken, domain.ContextLibrary, filename, files[filename])
		h.mustAnswer(t, upload, http.StatusCreated)
		material := h.createMaterial(t, parties.tutorToken, uploadedID(t, upload), filename, nil)
		materials[filename] = material
		ids = append(ids, material.ID)
	}
	h.attached(t, parties.tutorToken, lesson, 1, ids...)
	return lesson, materials
}

func (h *authHarness) lessonWithPresentation(t *testing.T, parties visibilityParties) presentedLesson {
	t.Helper()
	files := map[string][]byte{"dialogue.mp3": mp3File(), "first.pdf": pdfFile(700), "second.pdf": pdfFile(300)}
	lesson, materials := h.lessonWithMaterials(t, parties, files, []string{"dialogue.mp3", "first.pdf", "second.pdf"})
	presented := presentedLesson{lesson: lesson, presentation: files["first.pdf"], attachment: materials["first.pdf"].AttachmentID}
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT storage_key FROM attachments WHERE id = $1::uuid", presented.attachment).Scan(&presented.storageKey); err != nil {
		t.Fatal(err)
	}
	return presented
}

func (h *authHarness) presentationInstance(t *testing.T, signer meetings.PresentationSigner, providerCall time.Duration) string {
	t.Helper()
	timing := meetings.DefaultRoomTiming()
	timing.ProviderCall = providerCall
	rooms := meetings.NewRooms(meetings.RoomDependencies{
		Store: h.store, Provider: h.provider, Presentations: signer, Kind: domain.MeetingProviderFake, Clock: h.clock, Random: h.random, Logger: h.logger,
		Monotonic: clock.System{}.Now, Timing: timing,
	})
	options := h.options
	options.Meetings = meetings.NewLifecycle(meetings.LifecycleDependencies{
		Rooms: rooms, Store: h.store, Provider: h.provider, Clock: h.clock, Random: h.random, Keys: h.keys, Logger: h.logger,
		Enabled: true, ProviderCall: timing.ProviderCall,
	})
	return h.serve(t, options)
}

func (h *authHarness) unsignableStorage() *storage.Presigner {
	return storage.NewPresigner(core.StorageSettings{Backend: core.StorageBackendS3, Region: h.storage.Region, Bucket: h.storage.Bucket}, h.storage.Endpoint)
}

func (h *authHarness) receivedPresentation(t *testing.T, lesson string) (int64, [sha256.Size]byte, bool) {
	t.Helper()
	room := h.currentRoom(t, lesson)
	received, found, err := h.provider.ReceivedPresentation(context.Background(), room.ID)
	if err != nil {
		t.Fatal(err)
	}
	return received.Size, received.SHA256, found
}

func (h *authHarness) expectNoPresentationAddress(t *testing.T, what string, presented presentedLesson, answers ...response) {
	t.Helper()
	private := append([]string{presented.storageKey}, signedQueryMarkers...)
	for _, marker := range private {
		if strings.Contains(h.logs.String(), marker) {
			t.Fatalf("%s: the logs hold %q", what, marker)
		}
		for _, answer := range answers {
			if strings.Contains(answer.body, marker) || strings.Contains(strings.Join(answer.headers.Values("Location"), " "), marker) {
				t.Fatalf("%s: an answer holds %q", what, marker)
			}
		}
		for _, query := range storedRowQueries {
			if h.countRows(t, query, marker) != 0 {
				t.Fatalf("%s: a stored row holds %q", what, marker)
			}
		}
	}
}

func TestFakeMeetingReceivesPresentation(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	presented := h.lessonWithPresentation(t, parties)

	h.joined(t, parties.tutorToken, presented.lesson)

	size, digest, found := h.receivedPresentation(t, presented.lesson)
	if !found || size != int64(len(presented.presentation)) || digest != sha256.Sum256(presented.presentation) {
		t.Fatalf("the fake received %d bytes (found %v) instead of the first PDF", size, found)
	}
	if !strings.Contains(h.logs.String(), presentationOffered) || !strings.Contains(h.logs.String(), `"attachment_id":"`+presented.attachment+`"`) ||
		strings.Contains(h.logs.String(), presentationUnavailable) {
		t.Fatal("the offered presentation was not reported with its attachment")
	}
}

func TestLessonWithoutPDFSendsNoPresentation(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson, _ := h.lessonWithMaterials(t, parties, map[string][]byte{"dialogue.mp3": mp3File()}, []string{"dialogue.mp3"})

	h.joined(t, parties.tutorToken, lesson)

	if _, _, found := h.receivedPresentation(t, lesson); found {
		t.Fatal("a lesson without a PDF sent a presentation")
	}
	if strings.Contains(h.logs.String(), `"msg":"meeting.presentation.`) {
		t.Fatal("a lesson without a PDF reported a presentation")
	}
}

func TestMeetingCreatedWithoutPresentationWhenSigningFails(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	presented := h.lessonWithPresentation(t, parties)
	unsigned := h.presentationInstance(t, h.unsignableStorage(), meetings.DefaultRoomTiming().ProviderCall)

	join, err := h.trySendTo(unsigned, http.MethodPost, lessonRoute(presented.lesson, "join"), parties.tutorToken)

	if err != nil {
		t.Fatal(err)
	}
	decodeJoin(t, join)
	if _, _, found := h.receivedPresentation(t, presented.lesson); found {
		t.Fatal("a failed signing still sent a presentation")
	}
	if !strings.Contains(h.logs.String(), presentationUnavailable) || strings.Contains(h.logs.String(), presentationOffered) {
		t.Fatal("the signing failure was not reported as an unavailable presentation")
	}
}

func TestPresentationURLNeverLogged(t *testing.T) {
	t.Run("a successful create", func(t *testing.T) {
		t.Parallel()
		h := newMeetingHarness(t)
		parties := h.visibilityParties(t)
		presented := h.lessonWithPresentation(t, parties)

		join := h.joinCall(t, parties.tutorToken, presented.lesson)

		decodeJoin(t, join)
		h.expectNoPresentationAddress(t, "a successful create", presented, join)
	})
	t.Run("a provider error", func(t *testing.T) {
		t.Parallel()
		h := newMeetingHarness(t)
		parties := h.visibilityParties(t)
		presented := h.lessonWithPresentation(t, parties)
		if err := h.provider.SimulateUnavailable(context.Background(), true); err != nil {
			t.Fatal(err)
		}

		join := h.joinCall(t, parties.tutorToken, presented.lesson)

		expectRefusal(t, "an unavailable provider", join, http.StatusServiceUnavailable, "provider_unavailable")
		if !strings.Contains(h.logs.String(), presentationOffered) {
			t.Fatal("the failed create was not offered the presentation")
		}
		h.expectNoPresentationAddress(t, "a provider error", presented, join)
	})
	t.Run("a provider timeout", func(t *testing.T) {
		t.Parallel()
		h := newMeetingHarness(t)
		parties := h.visibilityParties(t)
		presented := h.lessonWithPresentation(t, parties)
		hurried := h.presentationInstance(t, storage.NewPresigner(h.storage, h.storage.Endpoint), shortProviderCall)
		release := h.holdProvider(t)

		join, err := h.trySendTo(hurried, http.MethodPost, lessonRoute(presented.lesson, "join"), parties.tutorToken)
		release()

		if err != nil {
			t.Fatal(err)
		}
		expectRefusal(t, "a provider past its deadline", join, http.StatusServiceUnavailable, "provider_unavailable")
		if !strings.Contains(h.logs.String(), presentationOffered) || !strings.Contains(h.logs.String(), `"kind":"deadline"`) {
			t.Fatal("the create did not time out while carrying the presentation")
		}
		h.expectNoPresentationAddress(t, "a provider timeout", presented, join)
	})
	t.Run("a signing failure", func(t *testing.T) {
		t.Parallel()
		h := newMeetingHarness(t)
		parties := h.visibilityParties(t)
		presented := h.lessonWithPresentation(t, parties)
		unsigned := h.presentationInstance(t, h.unsignableStorage(), meetings.DefaultRoomTiming().ProviderCall)

		join, err := h.trySendTo(unsigned, http.MethodPost, lessonRoute(presented.lesson, "join"), parties.tutorToken)

		if err != nil {
			t.Fatal(err)
		}
		decodeJoin(t, join)
		h.expectNoPresentationAddress(t, "a signing failure", presented, join)
	})
}
