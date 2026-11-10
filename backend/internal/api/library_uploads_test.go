package api_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	insertPendingLibraryRow = `INSERT INTO attachments (id, uploader_id, context, quota_tutor_id, storage_key, filename, media_type,
    size_bytes, sha256, state, expires_at, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, $3, $2::uuid, 'library/' || $1, 'notes.pdf', 'application/pdf', $4::bigint, sha256('x'), 'pending',
    $5::timestamptz + interval '1 day', $5::timestamptz, $5::timestamptz)`
)

var libraryContexts = []domain.AttachmentContext{domain.ContextLibrary, domain.ContextPromptImage}

func libraryRoute(uploaded domain.AttachmentContext) string {
	return uploadsRoute + "?context=" + string(uploaded)
}

func (h *authHarness) libraryUpload(t *testing.T, token string, uploaded domain.AttachmentContext, filename string, data []byte) response {
	t.Helper()
	body, contentType := chatForm(t, filename, data)
	return h.send(t, http.MethodPost, libraryRoute(uploaded), body, map[string]string{"Authorization": "Bearer " + token, "Content-Type": contentType})
}

func (h *authHarness) insertPendingLibrary(t *testing.T, id string, tutor domain.UserID, uploaded domain.AttachmentContext, size int64, created time.Time) {
	t.Helper()
	if _, err := h.database.Pool.Exec(t.Context(), insertPendingLibraryRow, id, tutor.String(), string(uploaded), size, created); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) capacityUsed(t *testing.T, class domain.UploadCapacityClass) int64 {
	t.Helper()
	return h.countRows(t, "SELECT used FROM upload_intent_capacity WHERE capacity_class = $1", string(class))
}

func (h *authHarness) fillCapacity(t *testing.T, class domain.UploadCapacityClass, used int) {
	t.Helper()
	if _, err := h.database.Pool.Exec(t.Context(), "UPDATE upload_intent_capacity SET used = $2 WHERE capacity_class = $1", string(class), used); err != nil {
		t.Fatal(err)
	}
}

func pdfFile(size int) []byte {
	return append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{'0'}, size)...)
}

func mp3File() []byte {
	return append([]byte("ID3\x04\x00\x00\x00\x00\x01\x00"), bytes.Repeat([]byte{0x55}, 512)...)
}

func uploadsInState(uploads []storedUpload, state domain.AttachmentState) []storedUpload {
	var matching []storedUpload
	for _, upload := range uploads {
		if upload.State == string(state) {
			matching = append(matching, upload)
		}
	}
	return matching
}

func mp4File(t *testing.T, major string) []byte {
	t.Helper()
	box := append([]byte{0, 0, 0, 0}, "ftyp"+major+"\x00\x00\x02\x00isommp42"...)
	binary.BigEndian.PutUint32(box, length32(t, len(box)))
	return append(append(box, 0, 0, 0, 8), append([]byte("free"), bytes.Repeat([]byte{0x22}, 512)...)...)
}

