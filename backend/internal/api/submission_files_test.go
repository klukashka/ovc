package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const pdfHeaderBytes = 9

func TestFileAnswerBindingAndRelease(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.workParties(t, workBlocks()...)
	first := h.answerFile(t, parties.studentToken, parties.submission, workQuestion(6), "page one.pdf", 300)
	second := h.answerFile(t, parties.studentToken, parties.submission, workQuestion(6), "page two.pdf", 200)
	before := h.fileUsage(t, parties.tutor)

	bound := h.saved(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(1), attached(workQuestion(6), first)))
	afterBinding := h.fileUsage(t, parties.tutor)
	unchanged := h.saved(t, parties.studentToken, parties.submission, answersJSON(t, bound.Version, saveID(2), attached(workQuestion(6), first)))
	afterRepeat := h.fileUsage(t, parties.tutor)
	replaced := h.saved(t, parties.studentToken, parties.submission, answersJSON(t, unchanged.Version, saveID(3), attached(workQuestion(6), second)))
	afterReplace := h.fileUsage(t, parties.tutor)
	reused := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, replaced.Version, saveID(4), attached(workQuestion(6), first)))

	answer, found := answerTo(bound.Answers, workQuestion(6))
	if !found || fileIDs(answer.Files) != first || answer.Files[0].Filename != "page one.pdf" || answer.Files[0].URL == "" {
		t.Fatalf("the bound file answer is %+v", bound.Answers)
	}
	if afterBinding-before != 300+pdfHeaderBytes || afterRepeat != afterBinding || afterReplace-before != 200+pdfHeaderBytes || h.chargesOf(t, first) != 0 || h.chargesOf(t, second) != 1 {
		t.Fatalf("the usage moved %d, %d, %d from %d", afterBinding, afterRepeat, afterReplace, before)
	}
	if !h.storageDeleteQueued(t, first) || h.attachmentStateOf(t, first) != "deleted" || h.storageDeleteQueued(t, second) {
		t.Fatal("the replaced file was not released with its storage deletion")
	}
	if replacedAnswer, _ := answerTo(replaced.Answers, workQuestion(6)); fileIDs(replacedAnswer.Files) != second {
		t.Fatalf("the replaced answer lists %+v", replacedAnswer.Files)
	}
	assertRefused(t, "a released file bound again", reused, http.StatusNotFound, "not_found")
	if h.submissionField(t, parties.submission, "file_bytes") != "209" {
		t.Fatalf("the submission counts %s file bytes", h.submissionField(t, parties.submission, "file_bytes"))
	}
	if h.countLogs(`"msg":"file.reference.added"`, `"ref_kind":"answer"`) != 2 || h.countLogs(`"msg":"file.reference.removed"`, `"ref_kind":"answer"`) != 1 {
		t.Fatal("the answer references were not recorded")
	}
}

func TestFileAnswerScopeAndQuota(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.workParties(t, workBlocks()...)
	otherCard := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	_, others := h.issueWork(t, parties.tutorToken, toCard(otherCard), "Other work", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	elsewhere := h.answerFile(t, h.studentToken(t, 10), others[0].id, workQuestion(6), "other.pdf", 64)
	own := h.answerFile(t, parties.studentToken, parties.submission, workQuestion(6), "mine.pdf", 64)

	_, sibling := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Sibling work", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	ownElsewhere := h.answerFile(t, parties.studentToken, sibling[0].id, workQuestion(6), "sibling.pdf", 64)
	otherQuestion := h.answerFile(t, parties.studentToken, parties.submission, workQuestion(6), "other-question.pdf", 64)
	h.exec2(t, "UPDATE attachments SET scope_question_id = $2::uuid WHERE id = $1::uuid", otherQuestion, workQuestion(5))
	expired := h.answerFile(t, parties.studentToken, parties.submission, workQuestion(6), "expired.pdf", 64)
	if _, err := h.database.Pool.Exec(t.Context(), "UPDATE attachments SET expires_at = created_at - interval '1 second' WHERE id = $1::uuid",
		expired); err != nil {
		t.Fatal(err)
	}

	foreign := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(1), attached(workQuestion(6), elsewhere)))
	refused := map[string]response{}
	for name, file := range map[string]string{"another submission's upload": ownElsewhere, "another question's upload": otherQuestion, "an expired upload": expired} {
		refused[name] = h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(5), attached(workQuestion(6), file)))
	}
	repeated := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(6), attached(workQuestion(6), own, strings.ToUpper(own))))
	h.setQuota(t, parties.tutor, 100, 90)
	full := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(2), attached(workQuestion(6), own)))

	assertRefused(t, "another student's upload", foreign, http.StatusNotFound, "not_found")
	for name, answer := range refused {
		assertRefused(t, name, answer, http.StatusNotFound, "not_found")
	}
	if repeated.status != http.StatusUnprocessableEntity || fieldList(t, repeated) != "body.answers[0].value.attachment_ids:invalid" {
		t.Fatalf("a file named twice in two spellings answered %s", describe(repeated))
	}
	assertRefused(t, "a binding over the tutor's quota", full, http.StatusInsufficientStorage, "quota_exceeded")
	if h.answerReferences(t, parties.submission) != 0 || h.attachmentStateOf(t, own) != "pending" {
		t.Fatal("a refused binding changed the references")
	}
	if h.countLogs(`"msg":"quota.exceeded"`, `"context":"answer"`) != 1 {
		t.Fatal("the quota refusal was not recorded")
	}
}

