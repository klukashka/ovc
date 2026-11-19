package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	feedbackSeenRecord      = `"msg":"feedback.seen"`
	feedbackThrottledLimit  = `"limit":"feedback_marks_hour"`
	feedbackMarksPurpose    = "feedback_marks_hour"
	feedbackMarksPerHour    = 600
	feedbackSessionLifetime = time.Minute
)

type feedbackItem struct {
	SubmissionID string `json:"submission_id"`
	FeedbackNew  *bool  `json:"feedback_new"`
}

type feedbackMarker struct {
	feedbackVersion *int64
	seenVersion     *int64
	version         int64
	updatedAt       time.Time
}

func optionalVersion(version *int64) string {
	if version == nil {
		return "null"
	}
	return strconv.FormatInt(*version, 10)
}

func (m feedbackMarker) key() string {
	return fmt.Sprintf("%s/%s/%d/%s", optionalVersion(m.feedbackVersion), optionalVersion(m.seenVersion), m.version, m.updatedAt.Format(time.RFC3339Nano))
}

func feedbackSeenRoute(submission string) string {
	return submissionsRoute + submission + "/feedback-seen"
}

func feedbackSeenBody(version int64) string {
	return fmt.Sprintf(`{"version":%d}`, version)
}

func (h *authHarness) markFeedbackSeen(t *testing.T, token string, submission string, version int64) response {
	t.Helper()
	return h.send(t, http.MethodPost, feedbackSeenRoute(submission), feedbackSeenBody(version), jsonCall(token))
}

func feedbackFlags(t *testing.T, raw json.RawMessage) map[string]bool {
	t.Helper()
	var items []feedbackItem
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("not a list of assignment items: %s", raw)
	}
	flags := map[string]bool{}
	for _, item := range items {
		if item.FeedbackNew == nil {
			t.Fatalf("an assignment item has no feedback_new: %s", raw)
		}
		flags[item.SubmissionID] = *item.FeedbackNew
	}
	return flags
}

func (h *authHarness) checkedFeedbackNew(t *testing.T, token string) map[string]bool {
	t.Helper()
	answer := h.assignmentsAnswer(t, token, url.Values{"status": {string(domain.AssignmentsChecked)}})
	h.mustAnswer(t, answer, http.StatusOK)
	var page struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(answer.body), &page); err != nil {
		t.Fatalf("not an assignment page: %s", answer.body)
	}
	return feedbackFlags(t, page.Items)
}

func (h *authHarness) homeFeedbackNew(t *testing.T, token string) map[string]bool {
	t.Helper()
	return feedbackFlags(t, h.home(t, token, "")["recently_checked"])
}

func (h *authHarness) feedbackMarker(t *testing.T, submission string) feedbackMarker {
	t.Helper()
	var marker feedbackMarker
	if err := h.database.Pool.QueryRow(context.Background(),
		"SELECT feedback_version, feedback_seen_version, version, updated_at FROM submissions WHERE id = $1::uuid", submission).
		Scan(&marker.feedbackVersion, &marker.seenVersion, &marker.version, &marker.updatedAt); err != nil {
		t.Fatal(err)
	}
	return marker
}

func (h *authHarness) storedVersionOr(t *testing.T, submission string, missing int64) int64 {
	t.Helper()
	if h.countRows(t, "SELECT count(*) FROM submissions WHERE id = $1::uuid", submission) == 0 {
		return missing
	}
	return h.feedbackMarker(t, submission).version
}

func (h *authHarness) feedbackMarkSlots(t *testing.T) int64 {
	t.Helper()
	return h.countRows(t, "SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose = $1", feedbackMarksPurpose)
}

