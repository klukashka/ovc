package api_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"log/slog"
	"math"
	"mime/multipart"
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
	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

const (
	uploadsRoute     = "/api/v1/uploads"
	attachmentsRoute = "/api/v1/attachments/"
	wordMain         = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	bindToMessage    = `WITH attachment AS (
    SELECT id, scope_id, uploader_id FROM attachments WHERE id = $1::uuid AND state = 'pending'
), advanced AS (
    UPDATE channels SET last_seq = channels.last_seq + 1
    FROM attachment
    WHERE channels.id = attachment.scope_id AND NOT EXISTS (SELECT 1 FROM messages WHERE messages.id = $2::uuid)
    RETURNING channels.id, channels.last_seq
), sent AS (
    INSERT INTO messages (id, channel_id, seq, sender_id, body, version, created_at, updated_at)
    SELECT $2::uuid, advanced.id, advanced.last_seq, attachment.uploader_id, '', 1, now(), now() FROM advanced, attachment
), bound AS (
    UPDATE attachments SET state = 'bound', expires_at = NULL FROM attachment WHERE attachments.id = attachment.id RETURNING attachments.id
)
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, created_at) SELECT id, 'message', $2::uuid, now() FROM bound`
	insertUploadingChatRow = `INSERT INTO attachments (id, uploader_id, context, scope_kind, scope_id, quota_tutor_id, storage_key, filename, state,
    reserved_bytes, multipart_upload_id, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, 'chat', 'channel', $3::uuid, $4::uuid, 'chat/' || $1, 'notes.txt', 'uploading', $5::bigint, NULLIF($6, ''),
    $7::timestamptz, $7::timestamptz)`
	insertCompletedChatRow = `INSERT INTO attachments (id, uploader_id, context, scope_kind, scope_id, quota_tutor_id, storage_key, filename, media_type,
    size_bytes, sha256, state, expires_at, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, 'chat', 'channel', $3::uuid, $4::uuid, 'chat/' || $1, 'notes.txt', 'text/plain', $5::bigint, sha256('x'), $6,
    CASE WHEN $6 = 'pending' THEN $7::timestamptz + interval '1 day' END, $7::timestamptz, $7::timestamptz)`
)

var stallingUploadTiming = api.UploadTiming{StallWindow: 300 * time.Millisecond, StallBytes: 16 << 10}

func uploadTimingHook(timing api.UploadTiming) optionsHook {
	return func(options *api.Options, _ *clock.Shared, _ *slog.Logger, _ *testdb.Database) {
		options.Uploads = timing
	}
}

func newUploadHarness(t *testing.T) *authHarness {
	t.Helper()
	return newFlowHarness(t)
}

func newStallHarness(t *testing.T) *authHarness {
	t.Helper()
	return newAuthHarnessStoring(t, flowProxies(), api.NewHandler, func(local *teststorage.Storage) storage.Store { return local.Store },
		uploadTimingHook(stallingUploadTiming))
}

type heldStorage struct {
	mutex   sync.Mutex
	matches func(*http.Request) bool
	reached chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *heldStorage) holdNext(matches func(*http.Request) bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.matches = matches
}

func (s *heldStorage) take(request *http.Request) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.matches == nil || !s.matches(request) {
		return false
	}
	s.matches = nil
	return true
}

func (s *heldStorage) resume() {
	s.once.Do(func() { close(s.release) })
}

func (s *heldStorage) awaitReached(t *testing.T) {
	t.Helper()
	select {
	case <-s.reached:
	case <-time.After(pauseDeadline):
		t.Fatal("the upload never reached the held storage request")
	}
}

func newHeldStorageHarness(t *testing.T) (*authHarness, *heldStorage) {
	t.Helper()
	held := &heldStorage{reached: make(chan struct{}), release: make(chan struct{})}
	h := newAuthHarnessStoring(t, flowProxies(), api.NewHandler, func(local *teststorage.Storage) storage.Store {
		target, err := url.Parse(local.Settings.Endpoint)
		if err != nil {
			t.Fatal(err)
		}
		forward := httputil.NewSingleHostReverseProxy(target)
		proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if held.take(request) {
				close(held.reached)
				<-held.release
			}
			forward.ServeHTTP(writer, request)
		}))
		t.Cleanup(proxy.Close)
		t.Cleanup(held.resume)
		settings := local.Settings
		settings.Endpoint = proxy.URL
		return storage.NewS3(settings)
	})
	return h, held
}

