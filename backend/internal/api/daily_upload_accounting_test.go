package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/files"
)

func TestDailyUploadCutoverRefusesBeforeTheBodyAndActivatesAtMidnight(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	activation, err := h.store.Repositories().DailyUploads.Activation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h.at(t, activation.Add(-time.Minute))
	parties := h.dailyUploadParties(t)
	channel := h.directChannel(t, parties.card.ID).ID
	request := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer %s\r\nContent-Type: multipart/form-data; boundary=waiting\r\nContent-Length: 1000\r\n\r\n", chatScope(channel), parties.studentToken)

	logStart := len(h.logs.String())

	before := h.rawRequest(t, request)
	cutoverLogs := h.logs.String()[logStart:]
	rowsBefore := len(h.uploadsBy(t, parties.student))
	tutor := h.chatUpload(t, parties.tutorToken, channel, "tutor.txt", plainText(8))
	h.at(t, activation)
	after := h.chatUpload(t, parties.studentToken, channel, "student.txt", plainText(8))
	total := h.dailyBytes(t, parties.student, parties.tutor, activation)

	assertDailyCutoverLogs(t, cutoverLogs, parties.student, parties.studentToken)
	if before.status != http.StatusServiceUnavailable || before.headers.Get("Retry-After") != "60" || before.headers.Get("Connection") != "close" || rowsBefore != 0 {
		t.Fatalf("cutover read the body or gave the wrong refusal: %s rows %d retry %q", describe(before), rowsBefore, before.headers.Get("Retry-After"))
	}
	if tutor.status != http.StatusCreated || after.status != http.StatusCreated || total != 8 {
		t.Fatalf("cutover blocked an exempt or active upload: tutor %s active %s total %d", describe(tutor), describe(after), total)
	}
}

