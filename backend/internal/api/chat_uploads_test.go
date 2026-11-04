package api_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
)

type uploadedBody struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	MediaType string `json:"media_type"`
	SizeBytes int64  `json:"size_bytes"`
}

func decodeUploaded(t *testing.T, result response) uploadedBody {
	t.Helper()
	var decoded uploadedBody
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil {
		t.Fatalf("upload %s", describe(result))
	}
	return decoded
}

func (h *authHarness) rawRequest(t *testing.T, text string) response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), pauseDeadline)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", h.server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := connection.SetDeadline(clock.System{}.Now().Add(pauseDeadline)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(connection, text); err != nil {
		t.Fatal(err)
	}
	answer, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(answer.Body)
	if err := errors.Join(readErr, answer.Body.Close()); err != nil {
		t.Fatal(err)
	}
	if answer.Close {
		answer.Header.Set("Connection", "close")
	}
	return response{status: answer.StatusCode, body: string(data), headers: answer.Header}
}

func isPart(number string) func(*http.Request) bool {
	return func(request *http.Request) bool {
		return request.Method == http.MethodPut && request.URL.Query().Get("partNumber") == number
	}
}

func TestAChatUploadCreatesItsRowFirstAndStreamsInParts(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	content := plainText(17 << 20)
	held.holdNext(isPart("2"))
	upload := h.startChatUpload(t, parties.studentToken, channel)
	upload.begin(t, "week 3 notes.txt", content[:1<<20])
	written := upload.completeInBackground(content[1<<20:])

	held.awaitReached(t)
	during := h.onlyUploadBy(t, parties.student)
	held.resume()
	result := upload.answer(t)
	stored := h.onlyUploadBy(t, parties.student)
	object, found := h.storedObject(t, stored.Key)
	sum := sha256.Sum256(content)

	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if during.State != string(domain.AttachmentUploading) || during.Multipart == nil || during.SizeBytes != nil ||
		during.Reserved == nil || *during.Reserved != domain.ChatMaxBytes {
		t.Fatalf("while the second part was held the row was %+v", during)
	}
	uploaded := decodeUploaded(t, result)
	if result.status != http.StatusCreated || uploaded.ID != stored.ID || uploaded.Filename != "week 3 notes.txt" ||
		uploaded.MediaType != "text/plain" || uploaded.SizeBytes != int64(len(content)) || strings.Contains(result.body, stored.Key) {
		t.Fatalf("the upload answered %s", describe(result))
	}
	if stored.State != string(domain.AttachmentPending) || stored.SizeBytes == nil || *stored.SizeBytes != int64(len(content)) ||
		!bytes.Equal(stored.Sum, sum[:]) || stored.MediaType == nil || *stored.MediaType != "text/plain" || stored.Width != nil {
		t.Fatalf("the completed row is %+v", stored)
	}
	if !found || !bytes.Equal(object, content) || len(h.incompleteUploads(t, stored.Key)) != 0 {
		t.Fatalf("the stored object has %d bytes, found %t", len(object), found)
	}
}

func TestAnUploadOverTheLimitIsAbortedAtTheLimit(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	upload := h.startChatUpload(t, parties.studentToken, channel)
	upload.begin(t, "too long.txt", plainText(1<<20))
	h.awaitUploading(t, parties.student)
	written := upload.completeInBackground(plainText(domain.ChatMaxBytes - 1<<20 + 1))

	overLimit := upload.answer(t)
	overLimitRow := h.onlyUploadBy(t, parties.student)
	webp := h.chatUpload(t, parties.studentToken, channel, "sticker.webp", largeWebP(t, domain.ChatWebPMaxBytes))
	declared := h.rawRequest(t, "POST "+chatScope(channel)+" HTTP/1.1\r\nHost: ovc.example\r\nAuthorization: Bearer "+parties.studentToken+
		"\r\nContent-Type: multipart/form-data; boundary=declared\r\nContent-Length: "+fmt.Sprint(domain.ChatMaxBytes+domain.UploadOverheadBytes+1)+
		"\r\nConnection: close\r\n\r\n")
	uploads := h.uploadsBy(t, parties.student)

	if err := <-written; err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Logf("the client stopped writing after the refusal: %v", err)
	}
	if overLimit.status != http.StatusRequestEntityTooLarge || code(t, overLimit) != "payload_too_large" {
		t.Fatalf("the oversized upload answered %s", describe(overLimit))
	}
	h.assertLeftNothing(t, overLimitRow)
	if webp.status != http.StatusRequestEntityTooLarge || fieldList(t, webp) != "body.file:webp_too_large" {
		t.Fatalf("the WebP over 8 MiB answered %s", describe(webp))
	}
	h.assertLeftNothing(t, uploads[1])
	if declared.status != http.StatusRequestEntityTooLarge || len(uploads) != 2 {
		t.Fatalf("a declared length over the limit answered %s with %d rows", describe(declared), len(uploads))
	}
}

