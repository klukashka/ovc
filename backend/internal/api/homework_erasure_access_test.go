package api_test

import (
	"net/http"
	"net/url"
	"testing"

	"ovc/backend/internal/domain"
)

func TestMarkedHomeworkIsUnavailableToBothRoles(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
	h := f.h
	h.submitted(t, f.studentToken, f.submission, h.studentWork(t, f.studentToken, f.submission).Version)
	corrected := h.correctedFile(t, f.tutorToken, f.submission, workQuestion(6), "corrected.pdf", 30)
	h.reviewSaved(t, f.tutorToken, f.submission, reviewJSON(t, 1, 0, saveID(40), "Feedback", fullyScored("Comment", corrected)...))
	h.published(t, f.tutorToken, f.submission, domain.ReviewCheck, 1)
	revision := h.revisionID(t, f.submission, 1)
	image := h.libraryUpload(t, f.tutorToken, domain.ContextPromptImage, "snapshot.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, image, http.StatusCreated)
	imageID := uploadedID(t, image)
	template := h.createTemplate(t, f.tutorToken, "Snapshot", nil, imageQuestion(blockID(43, 1), imageID))
	issued := h.issueAnswer(t, f.tutorToken, issueKey(99), issueJSON(template.ID, toCard(f.card)))
	h.mustAnswer(t, issued, http.StatusCreated)
	pictured := decodeLibrary[issueItemBody](t, issued)
	kinds := []struct{ file, reference string }{{imageID, "issue_snapshot:" + pictured.ID}, {f.file, "answer:" + f.submission}, {corrected, "review_item:" + revision}}
	addresses := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		answer, address := h.addressOf(t, f.studentToken, kind.file, "?ref="+kind.reference)
		h.mustAnswer(t, answer, http.StatusOK)
		addresses = append(addresses, address.URL)
	}
	_, pending := h.issueWork(t, f.tutorToken, toCard(f.card), "Needs checking", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	h.submitted(t, f.studentToken, pending[0].id, 1)
	beforeQueue := h.reviewQueue(t, f.tutorToken, nil)

	h.mustAnswer(t, h.removeStudent(t, f.tutorToken, f.card), http.StatusOK)
	denied := []response{
		h.issueDetailAnswer(t, f.tutorToken, f.issue), h.submissionAnswer(t, f.tutorToken, f.submission), h.submissionAnswer(t, f.studentToken, f.submission),
		h.withdrawIssue(t, f.tutorToken, f.issue), h.issueAnswer(t, f.tutorToken, issueKey(99), issueJSON(template.ID, toCard(f.card))),
		h.send(t, http.MethodPatch, issuesRoute+"/"+f.issue, `{"version":1,"deadline_at":null}`, jsonCall(f.tutorToken)),
		h.saveAnswers(t, f.studentToken, f.submission, answersJSON(t, 3, saveID(41), typedAnswer(workQuestion(5), "No access"))),
		h.submitWork(t, f.studentToken, f.submission, 3),
		h.publishReview(t, f.tutorToken, f.submission, domain.ReviewCheck, 1),
		h.send(t, http.MethodPut, submissionsRoute+f.submission+"/review", reviewJSON(t, 2, 0, saveID(42), "No access", fullyScored("No access")...), jsonCall(f.tutorToken)),
		h.homeworkUpload(t, f.tutorToken, homeworkRoute(domain.ContextCorrected, f.submission, workQuestion(6)), "refused.pdf", pdfFile(10)),
		h.homeworkUpload(t, f.studentToken, homeworkRoute(domain.ContextAnswer, f.submission, workQuestion(6)), "refused.pdf", pdfFile(10)),
	}
	for index, kind := range kinds {
		denied = append(denied, h.fetch(t, addresses[index], nil))
		for _, token := range []string{f.tutorToken, f.studentToken} {
			answer, _ := h.addressOf(t, token, kind.file, "?ref="+kind.reference)
			denied = append(denied, answer)
		}
	}

	for _, answer := range denied {
		assertRefused(t, "marked homework", answer, http.StatusNotFound, "not_found")
	}
	queue := h.reviewQueue(t, f.tutorToken, nil)
	if beforeQueue.Count != 1 || queue.Count != 0 {
		t.Fatalf("review queue count before=%d after=%d", beforeQueue.Count, queue.Count)
	}
	if len(h.assignments(t, f.studentToken, nil).Items) != 0 || len(h.issuesPage(t, f.tutorToken, url.Values{}).Items) != 0 || len(queue.Items) != 0 {
		t.Fatal("marked work remains in a list")
	}
}

func TestMarkedHomeworkLeavesStudentCardProgress(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.workParties(t, workBlocks()...)
	kept, _ := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Visible work", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	h.submitted(t, parties.studentToken, parties.submission, 1)
	filter := url.Values{"student": {parties.card.ID}}
	before := h.issuesPage(t, parties.tutorToken, filter)
	beforeProgress := studentCardProgress(before.Items)

	h.markIssueForErasure(t, parties.issue)
	after := h.issuesPage(t, parties.tutorToken, filter)
	afterProgress := studentCardProgress(after.Items)
	queue := h.reviewQueue(t, parties.tutorToken, filter)

	if beforeProgress != (issueProgressBody{Students: 2, Submitted: 1, NeedsChecking: 1}) {
		t.Fatalf("initial student-card progress: %+v", beforeProgress)
	}
	if len(after.Items) != 1 || after.Items[0].ID != kept || afterProgress != (issueProgressBody{Students: 1}) || queue.Count != 0 {
		t.Fatalf("marked student-card progress: items=%+v progress=%+v queue=%d", after.Items, afterProgress, queue.Count)
	}
}

func studentCardProgress(issues []issueItemBody) issueProgressBody {
	var total issueProgressBody
	for _, issue := range issues {
		total.Students += issue.Progress.Students
		total.Submitted += issue.Progress.Submitted
		total.Checked += issue.Progress.Checked
		total.Returned += issue.Progress.Returned
		total.Late += issue.Progress.Late
		total.NeedsChecking += issue.Progress.NeedsChecking
	}
	return total
}
