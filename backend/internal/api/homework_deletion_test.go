package api_test

import (
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func TestTutorDeletionClosesWork(t *testing.T) {
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
	h.reviewSaved(t, parties.tutorToken, returned[0].id, reviewJSON(t, 1, 0, saveID(10), "Please revise", fullyScored("Please revise")...))
	h.published(t, parties.tutorToken, returned[0].id, domain.ReviewReturn, 1)
	h.advance(t, time.Minute)
	later := h.answerFile(t, parties.studentToken, returned[0].id, workQuestion(6), "later.pdf", 120)
	revision := h.studentWork(t, parties.studentToken, returned[0].id)
	h.saved(t, parties.studentToken, returned[0].id, answersJSON(t, revision.Version, saveID(3), attached(workQuestion(6), earlier, later)))
	handedFile := h.answerFile(t, parties.studentToken, handedIn[0].id, workQuestion(6), "handed.pdf", 130)
	handedSave := h.saved(t, parties.studentToken, handedIn[0].id, answersJSON(t, 1, saveID(4), attached(workQuestion(6), handedFile)))
	h.submitted(t, parties.studentToken, handedIn[0].id, handedSave.Version)
	_, checked := h.issueWork(t, parties.tutorToken, card, "Checked", never, nil, workBlocks()...)
	h.submitted(t, parties.studentToken, checked[0].id, 1)
	h.reviewSaved(t, parties.tutorToken, checked[0].id, reviewJSON(t, 1, 0, saveID(11), "Checked feedback", fullyScored("Checked")...))
	h.published(t, parties.tutorToken, checked[0].id, domain.ReviewCheck, 1)
	h.reviewSaved(t, parties.tutorToken, checked[0].id, reviewJSON(t, 2, 0, saveID(12), "Updated feedback", fullyScored("Updated")...))
	h.published(t, parties.tutorToken, checked[0].id, domain.ReviewCheck, 2)
	versions := map[string]int64{}
	for _, id := range []string{unstarted[0].id, drafted[0].id, returned[0].id, handedIn[0].id, checked[0].id} {
		versions[id] = h.studentWork(t, parties.studentToken, id).Version
	}
	otherTutor, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 20, secondTutorEmail)
	otherCard := h.linkedSignedIn(t, otherToken, parties.studentToken, "Other tutor card", studentEmail)
	_, otherWork := h.issueWork(t, otherToken, toCard(otherCard.ID), "Other work", never, nil, workBlocks()...)
	otherFile := h.answerFile(t, parties.studentToken, otherWork[0].id, workQuestion(6), "other.pdf", 90)
	h.saved(t, parties.studentToken, otherWork[0].id, answersJSON(t, 1, saveID(8), attached(workQuestion(6), otherFile)))
	otherUsage := h.fileUsage(t, otherTutor)
	addressAnswer, draftAddress := h.addressOf(t, parties.studentToken, draftFile, "?ref=answer:"+drafted[0].id)
	h.mustAnswer(t, addressAnswer, http.StatusOK)
	frames := h.recordFrames(t)
	usage := h.fileUsage(t, parties.tutor)

	h.mustAnswer(t, h.deleteMe(t, parties.tutorToken, accountPassword), http.StatusNoContent)
	afterEnding := h.fileUsage(t, parties.tutor)
	h.tick(t)

	statuses := map[string]string{
		unstarted[0].id: "closed", drafted[0].id: "closed", returned[0].id: "closed", handedIn[0].id: "submitted", checked[0].id: "checked",
	}
	for submission, want := range statuses {
		if got := h.submissionField(t, submission, "status"); got != want {
			t.Errorf("the submission %s is %s, want %s", submission, got, want)
		}
	}
	for id, before := range versions {
		expected := before
		if statuses[id] == "closed" {
			expected++
		}
		if h.studentWork(t, parties.studentToken, id).Version != expected {
			t.Fatalf("closure version mismatch for %s", id)
		}
	}
	for _, subject := range []string{"Assignment checked", "Assignment returned", "Review updated"} {
		if messages := eventsWithSubject(h.mailTo(t, studentEmail), subject); len(messages) != 0 {
			t.Fatalf("queued %s reached the student after the tutor's deletion", subject)
		}
	}
	if dropped := h.countLogs(`"msg":"email.dropped"`, `"group":"results"`, `"reason":"tutor_inactive"`); dropped != 3 {
		t.Fatalf("the deletion dropped %d results emails as tutor_inactive, want 3", dropped)
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
	assertRefused(t, "released answer address", h.fetch(t, draftAddress.URL, nil), http.StatusNotFound, "not_found")
	if h.submissionField(t, otherWork[0].id, "status") != "draft" || h.fileUsage(t, otherTutor) != otherUsage || h.chargesOf(t, otherFile) != 1 {
		t.Fatal("tutor deletion changed another tutor's work")
	}
	sent := frames.settle(t, h)
	checkStudentHomeworkFrames(t, sent, parties.student)
	if hasFrame(sent, otherTutor, domain.EntitySubmission, otherWork[0].id) {
		t.Fatal("another tutor received a closure frame")
	}
	for _, submission := range []string{unstarted[0].id, drafted[0].id, returned[0].id} {
		if !hasFrame(sent, parties.student, domain.EntitySubmission, submission) || !hasFrame(sent, parties.tutor, domain.EntitySubmission, submission) {
			t.Errorf("closing %s sent the frames %v", submission, sent)
		}
	}
	if h.countLogs(`"msg":"homework.submissions.closed"`, `"count":3`, `"cause":"tutor_deleted"`) != 1 {
		t.Fatal("the closing was not recorded")
	}
	review := h.submitWork(t, parties.studentToken, unstarted[0].id, 2)
	assertRefused(t, "submitting closed work", review, http.StatusConflict, "not_editable")
}
