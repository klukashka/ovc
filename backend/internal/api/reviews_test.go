package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
)

func TestReviewDraftCopiesPublished(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	corrected := h.correctedFile(t, parties.tutorToken, parties.submission, workQuestion(6), "corrected.pdf", 80)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Good", fullyScored("Nice", corrected)...))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 1)
	usage := h.fileUsage(t, parties.tutor)

	draft := h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 2, 0, saveID(2), "Good", fullyScored("Nice", corrected)...))
	withoutFile := h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 2, 1, saveID(3), "Good", fullyScored("Nice")...))

	if len(draft.Revisions) != 2 || draft.Revisions[1].State != string(domain.ReviewDraft) || *draft.Revisions[1].Items[0].Points != 4 {
		t.Fatalf("the new draft is %+v", draft.Revisions)
	}
	copied := draft.Revisions[1].Items[1].CorrectedFiles
	if len(copied) != 1 || copied[0].AttachmentID != corrected || copied[0].URL == draft.Revisions[0].Items[1].CorrectedFiles[0].URL {
		t.Fatalf("the draft's corrected files are %+v", copied)
	}
	if h.fileUsage(t, parties.tutor) != usage || h.storageDeleteQueued(t, corrected) {
		t.Fatal("copying or removing a published file changed the tutor's charge")
	}
	if h.reviewItemReferences(t, corrected) != 1 || len(withoutFile.Revisions[1].Items[1].CorrectedFiles) != 0 || withoutFile.Revisions[1].Version != 2 {
		t.Fatalf("removing the file from the draft left %d references", h.reviewItemReferences(t, corrected))
	}
}

func TestReviewPublishRefusals(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	tutorToken := parties.tutorToken
	cards := map[byte]string{}
	for _, id := range []byte{10, 11, 12} {
		cards[id] = h.linkedCard(t, parties.tutor, id, domain.RelationshipActive)
	}
	revealing := h.submittedByCard(t, tutorToken, cards[10], h.studentToken(t, 10), domain.AnswerPolicyAfterSubmission)
	ending := h.submittedByCard(t, tutorToken, cards[11], h.studentToken(t, 11), domain.AnswerPolicyNever)
	_, open := h.issueWork(t, tutorToken, toCard(cards[12]), "Open", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	h.saved(t, h.studentToken(t, 12), open[0].id, answersJSON(t, 1, saveID(1), typedAnswer(workQuestion(5), "Draft")))
	unscored := []reviewItemJSON{scored(workQuestion(5), 4)}
	h.reviewSaved(t, tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "", unscored...))
	h.reviewSaved(t, tutorToken, revealing, reviewJSON(t, 1, 0, saveID(1), "Redo", fullyScored("Redo")...))
	h.reviewSaved(t, tutorToken, ending, reviewJSON(t, 1, 0, saveID(1), "Redo", fullyScored("Redo")...))
	h.mustAnswer(t, h.endAsTutor(t, tutorToken, cards[11]), http.StatusOK)

	stale := h.publishReview(t, tutorToken, parties.submission, domain.ReviewCheck, 5)
	unpointed := h.publishReview(t, tutorToken, parties.submission, domain.ReviewCheck, 1)
	uncommented := h.publishReview(t, tutorToken, parties.submission, domain.ReviewReturn, 1)
	afterSubmission := h.publishReview(t, tutorToken, revealing, domain.ReviewReturn, 1)
	afterEnding := h.publishReview(t, tutorToken, ending, domain.ReviewReturn, 1)
	draftWork := h.saveReview(t, tutorToken, open[0].id, reviewJSON(t, 1, 0, saveID(2), "x"))
	h.mustAnswer(t, h.endAsTutor(t, tutorToken, cards[12]), http.StatusOK)
	closedWork := h.saveReview(t, tutorToken, open[0].id, reviewJSON(t, 1, 0, saveID(3), "x"))
	h.reviewSaved(t, tutorToken, parties.submission, reviewJSON(t, 1, 1, saveID(4), "", fullyScored("")...))
	checked := h.published(t, tutorToken, parties.submission, domain.ReviewCheck, 1)
	again := h.publishReview(t, tutorToken, parties.submission, domain.ReviewCheck, 1)
	returnChecked := h.publishReview(t, tutorToken, parties.submission, domain.ReviewReturn, 2)

	assertRefused(t, "a stale revision", stale, http.StatusConflict, "conflict")
	assertRefused(t, "a check without manual points", unpointed, http.StatusUnprocessableEntity, "validation_failed")
	assertRefused(t, "a return without a comment", uncommented, http.StatusUnprocessableEntity, "validation_failed")
	assertRefused(t, "a return under after_submission", afterSubmission, http.StatusConflict, "return_not_allowed")
	assertRefused(t, "a return after the ending", afterEnding, http.StatusConflict, "return_not_allowed")
	assertRefused(t, "a review of draft work", draftWork, http.StatusConflict, "not_editable")
	assertRefused(t, "a review of closed work", closedWork, http.StatusConflict, "not_editable")
	assertRefused(t, "a return of checked work", returnChecked, http.StatusConflict, "not_editable")
	if !strings.Contains(unpointed.body, `"body.revision"`) || !strings.Contains(uncommented.body, `"required"`) {
		t.Fatalf("the publication refusals name %s and %s", unpointed.body, uncommented.body)
	}
	h.mustAnswer(t, again, http.StatusOK)
	if checked.Status != string(domain.SubmissionChecked) || decodeLibrary[reviewedWorkBody](t, again).Version != checked.Version {
		t.Fatalf("the repeated publication changed the work: %+v", again.body)
	}
	if h.submissionField(t, ending, "status") != string(domain.SubmissionSubmitted) {
		t.Fatal("the ending changed submitted work")
	}
}