func TestLibraryUploadKinds(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	accepted := []struct {
		filename  string
		data      []byte
		mediaType string
	}{
		{"worksheet.pdf", pdfFile(256), "application/pdf"},
		{"essay.docx", wordDocument(t, ""), "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"dialogue.mp3", mp3File(), "audio/mpeg"},
		{"lecture.mp4", mp4File(t, "isom"), "video/mp4"},
	}
	refused := []struct {
		filename string
		data     []byte
	}{
		{"lecture.mp4", pdfFile(256)},
		{"lecture.m4a", mp4File(t, "isom")},
		{"lecture.mp4", mp4File(t, "qt  ")},
		{"clip.mov", mp4File(t, "qt  ")},
		{"notes.txt", plainText(256)},
	}

	acceptedAnswers := make([]response, 0, len(accepted))
	for _, file := range accepted {
		acceptedAnswers = append(acceptedAnswers, h.libraryUpload(t, token, domain.ContextLibrary, file.filename, file.data))
	}
	refusedAnswers := make([]response, 0, len(refused))
	for _, file := range refused {
		refusedAnswers = append(refusedAnswers, h.libraryUpload(t, token, domain.ContextLibrary, file.filename, file.data))
	}
	uploads := h.uploadsIn(t, tutor, domain.ContextLibrary)

	for index, file := range accepted {
		answer := acceptedAnswers[index]
		if answer.status != http.StatusCreated || decodeUploaded(t, answer).MediaType != file.mediaType {
			t.Fatalf("the library upload of %s answered %s", file.filename, describe(answer))
		}
	}
	for index, file := range refused {
		answer := refusedAnswers[index]
		if answer.status != http.StatusUnsupportedMediaType || code(t, answer) != "unsupported_media_type" {
			t.Fatalf("the library upload of %s answered %s", file.filename, describe(answer))
		}
	}
	refusedRows := uploadsInState(uploads, domain.AttachmentDeleted)
	if len(uploads) != len(accepted)+len(refused) || len(refusedRows) != len(refused) {
		t.Fatalf("the uploads left the rows %+v", uploads)
	}
	described := h.countRows(t, `SELECT count(id) FROM attachments WHERE uploader_id = $1::uuid AND context = 'library' AND state = 'pending'
AND scope_kind IS NULL AND scope_id IS NULL AND quota_tutor_id = uploader_id AND storage_key LIKE 'library/%'`, tutor.String())
	if described != int64(len(accepted)) {
		t.Fatalf("%d accepted rows are pending in the tutor's own library", described)
	}
	for _, upload := range refusedRows {
		h.assertLeftNothing(t, upload)
	}
	if h.capacityUsed(t, domain.UploadCapacityLibrary) != 0 || h.capacityUsed(t, domain.UploadCapacityChat) != 0 {
		t.Fatal("finished library uploads kept capacity reserved")
	}
	if h.countLogs(`"msg":"upload.accepted"`, `"context":"library"`, `"media_type":"video/mp4"`) != 1 ||
		h.countLogs(`"msg":"upload.rejected"`, `"context":"library"`, `"reason":"unsupported"`) != len(refused) ||
		h.countLogs("channel_id") != 0 {
		t.Fatal("the library upload records do not carry their context without a channel")
	}
}

func TestPromptImageUploadsAcceptOnlyPictures(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)

	picture := h.libraryUpload(t, token, domain.ContextPromptImage, "diagram.jpg", samplePicture(t, 30, 20))
	document := h.libraryUpload(t, token, domain.ContextPromptImage, "worksheet.pdf", pdfFile(256))
	video := h.libraryUpload(t, token, domain.ContextPromptImage, "lecture.mp4", mp4File(t, "isom"))
	tooLarge := h.libraryUpload(t, token, domain.ContextPromptImage, "sticker.webp", largeWebP(t, domain.PromptImageMaxBytes))
	declared := h.rawRequest(t, "POST "+libraryRoute(domain.ContextPromptImage)+" HTTP/1.1\r\nHost: ovc.example\r\nAuthorization: Bearer "+token+
		"\r\nContent-Type: multipart/form-data; boundary=declared\r\nContent-Length: "+fmt.Sprint(domain.PromptImageMaxBytes+domain.UploadOverheadBytes+1)+
		"\r\nConnection: close\r\n\r\n")
	uploads := h.uploadsIn(t, tutor, domain.ContextPromptImage)

	if picture.status != http.StatusCreated || decodeUploaded(t, picture).MediaType != "image/jpeg" {
		t.Fatalf("the prompt picture answered %s", describe(picture))
	}
	for _, refused := range []response{document, video} {
		if refused.status != http.StatusUnsupportedMediaType {
			t.Fatalf("a prompt image that is not a picture answered %s", describe(refused))
		}
	}
	if tooLarge.status != http.StatusRequestEntityTooLarge || code(t, tooLarge) != "payload_too_large" {
		t.Fatalf("a prompt image over 5 MB answered %s", describe(tooLarge))
	}
	pending, refusedRows := uploadsInState(uploads, domain.AttachmentPending), uploadsInState(uploads, domain.AttachmentDeleted)
	if declared.status != http.StatusRequestEntityTooLarge || declared.headers.Get("Connection") != "close" {
		t.Fatalf("a declared length over the prompt image limit answered %s", describe(declared))
	}
	if len(pending) != 1 || len(refusedRows) != 3 || pending[0].Width == nil || *pending[0].Width != 30 || pending[0].Height == nil ||
		*pending[0].Height != 20 || !strings.HasPrefix(pending[0].Key, domain.PromptImageKeyPrefix) {
		t.Fatalf("the prompt uploads left %+v", uploads)
	}
	for _, upload := range refusedRows {
		h.assertLeftNothing(t, upload)
	}
}