func TestADisconnectedOrStalledUploadAbortsItsMultipartUpload(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		cutOff func(t *testing.T, h *authHarness, upload *streamingUpload)
		reason string
	}{
		{"a stalled body", func(t *testing.T, h *authHarness, _ *streamingUpload) {}, string(domain.RejectedStalled)},
		{"a disconnected client", func(t *testing.T, _ *authHarness, upload *streamingUpload) {
			if err := upload.pipe.CloseWithError(errors.New("the client went away")); err != nil {
				t.Fatal(err)
			}
		}, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			h := newStallHarness(t)
			parties := h.activeStudentInAGroup(t)
			channel := h.directChannel(t, parties.card.ID).ID
			upload := h.startChatUpload(t, parties.studentToken, channel)
			upload.begin(t, "slow.txt", plainText(1<<10))
			started := h.awaitUploading(t, parties.student)

			testCase.cutOff(t, h, upload)
			h.waitFor(t, "the abandoned upload's row to be deleted", func() bool {
				return h.uploadState(t, started.ID) == string(domain.AttachmentDeleted)
			})
			outcome := upload.outcome(t)

			if outcome.err == nil && outcome.result.status != http.StatusBadRequest {
				t.Fatalf("the abandoned upload answered %s", describe(outcome.result))
			}
			if testCase.reason != "" && h.countLogs(`"msg":"upload.rejected"`, `"reason":"`+testCase.reason+`"`) != 1 {
				t.Fatalf("no %s rejection was recorded", testCase.reason)
			}
			h.assertLeftNothing(t, started)
		})
	}
}

func TestAnUploadingRowLeftByACrashIsCleanedUp(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	crashed, recent := fixtureID(1, 1), fixtureID(1, 2)
	key := domain.ChatKeyPrefix + crashed
	multipartUpload, err := h.objects.CreateMultipartUpload(context.Background(), key, "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.objects.UploadPart(context.Background(), key, multipartUpload, 1, plainText(1<<10)); err != nil {
		t.Fatal(err)
	}
	h.insertUploading(t, crashed, parties.student, channel, parties.tutor, 1<<10, string(multipartUpload), authStart)
	h.insertUploading(t, recent, parties.student, channel, parties.tutor, 1<<10, "", authStart.Add(15*time.Minute))
	h.at(t, authStart.Add(domain.UploadDeadlineCeiling+domain.UploadAgeAfterDeadline+time.Minute))

	h.tick(t)

	if state := h.uploadState(t, crashed); state != string(domain.AttachmentDeleted) && state != "gone" {
		t.Fatalf("the crashed upload's row is %s", state)
	}
	if !h.deletionQueued(t, crashed) || len(h.incompleteUploads(t, key)) != 0 {
		t.Fatal("the crashed upload's multipart upload was not aborted or its deletion not queued")
	}
	if h.uploadState(t, recent) != string(domain.AttachmentUploading) || h.deletionQueued(t, recent) {
		t.Fatal("an upload younger than the age rule was cleaned up")
	}
}

func TestADisguisedFileIsRefused(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	withNul := plainText(9 << 20)
	withNul[9<<20-10] = 0
	genericZip := zipOf(t, map[string]string{"photos/one.jpg": "x"}, []string{"photos/one.jpg"})
	cases := []struct {
		id       byte
		name     string
		filename string
		data     []byte
	}{
		{1, "a PNG named as a JPEG", "photo.jpg", pngBytes(t, 3, 3)},
		{2, "markup named as a page", "page.html", []byte("<script>alert(1)</script>")},
		{3, "markup named as a picture", "drawing.svg", []byte("<svg onload=alert(1)>")},
		{4, "text without an extension", "README", []byte("plain words")},
		{5, "a program", "setup.exe", []byte("MZ\x90\x00program")},
		{6, "a generic ZIP named as a document", "essay.docx", genericZip},
		{7, "text with a NUL beyond its first part", "notes.txt", withNul},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			parties := h.uploadRefusalParties(t, testCase.id)
			channel := h.directChannel(t, parties.card.ID).ID

			result := h.chatUpload(t, parties.studentToken, channel, testCase.filename, testCase.data)
			uploads := h.uploadsBy(t, parties.student)

			if result.status != http.StatusUnsupportedMediaType || code(t, result) != "unsupported_media_type" || len(uploads) != 1 {
				t.Fatalf("the disguised file answered %s with rows %+v", describe(result), uploads)
			}
			h.assertLeftNothing(t, uploads[0])
		})
	}
}

