package api_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const insertPendingHomeworkRow = `INSERT INTO attachments (id, uploader_id, context, scope_kind, scope_id, scope_question_id, quota_tutor_id,
    storage_key, filename, media_type, size_bytes, sha256, state, expires_at, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, $3, 'submission', $4::uuid, $5::uuid, $6::uuid, 'answers/' || $1, 'photo.pdf', 'application/pdf', $7::bigint,
    sha256('x'), 'pending', $8::timestamptz + interval '1 day', $8::timestamptz, $8::timestamptz)`

type homeworkUploadParties struct {
	endingParties
	issue           string
	submission      string
	photoQuestion   string
	audioQuestion   string
	writtenQuestion string
}

type pendingHomeworkRow struct {
	id       string
	uploader domain.UserID
	context  domain.AttachmentContext
	question string
	size     int64
}

func fileBlock(id string, kinds ...string) templateBlockJSON {
	return templateBlockJSON{ID: id, Type: string(domain.BlockFile), Prompt: text("Send your work"), AllowedKinds: &kinds, MaxFiles: points(5), Points: points(5)}
}

func homeworkUploadBlocks() []templateBlockJSON {
	return []templateBlockJSON{
		fileBlock(blockID(60, 1), string(domain.AnswerFileImages), string(domain.AnswerFilePDF)),
		fileBlock(blockID(60, 2), string(domain.AnswerFileAudio)),
		{ID: blockID(60, 3), Type: string(domain.BlockLongText), Prompt: text("Describe the trip"), MaxLength: points(2000), Points: points(5)},
	}
}

func (h *authHarness) issueFileWork(t *testing.T, token string, card string) (string, string) {
	t.Helper()
	template := h.createTemplate(t, token, "Photo homework "+card[:8], nil, homeworkUploadBlocks()...)
	if !template.Ready {
		t.Fatalf("the file homework template is not ready: %+v", template.Problems)
	}
	issue := h.issued(t, token, issueJSON(template.ID, toCard(card)))
	submissions := h.submissionsOf(t, issue.ID)
	if len(submissions) != 1 {
		t.Fatalf("the file homework has the submissions %+v", submissions)
	}
	return issue.ID, submissions[0].id
}

func (h *authHarness) homeworkUploadParties(t *testing.T) homeworkUploadParties {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	issue, submission := h.issueFileWork(t, parties.tutorToken, parties.card.ID)
	return homeworkUploadParties{
		endingParties: parties, issue: issue, submission: submission, photoQuestion: blockID(60, 1), audioQuestion: blockID(60, 2),
		writtenQuestion: blockID(60, 3),
	}
}

func homeworkRoute(uploaded domain.AttachmentContext, submission string, question string) string {
	return uploadsRoute + "?context=" + string(uploaded) + "&scope=submission:" + submission + ":" + question
}

func (h *authHarness) homeworkUpload(t *testing.T, token string, route string, filename string, data []byte) response {
	t.Helper()
	body, contentType := chatForm(t, filename, data)
	return h.send(t, http.MethodPost, route, body, map[string]string{"Authorization": "Bearer " + token, "Content-Type": contentType})
}