func rangedRead(request *http.Request) bool {
	return request.Method == http.MethodGet && request.Header.Get("Range") != ""
}

func chatScope(channel string) string {
	return uploadsRoute + "?context=chat&scope=channel:" + channel
}

func chatForm(t *testing.T, filename string, data []byte) (string, string) {
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
	return body.String(), form.FormDataContentType()
}

func (h *authHarness) chatUpload(t *testing.T, token string, channel string, filename string, data []byte) response {
	t.Helper()
	body, contentType := chatForm(t, filename, data)
	return h.send(t, http.MethodPost, chatScope(channel), body, map[string]string{"Authorization": "Bearer " + token, "Content-Type": contentType})
}

func (h *authHarness) rawChatUpload(t *testing.T, token string, channel string, body string, contentType string) response {
	t.Helper()
	return h.send(t, http.MethodPost, chatScope(channel), body, map[string]string{"Authorization": "Bearer " + token, "Content-Type": contentType})
}

type uploadResult struct {
	result response
	err    error
}

type streamingUpload struct {
	pipe   *io.PipeWriter
	form   *multipart.Writer
	part   io.Writer
	result chan uploadResult
}

func (h *authHarness) startChatUpload(t *testing.T, token string, channel string) *streamingUpload {
	t.Helper()
	reader, writer := io.Pipe()
	upload := &streamingUpload{pipe: writer, form: multipart.NewWriter(writer), result: make(chan uploadResult, 1)}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, h.server.URL+chatScope(channel), reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", upload.form.FormDataContentType())
	go func() {
		answer, err := http.DefaultClient.Do(request)
		if err != nil {
			upload.result <- uploadResult{err: err}
			return
		}
		data, readErr := io.ReadAll(answer.Body)
		closeErr := answer.Body.Close()
		upload.result <- uploadResult{
			result: response{status: answer.StatusCode, body: string(data), headers: answer.Header}, err: errors.Join(readErr, closeErr),
		}
	}()
	t.Cleanup(func() {
		if err := writer.CloseWithError(errors.New("the test ended")); err != nil {
			t.Error(err)
		}
	})
	return upload
}

func (u *streamingUpload) begin(t *testing.T, filename string, data []byte) {
	t.Helper()
	part, err := u.form.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	u.part = part
	u.write(t, data)
}

func (u *streamingUpload) write(t *testing.T, data []byte) {
	t.Helper()
	if _, err := u.part.Write(data); err != nil {
		t.Fatal(err)
	}
}

func (u *streamingUpload) finish(t *testing.T, data []byte) response {
	t.Helper()
	u.write(t, data)
	if err := u.form.Close(); err != nil {
		t.Fatal(err)
	}
	if err := u.pipe.Close(); err != nil {
		t.Fatal(err)
	}
	return u.answer(t)
}

func (u *streamingUpload) completeInBackground(data []byte) <-chan error {
	written := make(chan error, 1)
	go func() {
		_, err := u.part.Write(data)
		if err == nil {
			err = u.form.Close()
		}
		if err == nil {
			err = u.pipe.Close()
		}
		written <- err
	}()
	return written
}

func (u *streamingUpload) outcome(t *testing.T) uploadResult {
	t.Helper()
	select {
	case outcome := <-u.result:
		return outcome
	case <-time.After(pauseDeadline):
		t.Fatal("the upload never answered")
		return uploadResult{}
	}
}

func (u *streamingUpload) answer(t *testing.T) response {
	t.Helper()
	outcome := u.outcome(t)
	if outcome.err != nil {
		t.Fatal(outcome.err)
	}
	return outcome.result
}

type storedUpload struct {
	ID        string
	State     string
	Key       string
	MediaType *string
	SizeBytes *int64
	Reserved  *int64
	Multipart *string
	Width     *int32
	Height    *int32
	Sum       []byte
}