func TestUploadBodiesWithExtraPartsOrOversizedHeadersAreRefused(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	const boundary = "chat-boundary"
	partOf := func(name string, filename string, extraHeaders string, body string) string {
		disposition := `form-data; name="` + name + `"`
		if filename != "" {
			disposition += `; filename="` + filename + `"`
		}
		return "--" + boundary + "\r\nContent-Disposition: " + disposition + "\r\n" + extraHeaders + "\r\n" + body + "\r\n"
	}
	end := "--" + boundary + "--\r\n"
	contentType := "multipart/form-data; boundary=" + boundary
	cases := []struct {
		id     byte
		name   string
		body   string
		status int
		fields string
		row    bool
	}{
		{1, "two file parts", partOf("file", "a.txt", "", "first") + partOf("file", "b.txt", "", "second") + end, http.StatusUnprocessableEntity, "body.file:invalid", true},
		{2, "a file then a field", partOf("file", "a.txt", "", "first") + partOf("note", "", "", "hello") + end, http.StatusUnprocessableEntity, "body.file:invalid", true},
		{3, "a field before the file", partOf("note", "", "", "hello") + partOf("file", "a.txt", "", "first") + end, http.StatusUnprocessableEntity, "body.file:required", false},
		{4, "part headers over 8 KiB", partOf("file", "a.txt", "X-Padding: "+strings.Repeat("p", 9<<10)+"\r\n", "first") + end,
			http.StatusUnprocessableEntity, "body.file:required", false},
		{5, "no part at all", end, http.StatusUnprocessableEntity, "body.file:required", false},
		{6, "a malformed body", "--" + boundary + "\r\nnot a header\r\n", http.StatusBadRequest, "", false},
		{7, "a malformed trailing part", partOf("file", "a.txt", "", "first") + "--" + boundary + "\r\nnot a header\r\n", http.StatusBadRequest, "", true},
		{8, "a second part with headers beyond the allowance", partOf("file", "a.txt", "", "first") +
			partOf("note", "", "X-Padding: "+strings.Repeat("p", 64<<10)+"\r\n", "hello") + end, http.StatusUnprocessableEntity, "body.file:invalid", true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			parties := h.uploadRefusalParties(t, testCase.id)
			channel := h.directChannel(t, parties.card.ID).ID

			result := h.rawChatUpload(t, parties.studentToken, channel, testCase.body, contentType)
			uploads := h.uploadsBy(t, parties.student)

			if result.status != testCase.status || (testCase.fields != "" && fieldList(t, result) != testCase.fields) {
				t.Fatalf("the body answered %s", describe(result))
			}
			if (len(uploads) == 1) != testCase.row || len(uploads) > 1 {
				t.Fatalf("the body left rows %+v", uploads)
			}
			for _, upload := range uploads {
				h.assertLeftNothing(t, upload)
			}
		})
	}
}

func TestUploadConcurrencyPendingAndHourlyCapsAreEnforced(t *testing.T) {
	t.Parallel()
	t.Run("four concurrent uploads per user", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		channel := h.directChannel(t, parties.card.ID).ID
		for index := range 4 {
			upload := h.startChatUpload(t, parties.studentToken, channel)
			upload.begin(t, fmt.Sprintf("part %d.txt", index), plainText(1<<10))
		}
		h.waitFor(t, "four uploading rows", func() bool { return len(h.uploadsBy(t, parties.student)) == 4 })

		fifth := h.chatUpload(t, parties.studentToken, channel, "fifth.txt", plainText(64))

		if fifth.status != http.StatusTooManyRequests || code(t, fifth) != "rate_limited" || fifth.headers.Get("Retry-After") == "" {
			t.Fatalf("the fifth concurrent upload answered %s", describe(fifth))
		}
	})
	t.Run("twenty open uploads per user", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		channel := h.directChannel(t, parties.card.ID).ID
		for index := range domain.ChatOpenUploadsPerUser - 1 {
			h.insertCompleted(t, fixtureID(2, byte(index)), parties.student, channel, parties.tutor, 10, domain.AttachmentPending, authStart)
		}
		h.insertCompleted(t, fixtureID(3, 1), parties.student, channel, parties.tutor, 10, domain.AttachmentPending, authStart.Add(-25*time.Hour))
		h.insertUploading(t, fixtureID(3, 2), parties.student, channel, parties.tutor, 10, "", authStart)

		twentyFirst := h.chatUpload(t, parties.studentToken, channel, "one more.txt", plainText(64))

		if twentyFirst.status != http.StatusTooManyRequests || code(t, twentyFirst) != "rate_limited" ||
			h.countLogs(`"msg":"chat.rate_limited"`, `"limit":"pending_uploads"`) != 1 {
			t.Fatalf("the twenty-first open upload answered %s", describe(twentyFirst))
		}
	})
	t.Run("sixty uploads per user and hour", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		channel := h.groupChannel(t, parties.group.ID).ID
		for index := range domain.ChatUploadsPerHour {
			h.mustAnswer(t, h.chatUpload(t, parties.tutorToken, channel, fmt.Sprintf("handout %d.txt", index), plainText(64)), http.StatusCreated)
			h.exec(t, "UPDATE attachments SET state = 'deleted', expires_at = NULL WHERE uploader_id = $1::uuid", parties.tutor.String())
		}

		sixtyFirst := h.chatUpload(t, parties.tutorToken, channel, "handout extra.txt", plainText(64))
		h.at(t, authStart.Add(time.Hour))
		nextHour := h.chatUpload(t, h.relogin(t, tutorEmail), channel, "handout next.txt", plainText(64))

		if sixtyFirst.status != http.StatusTooManyRequests || h.countLogs(`"msg":"chat.rate_limited"`, `"limit":"hourly_uploads"`) != 1 {
			t.Fatalf("the sixty-first upload in an hour answered %s", describe(sixtyFirst))
		}
		if nextHour.status != http.StatusCreated {
			t.Fatalf("an upload in the next hour answered %s", describe(nextHour))
		}
	})
}