func TestReturnReopensManualOnly(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Try again", commented(scored(workQuestion(5), 1), "Too short")))

	returned := h.published(t, parties.tutorToken, parties.submission, domain.ReviewReturn, 1)
	h.tick(t)

	view := h.studentReviewed(t, parties.studentToken, parties.submission)
	if returned.Status != string(domain.SubmissionReturned) || view.Status != string(domain.SubmissionReturned) {
		t.Fatalf("the returned work is %s for the tutor and %s for the student", returned.Status, view.Status)
	}
	if !slices.Equal(view.EditableQuestionIDs, []string{workQuestion(5), workQuestion(6)}) {
		t.Fatalf("returned work reopens %v", view.EditableQuestionIDs)
	}
	if view.Review == nil || view.Review.Feedback != "Try again" || len(view.Review.Items) != 1 || view.Review.Items[0].Comment != "Too short" {
		t.Fatalf("the student sees the review %+v", view.Review)
	}
	if rows := eventsNamed(h.notificationsOf(t, parties.studentToken), string(domain.NotificationSubmissionReturned)); len(rows) != 1 || rows[0].Label == nil {
		t.Fatalf("the student's returned rows are %+v", rows)
	}
	if mails := h.mailTo(t, studentEmail); !slices.ContainsFunc(mails, func(message mail.SinkMessage) bool { return strings.Contains(message.Subject, "Assignment returned") }) {
		t.Fatal("the student got no returned email")
	}
}

func TestReturnedWorkTakesOnlyManualAnswers(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Try again"))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewReturn, 1)
	version := h.studentReviewed(t, parties.studentToken, parties.submission).Version

	frozen := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, version, saveID(2), typedAnswer(workQuestion(3), "x")))
	manual := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, version, saveID(3), typedAnswer(workQuestion(5), "A longer story")))

	assertRefused(t, "a save of an automatic answer", frozen, http.StatusConflict, "answer_frozen")
	h.mustAnswer(t, manual, http.StatusOK)
}

func TestReturnOnProvisionalLink(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipProvisional)
	submission := h.submittedByCard(t, tutorToken, card, h.studentToken(t, 10), domain.AnswerPolicyNever)
	h.reviewSaved(t, tutorToken, submission, reviewJSON(t, 1, 0, saveID(1), "Again please"))

	returned := h.published(t, tutorToken, submission, domain.ReviewReturn, 1)

	if returned.Status != string(domain.SubmissionReturned) || h.studentReviewed(t, h.studentToken(t, 10), submission).Review == nil {
		t.Fatalf("the provisional student's work is %s", returned.Status)
	}
}