func (h *authHarness) uploadsBy(t *testing.T, uploader domain.UserID) []storedUpload {
	t.Helper()
	rows, err := h.database.Pool.Query(context.Background(),
		`SELECT id::text, state, storage_key, media_type, size_bytes, reserved_bytes, multipart_upload_id, width, height, sha256
FROM attachments WHERE uploader_id = $1::uuid AND context = 'chat' ORDER BY created_at, id`, uploader.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var uploads []storedUpload
	for rows.Next() {
		var upload storedUpload
		if err := rows.Scan(&upload.ID, &upload.State, &upload.Key, &upload.MediaType, &upload.SizeBytes, &upload.Reserved, &upload.Multipart,
			&upload.Width, &upload.Height, &upload.Sum); err != nil {
			t.Fatal(err)
		}
		uploads = append(uploads, upload)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return uploads
}

func (h *authHarness) onlyUploadBy(t *testing.T, uploader domain.UserID) storedUpload {
	t.Helper()
	uploads := h.uploadsBy(t, uploader)
	if len(uploads) != 1 {
		t.Fatalf("expected one upload row, found %+v", uploads)
	}
	return uploads[0]
}

func (h *authHarness) awaitUploading(t *testing.T, uploader domain.UserID) storedUpload {
	t.Helper()
	var found storedUpload
	h.waitFor(t, "a row carrying its multipart upload", func() bool {
		for _, upload := range h.uploadsBy(t, uploader) {
			if upload.Multipart != nil {
				found = upload
				return true
			}
		}
		return false
	})
	return found
}

func (h *authHarness) incompleteUploads(t *testing.T, key string) []storage.MultipartUploadID {
	t.Helper()
	uploads, err := h.objects.IncompleteUploads(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return uploads
}

func (h *authHarness) storedObject(t *testing.T, key string) ([]byte, bool) {
	t.Helper()
	body, err := h.objects.Open(context.Background(), key)
	if errors.Is(err, storage.ErrObjectNotFound) {
		return nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(body)
	if err := errors.Join(readErr, body.Close()); err != nil {
		t.Fatal(err)
	}
	return data, true
}

func (h *authHarness) assertLeftNothing(t *testing.T, upload storedUpload) {
	t.Helper()
	h.tick(t)
	if state := h.uploadState(t, upload.ID); state != "gone" && state != string(domain.AttachmentDeleted) {
		t.Fatalf("the refused upload's row is %s", state)
	}
	if _, stored := h.storedObject(t, upload.Key); stored {
		t.Fatal("the refused upload left its object in storage")
	}
	if incomplete := h.incompleteUploads(t, upload.Key); len(incomplete) != 0 {
		t.Fatalf("the refused upload left incomplete multipart uploads %v", incomplete)
	}
}

func messageID(n byte) string {
	return domain.EntityID{0x4d, 0x53, n}.String()
}

func (h *authHarness) bind(t *testing.T, attachment string, message string) {
	t.Helper()
	h.exec2(t, bindToMessage, attachment, message)
	if h.uploadState(t, attachment) != string(domain.AttachmentBound) {
		t.Fatalf("the attachment %s could not be bound", attachment)
	}
}

func (h *authHarness) boundFixture(t *testing.T, attachment string, channel string, message string) {
	t.Helper()
	h.exec2(t, `INSERT INTO attachments (id, uploader_id, context, scope_kind, scope_id, quota_tutor_id, storage_key, filename, media_type,
    size_bytes, sha256, state, expires_at, created_at, updated_at)
SELECT $1::uuid, tutor_id, 'chat', 'channel', id, tutor_id, 'chat/' || $1, 'notes.txt', 'text/plain', 10, sha256('x'), 'pending',
    now() + interval '1 day', now(), now()
FROM channels WHERE id = $2::uuid`, attachment, channel)
	h.bind(t, attachment, message)
}

func (h *authHarness) boundUpload(t *testing.T, token string, channel string, filename string, data []byte, message byte) string {
	t.Helper()
	uploaded := h.chatUpload(t, token, channel, filename, data)
	h.mustAnswer(t, uploaded, http.StatusCreated)
	id := uploadedID(t, uploaded)
	h.bind(t, id, messageID(message))
	return id
}

type downloadAddress struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (h *authHarness) addressOf(t *testing.T, token string, attachment string, query string) (response, downloadAddress) {
	t.Helper()
	result := h.send(t, http.MethodGet, attachmentsRoute+attachment+query, "", jsonCall(token))
	var address downloadAddress
	if result.status == http.StatusOK {
		if err := json.Unmarshal([]byte(result.body), &address); err != nil {
			t.Fatal(err)
		}
	}
	return result, address
}

func (h *authHarness) fetch(t *testing.T, address string, headers map[string]string) response {
	t.Helper()
	return h.send(t, http.MethodGet, address, "", headers)
}

func (h *authHarness) setQuota(t *testing.T, tutor domain.UserID, quota int64, usage int64) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), "UPDATE tutor_profiles SET file_quota_bytes = $2, file_usage_bytes = $3 WHERE user_id = $1",
		tutor.String(), quota, usage)
	if err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) insertUploading(t *testing.T, id string, uploader domain.UserID, channel string, tutor domain.UserID, reserved int64,
	multipartUpload string, created time.Time) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), insertUploadingChatRow, id, uploader.String(), channel, tutor.String(), reserved,
		multipartUpload, created)
	if err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) insertCompleted(t *testing.T, id string, uploader domain.UserID, channel string, tutor domain.UserID, size int64,
	state domain.AttachmentState, created time.Time) {
	t.Helper()
	_, err := h.database.Pool.Exec(context.Background(), insertCompletedChatRow, id, uploader.String(), channel, tutor.String(), size,
		string(state), created)
	if err != nil {
		t.Fatal(err)
	}
}