func TestChatUploadsNeedAWritableChannelMembershipAtBothSessions(t *testing.T) {
	t.Parallel()
	t.Run("a channel outside the caller's scope", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		world := h.sharedChatWorld(t)

		outOfScope := h.chatUpload(t, world.secondToken, world.direct.ID, "notes.txt", plainText(64))
		random := h.chatUpload(t, world.secondToken, randomChannelID, "notes.txt", plainText(64))
		malformed := h.send(t, http.MethodPost, uploadsRoute+"?context=chat&scope=group:"+world.group.ID, "", jsonCall(world.secondToken))

		if outOfScope.status != http.StatusNotFound || refusal(t, outOfScope) != refusal(t, random) {
			t.Fatalf("another tutor's channel answered %s", describe(outOfScope))
		}
		if malformed.status != http.StatusUnprocessableEntity || fieldList(t, malformed) != "query.scope:invalid" {
			t.Fatalf("a malformed scope answered %s", describe(malformed))
		}
	})
	t.Run("a read-only channel", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)

		readOnly := h.chatUpload(t, parties.studentToken, h.directChannel(t, parties.card.ID).ID, "notes.txt", plainText(64))

		if readOnly.status != http.StatusConflict || code(t, readOnly) != "channel_read_only" || len(h.uploadsBy(t, parties.student)) != 0 {
			t.Fatalf("a read-only channel answered %s", describe(readOnly))
		}
	})
	t.Run("membership lost while the body streams", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		upload := h.startChatUpload(t, parties.studentToken, h.groupChannel(t, parties.group.ID).ID)
		upload.begin(t, "notes.txt", plainText(1<<10))
		started := h.awaitUploading(t, parties.student)

		h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID)), http.StatusOK)
		result := upload.finish(t, plainText(1<<10))

		if result.status != http.StatusNotFound || h.countLogs(`"msg":"upload.rejected"`, `"reason":"scope"`) != 1 {
			t.Fatalf("an upload whose member left answered %s", describe(result))
		}
		h.assertLeftNothing(t, started)
	})
	t.Run("a member row suspended while the body streams", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		channel := h.directChannel(t, parties.card.ID).ID
		upload := h.startChatUpload(t, parties.studentToken, channel)
		upload.begin(t, "notes.txt", plainText(1<<10))
		started := h.awaitUploading(t, parties.student)

		h.exec2(t, "UPDATE channel_members SET suspended_since = now() WHERE channel_id = $1::uuid AND user_id = $2::uuid", channel, parties.student.String())
		result := upload.finish(t, plainText(1<<10))

		if result.status != http.StatusNotFound {
			t.Fatalf("an upload whose member row was suspended answered %s", describe(result))
		}
		h.assertLeftNothing(t, started)
	})
	t.Run("a removal committed while the second session waits for the channel lock", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		channel := h.groupChannel(t, parties.group.ID).ID
		upload := h.startChatUpload(t, parties.studentToken, channel)
		upload.begin(t, "notes.txt", plainText(1<<10))
		started := h.awaitUploading(t, parties.student)
		removal := h.holdChannelRemoval(t, channel, parties.student)

		written := upload.completeInBackground(plainText(1 << 10))
		h.awaitLockWaiters(t, 1)
		removal.commit(t)
		result := upload.answer(t)

		if err := <-written; err != nil {
			t.Fatal(err)
		}
		if result.status != http.StatusNotFound || h.countLogs(`"msg":"upload.rejected"`, `"reason":"scope"`) != 1 {
			t.Fatalf("an upload whose member was removed during its lock wait answered %s", describe(result))
		}
		h.assertLeftNothing(t, started)
	})
	t.Run("a channel that became read-only while the body streams", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		upload := h.startChatUpload(t, parties.studentToken, h.directChannel(t, parties.card.ID).ID)
		upload.begin(t, "notes.txt", plainText(1<<10))
		started := h.awaitUploading(t, parties.student)

		h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
		result := upload.finish(t, plainText(1<<10))

		if result.status != http.StatusConflict || code(t, result) != "channel_read_only" {
			t.Fatalf("an upload whose channel became read-only answered %s", describe(result))
		}
		h.assertLeftNothing(t, started)
	})
}