func TestSubmissionFileBytesCap(t *testing.T) {
	t.Parallel()
	h := newHomeworkHarnessWithCaps(t, domain.HomeworkCaps{SubmissionFileBytes: 400})
	parties := h.workParties(t, workBlocks()...)
	first := h.answerFile(t, parties.studentToken, parties.submission, workQuestion(6), "first.pdf", 250)
	second := h.answerFile(t, parties.studentToken, parties.submission, workQuestion(6), "second.pdf", 250)

	refused := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(1), attached(workQuestion(6), first, second)))
	accepted := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(2), attached(workQuestion(6), first)))

	if refused.status != http.StatusUnprocessableEntity || fieldList(t, refused) != "body.answers:too_many" {
		t.Fatalf("a save past the file bytes cap answered %s", describe(refused))
	}
	h.mustAnswer(t, accepted, http.StatusOK)
}

func TestWithdrawReleasesDraftFiles(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.workParties(t, workBlocks()...)
	file := h.answerFile(t, parties.studentToken, parties.submission, workQuestion(6), "draft.pdf", 128)
	h.saved(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(1), attached(workQuestion(6), file)))
	before := h.fileUsage(t, parties.tutor)

	h.mustAnswer(t, h.withdrawIssue(t, parties.tutorToken, parties.issue), http.StatusOK)

	if before-h.fileUsage(t, parties.tutor) != 128+pdfHeaderBytes || h.answerReferences(t, parties.submission) != 0 || !h.storageDeleteQueued(t, file) {
		t.Fatal("the withdrawal kept the draft file")
	}
	assertRefused(t, "the withdrawn work", h.submissionAnswer(t, parties.studentToken, parties.submission), http.StatusNotFound, "not_found")
	if h.submissionField(t, parties.submission, "file_bytes") != "0" {
		t.Fatal("the withdrawn submission still counts file bytes")
	}
}