func assertPictureFiltered(t *testing.T, h *authHarness, tutor domain.UserID, uploaded domain.AttachmentContext, answer response) {
	t.Helper()
	if answer.status != http.StatusCreated {
		t.Fatalf("the %s picture answered %s", uploaded, describe(answer))
	}
	uploads := h.uploadsIn(t, tutor, uploaded)
	if len(uploads) != 1 || uploads[0].Width == nil || *uploads[0].Width != 20 || uploads[0].Height == nil || *uploads[0].Height != 10 {
		t.Fatalf("the %s picture's dimensions were not recorded: %+v", uploaded, uploads)
	}
	stored, found := h.storedObject(t, uploads[0].Key)
	if !found || bytes.Contains(stored, []byte("GPS")) || bytes.Contains(stored, []byte("Exif")) {
		t.Fatalf("the stored %s picture kept its EXIF location", uploaded)
	}
	if int64(len(stored)) != decodeUploaded(t, answer).SizeBytes {
		t.Fatalf("the stored %s picture's size differs from the answer", uploaded)
	}
}

func TestPromptImageUploadFiltersMetadata(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)

	answer := h.libraryUpload(t, token, domain.ContextPromptImage, "map.jpg", pictureWithLocation(t))

	assertPictureFiltered(t, h, tutor, domain.ContextPromptImage, answer)
}

func TestLibraryPictureFiltersMetadata(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)

	answer := h.libraryUpload(t, token, domain.ContextLibrary, "map.jpg", pictureWithLocation(t))

	assertPictureFiltered(t, h, tutor, domain.ContextLibrary, answer)
}