func TestPendingUploadsCountTowardTheQuota(t *testing.T) {
	t.Parallel()
	const quota = 1_000_000
	cases := []struct {
		name     string
		arrange  func(t *testing.T, h *authHarness, parties endingParties, channel string)
		size     int
		status   int
		streamed bool
	}{
		{"a quota with no room left is refused before streaming", func(t *testing.T, h *authHarness, parties endingParties, _ string) {
			h.setQuota(t, parties.tutor, quota, quota)
		}, 100, http.StatusInsufficientStorage, false},
		{"an uploading row counts with its reserved bytes", func(t *testing.T, h *authHarness, parties endingParties, channel string) {
			h.setQuota(t, parties.tutor, quota, 0)
			h.insertUploading(t, fixtureID(4, 1), parties.tutor, channel, parties.tutor, quota, "", authStart)
		}, 100, http.StatusInsufficientStorage, false},
		{"a pending row counts at completion, which deletes the object", func(t *testing.T, h *authHarness, parties endingParties, channel string) {
			h.setQuota(t, parties.tutor, quota, 0)
			h.insertCompleted(t, fixtureID(4, 2), parties.tutor, channel, parties.tutor, 700_000, domain.AttachmentPending, authStart)
		}, 400_000, http.StatusInsufficientStorage, true},
		{"the completing row is excluded and counts with its final size", func(t *testing.T, h *authHarness, parties endingParties, _ string) {
			h.setQuota(t, parties.tutor, quota, 50_000)
		}, 900_000, http.StatusCreated, true},
		{"charged usage counts too", func(t *testing.T, h *authHarness, parties endingParties, _ string) {
			h.setQuota(t, parties.tutor, quota, 200_000)
		}, 900_000, http.StatusInsufficientStorage, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			h := newUploadHarness(t)
			parties := h.activeStudentInAGroup(t)
			channel := h.directChannel(t, parties.card.ID).ID
			testCase.arrange(t, h, parties, channel)

			result := h.chatUpload(t, parties.studentToken, channel, "essay.txt", plainText(testCase.size))
			uploads := h.uploadsBy(t, parties.student)

			if result.status != testCase.status || (len(uploads) == 1) != testCase.streamed {
				t.Fatalf("the upload answered %s with rows %+v", describe(result), uploads)
			}
			if testCase.status == http.StatusInsufficientStorage && (code(t, result) != "quota_exceeded" || h.countLogs(`"msg":"quota.exceeded"`) != 1) {
				t.Fatalf("the quota refusal is %s", describe(result))
			}
			if testCase.status != http.StatusCreated {
				for _, upload := range uploads {
					h.assertLeftNothing(t, upload)
				}
			}
		})
	}
	t.Run("two racing completions are bounded at binding", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		channel := h.directChannel(t, parties.card.ID).ID
		h.setQuota(t, parties.tutor, 100*quota, 0)
		studentUpload := h.startChatUpload(t, parties.studentToken, channel)
		tutorUpload := h.startChatUpload(t, parties.tutorToken, channel)
		studentUpload.begin(t, "student.txt", plainText(400_000))
		tutorUpload.begin(t, "tutor.txt", plainText(400_000))
		studentDone, tutorDone := studentUpload.completeInBackground(nil), tutorUpload.completeInBackground(nil)
		for _, done := range []<-chan error{studentDone, tutorDone} {
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
		completed := []response{studentUpload.answer(t), tutorUpload.answer(t)}
		h.setQuota(t, parties.tutor, quota, 300_000)
		release := h.holdTutorProfile(t, parties.tutor)

		studentSend := h.sendMessageInBackground(parties.studentToken, channel, "Mine", []string{uploadedID(t, completed[0])})
		h.waitForLockWaiters(t, 1)
		tutorSend := h.sendMessageInBackground(parties.tutorToken, channel, "Mine too", []string{uploadedID(t, completed[1])})
		h.waitForLockWaiters(t, 2)
		release()
		answers := []response{awaitAnswer(t, studentSend), awaitAnswer(t, tutorSend)}

		for _, upload := range completed {
			h.mustAnswer(t, upload, http.StatusCreated)
		}
		statuses := []int{answers[0].status, answers[1].status}
		slices.Sort(statuses)
		if !slices.Equal(statuses, []int{http.StatusCreated, http.StatusInsufficientStorage}) || h.usage(t, parties.tutor) != 700_000 {
			t.Fatalf("the racing bindings answered %v and charged %d bytes", statuses, h.usage(t, parties.tutor))
		}
	})
}