func TestResubmissionReturnsToQueue(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Redo", fullyScored("Redo")...))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewReturn, 1)
	view := h.studentReviewed(t, parties.studentToken, parties.submission)
	draft := h.saved(t, parties.studentToken, parties.submission, answersJSON(t, view.Version, saveID(2), typedAnswer(workQuestion(5), "Better")))

	h.submitted(t, parties.studentToken, parties.submission, draft.Version)

	work := h.reviewedWork(t, parties.tutorToken, parties.submission)
	queue := h.reviewQueue(t, parties.tutorToken, nil)
	submittedRows := eventsNamed(h.notificationsOf(t, parties.tutorToken), string(domain.NotificationSubmitted))
	if work.Status != string(domain.SubmissionSubmitted) || len(queue.Items) != 1 || queue.Items[0].SubmissionID != parties.submission {
		t.Fatalf("the resubmitted work is %s and the queue %+v", work.Status, queue)
	}
	if len(submittedRows) != 2 || work.FinalPoints == nil || *work.FinalPoints != 2+4+3 {
		t.Fatalf("the tutor got %d submitted rows and the final points %v", len(submittedRows), work.FinalPoints)
	}
}

func TestPointsOnlyRevisionInvisible(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyAfterCheck)
	corrected := h.correctedFile(t, parties.tutorToken, parties.submission, workQuestion(6), "corrected.pdf", 80)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Good", fullyScored("Nice", corrected)...))
	later := h.correctedFile(t, parties.tutorToken, parties.submission, workQuestion(6), "later.pdf", 60)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 1, saveID(5), "Good", fullyScored("Nice", corrected, later)...))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 1)
	_, automatic := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Quick", string(domain.AnswerPolicyNever), nil, automaticBlocks()...)
	h.submitted(t, parties.studentToken, automatic[0].id, 1)
	h.tick(t)
	before := addresslessBody(t, h.submissionAnswer(t, parties.studentToken, parties.submission).body)
	beforeAutomatic := addresslessBody(t, h.submissionAnswer(t, parties.studentToken, automatic[0].id).body)
	checkedQuery := url.Values{"status": {string(domain.AssignmentsChecked)}}
	checkedBefore := h.assignmentsAnswer(t, parties.studentToken, checkedQuery).body
	homeBefore := string(h.home(t, parties.studentToken, "")["recently_checked"])
	versionBefore := decodeLibrary[studentSubmissionBody](t, h.submissionAnswer(t, parties.studentToken, parties.submission)).Version
	automaticVersion := decodeLibrary[studentSubmissionBody](t, h.submissionAnswer(t, parties.studentToken, automatic[0].id)).Version
	rows, mails := len(h.notificationsOf(t, parties.studentToken)), len(h.mailTo(t, studentEmail))
	frames := h.recordFrames(t)
	changedPoints := []reviewItemJSON{commented(scored(workQuestion(5), 1), "Nice"), commented(scored(workQuestion(6), 5), "", corrected, later)}
	draftOnly := h.correctedFile(t, parties.tutorToken, parties.submission, workQuestion(6), "draft-only.pdf", 70)
	laterDraft := []reviewItemJSON{commented(scored(workQuestion(5), 1), "Draft comment sentinel"), commented(scored(workQuestion(6), 5), "", corrected, later, draftOnly)}

	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 2, 0, saveID(2), "Good", changedPoints...))
	pointsOnly := h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 2)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 3, 0, saveID(6), "Draft feedback sentinel", laterDraft...))
	h.reviewSaved(t, parties.tutorToken, automatic[0].id, reviewJSON(t, 1, 0, saveID(3), "", overridden(workQuestion(1), 0, "Copied")))
	h.published(t, parties.tutorToken, automatic[0].id, domain.ReviewCheck, 1)
	h.tick(t)
	quiet := framesTo(frames.settle(t, h), parties.student)
	checkedAfter := h.assignmentsAnswer(t, parties.studentToken, checkedQuery).body
	homeAfter := string(h.home(t, parties.studentToken, "")["recently_checked"])
	seenAfter := h.markFeedbackSeen(t, parties.studentToken, parties.submission, versionBefore)
	seenAutomatic := h.markFeedbackSeen(t, parties.studentToken, automatic[0].id, automaticVersion)

	after := addresslessBody(t, h.submissionAnswer(t, parties.studentToken, parties.submission).body)
	afterAutomatic := addresslessBody(t, h.submissionAnswer(t, parties.studentToken, automatic[0].id).body)
	if before != after || beforeAutomatic != afterAutomatic || !strings.Contains(afterAutomatic, `"review":null`) {
		t.Fatalf("a points-only publication or a later draft changed the student's view:\n%s\n%s", before, after)
	}
	if containsAny(after, "Draft comment sentinel", "Draft feedback sentinel", "draft-only.pdf") {
		t.Fatalf("the later draft reached the student: %s", after)
	}
	if len(quiet) != 0 || len(h.notificationsOf(t, parties.studentToken)) != rows || len(h.mailTo(t, studentEmail)) != mails {
		t.Fatalf("a points-only publication reached the student: %+v", quiet)
	}
	var keptOrder int64
	if err := h.database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM attachment_references AS copied
