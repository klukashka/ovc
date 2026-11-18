package api_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

type closingResults struct {
	card     string
	work     readableWork
	closesAt time.Time
}

func (h *authHarness) checkedResultsOfADeletedTutor(t *testing.T, endedFirst bool) closingResults {
	t.Helper()
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	template := h.readableTemplate(t, tutorToken, 81)
	student := h.seedStudentAccount(t, 10, domain.AccountActive)
	card := h.seedCard(t, tutor, 10, domain.RelationshipActive, &student)
	token := h.studentToken(t, 10)
	work := readableWork{image: template.image}
	work.lesson = h.scheduled(t, tutorToken, oneOffFor(card, "2030-03-12", "05:00")).Lessons[0].ID
	work.issue = h.issued(t, tutorToken, issueJSONWith(template.template, toCard(card), &work.lesson, nil, string(domain.AnswerPolicyAfterCheck))).ID
	work.submission = h.submissionsOf(t, work.issue)[0].id
	work.answer = h.answerFile(t, token, work.submission, workQuestion(6), "answer.pdf", 48)
	draft := h.saved(t, token, work.submission, answersJSON(t, 1, saveID(1), attached(workQuestion(6), work.answer), typedAnswer(workQuestion(5), "We saw the tower")))
	h.submitted(t, token, work.submission, draft.Version)
	work.corrected = h.correctedFile(t, tutorToken, work.submission, workQuestion(6), "corrected.pdf", 40)
	h.reviewSaved(t, tutorToken, work.submission, reviewJSON(t, 1, 0, saveID(1), "Good", fullyScored("Nice", work.corrected)...))
	h.published(t, tutorToken, work.submission, domain.ReviewCheck, 1)
	work.revision = h.revisionID(t, work.submission, 1)
	if endedFirst {
		h.mustAnswer(t, h.endAsTutor(t, tutorToken, card), http.StatusOK)
	}
	h.tick(t)
	h.mustAnswer(t, h.deleteMe(t, tutorToken, accountPassword), http.StatusNoContent)
	return closingResults{card: card, work: work, closesAt: h.deletedAtOf(t, tutor).Add(domain.DeletedAccountRetention)}
}

type resultsView struct {
	read        response
	checked     int
	tutors      []myTutor
	tutorFilter response
	calendar    response
	snapshot    response
	answer      response
	corrected   response
	write       response
	upload      response
}

func (h *authHarness) resultsViewOf(t *testing.T, results closingResults) resultsView {
	t.Helper()
	token := h.studentToken(t, 10)
	_, tutors := h.myTutors(t, token)
	snapshot, _ := h.addressOf(t, token, results.work.image, "?ref=issue_snapshot:"+results.work.issue)
	answer, _ := h.addressOf(t, token, results.work.answer, "?ref=answer:"+results.work.submission)
	corrected, _ := h.addressOf(t, token, results.work.corrected, "?ref=review_item:"+results.work.revision)
	now := h.now(t)
	return resultsView{
		read:        h.submissionAnswer(t, token, results.work.submission),
		checked:     len(h.assignments(t, token, url.Values{"status": {string(domain.AssignmentsChecked)}}).Items),
		tutors:      tutors,
		tutorFilter: h.assignmentsAnswer(t, token, url.Values{"tutor": {results.card}}),
		calendar:    h.calendarCall(t, token, now.Add(-24*time.Hour), now.Add(24*time.Hour), results.card),
		snapshot:    snapshot,
		answer:      answer,
		corrected:   corrected,
		write:       h.saveAnswersNow(t, token, results.work.submission, answersJSON(t, 2, saveID(2), typedAnswer(workQuestion(5), "Later"))),
		upload:      h.homeworkUpload(t, token, homeworkRoute(domain.ContextAnswer, results.work.submission, workQuestion(6)), "late.pdf", pdfFile(16)),
	}
}

func (h *authHarness) closedResultsProblems(t *testing.T, view resultsView, served response) []string {
	t.Helper()
	token := h.studentToken(t, 10)
	randomRead := h.submissionAnswer(t, token, randomSubmissionID)
	randomMint, _ := h.addressOf(t, token, fixtureID(81, 9), "?ref=answer:"+randomSubmissionID)
	var problems []string
	for name, refused := range map[string]response{
		"the read": view.read, "the tutor filter": view.tutorFilter, "the calendar filter": view.calendar, "the write": view.write,
	} {
		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, randomRead) {
			problems = append(problems, name+" answered "+describe(refused))
		}
	}
	for name, refused := range map[string]response{
		"the snapshot mint": view.snapshot, "the answer mint": view.answer, "the corrected file mint": view.corrected, "the earlier address": served,
	} {
		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, randomMint) {
			problems = append(problems, name+" answered "+describe(refused))
		}
	}
	if view.upload.status != http.StatusNotFound {
		problems = append(problems, "the answer upload answered "+describe(view.upload))
	}
	if view.checked != 0 || len(view.tutors) != 0 {
		problems = append(problems, "the checked list or My tutors still lists the deleted tutor's work")
	}
	return problems
}