func TestTheDailyAttachmentCapRefusesWithRateLimited(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		arrange  func(t *testing.T, h *authHarness, parties endingParties, channel string)
		uploader func(parties endingParties) string
		status   int
		streamed bool
	}{
		{"a day already at its cap refuses before streaming", func(t *testing.T, h *authHarness, parties endingParties, channel string) {
			h.insertCompleted(t, fixtureID(5, 1), parties.student, channel, parties.tutor, 150_000_000, domain.AttachmentDeleted, authStart)
			h.insertCompleted(t, fixtureID(5, 2), parties.student, channel, parties.tutor, 50_000_000, domain.AttachmentPending, authStart)
		}, func(parties endingParties) string { return parties.studentToken }, http.StatusTooManyRequests, false},
		{"an upload that would pass the cap is refused at completion", func(t *testing.T, h *authHarness, parties endingParties, channel string) {
			h.insertCompleted(t, fixtureID(5, 3), parties.student, channel, parties.tutor, domain.ChatDailyBytesPerTutor-10_000, domain.AttachmentPending, authStart)
		}, func(parties endingParties) string { return parties.studentToken }, http.StatusTooManyRequests, true},
		{"yesterday's bytes do not count", func(t *testing.T, h *authHarness, parties endingParties, channel string) {
			h.insertCompleted(t, fixtureID(5, 4), parties.student, channel, parties.tutor, domain.ChatDailyBytesPerTutor, domain.AttachmentDeleted,
				authStart.Add(-24*time.Hour))
		}, func(parties endingParties) string { return parties.studentToken }, http.StatusCreated, true},
		{"the tutor's own uploads are not capped", func(t *testing.T, h *authHarness, parties endingParties, channel string) {
			h.insertCompleted(t, fixtureID(5, 5), parties.tutor, channel, parties.tutor, domain.ChatDailyBytesPerTutor, domain.AttachmentDeleted, authStart)
		}, func(parties endingParties) string { return parties.tutorToken }, http.StatusCreated, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			h := newUploadHarness(t)
			parties := h.activeStudentInAGroup(t)
			channel := h.directChannel(t, parties.card.ID).ID
			testCase.arrange(t, h, parties, channel)
			before := len(h.uploadsBy(t, parties.student)) + len(h.uploadsBy(t, parties.tutor))

			result := h.chatUpload(t, testCase.uploader(parties), channel, "scan.txt", plainText(20_000))
			created := len(h.uploadsBy(t, parties.student)) + len(h.uploadsBy(t, parties.tutor)) - before

			if result.status != testCase.status || (created == 1) != testCase.streamed {
				t.Fatalf("the upload answered %s and created %d rows", describe(result), created)
			}
			if testCase.status == http.StatusTooManyRequests &&
				(result.headers.Get("Retry-After") == "" || h.countLogs(`"msg":"chat.rate_limited"`, `"limit":"attachment_bytes"`) != 1) {
				t.Fatalf("the daily cap refusal is %s with Retry-After %q", describe(result), result.headers.Get("Retry-After"))
			}
		})
	}
}