JOIN attachment_references AS original ON original.attachment_id = copied.attachment_id AND original.ref_kind = 'review_item'
    AND original.ref_id = $2::uuid AND original.created_at = copied.created_at
WHERE copied.ref_kind = 'review_item' AND copied.ref_id = $1::uuid`, h.revisionID(t, parties.submission, 2), h.revisionID(t, parties.submission, 1)).Scan(&keptOrder); err != nil {
		t.Fatal(err)
	}
	if keptOrder != 2 {
		t.Fatalf("the copied corrected files keep the order of %d of 2 files", keptOrder)
	}
	if checkedBefore != checkedAfter || homeBefore != homeAfter || !strings.Contains(checkedAfter, `"feedback_new":true`) {
		t.Fatalf("a points-only publication changed the checked list or Home:\n%s\n%s\n%s\n%s", checkedBefore, checkedAfter, homeBefore, homeAfter)
	}
	if seenAfter.status != http.StatusNoContent || seenAutomatic.status != http.StatusNoContent {
		t.Fatalf("the marks with the versions from before the points-only publications answered %s and %s", describe(seenAfter), describe(seenAutomatic))
	}
	if pointsOnly.FinalPoints == nil || *pointsOnly.FinalPoints != 2+1+5 {
		t.Fatalf("the points-only revision stored %v", pointsOnly.FinalPoints)
	}
}

func TestVisibleRevisionSendsReviewUpdated(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Good", fullyScored("Nice")...))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 1)
	h.tick(t)
	mails := len(h.mailTo(t, studentEmail))
	frames := h.recordFrames(t)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 2, 0, saveID(2), "Even better", fullyScored("Nice")...))

	h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 2)
	h.tick(t)

	updated := eventsNamed(h.notificationsOf(t, parties.studentToken), string(domain.NotificationReviewUpdated))
	sent := h.mailTo(t, studentEmail)
	if len(updated) != 1 || len(framesTo(frames.settle(t, h), parties.student)) == 0 || len(sent) != mails+1 {
		t.Fatalf("a visible revision sent %d updated rows and %d new emails", len(updated), len(sent)-mails)
	}
	latest := sent[len(sent)-1]
	if !strings.Contains(latest.Subject, "Review updated") || containsAny(latest.Text+latest.HTML, "Even better", "Nice", "points") {
		t.Fatalf("the review updated email is %q", latest.Text)
	}
}

func addresslessBody(t *testing.T, body string) string {
	t.Helper()
	return string(withoutAddresses(t, json.RawMessage(body)))
}

func withoutAddresses(t *testing.T, raw json.RawMessage) json.RawMessage {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return trimmed
	}
	var encoded []byte
	var err error
	switch trimmed[0] {
	case '{':
		var decoded map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &decoded); err != nil {
			t.Fatal(err)
		}
		delete(decoded, "url")
		delete(decoded, "expires_at")
		for key, nested := range decoded {
			decoded[key] = withoutAddresses(t, nested)
		}
		encoded, err = json.Marshal(decoded)
	case '[':
		var decoded []json.RawMessage
		if err := json.Unmarshal(trimmed, &decoded); err != nil {
			t.Fatal(err)
		}
		for index, nested := range decoded {
			decoded[index] = withoutAddresses(t, nested)
		}
		encoded, err = json.Marshal(decoded)
	default:
		return trimmed
	}
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestDraftCorrectedFileRefusedToStudent(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	published := h.correctedFile(t, parties.tutorToken, parties.submission, workQuestion(6), "published.pdf", 80)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Good", fullyScored("Nice", published)...))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 1)
	drafted := h.correctedFile(t, parties.tutorToken, parties.submission, workQuestion(6), "drafted.pdf", 90)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 2, 0, saveID(2), "Good", fullyScored("Nice", published, drafted)...))
	first, second := h.revisionID(t, parties.submission, 1), h.revisionID(t, parties.submission, 2)

	draftMint, _ := h.addressOf(t, parties.studentToken, drafted, "?ref=review_item:"+second)
	copiedMint, _ := h.addressOf(t, parties.studentToken, published, "?ref=review_item:"+second)
	publishedMint, address := h.addressOf(t, parties.studentToken, published, "?ref=review_item:"+first)
	tutorMint, _ := h.addressOf(t, parties.tutorToken, drafted, "?ref=review_item:"+second)

	assertRefused(t, "the student's mint of a draft file", draftMint, http.StatusNotFound, "not_found")
	assertRefused(t, "the student's mint through the draft", copiedMint, http.StatusNotFound, "not_found")
	h.mustAnswer(t, publishedMint, http.StatusOK)
	h.mustAnswer(t, tutorMint, http.StatusOK)
	if served := h.fetch(t, address.URL, nil); served.status != http.StatusOK || !strings.Contains(served.headers.Get("Content-Disposition"), "published.pdf") {
		t.Fatalf("the published corrected file answered %s", describe(served))
	}
}

func TestReviewQueueOrderAndCount(t *testing.T) {
	t.Parallel()
	h := newHomeworkHarnessWithCaps(t, domain.HomeworkCaps{ReviewQueueCount: 2})
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	template := h.workTemplate(t, tutorToken, "Trip", workBlocks()...)
	automatic := h.workTemplate(t, tutorToken, "Quick", automaticBlocks()...)
	start := h.now(t)
	var submissions []string
	cards := map[byte]string{}
	for index, id := range []byte{12, 10, 11} {
		card := h.linkedCard(t, tutor, id, domain.RelationshipActive)
		cards[id] = card
		_, work := h.issueTemplate(t, tutorToken, template, toCard(card), string(domain.AnswerPolicyNever), nil)
		h.submitStored(t, work[0].id, start.Add(time.Duration(index)*time.Minute))
		submissions = append(submissions, work[0].id)
		_, quick := h.issueTemplate(t, tutorToken, automatic, toCard(card), string(domain.AnswerPolicyNever), nil)
		h.submitStored(t, quick[0].id, start)
	}
	h.reviewSaved(t, tutorToken, submissions[1], reviewJSON(t, 1, 0, saveID(1), "Draft"))

	first := h.reviewQueue(t, tutorToken, url.Values{"limit": {"2"}})
	rest := h.reviewQueue(t, tutorToken, url.Values{"limit": {"2"}, "cursor": {*first.NextCursor}})
	filtered := h.reviewQueue(t, tutorToken, url.Values{"student": {cards[11]}})

	listed := []string{first.Items[0].SubmissionID, first.Items[1].SubmissionID, rest.Items[0].SubmissionID}
	if !slices.Equal(listed, submissions) || rest.NextCursor != nil || len(rest.Items) != 1 {
		t.Fatalf("the queue lists %v, want %v", listed, submissions)
	}
	if first.Count != 2 || filtered.Count != 1 || len(filtered.Items) != 1 || filtered.Items[0].SubmissionID != submissions[2] {
		t.Fatalf("the counts are %d and %d", first.Count, filtered.Count)
	}
	if detail := h.issueDetail(t, tutorToken, issueOf(t, h, submissions[1])); len(detail.Submissions) != 1 || !detail.Submissions[0].HasDraft {
		t.Fatalf("the issue page does not flag the draft: %+v", detail.Submissions)
	}
	if first.Items[0].HasDraft || !first.Items[1].HasDraft {
		t.Fatal("the draft flags are wrong")
	}
}

func TestSubmissionEmailDailyCap(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	window, _ := domain.FixedWindow(h.now(t), domain.LessonEmailWindow)
	subject := h.keys.ThrottleSubject(string(domain.PurposeSubmissionEmails), parties.tutor.String())
	if _, err := h.database.Pool.Exec(t.Context(), "INSERT INTO rate_limit_counters (purpose, subject, window_start, count) VALUES ($1, $2, $3, $4)",
		string(domain.PurposeSubmissionEmails), subject, window, domain.SubmissionEmailsPerDay); err != nil {
		t.Fatal(err)
	}

	h.submittedByCard(t, parties.tutorToken, parties.card.ID, parties.studentToken, domain.AnswerPolicyNever)
	h.tick(t)

	if len(h.mailTo(t, tutorEmail)) != 0 || !strings.Contains(h.logs.String(), `"reason":"daily_cap"`) {
		t.Fatal("the 51st submissions email of the day was not dropped as daily_cap")
	}
	if rows := eventsNamed(h.notificationsOf(t, parties.tutorToken), string(domain.NotificationSubmitted)); len(rows) != 1 {
		t.Fatalf("the capped tutor's rows are %+v", rows)
	}
}

func TestReviewSaveBounds(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	long := strings.Repeat("ж", domain.ReviewCommentMaxRunes+1)
	tooMany := make([]string, 0, domain.CorrectedFilesPerAnswer+1)
	for range domain.CorrectedFilesPerAnswer + 1 {
		tooMany = append(tooMany, randomSubmissionID)
	}
	bounds := map[string]struct {
		item reviewItemJSON
		path string
		code string
	}{
		"points on an automatic question":  {scored(workQuestion(1), 1), "body.items[0].points", "not_allowed"},
		"points above the maximum":         {scored(workQuestion(5), 6), "body.items[0].points", "invalid"},
		"an override on a manual question": {overridden(workQuestion(5), 1, "Why"), "body.items[0].override_points", "not_allowed"},
		"an override without a reason":     {overridden(workQuestion(1), 1, ""), "body.items[0].override_reason", "required"},
		"a long reason": {overridden(workQuestion(1), 1, strings.Repeat("r", domain.OverrideReasonMaxRunes+1)), "body.items[0].override_reason",
			"too_long"},
		"a long comment":           {commented(scored(workQuestion(5), 1), long), "body.items[0].comment", "too_long"},
		"files on a text question": {commented(scored(workQuestion(5), 1), "", randomSubmissionID), "body.items[0].corrected_attachment_ids", "not_allowed"},
		"too many files":           {commented(scored(workQuestion(6), 1), "", tooMany...), "body.items[0].corrected_attachment_ids", "too_many"},
	}
	answers := map[string]response{}

	for name, bound := range bounds {
		answers[name] = h.saveReview(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "", bound.item))
	}

	for name, bound := range bounds {
		assertRefused(t, name, answers[name], http.StatusUnprocessableEntity, "validation_failed")
		if !strings.Contains(answers[name].body, `"path":"`+bound.path+`","code":"`+bound.code+`"`) {
			t.Errorf("%s answered %s", name, answers[name].body)
		}
	}
}

func TestReviewCorrectedFileRefusals(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	classmate := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	otherWork := h.submittedByCard(t, parties.tutorToken, classmate, h.studentToken(t, 10), domain.AnswerPolicyNever)
	foreignScope := h.correctedFile(t, parties.tutorToken, otherWork, workQuestion(6), "other.pdf", 30)

	unknown := h.saveReview(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "", commented(scored(workQuestion(6), 1), "", randomSubmissionID)))
	otherFile := h.saveReview(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "", commented(scored(workQuestion(6), 1), "", foreignScope)))

	assertRefused(t, "an unknown corrected file", unknown, http.StatusNotFound, "not_found")
	assertRefused(t, "another submission's corrected upload", otherFile, http.StatusNotFound, "not_found")
}

func TestReviewDraftReplayAndConflict(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)

	skipping := h.saveReview(t, parties.tutorToken, parties.submission, reviewJSON(t, 2, 0, saveID(1), ""))
	created := h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "First"))
	replayed := h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "First"))
	stale := h.saveReview(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(2), "Second"))

	assertRefused(t, "a draft skipping a revision", skipping, http.StatusConflict, "conflict")
	assertRefused(t, "a stale draft version", stale, http.StatusConflict, "conflict")
	if created.Revisions[0].Version != 1 || replayed.Revisions[0].Version != 1 {
		t.Fatalf("the replay changed the draft: %+v", replayed.Revisions)
	}
}

func TestReviewSaveOverQuota(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	pending := h.correctedFile(t, parties.tutorToken, parties.submission, workQuestion(6), "over.pdf", 10)
	if _, err := h.database.Pool.Exec(t.Context(), "UPDATE tutor_profiles SET file_quota_bytes = file_usage_bytes WHERE user_id = $1::uuid",
		parties.tutor.String()); err != nil {
		t.Fatal(err)
	}

	overQuota := h.saveReview(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "", commented(scored(workQuestion(6), 1), "", pending)))

	assertRefused(t, "a corrected file over the quota", overQuota, http.StatusInsufficientStorage, "quota_exceeded")
	if h.countRows(t, "SELECT count(id) FROM review_revisions WHERE submission_id = $1::uuid", parties.submission) != 0 {
		t.Fatal("a refused save kept its draft")
	}
}

func TestReviewSaveRateLimit(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	var statuses []int

	for index := range 3 {
		answer := h.send(t, http.MethodPut, submissionsRoute+parties.submission+"/review", reviewJSON(t, 9, 0, saveID(byte(10+index)), "Burst"),
			jsonCall(parties.tutorToken))
		statuses = append(statuses, answer.status)
	}

	if !slices.Contains(statuses, http.StatusTooManyRequests) {
		t.Fatalf("three review saves within one second answered %v", statuses)
	}
}

func TestReviewRevisionCap(t *testing.T) {
	t.Parallel()
	h := newHomeworkHarnessWithCaps(t, domain.HomeworkCaps{RevisionsPerReview: 1})
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "", fullyScored("")...))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 1)

	capped := h.saveReview(t, parties.tutorToken, parties.submission, reviewJSON(t, 2, 0, saveID(2), "More"))

	assertRefused(t, "a revision past the cap", capped, http.StatusUnprocessableEntity, "validation_failed")
	if !strings.Contains(capped.body, `"path":"body","code":"too_many"`) {
		t.Fatalf("the cap answered %s", capped.body)
	}
}

func TestTutorPurgeReviews(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	corrected := h.correctedFile(t, parties.tutorToken, parties.submission, workQuestion(6), "corrected.pdf", 80)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Good", fullyScored("Nice", corrected)...))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 1)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 2, 0, saveID(2), "Draft", fullyScored("Nice", corrected)...))
	h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	for name, query := range map[string]string{
		"revisions":  "SELECT count(id) FROM review_revisions WHERE tutor_id = $1::uuid",
		"references": "SELECT count(attachment_id) FROM attachment_references WHERE ref_kind = 'review_item' AND $1::text <> ''",
		"charges":    "SELECT count(*) FROM quota_charges WHERE tutor_id = $1::uuid",
	} {
		if count := h.countRows(t, query, parties.tutor.String()); count != 0 {
			t.Errorf("the purged tutor keeps %d %s", count, name)
		}
	}
	if !h.storageDeleteQueued(t, corrected) {
		t.Fatal("the corrected file's deletion was not queued")
	}
}

func TestStudentPurgeReviews(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	corrected := h.correctedFile(t, parties.tutorToken, parties.submission, workQuestion(6), "corrected.pdf", 80)
	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "Good", fullyScored("Nice", corrected)...))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewCheck, 1)
	usage := h.fileUsage(t, parties.tutor)
	h.mustAnswer(t, h.deleteMe(t, parties.studentToken, accountPassword), http.StatusNoContent)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)

	if h.countRows(t, "SELECT count(id) FROM review_revisions WHERE submission_id = $1::uuid", parties.submission) != 0 ||
		h.reviewItemReferences(t, corrected) != 0 || usage-h.fileUsage(t, parties.tutor) != 80+pdfHeaderBytes || !h.storageDeleteQueued(t, corrected) {
		t.Fatal("the student's purge kept the review or its corrected file")
	}
}

func TestReviewRecordsCarryNoContent(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.submittedForReview(t, domain.AnswerPolicyNever)
	corrected := h.correctedFile(t, parties.tutorToken, parties.submission, workQuestion(6), "secret-correction.pdf", 80)

	h.reviewSaved(t, parties.tutorToken, parties.submission, reviewJSON(t, 1, 0, saveID(1), "hush-feedback", fullyScored("murmured-comment", corrected)...))
	h.published(t, parties.tutorToken, parties.submission, domain.ReviewReturn, 1)
	h.tick(t)

	if h.countLogs(`"msg":"homework.review.saved"`) != 1 || h.countLogs(`"msg":"homework.review.published"`, `"action":"return"`) != 1 {
		t.Fatal("the review was not recorded")
	}
	for _, secret := range []string{"hush-feedback", "murmured-comment", "secret-correction", "Trip to Paris", "/api/v1/files/", "We saw the tower"} {
		if h.countLogs(secret) != 0 {
			t.Errorf("a record carries %q", secret)
		}
	}
	if h.countLogs(`"points"`) != 0 || h.countLogs(`"final_points"`) != 0 {
		t.Fatal("a record carries a score")
	}
}

func TestCorrectedFileMintsFollowReadability(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	type mintParty struct {
		id         byte
		card       string
		issue      string
		submission string
		file       string
		revision   string
	}
	parties := map[string]*mintParty{"readable": {id: 10}, "reclaim_pending": {id: 11}, "discarded": {id: 12}, "ended": {id: 13}, "marked": {id: 14}}
	for _, party := range parties {
		party.card = h.linkedCard(t, tutor, party.id, domain.RelationshipActive)
		party.submission = h.submittedByCard(t, tutorToken, party.card, h.studentToken(t, party.id), domain.AnswerPolicyNever)
		party.issue = issueOf(t, h, party.submission)
		party.file = h.correctedFile(t, tutorToken, party.submission, workQuestion(6), "corrected.pdf", 40)
		h.reviewSaved(t, tutorToken, party.submission, reviewJSON(t, 1, 0, saveID(1), "Good", fullyScored("Nice", party.file)...))
		h.published(t, tutorToken, party.submission, domain.ReviewCheck, 1)
		party.revision = h.revisionID(t, party.submission, 1)
	}

	readableParty := parties["readable"]
	h.linkedCard(t, tutor, 16, domain.RelationshipActive)
	sharedStudent := domain.UserID{readableParty.id}
	secondTutor, secondTutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, memberEmail(2))
	h.seedCard(t, secondTutor, 30, domain.RelationshipActive, &sharedStudent)
	crossParty := []crossPartyMint{
		{label: "a classmate's corrected file mint", token: h.studentToken(t, 16), attachment: readableParty.file, reference: "review_item:" + readableParty.revision},
		{label: "a second tutor's corrected file mint", token: secondTutorToken, attachment: readableParty.file, reference: "review_item:" + readableParty.revision},
	}

	h.setRelationshipState(t, parties["reclaim_pending"].card, domain.RelationshipReclaimPending)
	h.setRelationshipState(t, parties["discarded"].card, domain.RelationshipDiscarded)
	h.setRelationshipState(t, parties["ended"].card, domain.RelationshipEnded)
	h.markIssueForErasure(t, parties["marked"].issue)

	crossAnswers := h.crossPartyMints(t, crossParty)
	minted, read := map[string]response{}, map[string]response{}
	for name, party := range parties {
		token := h.studentToken(t, party.id)
		minted[name], _ = h.addressOf(t, token, party.file, "?ref=review_item:"+party.revision)
		read[name] = h.submissionAnswer(t, token, party.submission)
	}
	tutorMint, _ := h.addressOf(t, tutorToken, parties["discarded"].file, "?ref=review_item:"+parties["discarded"].revision)

	h.mustAnswer(t, tutorMint, http.StatusOK)
	assertCrossPartyMintsRefused(t, crossAnswers)
	for name := range parties {
		minted, read := minted[name], read[name]
		readable := name == "readable" || name == "ended"
		if (minted.status == http.StatusOK) != readable || (read.status == http.StatusOK) != readable {
			t.Errorf("%s: the corrected file mint answered %s and the read %s", name, describe(minted), describe(read))
		}
		if readable && !strings.Contains(read.body, `"comment":"Nice"`) {
			t.Errorf("%s: the review is missing from %s", name, read.body)
		}
	}
}