func (u *streamingUpload) streamInBackground(chunk []byte, chunks int, tail []byte) <-chan error {
	written := make(chan error, 1)
	go func() {
		var err error
		for range chunks {
			if _, err = u.part.Write(chunk); err != nil {
				break
			}
		}
		if err == nil {
			_, err = u.part.Write(tail)
		}
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

func TestALibraryUploadOverOneHundredMegabytesIsRefused(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	head := pdfFile(1<<20 - len("%PDF-1.7\n"))
	chunk := bytes.Repeat([]byte{'0'}, 1<<20)
	chunks := domain.LibraryMaxBytes/len(chunk) - 1
	tail := bytes.Repeat([]byte{'0'}, domain.LibraryMaxBytes-len(head)-chunks*len(chunk)+1)
	upload := h.startUploadTo(t, token, libraryRoute(domain.ContextLibrary))
	upload.begin(t, "too long.pdf", head)
	written := upload.streamInBackground(chunk, chunks, tail)

	overLimit := upload.answer(t)
	uploads := h.uploadsIn(t, tutor, domain.ContextLibrary)
	declared := h.rawRequest(t, "POST "+libraryRoute(domain.ContextLibrary)+" HTTP/1.1\r\nHost: ovc.example\r\nAuthorization: Bearer "+token+
		"\r\nContent-Type: multipart/form-data; boundary=declared\r\nContent-Length: "+fmt.Sprint(domain.LibraryMaxBytes+domain.UploadOverheadBytes+1)+
		"\r\nConnection: close\r\n\r\n")
	afterDeclared := h.uploadsIn(t, tutor, libraryContexts...)

	if err := <-written; err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Logf("the client stopped writing after the refusal: %v", err)
	}
	if overLimit.status != http.StatusRequestEntityTooLarge || code(t, overLimit) != "payload_too_large" || len(uploads) != 1 {
		t.Fatalf("a body of 100 MB plus one byte answered %s with rows %+v", describe(overLimit), uploads)
	}
	if declared.status != http.StatusRequestEntityTooLarge || declared.headers.Get("Connection") != "close" || len(afterDeclared) != 1 {
		t.Fatalf("a declared length over the library limit answered %s", describe(declared))
	}
	h.assertLeftNothing(t, uploads[0])
}

func TestLibraryUploadsAreRefusedBeforeTheBodyForOthersThanConfirmedTutors(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	unconfirmed, unconfirmedToken := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 2, studentEmail)
	_, confirmedToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, newcomerEmail)
	unconfirmedUpload := h.startUploadTo(t, unconfirmedToken, libraryRoute(domain.ContextLibrary))
	studentUpload := h.startUploadTo(t, studentToken, libraryRoute(domain.ContextPromptImage))

	unconfirmedAnswer := unconfirmedUpload.answer(t)
	studentAnswer := studentUpload.answer(t)
	scoped := h.send(t, http.MethodPost, libraryRoute(domain.ContextLibrary)+"&scope=channel:"+randomChannelID, "", map[string]string{
		"Authorization": "Bearer " + confirmedToken, "Content-Type": "multipart/form-data; boundary=scoped",
	})
	emptyScope := h.send(t, http.MethodPost, libraryRoute(domain.ContextPromptImage)+"&scope=", "", map[string]string{
		"Authorization": "Bearer " + confirmedToken, "Content-Type": "multipart/form-data; boundary=scoped",
	})

	if unconfirmedAnswer.status != http.StatusForbidden || code(t, unconfirmedAnswer) != "email_not_verified" || len(h.uploadsIn(t, unconfirmed, libraryContexts...)) != 0 {
		t.Fatalf("the unconfirmed tutor's library upload answered %s before its body was sent", describe(unconfirmedAnswer))
	}
	if studentAnswer.status != http.StatusNotFound || code(t, studentAnswer) != "not_found" || len(h.uploadsIn(t, student, libraryContexts...)) != 0 {
		t.Fatalf("the student's prompt image upload answered %s before its body was sent", describe(studentAnswer))
	}
	for _, refused := range []response{scoped, emptyScope} {
		if refused.status != http.StatusUnprocessableEntity || fieldList(t, refused) != "query.scope:not_allowed" {
			t.Fatalf("a library upload with a scope answered %s", describe(refused))
		}
	}
}

func TestLibraryUploadCountsAreSharedByBothContexts(t *testing.T) {
	t.Parallel()
	t.Run("twenty open uploads per tutor", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		for index := range domain.LibraryOpenUploadsPerTutor {
			h.insertPendingLibrary(t, fixtureID(21, byte(index)), tutor, libraryContexts[index%2], 10, authStart)
		}

		twentyFirst := h.libraryUpload(t, token, domain.ContextPromptImage, "diagram.jpg", samplePicture(t, 4, 4))

		if twentyFirst.status != http.StatusTooManyRequests || code(t, twentyFirst) != "rate_limited" || twentyFirst.headers.Get("Retry-After") == "" ||
			h.countLogs(`"msg":"upload.rejected"`, `"context":"prompt_image"`, `"reason":"pending_cap"`) != 1 || h.countLogs(`"msg":"chat.rate_limited"`) != 0 {
			t.Fatalf("the twenty-first open library upload answered %s", describe(twentyFirst))
		}
	})
	t.Run("sixty starts per tutor and hour", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		token := h.sixtyLibraryStarts(t)

		sixtyFirst := h.libraryUpload(t, token, domain.ContextLibrary, "diagram extra.jpg", samplePicture(t, 4, 4))

		if sixtyFirst.status != http.StatusTooManyRequests || h.countLogs(`"msg":"upload.rejected"`, `"context":"library"`, `"reason":"hourly_cap"`) != 1 {
			t.Fatalf("the sixty-first library upload in an hour answered %s", describe(sixtyFirst))
		}
	})
	t.Run("a start in the next hour", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		h.sixtyLibraryStarts(t)
		h.at(t, authStart.Add(time.Hour))
		token := h.relogin(t, tutorEmail)

		nextHour := h.libraryUpload(t, token, domain.ContextPromptImage, "diagram next.jpg", samplePicture(t, 4, 4))

		if nextHour.status != http.StatusCreated {
			t.Fatalf("a library upload in the next hour answered %s", describe(nextHour))
		}
	})
}