func TestUnboundChatUploadsExpire(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.sharedChatWorld(t)
	direct := world.direct.ID
	uploaded := h.chatUpload(t, world.parties.studentToken, direct, "draft.txt", plainText(128))
	h.mustAnswer(t, uploaded, http.StatusCreated)
	expiring := h.onlyUploadBy(t, world.parties.student)
	missingChannel, notMember, bindable := fixtureID(6, 1), fixtureID(6, 2), fixtureID(6, 3)
	h.insertCompleted(t, missingChannel, world.parties.student, randomChannelID, world.parties.tutor, 10, domain.AttachmentPending, authStart.Add(time.Hour))
	h.insertCompleted(t, notMember, world.parties.student, world.secondChat.ID, world.parties.tutor, 10, domain.AttachmentPending, authStart.Add(time.Hour))
	h.exec(t, "DELETE FROM channel_members WHERE channel_id = $1::uuid AND user_id <> (SELECT tutor_id FROM channels WHERE id = $1::uuid)", world.secondChat.ID)
	h.insertCompleted(t, bindable, world.parties.student, direct, world.parties.tutor, 10, domain.AttachmentPending, authStart.Add(time.Hour))

	h.at(t, authStart.Add(domain.PendingUploadLifetime+time.Second))
	h.tick(t)
	h.tick(t)

	if state := h.uploadState(t, expiring.ID); state != "gone" {
		t.Fatalf("the expired upload's row is %s", state)
	}
	if _, stored := h.storedObject(t, expiring.Key); stored {
		t.Fatal("the expired upload kept its object")
	}
	for _, unbindable := range []string{missingChannel, notMember} {
		if state := h.uploadState(t, unbindable); state != "gone" && state != string(domain.AttachmentDeleted) {
			t.Fatalf("an unbindable pending upload is %s", state)
		}
	}
	if h.uploadState(t, bindable) != string(domain.AttachmentPending) {
		t.Fatal("a bindable pending upload was removed before its expiry")
	}
}

func TestATutorPurgeRemovesTheStudentsUnboundUploadsChargedToTheTutor(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	h.mustAnswer(t, h.chatUpload(t, parties.studentToken, h.directChannel(t, parties.card.ID).ID, "draft.txt", plainText(64)), http.StatusCreated)
	pending := h.onlyUploadBy(t, parties.student)
	h.insertUploading(t, fixtureID(11, 1), parties.student, h.groupChannel(t, parties.group.ID).ID, parties.tutor, 64, "", authStart)
	h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)

	h.at(t, authStart.Add(domain.DeletedAccountRetention+time.Hour))
	h.tick(t)
	h.tick(t)

	if h.countRows(t, "SELECT count(id) FROM users WHERE id = $1::uuid", parties.tutor.String()) != 0 {
		t.Fatal("the tutor was not purged")
	}
	if h.uploadState(t, pending.ID) != "gone" || h.uploadState(t, fixtureID(11, 1)) != "gone" {
		t.Fatal("the student's uploads charged to the purged tutor survived")
	}
	if _, stored := h.storedObject(t, pending.Key); stored {
		t.Fatal("the student's upload kept its object after the tutor's purge")
	}
}

func TestThirtyTwoConcurrentUploadsFillTheInstance(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	uploaders := []string{tutorToken}
	channels := []string{}
	var students []domain.UserID
	for index := range 8 {
		email := fmt.Sprintf("student-%d@example.org", index)
		student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, byte(20+index), email)
		card := h.linkedSignedIn(t, tutorToken, studentToken, fmt.Sprintf("Card %d", index), email)
		channels = append(channels, h.directChannel(t, card.ID).ID)
		uploaders = append(uploaders, studentToken)
		students = append(students, student)
	}
	holding := []domain.UserID{tutor}
	holding = append(holding, students[:7]...)
	var streams []*streamingUpload
	for index, token := range uploaders[:8] {
		channel := channels[max(index-1, 0)]
		for range 4 {
			stream := h.startChatUpload(t, token, channel)
			stream.begin(t, "held.txt", plainText(1<<10))
			streams = append(streams, stream)
		}
	}
	h.waitFor(t, "thirty-two uploading rows", func() bool {
		total := 0
		for _, user := range holding {
			total += len(h.uploadsBy(t, user))
		}
		return total == 32
	})

	thirtyThird := h.chatUpload(t, uploaders[8], channels[7], "one more.txt", plainText(64))
	for _, stream := range streams {
		h.mustAnswer(t, stream.finish(t, nil), http.StatusCreated)
	}
	afterRelease := h.chatUpload(t, uploaders[8], channels[7], "after.txt", plainText(64))

	if thirtyThird.status != http.StatusTooManyRequests || code(t, thirtyThird) != "rate_limited" || thirtyThird.headers.Get("Retry-After") == "" ||
		len(h.uploadsBy(t, students[7])) != 1 {
		t.Fatalf("the thirty-third concurrent upload answered %s", describe(thirtyThird))
	}
	if afterRelease.status != http.StatusCreated {
		t.Fatalf("an upload after the instance's uploads ended answered %s", describe(afterRelease))
	}
}