func (h *authHarness) dailyBytes(t *testing.T, student domain.UserID, tutor domain.UserID, instant time.Time) int64 {
	t.Helper()
	value, err := h.store.Repositories().DailyUploads.Bytes(context.Background(), student, tutor, instant.UTC().Truncate(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func (h *authHarness) addDailyBytes(t *testing.T, student domain.UserID, tutor domain.UserID, instant time.Time, size int64) {
	t.Helper()
	if err := h.store.Repositories().DailyUploads.Add(context.Background(), student, tutor, instant.UTC().Truncate(24*time.Hour), size); err != nil {
		t.Fatal(err)
	}
}

func TestDailyUploadAccountingRejectsInvalidContentAndQuotaWithoutCharging(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.dailyUploadParties(t)
	channel := h.directChannel(t, parties.card.ID).ID
	h.setQuota(t, parties.tutor, 10, 0)

	invalid := h.chatUpload(t, parties.studentToken, channel, "notes.exe", plainText(1))
	oversize := h.chatUpload(t, parties.studentToken, channel, "notes.txt", plainText(20))
	total := h.dailyBytes(t, parties.student, parties.tutor, authStart)

	if invalid.status != http.StatusUnsupportedMediaType || oversize.status != http.StatusInsufficientStorage || total != 0 {
		t.Fatalf("refused uploads charged bytes: invalid %s quota %s total %d", describe(invalid), describe(oversize), total)
	}
}

func TestDailyUploadCleanupRetainsUploadingDaysAndSkipsLockedStudents(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.dailyUploadParties(t)
	_, other := h.squat(t, parties.tutorToken, newcomerEmail, newcomerEmail)
	student, tutor := parties.student, parties.tutor
	channel := h.directChannel(t, parties.card.ID)
	yesterday := authStart.Add(-24 * time.Hour)
	h.addDailyBytes(t, student, tutor, yesterday, 40)
	h.addDailyBytes(t, student, tutor, yesterday.Add(-24*time.Hour), 30)
	h.addDailyBytes(t, other, tutor, yesterday, 50)
	h.addDailyBytes(t, student, tutor, authStart, 60)
	h.insertUploading(t, fixtureID(90, 1), student, channel.ID, tutor, 1, "", yesterday)
	release := h.holdUserRow(t, student)

	deleted, err := h.files.CleanupDailyUploads(context.Background(), authStart)
	release()
	kept := h.dailyBytes(t, student, tutor, yesterday)
	removed := h.dailyBytes(t, other, tutor, yesterday)
	today := h.dailyBytes(t, student, tutor, authStart)
	lockedPast := h.dailyBytes(t, student, tutor, yesterday.Add(-24*time.Hour))

	if err != nil || deleted != 1 || kept != 40 || removed != 0 || today != 60 || lockedPast != 30 {
		t.Fatalf("cleanup lost accounting or blocked progress: %v deleted %d kept %d removed %d current %d", err, deleted, kept, removed, today)
	}
}

func TestDailyUploadCleanupIsBoundedByRowsAndStudents(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.dailyUploadParties(t)
	_, err := h.database.Pool.Exec(context.Background(), `WITH accounts AS (
 INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme, consent_version, consent_at, unverified_since, created_at, updated_at)
 SELECT md5('daily-student-' || n)::uuid, 'student', 'active', 'daily-' || n || '@example.org', $1, '$argon2id$fixture', 'Student', 'en', 'UTC', 'system', '1', $1, $1, $1, $1
 FROM generate_series(1,101) n RETURNING id
)
INSERT INTO daily_chat_upload_bytes (student_id, tutor_id, upload_day, completed_bytes)
SELECT id, $2, date_trunc('day', $1::timestamptz AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' - interval '24 hours', 1 FROM accounts`, authStart, parties.tutor)
	if err != nil {
		t.Fatal(err)
	}

	_, err = h.database.Pool.Exec(context.Background(), `INSERT INTO daily_chat_upload_bytes (student_id, tutor_id, upload_day, completed_bytes)
SELECT $1, $2, date_trunc('day', $3::timestamptz AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' - n * interval '1 day', 1
FROM generate_series(1,1001) n`, parties.student, parties.tutor, authStart)
	if err != nil {
		t.Fatal(err)
	}
	release := h.holdUserRow(t, parties.student)

	studentsDeleted, studentsErr := h.files.CleanupDailyUploads(context.Background(), authStart)
	var otherStudentsRemaining, lockedRowsRemaining int
	studentsReadErr := h.database.Pool.QueryRow(context.Background(), "SELECT count(*) FILTER (WHERE student_id <> $1), count(*) FILTER (WHERE student_id = $1) FROM daily_chat_upload_bytes", parties.student).Scan(&otherStudentsRemaining, &lockedRowsRemaining)
	release()
	rowsDeleted, rowsErr := h.files.CleanupDailyUploads(context.Background(), authStart)
	var rowsRemaining int
	rowsReadErr := h.database.Pool.QueryRow(context.Background(), "SELECT count(*) FROM daily_chat_upload_bytes").Scan(&rowsRemaining)

	if studentsErr != nil || studentsReadErr != nil || studentsDeleted != 100 || otherStudentsRemaining != 1 || lockedRowsRemaining != 1001 {
		t.Errorf("cleanup exceeded its student budget or changed locked rows: %v read %v deleted %d other students %d locked rows %d", studentsErr, studentsReadErr, studentsDeleted, otherStudentsRemaining, lockedRowsRemaining)
	}
	if rowsErr != nil || rowsReadErr != nil || rowsDeleted != 1000 || rowsRemaining != 2 {
		t.Errorf("cleanup exceeded its row budget: %v read %v deleted %d remaining %d", rowsErr, rowsReadErr, rowsDeleted, rowsRemaining)
	}
}

func TestUploadCrossingMidnightKeepsItsCreationDayUntilSettlement(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	parties := h.dailyUploadParties(t)
	channel := h.directChannel(t, parties.card.ID).ID
	h.at(t, authStart.UTC().Truncate(24*time.Hour).Add(24*time.Hour-time.Second))
	token := h.relogin(t, studentEmail)
	h.addDailyBytes(t, parties.student, parties.tutor, authStart, 4)
	held.holdNext(isPart("1"))
	upload := h.startChatUpload(t, token, channel)
	upload.begin(t, "midnight.txt", plainText(8))
	written := upload.completeInBackground(nil)
	held.awaitReached(t)
	nextDay := authStart.UTC().Truncate(24 * time.Hour).Add(24*time.Hour + time.Second)

	h.at(t, nextDay)
	before, beforeErr := h.files.CleanupDailyUploads(context.Background(), nextDay)
	held.resume()
	result := upload.answer(t)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	previousTotal := h.dailyBytes(t, parties.student, parties.tutor, authStart)
	currentTotal := h.dailyBytes(t, parties.student, parties.tutor, nextDay)
	after, afterErr := h.files.CleanupDailyUploads(context.Background(), nextDay)

	if beforeErr != nil || afterErr != nil || before != 0 || after != 1 || result.status != http.StatusCreated || previousTotal != 12 || currentTotal != 0 {
		t.Fatalf("midnight cleanup raced settlement: before %d/%v after %d/%v upload %s totals %d/%d", before, beforeErr, after, afterErr, describe(result), previousTotal, currentTotal)
	}
}

func TestQueuedUploadCreationUsesThePostLockDay(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.dailyUploadParties(t)
	principal, err := h.accounts.Authenticate(context.Background(), parties.studentToken)
	if err != nil {
		t.Fatal(err)
	}
	channel := parsedSessionResource[domain.ChannelID](t, h.directChannel(t, parties.card.ID).ID)
	before := authStart.UTC().Truncate(24 * time.Hour).Add(24*time.Hour - time.Second)
	h.at(t, before)
	release := h.holdUserRow(t, parties.student)
	type admissionResult struct {
		admission *files.ChatUploadAdmission
		err       error
	}
	finished := make(chan admissionResult, 1)
	go func() {
		admission, err := h.files.BeginChatUpload(context.Background(), principal, channel, 1)
		finished <- admissionResult{admission, err}
	}()
	h.waitForLockWaiters(t, 1)

	h.at(t, before.Add(2*time.Second))
	release()
	result := <-finished
	if result.admission != nil {
		defer result.admission.Close(context.Background())
	}
	var created time.Time
	readErr := h.database.Pool.QueryRow(context.Background(), "SELECT created_at FROM attachments WHERE uploader_id = $1 AND context = 'chat'", parties.student).Scan(&created)

	if result.err != nil || readErr != nil || !created.Equal(before.Add(2*time.Second)) {
		t.Fatalf("queued upload kept the pre-lock day: %v read %v created %s", result.err, readErr, created)
	}
}

func TestConcurrentUploadCompletionsCannotExceedDailyAllowance(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	parties := h.dailyUploadParties(t)
	channel := h.directChannel(t, parties.card.ID).ID
	h.addDailyBytes(t, parties.student, parties.tutor, authStart, domain.ChatDailyBytesPerTutor-1024)
	held.holdNext(isPart("1"))
	first := h.startChatUpload(t, parties.studentToken, channel)
	first.begin(t, "first.txt", plainText(1024))
	written := first.completeInBackground(nil)
	held.awaitReached(t)

	second := h.chatUpload(t, parties.studentToken, channel, "second.txt", plainText(1024))
	held.resume()
	firstResult := first.answer(t)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	total := h.dailyBytes(t, parties.student, parties.tutor, authStart)

	if second.status != http.StatusCreated || firstResult.status != http.StatusTooManyRequests || total != domain.ChatDailyBytesPerTutor {
		t.Fatalf("racing completions exceeded the daily cap: first %s second %s total %d", describe(firstResult), describe(second), total)
	}
}

func TestDailyUploadAllowancesAreIndependentAndResetNextDay(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.sharedChatWorld(t)
	h.addDailyBytes(t, world.parties.student, world.parties.tutor, authStart, domain.ChatDailyBytesPerTutor)
	other, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 9, "independent@example.org")
	otherCard := h.linkedSignedIn(t, world.parties.tutorToken, otherToken, "Independent student", "independent@example.org")
	otherChannel := h.directChannel(t, otherCard.ID).ID

	refused := h.chatUpload(t, world.parties.studentToken, world.direct.ID, "full.txt", plainText(1))
	otherTutor := h.chatUpload(t, world.parties.studentToken, world.secondChat.ID, "other tutor.txt", plainText(1))
	tutorOwn := h.chatUpload(t, world.parties.tutorToken, world.direct.ID, "tutor.txt", plainText(1))
	otherStudent := h.chatUpload(t, otherToken, otherChannel, "independent.txt", plainText(1))
	otherBytes := h.dailyBytes(t, other, world.parties.tutor, authStart)
	h.at(t, authStart.Add(24*time.Hour))
	newToken := accessToken(t, h.login(t, studentEmail, accountPassword))
	nextDay := h.chatUpload(t, newToken, world.direct.ID, "next day.txt", plainText(1))

	if refused.status != http.StatusTooManyRequests || otherTutor.status != http.StatusCreated || tutorOwn.status != http.StatusCreated || nextDay.status != http.StatusCreated || otherStudent.status != http.StatusCreated || otherBytes != 1 {
		t.Fatalf("daily scopes: full %s other tutor %s tutor own %s next day %s", describe(refused), describe(otherTutor), describe(tutorOwn), describe(nextDay))
	}
}

func TestDailyUploadAccountingSurvivesExpiryErasureAndRelationshipEnding(t *testing.T) {
	for _, operation := range []string{"expiry", "erasure", "ending"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			h := newUploadHarness(t)
			parties := h.dailyUploadParties(t)
			channel := h.directChannel(t, parties.card.ID).ID
			file := h.pendingUpload(t, parties.studentToken, channel, "retained.txt", 4)
			if operation == "expiry" {
				if _, err := h.database.Pool.Exec(context.Background(), "UPDATE attachments SET expires_at = $2 WHERE id = $1::uuid", file, authStart.Add(-time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			if operation == "erasure" {
				h.sent(t, parties.studentToken, channel, "", file)
				if _, err := h.database.Pool.Exec(context.Background(), "UPDATE channels SET discarded_at = $2, erase_pending_since = $2 WHERE id = $1::uuid", channel, authStart); err != nil {
					t.Fatal(err)
				}
			}

			if operation == "ending" {
				h.mustAnswer(t, h.stopStudying(t, parties.studentToken, parties.card.ID), http.StatusOK)
			}
			h.tick(t)
			total := h.dailyBytes(t, parties.student, parties.tutor, authStart)
			state := h.uploadState(t, file)

			if total != 4 || (operation != "ending" && state != "gone") {
				t.Fatalf("%s refunded bytes or did not remove the attachment: bytes %d state %s", operation, total, state)
			}
		})
	}
}

func TestDailyUploadAccountingSurvivesBlockAndSoftDeletion(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.dailyUploadParties(t)
	h.pendingUpload(t, parties.studentToken, h.directChannel(t, parties.card.ID).ID, "retained.txt", 6)

	var observed []int64
	for _, email := range []string{studentEmail, tutorEmail} {
		if _, err := h.operators().Block(context.Background(), email); err != nil {
			t.Fatal(err)
		}
		if _, err := h.operators().Unblock(context.Background(), email); err != nil {
			t.Fatal(err)
		}
		observed = append(observed, h.dailyBytes(t, parties.student, parties.tutor, authStart))
		token := h.relogin(t, email)
		h.mustAnswer(t, h.deleteMe(t, token, accountPassword), http.StatusNoContent)
		if _, err := h.operators().Restore(context.Background(), email); err != nil {
			t.Fatal(err)
		}
		observed = append(observed, h.dailyBytes(t, parties.student, parties.tutor, authStart))
	}

	for index, total := range observed {
		if total != 6 {
			t.Errorf("state change %d refunded daily bytes: %d", index, total)
		}
	}
}

func TestFinalAccountPurgesCannotRecreateDailyAccountingFromRacingUploads(t *testing.T) {
	for _, role := range []domain.Role{domain.RoleStudent, domain.RoleTutor} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			h, held := newHeldStorageHarness(t)
			world := h.sharedChatWorld(t)
			parties := world.parties
			h.addDailyBytes(t, parties.student, parties.tutor, authStart, 6)
			h.addDailyBytes(t, world.squatter, world.second, authStart, 7)
			held.holdNext(func(request *http.Request) bool {
				return request.Method == http.MethodPost && request.URL.Query().Has("uploadId")
			})
			upload := h.startChatUpload(t, parties.studentToken, world.direct.ID)
			upload.begin(t, "racing.txt", plainText(8))
			written := upload.completeInBackground(nil)
			held.awaitReached(t)
			token := parties.studentToken
			if role == domain.RoleTutor {
				token = parties.tutorToken
			}
			h.mustAnswer(t, h.deleteMe(t, token, accountPassword), http.StatusNoContent)
			later := authStart.Add(domain.DeletedAccountRetention + time.Hour)
			h.at(t, later)

			_, purgeErr := h.purger.PurgeDeleted(context.Background(), later)
			held.resume()
			result := upload.answer(t)
			if err := <-written; err != nil {
				t.Fatal(err)
			}
			purgedTotal := h.dailyBytes(t, parties.student, parties.tutor, authStart)
			liveTotal := h.dailyBytes(t, world.squatter, world.second, authStart)

			if purgeErr != nil || result.status == http.StatusCreated || purgedTotal != 0 || liveTotal != 7 {
				t.Fatalf("final %s purge lost daily ownership: %v upload %s purged %d live %d", role, purgeErr, describe(result), purgedTotal, liveTotal)
			}
		})
	}
}