func (h *authHarness) sixtyLibraryStarts(t *testing.T) string {
	t.Helper()
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	for index := range domain.LibraryUploadsPerHour {
		h.mustAnswer(t, h.libraryUpload(t, token, libraryContexts[index%2], fmt.Sprintf("diagram %d.jpg", index), samplePicture(t, 4, 4)), http.StatusCreated)
		h.exec(t, "UPDATE attachments SET state = 'deleted', expires_at = NULL WHERE uploader_id = $1::uuid", tutor.String())
	}
	return token
}

func TestALibraryUploadOverQuotaIsRefused(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.setQuota(t, tutor, 1_000_000, 1_000_000)

	refused := h.libraryUpload(t, token, domain.ContextLibrary, "worksheet.pdf", pdfFile(64))

	if refused.status != http.StatusInsufficientStorage || code(t, refused) != "quota_exceeded" ||
		h.countLogs(`"msg":"quota.exceeded"`, `"context":"library"`) != 1 || len(h.uploadsIn(t, tutor, libraryContexts...)) != 0 {
		t.Fatalf("a library upload over quota answered %s", describe(refused))
	}
}

func TestUploadAdmissionAcrossContexts(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	h.setQuota(t, parties.tutor, 1_000_000, 0)
	h.insertPendingLibrary(t, fixtureID(22, 1), parties.tutor, domain.ContextLibrary, 600_000, authStart)
	h.insertPendingLibrary(t, fixtureID(22, 2), parties.tutor, domain.ContextPromptImage, 400_000, authStart)

	chat := h.chatUpload(t, parties.studentToken, channel, "essay.txt", plainText(64))
	library := h.libraryUpload(t, parties.tutorToken, domain.ContextLibrary, "worksheet.pdf", pdfFile(64))

	if chat.status != http.StatusInsufficientStorage || code(t, chat) != "quota_exceeded" || len(h.uploadsBy(t, parties.student)) != 0 {
		t.Fatalf("a chat upload with the quota filled by pending library uploads answered %s", describe(chat))
	}
	if library.status != http.StatusInsufficientStorage {
		t.Fatalf("a library upload with the quota filled by pending library uploads answered %s", describe(library))
	}
}