func TestEndingClosesOpenWork(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	card := toCard(parties.card.ID)
	never := string(domain.AnswerPolicyNever)
	_, unstarted := h.issueWork(t, parties.tutorToken, card, "Unstarted", never, nil, workBlocks()...)
	_, drafted := h.issueWork(t, parties.tutorToken, card, "Drafted", never, nil, workBlocks()...)
	_, returned := h.issueWork(t, parties.tutorToken, card, "Returned", never, nil, workBlocks()...)
	_, handedIn := h.issueWork(t, parties.tutorToken, card, "Handed in", never, nil, workBlocks()...)
	draftFile := h.answerFile(t, parties.studentToken, drafted[0].id, workQuestion(6), "draft.pdf", 100)
	h.saved(t, parties.studentToken, drafted[0].id, answersJSON(t, 1, saveID(1), attached(workQuestion(6), draftFile)))
	earlier := h.answerFile(t, parties.studentToken, returned[0].id, workQuestion(6), "earlier.pdf", 110)
	firstSave := h.saved(t, parties.studentToken, returned[0].id, answersJSON(t, 1, saveID(2), attached(workQuestion(6), earlier)))
	h.submitted(t, parties.studentToken, returned[0].id, firstSave.Version)
	h.setSubmissionStatus(t, returned[0].id, domain.SubmissionReturned)
	h.advance(t, time.Minute)
	later := h.answerFile(t, parties.studentToken, returned[0].id, workQuestion(6), "later.pdf", 120)
	revision := h.studentWork(t, parties.studentToken, returned[0].id)
	h.saved(t, parties.studentToken, returned[0].id, answersJSON(t, revision.Version, saveID(3), attached(workQuestion(6), earlier, later)))
	handedFile := h.answerFile(t, parties.studentToken, handedIn[0].id, workQuestion(6), "handed.pdf", 130)
	handedSave := h.saved(t, parties.studentToken, handedIn[0].id, answersJSON(t, 1, saveID(4), attached(workQuestion(6), handedFile)))
	h.submitted(t, parties.studentToken, handedIn[0].id, handedSave.Version)
	frames := h.recordFrames(t)
	usage := h.fileUsage(t, parties.tutor)

	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	afterEnding := h.fileUsage(t, parties.tutor)
	h.tick(t)

	statuses := map[string]string{
		unstarted[0].id: "closed", drafted[0].id: "closed", returned[0].id: "closed", handedIn[0].id: "submitted",
	}
	for submission, want := range statuses {
		if got := h.submissionField(t, submission, "status"); got != want {
			t.Errorf("the submission %s is %s, want %s", submission, got, want)
		}
	}
	if afterEnding != usage {
		t.Fatalf("the ending released charges itself: %d before, %d after", usage, afterEnding)
	}
	if usage-h.fileUsage(t, parties.tutor) != 220+2*pdfHeaderBytes || h.chargesOf(t, earlier) != 1 || h.chargesOf(t, handedFile) != 1 {
		t.Fatalf("the cleanup released %d bytes", usage-h.fileUsage(t, parties.tutor))
	}
	if !h.storageDeleteQueued(t, draftFile) || !h.storageDeleteQueued(t, later) || h.storageDeleteQueued(t, earlier) {
		t.Fatal("the cleanup did not release exactly the never-submitted files")
	}
	if h.countRows(t, "SELECT count(id) FROM submissions WHERE release_pending_since IS NOT NULL AND relationship_id = $1::uuid", parties.card.ID) != 0 {
		t.Fatal("the cleanup left releases pending")
	}
	closedWork := h.studentWork(t, parties.studentToken, returned[0].id)
	if answer, _ := answerTo(closedWork.Answers, workQuestion(6)); fileIDs(answer.Files) != earlier || closedWork.Status != "closed" {
		t.Fatalf("the closed work lists %+v", closedWork.Answers)
	}
	sent := frames.settle(t, h)
	checkStudentHomeworkFrames(t, sent, parties.student)
	for _, submission := range []string{unstarted[0].id, drafted[0].id, returned[0].id} {
		if !hasFrame(sent, parties.student, domain.EntitySubmission, submission) || !hasFrame(sent, parties.tutor, domain.EntitySubmission, submission) {
			t.Errorf("closing %s sent the frames %v", submission, sent)
		}
	}
	if h.countLogs(`"msg":"homework.submissions.closed"`, `"count":3`, `"cause":"ending"`) != 1 {
		t.Fatal("the closing was not recorded")
	}
	review := h.submitWork(t, parties.studentToken, unstarted[0].id, 2)
	assertRefused(t, "submitting closed work", review, http.StatusConflict, "not_editable")
}

