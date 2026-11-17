package api_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

func TestReclaimKeepCuesWork(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"explicit keep", "invitation merge"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			parties := h.issueParties(t)
			card, student := h.squat(t, parties.tutorToken, newcomerEmail, newcomerEmail)
			squatter := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
			h.setAccount(t, student, "Earlier account holder sentinel", domain.LanguageEnglish)
			issue, work := h.issueWork(t, parties.tutorToken, toCard(card.ID), "Kept work", string(domain.AnswerPolicyNever), nil, workBlocks()...)
			h.saved(t, squatter, work[0].id, answersJSON(t, 1, saveID(1), typedAnswer(workQuestion(5), "Preserved answer")))
			_, submitted := h.issueWork(t, parties.tutorToken, toCard(card.ID), "For review", string(domain.AnswerPolicyNever), nil, workBlocks()...)
			h.submitted(t, squatter, submitted[0].id, 1)
			owner := h.reclaimed(t, newcomerEmail)
			h.setAccount(t, student, "Current account holder", domain.LanguageEnglish)
			link := ""
			if path == "invitation merge" {
				link = linkToken(t, h.newCard(t, parties.tutorToken, "Replacement", "").Invitation)
			}

			if path == "explicit keep" {
				h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
			} else {
				h.mustAnswer(t, h.acceptSignedIn(t, owner, link), http.StatusOK)
			}
			fresh, freshWork := h.issueWork(t, parties.tutorToken, toCard(card.ID), "New work", string(domain.AnswerPolicyNever), nil, workBlocks()...)
			projections := []response{
				h.submissionAnswer(t, owner, work[0].id), h.submissionAnswer(t, parties.tutorToken, work[0].id),
				h.assignmentsAnswer(t, owner, nil), h.issuesAnswer(t, parties.tutorToken, url.Values{"student": {card.ID}}),
				h.issueDetailAnswer(t, parties.tutorToken, issue), h.reviewQueueAnswer(t, parties.tutorToken, nil),
			}

			for _, projection := range projections {
				h.mustAnswer(t, projection, http.StatusOK)
				if !strings.Contains(projection.body, `"issued_before_reclaim":true`) || strings.Contains(projection.body, "Earlier account holder sentinel") || strings.Contains(projection.body, newcomerEmail) {
					t.Fatalf("unsafe or absent reclaim cue: %s", projection.body)
				}
			}
			kept := h.studentWork(t, owner, work[0].id)
			if kept.Status != "draft" || !strings.Contains(projections[0].body, "Preserved answer") {
				t.Fatal("keep discarded answers or closed the work")
			}
			if strings.Contains(h.submissionAnswer(t, owner, freshWork[0].id).body, `"issued_before_reclaim":true`) || strings.Contains(h.issueDetailAnswer(t, parties.tutorToken, fresh).body, `"issued_before_reclaim":true`) {
				t.Fatal("new work inherited the reclaim cue")
			}
		})
	}
}