func TestLibraryUploadCapacityIsSeparateFromChat(t *testing.T) {
	t.Parallel()
	t.Run("a full library class refuses library uploads while chat uploads start", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		channel := h.directChannel(t, parties.card.ID).ID
		h.fillCapacity(t, domain.UploadCapacityLibrary, domain.LibraryUploadIntentCapacity)

		library := h.rawRequest(t, fmt.Sprintf("POST %s HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer %s\r\n"+
			"Content-Type: multipart/form-data; boundary=waiting\r\nContent-Length: 1000\r\n\r\n", libraryRoute(domain.ContextLibrary), parties.tutorToken))
		prompt := h.libraryUpload(t, parties.tutorToken, domain.ContextPromptImage, "diagram.jpg", samplePicture(t, 4, 4))
		chat := h.chatUpload(t, parties.tutorToken, channel, "welcome.txt", plainText(64))

		for _, refused := range []response{library, prompt} {
			if refused.status != http.StatusServiceUnavailable || code(t, refused) != "service_unavailable" {
				t.Fatalf("a library upload with its class full answered %s", describe(refused))
			}
		}
		if library.headers.Get("Connection") != "close" {
			t.Fatalf("a library upload with its class full kept the connection: %s", describe(library))
		}
		if chat.status != http.StatusCreated {
			t.Fatalf("a chat upload beside a full library class answered %s", describe(chat))
		}
		if len(h.uploadsIn(t, parties.tutor, libraryContexts...)) != 0 ||
			h.countLogs(`"msg":"upload.capacity.rejected"`, `"context":"prompt_image"`, `"capacity_class":"library"`) != 1 {
			t.Fatal("the full library class created rows or was not recorded")
		}
	})
	t.Run("a full chat class refuses chat uploads while library uploads start", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		channel := h.directChannel(t, parties.card.ID).ID
		h.fillCapacity(t, domain.UploadCapacityChat, domain.UploadIntentCapacity)

		chat := h.chatUpload(t, parties.tutorToken, channel, "welcome.txt", plainText(64))
		library := h.libraryUpload(t, parties.tutorToken, domain.ContextLibrary, "worksheet.pdf", pdfFile(64))

		if chat.status != http.StatusServiceUnavailable || code(t, chat) != "service_unavailable" {
			t.Fatalf("a chat upload with its class full answered %s", describe(chat))
		}
		if library.status != http.StatusCreated || h.capacityUsed(t, domain.UploadCapacityLibrary) != 0 {
			t.Fatalf("a library upload beside a full chat class answered %s", describe(library))
		}
	})
}

func TestUnbindableLibraryUploadsExpire(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	blocked, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	active, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, newcomerEmail)
	h.insertPendingLibrary(t, fixtureID(23, 1), blocked, domain.ContextLibrary, 10, authStart)
	h.insertPendingLibrary(t, fixtureID(23, 2), blocked, domain.ContextPromptImage, 10, authStart)
	h.insertPendingLibrary(t, fixtureID(23, 3), active, domain.ContextLibrary, 10, authStart)
	h.exec(t, "UPDATE users SET state = 'blocked', state_before_block = 'active' WHERE id = $1::uuid", blocked.String())

	h.tick(t)
	h.tick(t)

	for _, unbindable := range []string{fixtureID(23, 1), fixtureID(23, 2)} {
		if state := h.uploadState(t, unbindable); state != "gone" && state != string(domain.AttachmentDeleted) {
			t.Fatalf("a blocked tutor's pending library upload is %s", state)
		}
	}
	if h.uploadState(t, fixtureID(23, 3)) != string(domain.AttachmentPending) {
		t.Fatal("an active tutor's pending library upload was removed before its expiry")
	}
}

func TestATutorPurgeRemovesTheTutorsUnboundLibraryUploads(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.mustAnswer(t, h.libraryUpload(t, token, domain.ContextLibrary, "worksheet.pdf", pdfFile(64)), http.StatusCreated)
	h.mustAnswer(t, h.libraryUpload(t, token, domain.ContextPromptImage, "diagram.jpg", samplePicture(t, 4, 4)), http.StatusCreated)
	uploads := h.uploadsIn(t, tutor, libraryContexts...)
	h.mustAnswer(t, h.deleteMe(t, token, accountPassword), http.StatusNoContent)

	h.at(t, authStart.Add(domain.DeletedAccountRetention+time.Hour))
	h.tick(t)
	h.tick(t)

	if h.countRows(t, "SELECT count(id) FROM users WHERE id = $1::uuid", tutor.String()) != 0 || len(uploads) != 2 {
		t.Fatal("the tutor was not purged")
	}
	for _, upload := range uploads {
		if h.uploadState(t, upload.ID) != "gone" {
			t.Fatalf("the purged tutor's library upload %s survived", upload.ID)
		}
		if _, stored := h.storedObject(t, upload.Key); stored {
			t.Fatal("the purged tutor's library upload kept its object")
		}
	}
}