func assertDailyCutoverLogs(t *testing.T, records string, student domain.UserID, token string) {
	t.Helper()
	for _, forbidden := range []string{token, studentEmail, accountPassword, "upload.accepted", "storage_key", "filename", "chat_upload_accounting_activation"} {
		if strings.Contains(records, forbidden) {
			t.Errorf("cutover diagnostics exposed forbidden content %q", forbidden)
		}
	}
	matched := 0
	for _, line := range strings.Split(strings.TrimSpace(records), "\n") {
		if !strings.Contains(line, `"msg":"upload.accounting.cutover"`) {
			continue
		}
		var event struct {
			Time      time.Time   `json:"time"`
			Level     string      `json:"level"`
			Message   string      `json:"msg"`
			RequestID string      `json:"request_id"`
			UserID    string      `json:"user_id"`
			Role      domain.Role `json:"role"`
		}
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&event); err != nil {
			t.Fatalf("cutover event contains unexpected fields: %v", err)
		}
		if event.Level != "INFO" || event.Message != "upload.accounting.cutover" || event.RequestID == "" || event.UserID != student.String() || event.Role != domain.RoleStudent || event.Time.IsZero() {
			t.Errorf("unexpected cutover event %+v", event)
		}
		matched++
	}
	if matched != 1 {
		t.Errorf("expected one safe cutover event, got %d", matched)
	}
}