func TestReclaimPendingHidesWork(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.issueParties(t)
	card, _ := h.squat(t, parties.tutorToken, newcomerEmail, newcomerEmail)
	squatter := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	upload := h.libraryUpload(t, parties.tutorToken, domain.ContextPromptImage, "hidden.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, upload, http.StatusCreated)
	file := uploadedID(t, upload)
	blocks := append(workBlocks(), imageQuestion(blockID(43, 1), file))
	template := h.workTemplate(t, parties.tutorToken, "Hidden work", blocks...)
	issue := h.issued(t, parties.tutorToken, issueJSON(template, toCard(card.ID)))
	reviewSubmission := h.submittedByCard(t, parties.tutorToken, card.ID, squatter, domain.AnswerPolicyNever)
	h.reviewSaved(t, parties.tutorToken, reviewSubmission, reviewJSON(t, 1, 0, saveID(20), "Hidden feedback", fullyScored("Hidden")...))
	submission := h.submissionsOf(t, issue.ID)[0].id
	before := h.notificationsOf(t, squatter)
	h.mustAnswer(t, h.markRead(t, squatter, fmt.Sprintf(`{"ids":[%q]}`, before[0].ID)), http.StatusOK)
	before = h.notificationsOf(t, squatter)
	owner := h.reclaimed(t, newcomerEmail)
	student := h.userID(t, newcomerEmail)
	frames := h.recordFrames(t)
	mails := len(h.mailTo(t, newcomerEmail))

	hiddenRead := h.markRead(t, owner, fmt.Sprintf(`{"ids":[%q]}`, before[0].ID))
	hiddenThrough := h.markRead(t, owner, fmt.Sprintf(`{"through":%q}`, before[0].ID))
	hiddenAnswer, hidden := h.notificationPage(t, owner, "?limit=1")
	inaccessible := h.submissionAnswer(t, owner, submission)
	assignments := h.assignments(t, owner, nil)
	deniedWrites := []response{
		h.saveAnswers(t, owner, submission, answersJSON(t, 1, saveID(21), typedAnswer(workQuestion(5), "Refused"))),
		h.submitWork(t, owner, submission, 1),
		h.homeworkUpload(t, owner, homeworkRoute(domain.ContextAnswer, submission, workQuestion(6)), "refused.pdf", pdfFile(20)),
	}
	minted, _ := h.addressOf(t, owner, file, "?ref=issue_snapshot:"+issue.ID)
	deniedWrites = append(deniedWrites, minted)
	h.published(t, parties.tutorToken, reviewSubmission, domain.ReviewCheck, 1)
	h.tick(t)
	hiddenFrames := frames.settle(t, h)
	checkStudentHomeworkFrames(t, hiddenFrames, student)
	hiddenMails := len(h.mailTo(t, newcomerEmail))
	keepFrames := h.recordFrames(t)
	h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
	restored := h.notificationsOf(t, owner)
	restoredFrames := keepFrames.settle(t, h)
	h.mustAnswer(t, h.withdrawIssue(t, parties.tutorToken, issue.ID), http.StatusOK)
	withdrawn := h.notificationsOf(t, owner)

	assertRefused(t, "hidden notification read", hiddenRead, http.StatusNotFound, "not_found")
	assertRefused(t, "hidden notification read through", hiddenThrough, http.StatusNotFound, "not_found")
	h.mustAnswer(t, hiddenAnswer, http.StatusOK)
	if len(hidden.Items) != 0 || hidden.UnreadCount != 0 || hidden.NextCursor != nil || len(assignments.Items) != 0 {
		t.Fatalf("hidden work leaked: %+v", hidden)
	}
	assertRefused(t, "reclaimed submission", inaccessible, http.StatusNotFound, "not_found")
	for _, answer := range deniedWrites {
		assertRefused(t, "reclaimed write or mint", answer, http.StatusNotFound, "not_found")
	}
	if hasFrame(hiddenFrames, student, domain.EntitySubmission, reviewSubmission) || notificationFrames(hiddenFrames, student) != 0 || hiddenMails != mails || notificationFrames(restoredFrames, student) != 0 {
		t.Fatal("hidden work reached the student or keep replayed its notifications")
	}
	if len(before) != len(restored) || len(restored) == 0 || before[0].ID != restored[0].ID || before[0].ReadAt == nil || restored[0].ReadAt == nil || !before[0].ReadAt.Equal(*restored[0].ReadAt) {
		t.Fatal("keep did not restore stored notification state")
	}
	found := false
	for _, item := range withdrawn {
		if item.Event == "issue.withdrawn" {
			found = true
			if item.Label != nil || item.Link != nil {
				t.Fatal("withdrawn work retained a title or link")
			}
		}
	}
	if !found {
		t.Fatal("lifecycle filtering hid issue.withdrawn")
	}
}
