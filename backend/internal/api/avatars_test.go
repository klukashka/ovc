package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/teststorage"
)

const tinyLosslessWebP = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

func pngBytes(t *testing.T, width int, height int) []byte {
	t.Helper()
	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	canvas.Set(0, 0, color.NRGBA{R: 200, G: 80, B: 20, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

const (
	metadataMarker  = "SECRET-GPS-51.5N"
	metadataSegment = "Exif\x00\x00" + metadataMarker
)

func jpegWithMetadata(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	segment := append([]byte{0xFF, 0xE1}, binary.BigEndian.AppendUint16(nil, uint16(len(metadataSegment)+2))...)
	segment = append(segment, metadataSegment...)
	original := encoded.Bytes()
	return append(append(append([]byte{}, original[:2]...), segment...), original[2:]...)
}

func pngWithDeclaredWidth(t *testing.T, width uint32) []byte {
	t.Helper()
	data := pngBytes(t, 1, 1)
	binary.BigEndian.PutUint32(data[16:20], width)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	return data
}

func sixteenBitPNG(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA64(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func (h *authHarness) upload(t *testing.T, token string, filename string, data []byte) response {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	return h.send(t, http.MethodPost, "/api/v1/uploads?context=avatar", body.String(), map[string]string{
		"Authorization": "Bearer " + token, "Content-Type": form.FormDataContentType(),
	})
}

func (h *authHarness) patchMe(t *testing.T, token string, body string) response {
	t.Helper()
	return h.send(t, http.MethodPatch, "/api/v1/users/me", body, map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"})
}

func uploadedID(t *testing.T, result response) string {
	t.Helper()
	var decoded struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || decoded.ID == "" {
		t.Fatalf("upload %d %s", result.status, result.body)
	}
	return decoded.ID
}

func (h *authHarness) storageKey(t *testing.T, id string) string {
	t.Helper()
	parsed, ok := domain.ParseUUID[domain.AttachmentID](id)
	if !ok {
		t.Fatalf("malformed identifier %s", id)
	}
	attachment, err := h.store.Repositories().Attachments.ByID(context.Background(), parsed)
	if err != nil {
		t.Fatal(err)
	}
	return attachment.StorageKey
}

func (h *authHarness) objectExists(t *testing.T, key string) bool {
	t.Helper()
	body, err := h.objects.Open(context.Background(), key)
	if errors.Is(err, storage.ErrObjectNotFound) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	return true
}

func (h *authHarness) signedInTutor(t *testing.T) string {
	t.Helper()
	h.seedUser(t, 1, tutorEmail, "correct horse battery", currentHash)
	return accessToken(t, h.login(t, tutorEmail, "correct horse battery"))
}

func TestAvatarIsBoundServedAndReplaced(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	first := uploadedID(t, h.upload(t, token, "Me.PNG", pngBytes(t, 4, 4)))
	firstKey := h.storageKey(t, first)

	bound := h.patchMe(t, token, fmt.Sprintf(`{"avatar_id":%q}`, first))
	served := h.send(t, http.MethodGet, "/api/v1/avatars/"+first, "", nil)
	second := uploadedID(t, h.upload(t, token, "next.jpg", jpegWithMetadata(t)))
	replaced := h.patchMe(t, token, fmt.Sprintf(`{"avatar_id":%q}`, second))
	oldServed := h.send(t, http.MethodGet, "/api/v1/avatars/"+first, "", nil)
	h.tick(t)

	if bound.status != http.StatusOK || !strings.Contains(bound.body, `"avatar_id":"`+first+`"`) {
		t.Fatalf("binding answered %d %s", bound.status, bound.body)
	}
	wantHeaders := map[string]string{
		"Content-Type": "image/png", "Content-Disposition": "inline", "Cache-Control": "private, max-age=86400",
		"Content-Security-Policy": "default-src 'none'; sandbox", "Cross-Origin-Resource-Policy": "same-origin", "X-Content-Type-Options": "nosniff",
	}
	for name, want := range wantHeaders {
		if served.headers.Get(name) != want {
			t.Fatalf("%s is %q, want %q", name, served.headers.Get(name), want)
		}
	}
	if decoded, err := png.Decode(strings.NewReader(served.body)); served.status != http.StatusOK || err != nil || decoded.Bounds().Dx() != 4 {
		t.Fatalf("served %d, decode %v", served.status, err)
	}
	if served.headers.Get("Content-Length") != strconv.Itoa(len(served.body)) {
		t.Fatalf("Content-Length is %q for %d bytes", served.headers.Get("Content-Length"), len(served.body))
	}
	if replaced.status != http.StatusOK || oldServed.status != http.StatusNotFound || h.objectExists(t, firstKey) {
		t.Fatalf("replacement %d, old avatar %d, old object kept %v", replaced.status, oldServed.status, h.objectExists(t, firstKey))
	}
}

func TestAvatarRefusesOversizeAndDisguisedFiles(t *testing.T) {
	t.Parallel()
	webp, err := base64.StdEncoding.DecodeString(tinyLosslessWebP)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		filename string
		data     []byte
		status   int
	}{
		"over 5 MB":             {filename: "big.png", data: append(pngBytes(t, 1, 1), make([]byte, domain.AvatarMaxBytes)...), status: http.StatusRequestEntityTooLarge},
		"GIF":                   {filename: "anim.gif", data: []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), status: http.StatusUnsupportedMediaType},
		"SVG":                   {filename: "logo.svg", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), status: http.StatusUnsupportedMediaType},
		"text named PNG":        {filename: "note.png", data: []byte("hello"), status: http.StatusUnsupportedMediaType},
		"PNG named JPEG":        {filename: "photo.jpg", data: pngBytes(t, 2, 2), status: http.StatusUnsupportedMediaType},
		"WebP named PNG":        {filename: "photo.png", data: webp, status: http.StatusUnsupportedMediaType},
		"wider than 4096 pixel": {filename: "wide.png", data: pngWithDeclaredWidth(t, 4097), status: http.StatusUnsupportedMediaType},
		"16-bit PNG":            {filename: "deep.png", data: sixteenBitPNG(t), status: http.StatusUnsupportedMediaType},
		"exactly 4096 pixels":   {filename: "wide.png", data: pngBytes(t, 4096, 1), status: http.StatusCreated},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			token := h.signedInTutor(t)

			result := h.upload(t, token, testCase.filename, testCase.data)

			if result.status != testCase.status {
				t.Fatalf("answered %d %s", result.status, result.body)
			}
		})
	}
}

func TestWebPAvatarIsStoredAsPNG(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	webp, err := base64.StdEncoding.DecodeString(tinyLosslessWebP)
	if err != nil {
		t.Fatal(err)
	}

	result := h.upload(t, token, "face.webp", webp)

	if result.status != http.StatusCreated || !strings.Contains(result.body, `"media_type":"image/png"`) || !strings.Contains(result.body, `"filename":"avatar.png"`) {
		t.Fatalf("answered %d %s", result.status, result.body)
	}
}

func TestUnboundAttachmentIsNotServedAsAvatar(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	h.seedUser(t, 2, "other@example.org", "correct horse battery", currentHash)
	other := accessToken(t, h.login(t, "other@example.org", "correct horse battery"))
	pending := uploadedID(t, h.upload(t, token, "me.png", pngBytes(t, 2, 2)))
	othersPending := uploadedID(t, h.upload(t, other, "other.png", pngBytes(t, 2, 2)))
	blockedOwner := uploadedID(t, h.upload(t, other, "other.png", pngBytes(t, 2, 2)))
	if bound := h.patchMe(t, other, fmt.Sprintf(`{"avatar_id":%q}`, blockedOwner)); bound.status != http.StatusOK {
		t.Fatalf("binding the other avatar answered %d %s", bound.status, bound.body)
	}
	replaced := uploadedID(t, h.upload(t, token, "old.png", pngBytes(t, 2, 2)))
	h.patchMe(t, token, fmt.Sprintf(`{"avatar_id":%q}`, replaced))
	h.patchMe(t, token, `{"avatar_id":null}`)
	if _, err := h.store.Repositories().Users.Block(context.Background(), domain.UserID{2}, authStart); err != nil {
		t.Fatal(err)
	}

	unbound := h.send(t, http.MethodGet, "/api/v1/avatars/"+pending, "", nil)
	malformed := h.send(t, http.MethodGet, "/api/v1/avatars/not-an-id", "", nil)
	blocked := h.send(t, http.MethodGet, "/api/v1/avatars/"+blockedOwner, "", nil)
	stolen := h.patchMe(t, token, fmt.Sprintf(`{"avatar_id":%q}`, blockedOwner))
	stolenPending := h.patchMe(t, token, fmt.Sprintf(`{"avatar_id":%q}`, othersPending))
	revived := h.patchMe(t, token, fmt.Sprintf(`{"avatar_id":%q}`, replaced))

	for name, result := range map[string]response{"unbound": unbound, "malformed": malformed, "blocked owner": blocked} {
		if result.status != http.StatusNotFound || code(t, result) != "not_found" {
			t.Fatalf("%s avatar answered %d %s", name, result.status, result.body)
		}
	}
	for name, result := range map[string]response{"another user's avatar": stolen, "another user's pending upload": stolenPending, "a removed avatar": revived} {
		if result.status != http.StatusUnprocessableEntity || !strings.Contains(strings.Join(fields(t, result), ","), "body.avatar_id:invalid") {
			t.Fatalf("binding %s answered %d %s", name, result.status, result.body)
		}
	}
}

func TestAvatarMetadataIsRemoved(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	id := uploadedID(t, h.upload(t, token, "photo.jpeg", jpegWithMetadata(t)))
	h.patchMe(t, token, fmt.Sprintf(`{"avatar_id":%q}`, id))

	served := h.send(t, http.MethodGet, "/api/v1/avatars/"+id, "", nil)

	if served.status != http.StatusOK || served.headers.Get("Content-Type") != "image/jpeg" || strings.Contains(served.body, metadataMarker) {
		t.Fatalf("served %d %s with metadata %v", served.status, served.headers.Get("Content-Type"), strings.Contains(served.body, metadataMarker))
	}
}

func TestPendingUploadsAreLimitedAndExpireAfter24Hours(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	var keys []string
	for range 3 {
		keys = append(keys, h.storageKey(t, uploadedID(t, h.upload(t, token, "me.png", pngBytes(t, 2, 2)))))
	}

	fourth := h.upload(t, token, "me.png", pngBytes(t, 2, 2))
	h.at(t, authStart.Add(24*time.Hour))
	h.tick(t)
	afterExpiry := h.upload(t, accessToken(t, h.login(t, tutorEmail, "correct horse battery")), "me.png", pngBytes(t, 2, 2))

	if fourth.status != http.StatusTooManyRequests || code(t, fourth) != "rate_limited" {
		t.Fatalf("the fourth pending upload answered %d %s", fourth.status, fourth.body)
	}
	for _, key := range keys {
		if h.objectExists(t, key) {
			t.Fatal("an expired upload kept its object")
		}
	}
	if afterExpiry.status != http.StatusCreated {
		t.Fatalf("an upload after expiry answered %d %s", afterExpiry.status, afterExpiry.body)
	}
}

func TestProfileUpdateValidatesFieldsAndKeepsTutorTextForTutors(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor := h.signedInTutor(t)
	h.seedUserAs(t, domain.RoleStudent, 2, "student@example.org", "correct horse battery", currentHash)
	student := accessToken(t, h.login(t, "student@example.org", "correct horse battery"))

	saved := h.patchMe(t, tutor, `{"name":" Anna ","language":"ru","timezone":"Asia/Yekaterinburg","theme":"dark","bio":"Grammar and speaking","subjects":["English"," IELTS "]}`)
	studentBio := h.patchMe(t, student, `{"bio":"I study"}`)
	refusals := map[string]response{
		"body.timezone:invalid": h.patchMe(t, tutor, `{"timezone":"Mars/Olympus_Mons"}`),
		"body.name:invalid":     h.patchMe(t, tutor, `{"name":"   "}`),
		"body.subjects:invalid": h.patchMe(t, tutor, `{"subjects":["English","English"]}`),
	}
	withoutType := h.send(t, http.MethodPatch, "/api/v1/users/me", `{"name":"Anna"}`, map[string]string{"Authorization": "Bearer " + tutor})
	me := h.me(t, tutor)

	for _, part := range []string{`"name":"Anna"`, `"language":"ru"`, `"timezone":"Asia/Yekaterinburg"`, `"theme":"dark"`, `"bio":"Grammar and speaking"`, `"subjects":["English","IELTS"]`} {
		if saved.status != http.StatusOK || !strings.Contains(me.body, part) {
			t.Fatalf("saved %d %s; me %s lacks %s", saved.status, saved.body, me.body, part)
		}
	}
	if studentBio.status != http.StatusUnprocessableEntity || !strings.Contains(strings.Join(fields(t, studentBio), ","), "body.bio:not_allowed") {
		t.Fatalf("a student's bio answered %d %s", studentBio.status, studentBio.body)
	}
	for field, result := range refusals {
		if result.status != http.StatusUnprocessableEntity || !strings.Contains(strings.Join(fields(t, result), ","), field) {
			t.Fatalf("expected %s, got %d %s", field, result.status, result.body)
		}
	}
	if withoutType.status != http.StatusUnsupportedMediaType {
		t.Fatalf("a body without a content type answered %d", withoutType.status)
	}
}

func TestReadinessReportsAnUnreachableBucket(t *testing.T) {
	t.Parallel()
	settings := teststorage.New(t).Settings
	settings.Bucket = "ovc-missing-" + core.NewRandomID()[:12]
	logs := &safeBuffer{}
	handler, err := api.NewHandler(api.Options{
		Logger: slog.New(slog.NewJSONHandler(logs, nil)), Now: clock.System{}.Now, NewRequestID: core.NewRandomID,
		Readiness: api.AllReady(
			api.ReadinessCheck{Name: "database", Check: func(context.Context) error { return nil }},
			api.ReadinessCheck{Name: "storage", Check: storage.NewS3(settings).Ping},
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/v1/health/ready", nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := http.DefaultClient.Do(request)

	if err != nil {
		t.Fatal(err)
	}
	if err := result.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusServiceUnavailable || !strings.Contains(logs.String(), `"kind":"storage"`) {
		t.Fatalf("readiness answered %d; log %s", result.StatusCode, logs.String())
	}
}

func TestAnUploadWithoutAFilePartIsRefusedAsMissing(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	result := h.send(t, http.MethodPost, "/api/v1/uploads?context=avatar", body.String(), map[string]string{
		"Authorization": "Bearer " + token, "Content-Type": form.FormDataContentType(),
	})

	if result.status != http.StatusUnprocessableEntity || !strings.Contains(strings.Join(fields(t, result), ","), "body.file:required") {
		t.Fatalf("an empty form answered %d %s", result.status, result.body)
	}
}

func (h *authHarness) uploadForm(t *testing.T, token string, body string, contentType string) response {
	t.Helper()
	return h.send(t, http.MethodPost, "/api/v1/uploads?context=avatar", body, map[string]string{"Authorization": "Bearer " + token, "Content-Type": contentType})
}

func TestAFormWithTwoPartsStoresNothingAndAMalformedOneIsABadRequest(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, name := range []string{"first.png", "second.png"} {
		part, err := form.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(pngBytes(t, 2, 2)); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	brokenTrailer := "--edge\r\nContent-Disposition: form-data; name=\"file\"; filename=\"me.png\"\r\n\r\n" + string(pngBytes(t, 2, 2)) +
		"\r\n--edge\r\nnot a header\r\n"

	twoParts := h.uploadForm(t, token, body.String(), form.FormDataContentType())
	malformed := h.uploadForm(t, token, "--edge\r\nnot a header\r\n", "multipart/form-data; boundary=edge")
	malformedTrailer := h.uploadForm(t, token, brokenTrailer, "multipart/form-data; boundary=edge")

	if twoParts.status != http.StatusUnprocessableEntity || h.countRows(t, "SELECT count(id) FROM attachments WHERE context <> $1", "") != 0 {
		t.Fatalf("two parts answered %d %s", twoParts.status, twoParts.body)
	}
	for name, result := range map[string]response{"malformed form": malformed, "malformed trailing part": malformedTrailer} {
		if result.status != http.StatusBadRequest || code(t, result) != "bad_request" {
			t.Fatalf("a %s answered %d %s", name, result.status, result.body)
		}
	}
	if stored := h.countRows(t, "SELECT count(id) FROM attachments WHERE context <> $1", ""); stored != 0 {
		t.Fatalf("%d attachments were stored", stored)
	}
}

func TestAnExpiredUploadCannotBeBoundBeforeTheWorkerRemovesIt(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	id := uploadedID(t, h.upload(t, token, "me.png", pngBytes(t, 2, 2)))
	h.at(t, authStart.Add(24*time.Hour))
	fresh := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))

	bound := h.patchMe(t, fresh, fmt.Sprintf(`{"avatar_id":%q}`, id))

	if bound.status != http.StatusUnprocessableEntity || !strings.Contains(strings.Join(fields(t, bound), ","), "body.avatar_id:invalid") {
		t.Fatalf("binding an expired upload answered %d %s", bound.status, bound.body)
	}
}

func TestExpiredPendingUploadsDoNotCountBeforeTheExpiryJobRuns(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	for range 3 {
		uploadedID(t, h.upload(t, token, "me.png", pngBytes(t, 2, 2)))
	}
	h.at(t, authStart.Add(24*time.Hour+time.Minute))
	fresh := accessToken(t, h.login(t, tutorEmail, "correct horse battery"))

	upload := h.upload(t, fresh, "me.png", pngBytes(t, 2, 2))

	if upload.status != http.StatusCreated {
		t.Fatalf("expired pending uploads still blocked a new one: %d %s", upload.status, upload.body)
	}
}

func TestAnAvatarWhoseObjectIsMissingAnswersNotFoundAndIsReported(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	token := h.signedInTutor(t)
	avatar := uploadedID(t, h.upload(t, token, "me.png", pngBytes(t, 2, 2)))
	h.patchMe(t, token, fmt.Sprintf(`{"avatar_id":%q}`, avatar))
	key := h.storageKey(t, avatar)
	if err := h.objects.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}

	served := h.send(t, http.MethodGet, "/api/v1/avatars/"+avatar, "", nil)

	if served.status != http.StatusNotFound {
		t.Fatalf("a missing avatar object answered %d", served.status)
	}
	logs := h.logs.String()
	if strings.Count(logs, `"level":"WARN","msg":"avatars.object_missing"`) != 1 || strings.Contains(logs, key) {
		t.Fatalf("the missing object was not reported once without its key: %s", logs)
	}
}

func noisyPNG(t *testing.T, side int) []byte {
	t.Helper()
	canvas := image.NewNRGBA(image.Rect(0, 0, side, side))
	state := uint32(2463534242)
	for index := 0; index+4 <= len(canvas.Pix); index += 4 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		binary.LittleEndian.PutUint32(canvas.Pix[index:], state)
	}
	for index := 3; index < len(canvas.Pix); index += 4 {
		canvas.Pix[index] = 255
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func (h *authHarness) boundAvatarWithStoredObject(t *testing.T, stored func(original []byte) []byte) (string, int) {
	t.Helper()
	token := h.signedInTutor(t)
	avatar := uploadedID(t, h.upload(t, token, "me.png", noisyPNG(t, 128)))
	h.patchMe(t, token, fmt.Sprintf(`{"avatar_id":%q}`, avatar))
	original := h.send(t, http.MethodGet, "/api/v1/avatars/"+avatar, "", nil).body
	if len(original) < 16*1024 {
		t.Fatalf("the avatar is only %d bytes, too small to pass the server's write buffers", len(original))
	}
	if err := h.objects.Put(context.Background(), h.storageKey(t, avatar), stored([]byte(original)), "image/png"); err != nil {
		t.Fatal(err)
	}
	return avatar, len(original)
}

type fetchedAvatar struct {
	declared int64
	received int
	err      error
}

func (h *authHarness) fetchAvatar(t *testing.T, avatar string) fetchedAvatar {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, h.server.URL+"/api/v1/avatars/"+avatar, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		return fetchedAvatar{declared: -1, err: err}
	}
	body, readErr := io.ReadAll(result.Body)
	return fetchedAvatar{declared: result.ContentLength, received: len(body), err: errors.Join(readErr, result.Body.Close())}
}

func (f fetchedAvatar) incomplete() bool {
	return f.err != nil && (f.declared < 0 || int64(f.received) < f.declared)
}

func TestAnAvatarObjectShorterThanRecordedAbortsTheResponseAndIsReported(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	avatar, _ := h.boundAvatarWithStoredObject(t, func(original []byte) []byte { return original[:len(original)/2] })

	fetched := h.fetchAvatar(t, avatar)

	if !fetched.incomplete() {
		t.Fatalf("a short object was served as complete: %+v", fetched)
	}
	if !strings.Contains(h.logs.String(), `"msg":"avatars.get.failed","kind":"size_mismatch"`) {
		t.Fatalf("the short object was not reported: %s", h.logs.String())
	}
}

func TestAnAvatarObjectLongerThanRecordedAbortsTheResponseAndIsReported(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	avatar, _ := h.boundAvatarWithStoredObject(t, func(original []byte) []byte { return append(original, 0) })

	fetched := h.fetchAvatar(t, avatar)

	if !fetched.incomplete() {
		t.Fatalf("a long object was served as complete: %+v", fetched)
	}
	if !strings.Contains(h.logs.String(), `"msg":"avatars.get.failed","kind":"size_mismatch"`) {
		t.Fatalf("the long object was not reported: %s", h.logs.String())
	}
}