func TestAutosaveRacesEnding(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.workParties(t, workBlocks()...)
	file := h.answerFile(t, parties.studentToken, parties.submission, workQuestion(6), "racing.pdf", 140)
	usage := h.fileUsage(t, parties.tutor)

	release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM relationships WHERE id = $1::uuid FOR UPDATE", parties.card.ID)
	saving := h.sendInBackground(http.MethodPut, submissionsRoute+parties.submission+"/answers",
		answersJSON(t, 1, saveID(1), attached(workQuestion(6), file)), jsonCall(parties.studentToken))
	h.waitForLockWaiters(t, 1)
	ending := h.sendInBackground(http.MethodPost, "/api/v1/students/"+parties.card.ID+"/end", "", jsonCall(parties.tutorToken))
	h.waitForBlockedStatements(t, 2)
	release()
	saved := awaitAnswer(t, saving)
	ended := awaitAnswer(t, ending)
	afterEnding := h.fileUsage(t, parties.tutor)
	h.tick(t)

	h.mustAnswer(t, saved, http.StatusOK)
	h.mustAnswer(t, ended, http.StatusOK)
	if h.submissionField(t, parties.submission, "status") != "closed" || afterEnding-usage != 140+pdfHeaderBytes {
		t.Fatalf("after the race the work is %s with %d bytes charged", h.submissionField(t, parties.submission, "status"), afterEnding-usage)
	}
	if h.fileUsage(t, parties.tutor) != usage || !h.storageDeleteQueued(t, file) {
		t.Fatal("the cleanup did not release the draft file bound during the ending")
	}
}

func TestIssueWithdrawRacesSubmit(t *testing.T) {
	t.Parallel()
	t.Run("withdrawal first", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		parties := h.workParties(t, workBlocks()...)

		release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM issues WHERE id = $1::uuid FOR UPDATE", parties.issue)
		withdrawal := h.sendInBackground(http.MethodPost, issuesRoute+"/"+parties.issue+"/withdraw", "", jsonCall(parties.tutorToken))
		h.waitForLockWaiters(t, 1)
		submission := h.sendInBackground(http.MethodPost, submissionsRoute+parties.submission+"/submit", `{"version":1}`, jsonCall(parties.studentToken))
		h.waitForLockWaiters(t, 2)
		release()

		h.mustAnswer(t, awaitAnswer(t, withdrawal), http.StatusOK)
		assertRefused(t, "a submission of withdrawn work", awaitAnswer(t, submission), http.StatusNotFound, "not_found")
	})
	t.Run("submission first", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		parties := h.workParties(t, workBlocks()...)

		release := h.holdRow(t, "SELECT "+lockedRowPosition+" FROM issues WHERE id = $1::uuid FOR UPDATE", parties.issue)
		submission := h.sendInBackground(http.MethodPost, submissionsRoute+parties.submission+"/submit", `{"version":1}`, jsonCall(parties.studentToken))
		h.waitForLockWaiters(t, 1)
		withdrawal := h.sendInBackground(http.MethodPost, issuesRoute+"/"+parties.issue+"/withdraw", "", jsonCall(parties.tutorToken))
		h.waitForLockWaiters(t, 2)
		release()

		h.mustAnswer(t, awaitAnswer(t, submission), http.StatusOK)
		assertRefused(t, "a withdrawal of submitted work", awaitAnswer(t, withdrawal), http.StatusConflict, "issue_has_submissions")
	})
}

func TestSubmissionRecordsCarryNoContent(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	_, submissions := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Confidential voyage", string(domain.AnswerPolicyNever), nil,
		workBlocks()...)
	submission := submissions[0].id
	file := h.answerFile(t, parties.studentToken, submission, workQuestion(6), "private-diary.pdf", 64)

	draft := h.saved(t, parties.studentToken, submission, answersJSON(t, 1, saveID(1), typedAnswer(workQuestion(3), "whispered-answer"),
		typedAnswer(workQuestion(5), "an intimate essay"), attached(workQuestion(6), file)))
	h.submitted(t, parties.studentToken, submission, draft.Version)
	h.tutorWork(t, parties.tutorToken, submission)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	h.tick(t)

	if h.countLogs(`"msg":"homework.submission.saved"`) != 1 || h.countLogs(`"msg":"homework.submission.submitted"`, `"submission_id":"`+submission+`"`) != 1 {
		t.Fatal("the save and the submission were not recorded")
	}
	for _, secret := range []string{"Confidential voyage", "whispered", "intimate essay", "private-diary", "Name the tree", "Ёлка", "елка", "/api/v1/files/"} {
		if h.countLogs(secret) != 0 {
			t.Errorf("a record carries %q", secret)
		}
	}
	if h.countLogs(`"auto_points"`) != 0 || h.countLogs(`"final_points"`) != 0 {
		t.Fatal("a record carries a score")
	}
}