func (h *authHarness) insertPendingHomework(t *testing.T, parties homeworkUploadParties, row pendingHomeworkRow) {
	t.Helper()
	if _, err := h.database.Pool.Exec(t.Context(), insertPendingHomeworkRow, row.id, row.uploader.String(), string(row.context), parties.submission,
		row.question, parties.tutor.String(), row.size, authStart); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) dailyAnswerBytes(t *testing.T, student domain.UserID, tutor domain.UserID, instant time.Time) int64 {
	t.Helper()
	var total int64
	err := h.database.Pool.QueryRow(t.Context(), `SELECT COALESCE(sum(completed_bytes), 0)::bigint FROM daily_answer_upload_bytes
WHERE student_id = $1 AND tutor_id = $2 AND upload_day = date_trunc('day', $3::timestamptz AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`,
		student, tutor, instant).Scan(&total)
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func (h *authHarness) addDailyAnswerBytes(t *testing.T, student domain.UserID, tutor domain.UserID, instant time.Time, size int64) {
	t.Helper()
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO daily_answer_upload_bytes (student_id, tutor_id, upload_day, completed_bytes)
VALUES ($1, $2, date_trunc('day', $3::timestamptz AT TIME ZONE 'UTC') AT TIME ZONE 'UTC', $4)`, student, tutor, instant, size); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) awaitUploadingIn(t *testing.T, uploader domain.UserID, uploaded domain.AttachmentContext) storedUpload {
	t.Helper()
	var found storedUpload
	h.waitFor(t, "a homework row carrying its multipart upload", func() bool {
		for _, upload := range h.uploadsIn(t, uploader, uploaded) {
			if upload.Multipart != nil {
				found = upload
				return true
			}
		}
		return false
	})
	return found
}

func assertRefused(t *testing.T, label string, answer response, status int, errorCode string) {
	t.Helper()
	if answer.status != status || code(t, answer) != errorCode {
		t.Fatalf("%s answered %s", label, describe(answer))
	}
}

func TestAnswerUploadScope(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.homeworkUploadParties(t)
	otherCard := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	_, otherSubmission := h.issueFileWork(t, parties.tutorToken, otherCard)
	_, unconfirmedToken := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 11, newcomerEmail)
	photo := homeworkRoute(domain.ContextAnswer, parties.submission, parties.photoQuestion)

	accepted := h.homeworkUpload(t, parties.studentToken, photo, "page one.pdf", pdfFile(256))
	wrongKind := h.homeworkUpload(t, parties.studentToken, photo, "reading.mp3", mp3File())
	audio := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, parties.submission, parties.audioQuestion), "reading.mp3", mp3File())
	video := h.homeworkUpload(t, parties.studentToken, photo, "lecture.mp4", mp4File(t, "isom"))
	another := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, otherSubmission, parties.photoQuestion), "page.pdf", pdfFile(64))
	written := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, parties.submission, parties.writtenQuestion), "page.pdf", pdfFile(64))
	unknownQuestion := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, parties.submission, blockID(61, 1)), "page.pdf", pdfFile(64))
	byTutor := h.homeworkUpload(t, parties.tutorToken, photo, "page.pdf", pdfFile(64))
	unconfirmed := h.startUploadTo(t, unconfirmedToken, photo).answer(t)
	missingScope := h.homeworkUpload(t, parties.studentToken, uploadsRoute+"?context=answer", "page.pdf", pdfFile(64))
	malformedScope := h.homeworkUpload(t, parties.studentToken, uploadsRoute+"?context=answer&scope=submission:"+parties.submission, "page.pdf", pdfFile(64))
	channelScope := h.homeworkUpload(t, parties.studentToken, uploadsRoute+"?context=answer&scope=channel:"+randomChannelID, "page.pdf", pdfFile(64))
	uploads := h.uploadsIn(t, parties.student, domain.ContextAnswer)

	if accepted.status != http.StatusCreated || decodeUploaded(t, accepted).MediaType != "application/pdf" {
		t.Fatalf("the student's answer upload answered %s", describe(accepted))
	}
	if audio.status != http.StatusCreated || decodeUploaded(t, audio).MediaType != "audio/mpeg" {
		t.Fatalf("an audio answer to an audio question answered %s", describe(audio))
	}
	assertRefused(t, "an audio answer to a picture and PDF question", wrongKind, http.StatusUnsupportedMediaType, "unsupported_media_type")
	assertRefused(t, "a video answer", video, http.StatusUnsupportedMediaType, "unsupported_media_type")
	for label, refused := range map[string]response{
		"another student's submission": another, "a written question": written, "an unknown question": unknownQuestion, "the tutor's answer upload": byTutor,
	} {
		assertRefused(t, label, refused, http.StatusNotFound, "not_found")
	}
	assertRefused(t, "an unconfirmed student's answer upload", unconfirmed, http.StatusForbidden, "email_not_verified")
	if missingScope.status != http.StatusUnprocessableEntity || fieldList(t, missingScope) != "query.scope:required" {
		t.Fatalf("an answer upload without a scope answered %s", describe(missingScope))
	}
	for _, refused := range []response{malformedScope, channelScope} {
		if refused.status != http.StatusUnprocessableEntity || fieldList(t, refused) != "query.scope:invalid" {
			t.Fatalf("an answer upload with a malformed scope answered %s", describe(refused))
		}
	}
	scoped := h.countRows(t, `SELECT count(id) FROM attachments WHERE uploader_id = $1::uuid AND context = 'answer' AND state = 'pending'
AND scope_kind = 'submission' AND scope_id IS NOT NULL AND scope_question_id IS NOT NULL AND quota_tutor_id <> uploader_id AND storage_key LIKE 'answers/%'`,
		parties.student.String())
	if scoped != 2 || len(uploadsInState(uploads, domain.AttachmentDeleted)) != 2 {
		t.Fatalf("the answer uploads left the rows %+v", uploads)
	}
	for _, upload := range uploadsInState(uploads, domain.AttachmentDeleted) {
		h.assertLeftNothing(t, upload)
	}
	if h.countLogs(`"msg":"upload.accepted"`, `"context":"answer"`, `"submission_id":"`+parties.submission+`"`) != 2 ||
		h.countLogs(`"msg":"upload.rejected"`, `"context":"answer"`, `"reason":"scope"`) != 4 || h.countLogs(`"context":"answer"`, "channel_id") != 0 {
		t.Fatal("the answer upload records do not carry their context and submission")
	}
	if h.countLogs("page one") != 0 || h.countLogs(domain.AnswerKeyPrefix) != 0 {
		t.Fatal("an answer upload record carries a file name or a storage key")
	}
	if h.capacityUsed(t, domain.UploadCapacityHomework) != 0 {
		t.Fatal("finished answer uploads kept homework capacity reserved")
	}
}

func TestAnswerUploadScopeFollowsTheSubmissionAndTheLink(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		arrange func(t *testing.T, h *authHarness, parties homeworkUploadParties)
		status  int
		code    string
	}{
		{"a reclaim-pending link hides the work", func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.setRelationshipState(t, parties.card.ID, domain.RelationshipReclaimPending)
		}, http.StatusNotFound, "not_found"},
		{"an issue marked for erasure hides the work", func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.markIssueForErasure(t, parties.issue)
		}, http.StatusNotFound, "not_found"},
		{"a withdrawn issue hides the work", func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.mustAnswer(t, h.withdrawIssue(t, parties.tutorToken, parties.issue), http.StatusOK)
		}, http.StatusNotFound, "not_found"},
		{"submitted work is not editable", func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.submitStored(t, parties.submission, authStart)
		}, http.StatusConflict, "not_editable"},
		{"an ended link keeps the work readable but not editable", func(t *testing.T, h *authHarness, parties homeworkUploadParties) {
			h.setRelationshipState(t, parties.card.ID, domain.RelationshipEnded)
		}, http.StatusConflict, "not_editable"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			h := newUploadHarness(t)
			parties := h.homeworkUploadParties(t)
			testCase.arrange(t, h, parties)

			refused := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, parties.submission, parties.photoQuestion), "page.pdf", pdfFile(64))

			assertRefused(t, testCase.name, refused, testCase.status, testCase.code)
			if len(h.uploadsIn(t, parties.student, domain.ContextAnswer)) != 0 {
				t.Fatal("a refused answer upload left a row")
			}
		})
	}
	t.Run("a submission submitted while the file streamed refuses the completion", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.homeworkUploadParties(t)
		upload := h.startUploadTo(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, parties.submission, parties.photoQuestion))
		upload.begin(t, "page.pdf", pdfFile(1<<10))
		started := h.awaitUploadingIn(t, parties.student, domain.ContextAnswer)
		h.submitStored(t, parties.submission, authStart)

		refused := upload.finish(t, []byte("0"))
		uploads := h.uploadsIn(t, parties.student, domain.ContextAnswer)

		assertRefused(t, "an answer upload completed after the submission", refused, http.StatusConflict, "not_editable")
		if len(uploads) != 1 || uploads[0].State == string(domain.AttachmentPending) {
			t.Fatalf("the late answer upload left %+v", uploads)
		}
		h.assertLeftNothing(t, started)
		if h.countLogs(`"msg":"upload.rejected"`, `"context":"answer"`, `"reason":"not_editable"`) != 1 {
			t.Fatal("the late answer upload did not record its refusal")
		}
	})
}

func TestAnswerUploadDailyBytes(t *testing.T) {
	t.Parallel()
	t.Run("an upload starts only below two hundred megabytes completed that day", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.homeworkUploadParties(t)
		h.addDailyAnswerBytes(t, parties.student, parties.tutor, authStart, domain.AnswerDailyBytesPerTutor)

		full := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, parties.submission, parties.photoQuestion), "page.pdf", pdfFile(64))

		if full.status != http.StatusTooManyRequests || code(t, full) != "rate_limited" || full.headers.Get("Retry-After") == "" ||
			h.countLogs(`"msg":"upload.rejected"`, `"context":"answer"`, `"reason":"daily_cap"`) != 1 || h.countLogs(`"msg":"chat.rate_limited"`) != 0 {
			t.Fatalf("an answer upload past the daily bytes answered %s", describe(full))
		}
	})
	t.Run("a full earlier day does not stop the next day's upload, which counts its bytes", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.homeworkUploadParties(t)
		h.addDailyAnswerBytes(t, parties.student, parties.tutor, authStart, domain.AnswerDailyBytesPerTutor)
		h.at(t, authStart.Add(24*time.Hour))
		token := h.relogin(t, studentEmail)

		nextDay := h.homeworkUpload(t, token, homeworkRoute(domain.ContextAnswer, parties.submission, parties.photoQuestion), "page.pdf", pdfFile(64))

		if nextDay.status != http.StatusCreated || h.dailyAnswerBytes(t, parties.student, parties.tutor, authStart.Add(24*time.Hour)) != decodeUploaded(t, nextDay).SizeBytes {
			t.Fatalf("an answer upload on the next day answered %s", describe(nextDay))
		}
	})
	t.Run("corrected uploads have no answer byte caps", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.homeworkUploadParties(t)
		h.submitStored(t, parties.submission, authStart)
		h.addDailyAnswerBytes(t, parties.tutor, parties.tutor, authStart, domain.AnswerDailyBytesPerTutor)
		h.insertPendingHomework(t, parties, pendingHomeworkRow{
			id: fixtureID(62, 2), uploader: parties.tutor, context: domain.ContextCorrected, question: parties.photoQuestion,
			size: domain.AnswerPendingBytesPerTutor,
		})

		corrected := h.homeworkUpload(t, parties.tutorToken, homeworkRoute(domain.ContextCorrected, parties.submission, parties.photoQuestion), "marked.pdf", pdfFile(64))

		if corrected.status != http.StatusCreated || h.dailyAnswerBytes(t, parties.tutor, parties.tutor, authStart) != domain.AnswerDailyBytesPerTutor {
			t.Fatalf("a corrected upload beside full answer byte caps answered %s", describe(corrected))
		}
	})
	t.Run("the exact daily check refuses a completion that passes the cap", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.homeworkUploadParties(t)
		h.addDailyAnswerBytes(t, parties.student, parties.tutor, authStart, domain.AnswerDailyBytesPerTutor-10)

		refused := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, parties.submission, parties.photoQuestion), "page.pdf", pdfFile(64))
		uploads := h.uploadsIn(t, parties.student, domain.ContextAnswer)

		assertRefused(t, "an answer completion past the daily bytes", refused, http.StatusTooManyRequests, "rate_limited")
		if len(uploads) != 1 || h.dailyAnswerBytes(t, parties.student, parties.tutor, authStart) != domain.AnswerDailyBytesPerTutor-10 {
			t.Fatalf("the refused completion changed the daily bytes or left %+v", uploads)
		}
		h.assertLeftNothing(t, uploads[0])
	})
	t.Run("pending answer bytes to a tutor stop at one hundred megabytes", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.homeworkUploadParties(t)
		h.insertPendingHomework(t, parties, pendingHomeworkRow{
			id: fixtureID(62, 1), uploader: parties.student, context: domain.ContextAnswer, question: parties.photoQuestion,
			size: domain.AnswerPendingBytesPerTutor - 100,
		})

		refused := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, parties.submission, parties.photoQuestion), "page.pdf", pdfFile(256))
		chat := h.chatUpload(t, parties.studentToken, h.directChannel(t, parties.card.ID).ID, "notes.txt", plainText(256))

		if refused.status != http.StatusTooManyRequests || refused.headers.Get("Retry-After") == "" ||
			h.countLogs(`"msg":"upload.rejected"`, `"context":"answer"`, `"reason":"pending_bytes"`) != 1 {
			t.Fatalf("an answer upload past the pending answer bytes answered %s", describe(refused))
		}
		if len(h.uploadsIn(t, parties.student, domain.ContextAnswer)) != 1 || chat.status != http.StatusCreated {
			t.Fatalf("the pending answer bytes refusal left rows or blocked chat: %s", describe(chat))
		}
	})
}

func TestAnswerPictureFiltersMetadata(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.homeworkUploadParties(t)

	answer := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, parties.submission, parties.photoQuestion), "map.jpg",
		pictureWithLocation(t))

	assertPictureFiltered(t, h, parties.student, domain.ContextAnswer, answer)
}

func TestCorrectedUploadScope(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.homeworkUploadParties(t)
	otherCard := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	_, unstarted := h.issueFileWork(t, parties.tutorToken, otherCard)
	_, intruderToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 12, "another-tutor@example.org")
	h.submitStored(t, parties.submission, authStart)
	corrected := homeworkRoute(domain.ContextCorrected, parties.submission, parties.photoQuestion)

	accepted := h.homeworkUpload(t, parties.tutorToken, corrected, "marked.pdf", pdfFile(256))
	audio := h.homeworkUpload(t, parties.tutorToken, corrected, "comments.mp3", mp3File())
	textFile := h.homeworkUpload(t, parties.tutorToken, corrected, "notes.txt", plainText(64))
	intruder := h.homeworkUpload(t, intruderToken, corrected, "marked.pdf", pdfFile(64))
	notStarted := h.homeworkUpload(t, parties.tutorToken, homeworkRoute(domain.ContextCorrected, unstarted, parties.photoQuestion), "marked.pdf", pdfFile(64))
	written := h.homeworkUpload(t, parties.tutorToken, homeworkRoute(domain.ContextCorrected, parties.submission, parties.writtenQuestion), "marked.pdf", pdfFile(64))
	byStudent := h.homeworkUpload(t, parties.studentToken, corrected, "marked.pdf", pdfFile(64))

	for _, created := range []response{accepted, audio} {
		if created.status != http.StatusCreated {
			t.Fatalf("the tutor's corrected upload answered %s", describe(created))
		}
	}
	assertRefused(t, "a corrected plain text file", textFile, http.StatusUnsupportedMediaType, "unsupported_media_type")
	for label, refused := range map[string]response{
		"another tutor's corrected upload": intruder, "a corrected upload to unstarted work": notStarted, "a corrected file for a written question": written,
		"the student's corrected upload": byStudent,
	} {
		assertRefused(t, label, refused, http.StatusNotFound, "not_found")
	}
	scoped := h.countRows(t, `SELECT count(id) FROM attachments WHERE uploader_id = $1::uuid AND context = 'corrected' AND state = 'pending'
AND scope_kind = 'submission' AND quota_tutor_id = uploader_id AND storage_key LIKE 'corrected/%'`, parties.tutor.String())
	if scoped != 2 || h.countLogs(`"msg":"upload.accepted"`, `"context":"corrected"`, `"submission_id":"`+parties.submission+`"`) != 2 {
		t.Fatalf("%d corrected uploads are pending in the tutor's quota", scoped)
	}
}

func TestHomeworkUploadCounts(t *testing.T) {
	t.Parallel()
	t.Run("twenty open uploads per user across both contexts", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.homeworkUploadParties(t)
		h.submitStored(t, parties.submission, authStart)
		for index := range domain.HomeworkOpenUploadsPerUser {
			h.insertPendingHomework(t, parties, pendingHomeworkRow{
				id: fixtureID(63, byte(index)), uploader: parties.tutor, context: homeworkContextsInTest[index%2], question: parties.photoQuestion, size: 10,
			})
		}

		twentyFirst := h.homeworkUpload(t, parties.tutorToken, homeworkRoute(domain.ContextCorrected, parties.submission, parties.photoQuestion), "marked.pdf", pdfFile(64))

		if twentyFirst.status != http.StatusTooManyRequests || code(t, twentyFirst) != "rate_limited" || twentyFirst.headers.Get("Retry-After") == "" ||
			h.countLogs(`"msg":"upload.rejected"`, `"context":"corrected"`, `"reason":"pending_cap"`) != 1 {
			t.Fatalf("the twenty-first open homework upload answered %s", describe(twentyFirst))
		}
	})
	t.Run("sixty starts per user and hour", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.homeworkUploadParties(t)
		photo := homeworkRoute(domain.ContextAnswer, parties.submission, parties.photoQuestion)
		for index := range domain.HomeworkUploadsPerHour {
			h.mustAnswer(t, h.homeworkUpload(t, parties.studentToken, photo, fmt.Sprintf("page %d.pdf", index), pdfFile(64)), http.StatusCreated)
			h.exec(t, "UPDATE attachments SET state = 'deleted', expires_at = NULL WHERE uploader_id = $1::uuid", parties.student.String())
		}

		sixtyFirst := h.homeworkUpload(t, parties.studentToken, photo, "page extra.pdf", pdfFile(64))

		if sixtyFirst.status != http.StatusTooManyRequests || h.countLogs(`"msg":"upload.rejected"`, `"context":"answer"`, `"reason":"hourly_cap"`) != 1 {
			t.Fatalf("the sixty-first homework upload in an hour answered %s", describe(sixtyFirst))
		}
	})
}

var homeworkContextsInTest = []domain.AttachmentContext{domain.ContextAnswer, domain.ContextCorrected}

func TestHomeworkUploadCapacityIsSeparate(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.homeworkUploadParties(t)
	channel := h.directChannel(t, parties.card.ID).ID
	h.fillCapacity(t, domain.UploadCapacityHomework, domain.HomeworkUploadIntentCapacity)

	answer := h.rawRequest(t, fmt.Sprintf("POST %s HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer %s\r\n"+
		"Content-Type: multipart/form-data; boundary=waiting\r\nContent-Length: 1000\r\n\r\n",
		homeworkRoute(domain.ContextAnswer, parties.submission, parties.photoQuestion), parties.studentToken))
	chat := h.chatUpload(t, parties.studentToken, channel, "notes.txt", plainText(64))
	library := h.libraryUpload(t, parties.tutorToken, domain.ContextLibrary, "worksheet.pdf", pdfFile(64))

	if answer.status != http.StatusServiceUnavailable || code(t, answer) != "service_unavailable" || answer.headers.Get("Connection") != "close" {
		t.Fatalf("an answer upload with the homework class full answered %s", describe(answer))
	}
	if chat.status != http.StatusCreated || library.status != http.StatusCreated {
		t.Fatalf("chat and library uploads beside a full homework class answered %s and %s", describe(chat), describe(library))
	}
	if h.countLogs(`"msg":"upload.capacity.rejected"`, `"context":"answer"`, `"capacity_class":"homework"`) != 1 ||
		len(h.uploadsIn(t, parties.student, domain.ContextAnswer)) != 0 {
		t.Fatal("the full homework class did not record its refusal or left a row")
	}
}

func TestAnAnswerUploadOverTheTutorsQuotaIsRefused(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.homeworkUploadParties(t)
	h.setQuota(t, parties.tutor, 1_000_000, 1_000_000)

	refused := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, parties.submission, parties.photoQuestion), "page.pdf", pdfFile(64))

	if refused.status != http.StatusInsufficientStorage || code(t, refused) != "quota_exceeded" ||
		h.countLogs(`"msg":"quota.exceeded"`, `"context":"answer"`) != 1 || len(h.uploadsIn(t, parties.student, domain.ContextAnswer)) != 0 {
		t.Fatalf("an answer upload over the tutor's quota answered %s", describe(refused))
	}
}