func TestResultsReadableUntilAccountClosedUntil(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	results := h.checkedResultsOfADeletedTutor(t, false)

	h.at(t, results.closesAt.Add(-time.Minute))
	before := h.resultsViewOf(t, results)
	_, earlier := h.addressOf(t, h.studentToken(t, 10), results.work.corrected, "?ref=review_item:"+results.work.revision)
	servedBefore := h.fetch(t, earlier.URL, nil)
	h.at(t, results.closesAt)
	atClosure := h.resultsViewOf(t, results)
	servedAtClosure := h.fetch(t, earlier.URL, nil)
	h.tick(t)
	afterPurge := h.resultsViewOf(t, results)
	servedAfterPurge := h.fetch(t, earlier.URL, nil)

	for name, ok := range map[string]response{
		"the read": before.read, "the tutor filter": before.tutorFilter, "the calendar filter": before.calendar, "the snapshot mint": before.snapshot,
		"the answer mint": before.answer, "the corrected file mint": before.corrected, "the earlier address": servedBefore,
	} {
		if ok.status != http.StatusOK {
			t.Errorf("a minute before account_closed_until %s answered %s", name, describe(ok))
		}
	}
	if before.write.status != http.StatusConflict || before.upload.status != http.StatusConflict {
		t.Errorf("a minute before account_closed_until the write answered %s and the upload %s", describe(before.write), describe(before.upload))
	}
	if before.checked != 1 || len(before.tutors) != 1 || before.tutors[0].AccountClosedUntil == nil || !before.tutors[0].AccountClosedUntil.Equal(results.closesAt) {
		t.Errorf("a minute before account_closed_until the checked list holds %d items and My tutors %+v", before.checked, before.tutors)
	}
	if problems := h.closedResultsProblems(t, atClosure, servedAtClosure); len(problems) != 0 {
		t.Errorf("at account_closed_until before the purge: %v", problems)
	}
	if h.countRows(t, "SELECT count(id) FROM submissions WHERE id = $1::uuid", results.work.submission) != 0 {
		t.Error("the purge job kept the deleted tutor's submission")
	}
	if problems := h.closedResultsProblems(t, afterPurge, servedAfterPurge); len(problems) != 0 {
		t.Errorf("after the purge job: %v", problems)
	}
}

type closingRows struct {
	ended   []notificationItem
	checked []notificationItem
}

func (h *authHarness) closingRowsOf(t *testing.T) closingRows {
	t.Helper()
	items := h.notificationsOf(t, h.studentToken(t, 10))
	return closingRows{
		ended:   eventsNamed(items, string(domain.NotificationRelationshipEnded)),
		checked: eventsNamed(items, string(domain.NotificationSubmissionChecked)),
	}
}

func labelled(item notificationItem) bool {
	return item.Label != nil && item.Link != nil
}

func TestNotificationsOfADeletedTutorLoseNamesAtAccountClosedUntil(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	results := h.checkedResultsOfADeletedTutor(t, true)
	h.tick(t)

	h.at(t, results.closesAt.Add(-time.Minute))
	before := h.closingRowsOf(t)
	h.at(t, results.closesAt)
	atClosure := h.closingRowsOf(t)
	h.tick(t)
	afterPurge := h.closingRowsOf(t)

	if len(before.ended) != 1 || !labelled(before.ended[0]) || before.ended[0].Colour == nil || len(before.checked) != 1 || !labelled(before.checked[0]) {
		t.Fatalf("a minute before account_closed_until the rows are %+v", before)
	}
	if len(atClosure.ended) != 1 || atClosure.ended[0].Label != nil || atClosure.ended[0].Colour != nil || atClosure.ended[0].Link != nil ||
		len(atClosure.checked) != 1 || atClosure.checked[0].Label != nil || atClosure.checked[0].Link != nil || atClosure.checked[0].Colour != nil {
		t.Fatalf("at account_closed_until the rows are %+v", atClosure)
	}
	if len(afterPurge.ended) != 0 || len(afterPurge.checked) != 0 {
		t.Fatalf("after the purge job the rows are %+v", afterPurge)
	}
}