func TestAPurgeAbortsTheRecordedMultipartUploadsOfItsUploadingRows(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	streaming := fixtureID(12, 1)
	key := domain.ChatKeyPrefix + streaming
	multipartUpload, err := h.objects.CreateMultipartUpload(context.Background(), key, "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.objects.UploadPart(context.Background(), key, multipartUpload, 1, plainText(1<<10)); err != nil {
		t.Fatal(err)
	}
	h.insertUploading(t, streaming, parties.student, h.directChannel(t, parties.card.ID).ID, parties.tutor, 1<<10, string(multipartUpload),
		authStart.Add(domain.DeletedAccountRetention+time.Hour))
	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)

	h.at(t, authStart.Add(domain.DeletedAccountRetention+time.Hour))
	h.tick(t)
	h.tick(t)

	if h.countRows(t, "SELECT count(id) FROM users WHERE id = $1::uuid", parties.student.String()) != 0 || h.uploadState(t, streaming) != "gone" {
		t.Fatal("the student or the uploading row survived the purge")
	}
	if incomplete := h.incompleteUploads(t, key); len(incomplete) != 0 {
		t.Fatalf("the purged uploading row left its multipart upload %v", incomplete)
	}
}

func TestTheAgeRuleQueuesEveryAbortWithoutCallingStorage(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	var keys []string
	for index := range 3 {
		id := fixtureID(13, byte(index))
		key := domain.ChatKeyPrefix + id
		multipartUpload, err := h.objects.CreateMultipartUpload(context.Background(), key, "application/octet-stream")
		if err != nil {
			t.Fatal(err)
		}
		h.insertUploading(t, id, parties.student, channel, parties.tutor, 64, string(multipartUpload), authStart)
		keys = append(keys, key)
	}
	unbindable := fixtureID(13, 9)
	h.insertCompleted(t, unbindable, parties.student, randomChannelID, parties.tutor, 10, domain.AttachmentPending, authStart.Add(time.Hour))
	held.holdNext(func(*http.Request) bool { return true })
	later := authStart.Add(domain.UploadDeadlineCeiling + domain.UploadAgeAfterDeadline + time.Minute)

	type expiryRun struct {
		expired int64
		err     error
	}
	finished := make(chan expiryRun, 1)
	go func() {
		expired, err := h.files.ExpirePending(context.Background(), later)
		finished <- expiryRun{expired: expired, err: err}
	}()
	var run expiryRun
	reachedStorage := false
	select {
	case run = <-finished:
	case <-held.reached:
		reachedStorage = true
	case <-time.After(pauseDeadline):
		t.Fatal("the expiry run neither finished nor reached storage")
	}
	held.resume()
	if reachedStorage {
		run = <-finished
	}
	expired, err := run.expired, run.err
	h.at(t, later)
	h.tick(t)

	if err != nil || expired != 4 || reachedStorage {
		t.Fatalf("the expiry run removed %d rows and reached storage %t: %v", expired, reachedStorage, err)
	}
	for _, key := range keys {
		if incomplete := h.incompleteUploads(t, key); len(incomplete) != 0 {
			t.Fatalf("the queued deletion left the multipart upload %v", incomplete)
		}
	}
}

func TestUploadCompletionAfterARevokedSessionDiscardsTheObject(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	upload := h.startChatUpload(t, parties.studentToken, h.directChannel(t, parties.card.ID).ID)
	upload.begin(t, "notes.txt", plainText(1<<10))
	started := h.awaitUploading(t, parties.student)

	h.mustAnswer(t, h.send(t, http.MethodPost, "/api/v1/auth/logout-all", "", jsonCall(parties.studentToken)), http.StatusNoContent)
	result := upload.finish(t, plainText(1<<10))

	if result.status != http.StatusUnauthorized || code(t, result) != "unauthenticated" ||
		h.countLogs(`"msg":"upload.rejected"`, `"reason":"session_revoked"`) != 1 {
		t.Fatalf("an upload completed after its session was revoked answered %s", describe(result))
	}
	h.assertLeftNothing(t, started)
}

func TestUnconfirmedAccountsCannotUploadToChat(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, squatter := h.squat(t, tutorToken, newcomerEmail, newcomerEmail)
	channel := h.directChannel(t, card.ID).ID
	squatterToken := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	upload := h.startChatUpload(t, squatterToken, channel)

	refused := upload.answer(t)
	byTutor := h.chatUpload(t, tutorToken, channel, "welcome.txt", plainText(64))

	if refused.status != http.StatusForbidden || code(t, refused) != "email_not_verified" || len(h.uploadsBy(t, squatter)) != 0 {
		t.Fatalf("the unconfirmed account's upload answered %s before its body was sent", describe(refused))
	}
	if byTutor.status != http.StatusCreated {
		t.Fatalf("the tutor's upload to the provisional channel answered %s", describe(byTutor))
	}
}