func fixtureID(group byte, n byte) string {
	return domain.EntityID{0x55, group, n}.String()
}

func length32(t *testing.T, value int) uint32 {
	t.Helper()
	if value < 0 || value > math.MaxUint32 {
		t.Fatalf("a fixture length %d does not fit 32 bits", value)
		return 0
	}
	return uint32(value)
}

func length16(t *testing.T, value int) uint16 {
	t.Helper()
	if value < 0 || value > math.MaxUint16 {
		t.Fatalf("a fixture length %d does not fit 16 bits", value)
		return 0
	}
	return uint16(value)
}

func plainText(size int) []byte {
	line := []byte("A line of homework notes, written in plain text.\n")
	return bytes.Repeat(line, size/len(line)+1)[:size]
}

func samplePicture(t *testing.T, width int, height int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 30, G: 140, B: 200, A: 255}}, image.Point{}, draw.Src)
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, canvas, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func pictureWithLocation(t *testing.T) []byte {
	t.Helper()
	original := samplePicture(t, 20, 10)
	payload := append([]byte("Exif\x00\x00II*\x00\x08\x00\x00\x00\x00\x00\x00\x00\x00\x00"), []byte("GPS 55.7558N 37.6173E")...)
	segment := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(segment[2:], length16(t, len(payload)+2))
	return append(append(append(bytes.Clone(original[:2]), segment...), payload...), original[2:]...)
}

func zipOf(t *testing.T, entries map[string]string, order []string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range order {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, entries[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func wordDocument(t *testing.T, extraRelationship string) []byte {
	t.Helper()
	entries := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="` + wordMain + `"/></Types>`,
		"_rels/.rels": `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
			`</Relationships>`,
		"word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			extraRelationship + `</Relationships>`,
	}
	return zipOf(t, entries, []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/_rels/document.xml.rels"})
}

func largeWebP(t *testing.T, payload int) []byte {
	t.Helper()
	chunk := append([]byte("VP8L"), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(chunk[4:], length32(t, payload))
	chunk = append(chunk, 0x2F, 0x00, 0x00, 0x00, 0x00)
	chunk = append(chunk, make([]byte, payload-5)...)
	data := append([]byte("RIFF\x00\x00\x00\x00WEBP"), chunk...)
	binary.LittleEndian.PutUint32(data[4:], length32(t, len(data)-8))
	return data
}

func fieldList(t *testing.T, result response) string {
	t.Helper()
	return strings.Join(fieldsOf(t, result), ",")
}

func describe(result response) string {
	return fmt.Sprintf("%d %s", result.status, result.body)
}

type heldRemoval struct {
	commit func(t *testing.T)
}

func (h *authHarness) holdChannelRemoval(t *testing.T, channel string, member domain.UserID) *heldRemoval {
	t.Helper()
	tx, err := h.database.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	committed := false
	t.Cleanup(func() {
		if committed {
			return
		}
		if err := tx.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if _, err := tx.Exec(context.Background(), "SELECT id FROM channels WHERE id = $1::uuid FOR UPDATE", channel); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(), "DELETE FROM channel_members WHERE channel_id = $1::uuid AND user_id = $2::uuid", channel,
		member.String()); err != nil {
		t.Fatal(err)
	}
	return &heldRemoval{commit: func(t *testing.T) {
		t.Helper()
		committed = true
		if err := tx.Commit(context.Background()); err != nil {
			t.Fatal(err)
		}
	}}
}

func (h *authHarness) awaitLockWaiters(t *testing.T, count int64) {
	t.Helper()
	h.waitFor(t, "a transaction waiting for a row lock", func() bool {
		return h.countRows(t, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = $1", "Lock") >= count
	})
}