func (h *authHarness) fillFeedbackMarks(t *testing.T, student domain.UserID, count int) {
	t.Helper()
	window, _ := domain.FixedWindow(h.now(t), time.Hour)
	subject := h.keys.ThrottleSubject(feedbackMarksPurpose, student.String())
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO rate_limit_counters (purpose, subject, window_start, count) VALUES ($1, $2, $3, $4)
ON CONFLICT (purpose, subject, window_start) DO UPDATE SET count = excluded.count`, feedbackMarksPurpose, subject, window, count); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) rowCount(t *testing.T, query string) int64 {
	t.Helper()
	var count int64
	if err := h.database.Pool.QueryRow(context.Background(), query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func refusalShape(t *testing.T, result response) string {
	t.Helper()
	var envelope map[string]map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.body), &envelope); err != nil {
		t.Fatalf("not an error envelope: %s", result.body)
	}
	delete(envelope["error"], "request_id")
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return strconv.Itoa(result.status) + " " + string(encoded)
}

type checkedFeedback struct {
	reviewParties
	version int64
}

func (h *authHarness) checkedWithFeedback(t *testing.T) checkedFeedback {
	t.Helper()
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Good", fullyScored("Nice")...))
	checked := h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 1)
	return checkedFeedback{reviewParties: parties, version: checked.Version}
}

func assertFeedbackNew(t *testing.T, h *authHarness, token string, submission string, want bool) {
	t.Helper()
	checked, home := h.checkedFeedbackNew(t, token), h.homeFeedbackNew(t, token)
	inChecked, listed := checked[submission]
	inHome, shown := home[submission]
	if !listed || !shown || inChecked != want || inHome != want {
		t.Fatalf("feedback_new is %v (listed %v) on the checked list and %v (listed %v) on Home, want %v", inChecked, listed, inHome, shown, want)
	}
}

func TestFeedbackSeenMarker(t *testing.T) {
	t.Parallel()
	t.Run("the current version clears feedback_new on both reads", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		work := h.checkedWithFeedback(t)
		checkedBefore, homeBefore := h.checkedFeedbackNew(t, work.studentToken), h.homeFeedbackNew(t, work.studentToken)

		seen := h.markFeedbackSeen(t, work.studentToken, work.submission, work.version)

		if !checkedBefore[work.submission] || !homeBefore[work.submission] {
			t.Fatalf("the checked work was not new before the mark: %v %v", checkedBefore, homeBefore)
		}
		if seen.status != http.StatusNoContent || seen.body != "" || seen.headers.Get("Cache-Control") != "no-store" {
			t.Fatalf("the mark answered %s", describe(seen))
		}
		assertFeedbackNew(t, h, work.studentToken, work.submission, false)
		if marker := h.feedbackMarker(t, work.submission); marker.seenVersion == nil || *marker.seenVersion != work.version {
			t.Fatalf("the stored marker is %+v", marker)
		}
	})
	t.Run("a repeat answers 204, changes nothing and is logged once", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		work := h.checkedWithFeedback(t)
		h.mustAnswer(t, h.markFeedbackSeen(t, work.studentToken, work.submission, work.version), http.StatusNoContent)
		before := h.feedbackMarker(t, work.submission)

		repeat := h.markFeedbackSeen(t, work.studentToken, work.submission, work.version)

		if repeat.status != http.StatusNoContent || h.feedbackMarker(t, work.submission).key() != before.key() {
			t.Fatalf("the repeat answered %s and changed the marker", describe(repeat))
		}
		if count := strings.Count(h.logs.String(), feedbackSeenRecord); count != 1 {
			t.Fatalf("feedback.seen was logged %d times", count)
		}
	})
	t.Run("a stale or future version answers conflict", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		work := h.checkedWithFeedback(t)
		before := h.feedbackMarker(t, work.submission)

		stale := h.markFeedbackSeen(t, work.studentToken, work.submission, work.version-1)
		future := h.markFeedbackSeen(t, work.studentToken, work.submission, work.version+1)

		assertRefused(t, "a stale version", stale, http.StatusConflict, "conflict")
		assertRefused(t, "a future version", future, http.StatusConflict, "conflict")
		if h.feedbackMarker(t, work.submission).key() != before.key() || strings.Contains(h.logs.String(), feedbackSeenRecord) {
			t.Fatal("a refused version marked the feedback as seen")
		}
		assertFeedbackNew(t, h, work.studentToken, work.submission, true)
	})
	t.Run("work outside the caller's scope is not found", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		work := h.checkedWithFeedback(t)
		other := h.linkedCard(t, work.tutor, 10, domain.RelationshipActive)
		otherToken := h.studentToken(t, 10)
		h.submittedByCard(t, work.tutorToken, other, otherToken, domain.AnswerPolicyNever)
		before := h.feedbackMarker(t, work.submission)

		otherStudent := h.markFeedbackSeen(t, otherToken, work.submission, work.version)
		unknown := h.markFeedbackSeen(t, work.studentToken, randomSubmissionID, work.version)
		tutor := h.markFeedbackSeen(t, work.tutorToken, work.submission, work.version)

		for name, result := range map[string]response{"another student": otherStudent, "an unknown id": unknown, "the tutor": tutor} {
			assertRefused(t, name, result, http.StatusNotFound, "not_found")
		}
		if refusalShape(t, otherStudent) != refusalShape(t, unknown) ||
			refusalShape(t, unknown) != refusalShape(t, tutor) {
			t.Fatalf("the not-found answers differ: %s %s %s", otherStudent.body, unknown.body, tutor.body)
		}
		if h.feedbackMarker(t, work.submission).key() != before.key() {
			t.Fatal("an out-of-scope call changed the marker")
		}
	})
	t.Run("the mark advances nothing and reaches no tutor surface", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		work := h.checkedWithFeedback(t)
		h.tick(t)
		studentBefore := addresslessBody(t, h.submissionAnswer(t, work.studentToken, work.submission).body)
		tutorBefore := addresslessBody(t, h.submissionAnswer(t, work.tutorToken, work.submission).body)
		issueBefore := addresslessBody(t, h.send(t, http.MethodGet, "/api/v1/issues/"+work.issue, "", jsonCall(work.tutorToken)).body)
		queueBefore := h.send(t, http.MethodGet, reviewQueueRoute, "", jsonCall(work.tutorToken)).body
		marker := h.feedbackMarker(t, work.submission)
		outbox, notifications := h.rowCount(t, "SELECT count(*) FROM outbox"), h.rowCount(t, "SELECT count(*) FROM notifications")
		frames := h.recordFrames(t)

		h.mustAnswer(t, h.markFeedbackSeen(t, work.studentToken, work.submission, work.version), http.StatusNoContent)
		h.tick(t)

		after := h.feedbackMarker(t, work.submission)
		if after.version != marker.version || !after.updatedAt.Equal(marker.updatedAt) {
			t.Fatalf("the mark changed the version or updated_at: %+v then %+v", marker, after)
		}
		if addresslessBody(t, h.submissionAnswer(t, work.studentToken, work.submission).body) != studentBefore {
			t.Fatal("the mark changed the student's submission")
		}
		if addresslessBody(t, h.submissionAnswer(t, work.tutorToken, work.submission).body) != tutorBefore ||
			addresslessBody(t, h.send(t, http.MethodGet, "/api/v1/issues/"+work.issue, "", jsonCall(work.tutorToken)).body) != issueBefore ||
			h.send(t, http.MethodGet, reviewQueueRoute, "", jsonCall(work.tutorToken)).body != queueBefore {
			t.Fatal("the mark changed a tutor surface")
		}
		if containsAny(tutorBefore+issueBefore+queueBefore, "feedback_version", "feedback_seen_version", "feedback_new") {
			t.Fatal("a tutor surface carries the seen marker")
		}
		if h.rowCount(t, "SELECT count(*) FROM outbox") != outbox || h.rowCount(t, "SELECT count(*) FROM notifications") != notifications {
			t.Fatal("the mark wrote an outbox or notification row")
		}
		if sent := frames.settle(t, h); len(sent) != 0 {
			t.Fatalf("the mark sent frames %+v", sent)
		}
	})
	t.Run("a later visible publication turns feedback_new on again", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		work := h.checkedWithFeedback(t)
		h.mustAnswer(t, h.markFeedbackSeen(t, work.studentToken, work.submission, work.version), http.StatusNoContent)

		h.reviewSaved(t, work.tutorToken, work.submission, reviewJSON(t, 2, 0, saveID(2), "Even better", fullyScored("Nice")...))

		republished := h.published(t, work.tutorToken, work.submission, domain.ReviewCheck, 2)
		checkedAgain, homeAgain := h.checkedFeedbackNew(t, work.studentToken), h.homeFeedbackNew(t, work.studentToken)
		stale := h.markFeedbackSeen(t, work.studentToken, work.submission, work.version)
		fresh := h.markFeedbackSeen(t, work.studentToken, work.submission, republished.Version)

		if !checkedAgain[work.submission] || !homeAgain[work.submission] {
			t.Fatalf("the visible publication did not turn feedback_new on: %v %v", checkedAgain, homeAgain)
		}
		assertRefused(t, "the version before the publication", stale, http.StatusConflict, "conflict")
		h.mustAnswer(t, fresh, http.StatusNoContent)
		assertFeedbackNew(t, h, work.studentToken, work.submission, false)
	})
	t.Run("a returned review shows as new on the to-do list", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.submittedForReview(t, domain.AnswerPolicyNever)
		h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Try again"))
		returned := h.published(t, parties.tutorToken, parties.submission, domain.ReviewReturn, 1)
		before := h.assignmentsAnswer(t, parties.studentToken, nil).body

		h.mustAnswer(t, h.markFeedbackSeen(t, parties.studentToken, parties.submission, returned.Version), http.StatusNoContent)
		after := h.assignmentsAnswer(t, parties.studentToken, nil).body

		if !strings.Contains(before, `"feedback_new":true`) || !strings.Contains(after, `"feedback_new":false`) {
			t.Fatalf("the returned work's to-do item went from %s to %s", before, after)
		}
	})
	t.Run("checked work without visible review content is never new", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.submittedForReview(t, domain.AnswerPolicyNever)
		h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "", fullyScored("")...))

		h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 1)

		if marker := h.feedbackMarker(t, parties.submission); marker.feedbackVersion == nil {
			t.Fatalf("the visible publication set no feedback_version: %+v", marker)
		}
		if !strings.Contains(h.submissionAnswer(t, parties.studentToken, parties.submission).body, `"review":null`) {
			t.Fatal("the empty review is shown to the student")
		}
		assertFeedbackNew(t, h, parties.studentToken, parties.submission, false)
	})
	t.Run("malformed requests are refused before any slot is reserved", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		work := h.checkedWithFeedback(t)
		bodies := map[string]string{"empty": `{}`, "negative": `{"version":-1}`, "text": `{"version":"1"}`, "extra": `{"version":1,"seen":true}`}

		results := map[string]response{}
		for name, body := range bodies {
			results[name] = h.send(t, http.MethodPost, feedbackSeenRoute(work.submission), body, jsonCall(work.studentToken))
		}
		plain := h.send(t, http.MethodPost, feedbackSeenRoute(work.submission), feedbackSeenBody(work.version),
			map[string]string{"Authorization": "Bearer " + work.studentToken, "Content-Type": "text/plain"})

		for name, result := range results {
			assertRefused(t, name, result, http.StatusUnprocessableEntity, "validation_failed")
		}
		assertRefused(t, "a non-JSON body", plain, http.StatusUnsupportedMediaType, "unsupported_media_type")
		if h.feedbackMarkSlots(t) != 0 || h.feedbackMarker(t, work.submission).seenVersion != nil {
			t.Fatal("a malformed request reserved a slot or marked the feedback")
		}
	})
}

type accountChange struct {
	waitsForTheLock bool
	slotsAfter      int64
	apply           func(h *authHarness, student domain.UserID, token string) error
}

func revokeSession(h *authHarness, _ domain.UserID, token string) error {
	principal, err := h.accounts.Authenticate(context.Background(), token)
	if err != nil {
		return err
	}
	return h.accounts.Logout(context.Background(), principal)
}

func changeInBackground(h *authHarness, change accountChange, student domain.UserID, token string) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- change.apply(h, student, token)
	}()
	return done
}

func awaitAccountChange(t *testing.T, done <-chan error) {
	t.Helper()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) markWaitingBehind(t *testing.T, work checkedFeedback, change accountChange) response {
	t.Helper()
	release := h.holdUserRow(t, work.student)
	var pending <-chan pendingAnswer
	var done <-chan error
	if change.waitsForTheLock {
		done = changeInBackground(h, change, work.student, work.studentToken)
		h.waitForLockWaiters(t, 1)
		pending = h.sendInBackground(http.MethodPost, feedbackSeenRoute(work.submission), feedbackSeenBody(work.version), jsonCall(work.studentToken))
		h.waitForLockWaiters(t, 2)
	} else {
		pending = h.sendInBackground(http.MethodPost, feedbackSeenRoute(work.submission), feedbackSeenBody(work.version), jsonCall(work.studentToken))
		h.waitForLockWaiters(t, 1)
		applied := make(chan error, 1)
		applied <- change.apply(h, work.student, work.studentToken)
		done = applied
	}
	release()
	awaitAccountChange(t, done)
	return awaitAnswer(t, pending)
}

func TestFeedbackSeenRechecksTheSessionUnderTheAccountLock(t *testing.T) {
	t.Parallel()
	cases := map[string]accountChange{
		"a session revoked meanwhile": {apply: revokeSession},
		"a session that expired meanwhile": {slotsAfter: 1, apply: func(h *authHarness, _ domain.UserID, _ string) error {
			_, err := h.clock.Advance(context.Background(), 2*feedbackSessionLifetime)
			return err
		}},
		"an account blocked meanwhile": {waitsForTheLock: true, apply: func(h *authHarness, student domain.UserID, _ string) error {
			_, err := h.database.Pool.Exec(context.Background(), "UPDATE users SET state = 'blocked', state_before_block = 'active' WHERE id = $1::uuid",
				student.String())
			return err
		}},
		"an account deleted meanwhile": {waitsForTheLock: true, apply: func(h *authHarness, student domain.UserID, _ string) error {
			now, err := h.clock.Instant(context.Background())
			if err != nil {
				return err
			}
			_, err = h.database.Pool.Exec(context.Background(), "UPDATE users SET state = 'deleted', deleted_at = $2 WHERE id = $1::uuid",
				student.String(), now)
			return err
		}},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newUploadHarness(t)
			work := h.checkedWithFeedback(t)
			principal, err := h.accounts.Authenticate(context.Background(), work.studentToken)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.database.Pool.Exec(t.Context(), "UPDATE sessions SET expires_at = $2 WHERE id = $1", principal.SessionID,
				h.now(t).Add(feedbackSessionLifetime)); err != nil {
				t.Fatal(err)
			}

			result := h.markWaitingBehind(t, work, change)

			if result.status != http.StatusUnauthorized || code(t, result) != "unauthenticated" {
				t.Fatalf("the mark answered %s", describe(result))
			}
			marker, logged, slots := h.feedbackMarker(t, work.submission), strings.Contains(h.logs.String(), feedbackSeenRecord), h.feedbackMarkSlots(t)
			if marker.seenVersion != nil || logged || slots != change.slotsAfter {
				t.Fatalf("a refused actor left the marker %+v, the record %v and %d slots", marker, logged, slots)
			}
		})
	}
}

func TestFeedbackSeenRefusesARevokedSessionBeforeTheLimit(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	work := h.checkedWithFeedback(t)
	h.fillFeedbackMarks(t, work.student, feedbackMarksPerHour)

	result := h.markWaitingBehind(t, work, accountChange{apply: revokeSession})

	if result.status != http.StatusUnauthorized || code(t, result) != "unauthenticated" {
		t.Fatalf("the revoked mark over the limit answered %s", describe(result))
	}
	if strings.Contains(h.logs.String(), feedbackThrottledLimit) || h.feedbackMarkSlots(t) != feedbackMarksPerHour {
		t.Fatal("the revoked mark reached the limit")
	}
}

func TestFeedbackSeenRefusesTheSixHundredFirstMarkInAnHour(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	work := h.checkedWithFeedback(t)
	h.fillFeedbackMarks(t, work.student, feedbackMarksPerHour)

	inScope := h.markFeedbackSeen(t, work.studentToken, work.submission, work.version)
	stale := h.markFeedbackSeen(t, work.studentToken, work.submission, work.version-1)
	outOfScope := h.markFeedbackSeen(t, work.studentToken, randomSubmissionID, work.version)

	retryAfter, err := strconv.Atoi(inScope.headers.Get("Retry-After"))
	if inScope.status != http.StatusTooManyRequests || code(t, inScope) != "rate_limited" || err != nil || retryAfter <= 0 {
		t.Fatalf("the six hundred and first mark answered %s", describe(inScope))
	}
	for name, result := range map[string]response{"a stale version": stale, "an out-of-scope id": outOfScope} {
		if refusalShape(t, result) != refusalShape(t, inScope) || result.headers.Get("Retry-After") != inScope.headers.Get("Retry-After") {
			t.Fatalf("the throttled answer for %s differs: %s", name, describe(result))
		}
	}
	logs := h.logs.String()
	if strings.Count(logs, feedbackThrottledLimit) != 3 || strings.Contains(logs, feedbackSeenRecord) {
		t.Fatalf("the throttled marks were not recorded once each: %s", logs)
	}
	if h.feedbackMarker(t, work.submission).seenVersion != nil {
		t.Fatal("a throttled mark marked the feedback")
	}
}

func TestFeedbackSeenLimitIsPerStudentAndHourly(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	work := h.checkedWithFeedback(t)
	other := h.linkedCard(t, work.tutor, 10, domain.RelationshipActive)
	otherToken := h.studentToken(t, 10)
	otherWork := h.submittedByCard(t, work.tutorToken, other, otherToken, domain.AnswerPolicyNever)
	h.reviewSaved(t, work.tutorToken, otherWork, reviewJSON(t, 1, 0, saveID(1), "Fine", fullyScored("Ok")...))
	otherChecked := h.published(t, work.tutorToken, otherWork, domain.ReviewCheck, 1)
	h.fillFeedbackMarks(t, work.student, feedbackMarksPerHour)

	otherStudent := h.markFeedbackSeen(t, otherToken, otherWork, otherChecked.Version)
	h.advance(t, time.Hour)
	nextHour := h.markFeedbackSeen(t, accessToken(t, h.login(t, studentEmail, accountPassword)), work.submission, work.version)

	if otherStudent.status != http.StatusNoContent || nextHour.status != http.StatusNoContent {
		t.Fatalf("another student's mark answered %s and the next hour's %s", describe(otherStudent), describe(nextHour))
	}
}

func markedErasureFixture(t *testing.T) erasureFixture {
	t.Helper()
	f := newErasureFixture(t)
	h := f.h
	h.submitted(t, f.studentToken, f.submission, h.studentWork(t, f.studentToken, f.submission).Version)
	h.reviewSaved(t, f.tutorToken, f.submission, reviewJSON(t, 1, 0, saveID(2), "Good", fullyScored("Nice")...))
	checked := h.published(t, f.tutorToken, f.submission, domain.ReviewCheck, 1)
	h.mustAnswer(t, h.markFeedbackSeen(t, f.studentToken, f.submission, checked.Version), http.StatusNoContent)
	return f
}

func (f erasureFixture) markedRows(t *testing.T) int64 {
	t.Helper()
	return f.h.countRows(t, "SELECT count(*) FROM submissions WHERE id = $1::uuid AND feedback_version IS NOT NULL AND feedback_seen_version IS NOT NULL",
		f.submission)
}

type markerStage struct {
	byTutor bool
	run     func(t *testing.T, f *erasureFixture)
}

func TestTheFeedbackMarkerLeavesWithItsSubmission(t *testing.T) {
	t.Parallel()
	stages := map[string]markerStage{
		"the student purge": {run: func(t *testing.T, f *erasureFixture) {
			t.Helper()
			f.h.setRelationshipState(t, f.card, domain.RelationshipActive)
			f.h.mustAnswer(t, f.h.deleteMe(t, f.studentToken, accountPassword), http.StatusNoContent)
			f.h.at(t, f.h.now(t).Add(domain.DeletedAccountRetention))
			f.h.tick(t)
		}},
		"the tutor purge": {byTutor: true, run: func(t *testing.T, f *erasureFixture) {
			t.Helper()
			f.h.setRelationshipState(t, f.card, domain.RelationshipActive)
			f.h.mustAnswer(t, f.h.deleteMe(t, f.tutorToken, accountPassword), http.StatusNoContent)
			f.h.at(t, f.h.now(t).Add(domain.DeletedAccountRetention))
			f.h.tick(t)
		}},
		"the erasure of a dropped provisional link": {run: func(t *testing.T, f *erasureFixture) {
			t.Helper()
			f.h.mustAnswer(t, f.h.removeStudent(t, f.tutorToken, f.card), http.StatusOK)
			f.cleanup(t)
		}},
		"the immediate purge of an unconfirmed account": {run: func(t *testing.T, f *erasureFixture) {
			t.Helper()
			f.unverify(t)
			f.h.mustAnswer(t, f.h.deleteMe(t, f.studentToken, accountPassword), http.StatusNoContent)
			f.cleanup(t)
		}},
	}
	for name, stage := range stages {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := markedErasureFixture(t)
			marked := f.markedRows(t)
			query, owner := "SELECT count(*) FROM submissions WHERE student_id = $1::uuid", f.student
			if stage.byTutor {
				query, owner = "SELECT count(*) FROM submissions WHERE tutor_id = $1::uuid", f.tutor
			}

			stage.run(t, &f)

			if marked != 1 {
				t.Fatalf("the fixture holds %d marked submissions, want 1", marked)
			}
			if count := f.h.countRows(t, query, owner.String()); count != 0 {
				t.Fatalf("%d submissions remain after %s", count, name)
			}
		})
	}
}
