package api_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
)

func TestAutosaveReplayAndConflict(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.workParties(t, workBlocks()...)
	options := h.workOptions(t, parties.studentToken, parties.submission)
	first := answersJSON(t, 1, saveID(1), chosen(workQuestion(1), options[workQuestion(1)][0]))

	saved := h.saved(t, parties.studentToken, parties.submission, first)
	replayed := h.saved(t, parties.studentToken, parties.submission, first)
	stale := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(2), typedAnswer(workQuestion(5), "Late")))

	if saved.Status != string(domain.SubmissionDraft) || saved.Version != 2 || len(saved.Answers) != 1 {
		t.Fatalf("the first save answered %+v", saved)
	}
	if replayed.Version != 2 || len(replayed.Answers) != 1 {
		t.Fatalf("the replayed save answered %+v", replayed)
	}
	assertRefused(t, "a save of a stale version", stale, http.StatusConflict, "conflict")
	if h.countLogs(`"msg":"homework.save_conflict"`, `"kind":"answers"`) != 1 {
		t.Fatal("the conflict was not recorded")
	}
}

func TestAutosaveRefusals(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.workParties(t, workBlocks()...)
	options := h.workOptions(t, parties.studentToken, parties.submission)
	version := func() int64 { return h.studentWork(t, parties.studentToken, parties.submission).Version }
	tooLong := strings.Repeat("a", domain.AnswerShortTextMaxRunes+1)
	structural := map[string]struct {
		answers []answerJSON
		field   string
	}{
		"an unknown question":        {[]answerJSON{typedAnswer(blockID(71, 1), "x")}, "body.answers[0].question_id:invalid"},
		"a repeated question":        {[]answerJSON{typedAnswer(workQuestion(5), "a"), typedAnswer(workQuestion(5), "b")}, "body.answers[1].question_id:invalid"},
		"a value of the wrong shape": {[]answerJSON{typedAnswer(workQuestion(1), "Paris")}, "body.answers[0].value:invalid"},
		"an option of another question": {
			[]answerJSON{chosen(workQuestion(1), options[workQuestion(2)][0])}, "body.answers[0].value.option_ids:invalid",
		},
		"two options for a single choice": {
			[]answerJSON{chosen(workQuestion(1), options[workQuestion(1)]...)}, "body.answers[0].value.option_ids:invalid",
		},
		"a repeated option":          {[]answerJSON{chosen(workQuestion(2), options[workQuestion(2)][0], options[workQuestion(2)][0])}, "body.answers[0].value.option_ids:invalid"},
		"a long short text":          {[]answerJSON{typedAnswer(workQuestion(3), tooLong)}, "body.answers[0].value.text:too_long"},
		"a long number text":         {[]answerJSON{typedAnswer(workQuestion(4), strings.Repeat("1", 65))}, "body.answers[0].value.text:too_long"},
		"a text past its max length": {[]answerJSON{typedAnswer(workQuestion(5), strings.Repeat("я", 51))}, "body.answers[0].value.text:too_long"},
		"too many files": {
			[]answerJSON{attached(workQuestion(6), fixtureID(72, 1), fixtureID(72, 2), fixtureID(72, 3))}, "body.answers[0].value.attachment_ids:too_many",
		},
		"a file that is not an identifier": {[]answerJSON{attached(workQuestion(6), "photo")}, "body.answers[0].value.attachment_ids:invalid"},
	}

	structuralAnswers := map[string]response{}
	for name, testCase := range structural {
		structuralAnswers[name] = h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(3), testCase.answers...))
	}
	unknownFile := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(4), attached(workQuestion(6), fixtureID(72, 1))))
	exact := h.saved(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(5),
		typedAnswer(workQuestion(3), strings.Repeat("ё", domain.AnswerShortTextMaxRunes)), typedAnswer(workQuestion(5), strings.Repeat("я", 50))))
	submitted := h.submitted(t, parties.studentToken, parties.submission, exact.Version)
	afterSubmit := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, submitted.Version, saveID(6), typedAnswer(workQuestion(5), "x")))
	h.setSubmissionStatus(t, parties.submission, domain.SubmissionReturned)
	frozen := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, version(), saveID(7), typedAnswer(workQuestion(3), "Сосна")))
	revised := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, version(), saveID(8), typedAnswer(workQuestion(5), "Revised")))
	h.setSubmissionStatus(t, parties.submission, domain.SubmissionClosed)
	closed := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, version(), saveID(10), typedAnswer(workQuestion(5), "Closed")))
	h.setSubmissionStatus(t, parties.submission, domain.SubmissionReturned)
	h.setRelationshipState(t, parties.card.ID, domain.RelationshipEnded)
	ended := h.saveAnswers(t, parties.studentToken, parties.submission, answersJSON(t, version(), saveID(9), typedAnswer(workQuestion(5), "After")))

	for name, testCase := range structural {
		answer := structuralAnswers[name]
		if answer.status != http.StatusUnprocessableEntity || fieldList(t, answer) != testCase.field {
			t.Errorf("%s answered %s", name, describe(answer))
		}
	}
	assertRefused(t, "an attachment that is not the student's pending upload", unknownFile, http.StatusNotFound, "not_found")
	assertRefused(t, "a save of submitted work", afterSubmit, http.StatusConflict, "not_editable")
	assertRefused(t, "a save of an automatic answer of returned work", frozen, http.StatusConflict, "answer_frozen")
	h.mustAnswer(t, revised, http.StatusOK)
	assertRefused(t, "a save on an ended relationship", ended, http.StatusConflict, "not_editable")
	assertRefused(t, "a save of closed work", closed, http.StatusConflict, "not_editable")
}

func TestAutosaveRates(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.workParties(t, workBlocks()...)

	var answers []response
	for index := range domain.AnswerSavesPerSecond + 1 {
		answers = append(answers, h.saveAnswersNow(t, parties.studentToken, parties.submission,
			answersJSON(t, int64(index+1), saveID(byte(20+index)), typedAnswer(workQuestion(5), "Draft"))))
	}

	limited := answers[len(answers)-1]
	if limited.status != http.StatusTooManyRequests || code(t, limited) != "rate_limited" || limited.headers.Get("Retry-After") == "" {
		t.Fatalf("the third save in a second answered %s", describe(limited))
	}
	if h.countLogs(`"msg":"homework.rate_limited"`, `"limit":"answer_saves_second"`) != 1 {
		t.Fatal("the rate limit was not recorded")
	}
}

func TestSubmitScoresAndFreezes(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.workParties(t, workBlocks()...)
	options := h.workOptions(t, parties.studentToken, parties.submission)
	draft := h.saved(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(1),
		chosen(workQuestion(1), options[workQuestion(1)][0]), typedAnswer(workQuestion(3), "  ЕЛКА "), typedAnswer(workQuestion(4), "2,55"),
		typedAnswer(workQuestion(5), "We saw the tower")))

	submitted := h.submitted(t, parties.studentToken, parties.submission, draft.Version)
	review := h.tutorWork(t, parties.tutorToken, parties.submission)

	if submitted.Status != string(domain.SubmissionSubmitted) || len(submitted.EditableQuestionIDs) != 0 || submitted.Version != draft.Version+1 {
		t.Fatalf("the submission answered %+v", submitted)
	}
	if review.AutoPoints == nil || *review.AutoPoints != 7 || review.FinalPoints == nil || *review.FinalPoints != 7 || review.MaxPoints != 21 {
		t.Fatalf("the tutor sees the scores %v %v of %d", review.AutoPoints, review.FinalPoints, review.MaxPoints)
	}
	want := map[string]struct {
		points     int32
		correct    bool
		normalised string
	}{
		workQuestion(1): {2, true, ""}, workQuestion(2): {0, false, ""}, workQuestion(3): {3, true, "елка"}, workQuestion(4): {2, true, "2.55"},
	}
	frozen := 0
	for _, answer := range review.Answers {
		expected, automatic := want[answer.QuestionID]
		if !automatic {
			if answer.FrozenAt != nil || answer.AutoPoints != nil {
				t.Errorf("the manual answer %s was scored: %+v", answer.QuestionID, answer)
			}
			continue
		}
		frozen++
		normalised := ""
		if answer.Normalised != nil {
			normalised = *answer.Normalised
		}
		if answer.FrozenAt == nil || answer.AutoPoints == nil || *answer.AutoPoints != expected.points || answer.AutoCorrect == nil ||
			*answer.AutoCorrect != expected.correct || normalised != expected.normalised {
			t.Errorf("the automatic answer %s was stored as %+v", answer.QuestionID, answer)
		}
		textual := answer.QuestionID == workQuestion(3) || answer.QuestionID == workQuestion(4)
		if textual != (answer.NormaliserVersion != nil && *answer.NormaliserVersion == 1) {
			t.Errorf("the answer %s carries the normaliser version %v", answer.QuestionID, answer.NormaliserVersion)
		}
	}
	if frozen != 4 {
		t.Fatalf("%d automatic answers are frozen, the unanswered one included, want 4", frozen)
	}
	if len(review.Revisions) != 0 {
		t.Fatalf("the tutor sees revisions %v before any review", review.Revisions)
	}
}

func TestSubmitRetryAfterSuccess(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.workParties(t, workBlocks()...)
	draft := h.saved(t, parties.studentToken, parties.submission, answersJSON(t, 1, saveID(1), typedAnswer(workQuestion(5), "Done")))

	first := h.submitted(t, parties.studentToken, parties.submission, draft.Version)
	retried := h.submitted(t, parties.studentToken, parties.submission, draft.Version)
	stale := h.submitWork(t, parties.studentToken, parties.submission, 1)
	h.tick(t)

	if first.Status != string(domain.SubmissionSubmitted) || retried.Status != first.Status || retried.Version != first.Version {
		t.Fatalf("the retried submit answered %+v after %+v", retried, first)
	}
	h.mustAnswer(t, stale, http.StatusOK)
	if submitted := eventsNamed(h.notificationsOf(t, parties.tutorToken), string(domain.NotificationSubmitted)); len(submitted) != 1 ||
		submitted[0].Link == nil || submitted[0].Link.Kind != "review" || submitted[0].Label == nil || *submitted[0].Label != cardName {
		t.Fatalf("the tutor got the rows %+v", submitted)
	}
	if mail := h.mailTo(t, tutorEmail); len(mail) != 1 || !strings.Contains(mail[0].Text, "/homework/review/"+parties.submission) {
		t.Fatalf("the tutor got the emails %v", subjects(mail))
	}
}

func TestSubmitWithoutManualQuestionsChecks(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.workParties(t, automaticBlocks()...)
	frames := h.recordFrames(t)

	checked := h.submitted(t, parties.studentToken, parties.submission, 1)
	h.tick(t)

	if checked.Status != string(domain.SubmissionChecked) || h.submissionField(t, parties.submission, "checked_at") == "" {
		t.Fatalf("fully automatic work answered %+v", checked)
	}
	rows := eventsNamed(h.notificationsOf(t, parties.studentToken), string(domain.NotificationSubmissionChecked))
	if len(rows) != 1 || rows[0].Label == nil || *rows[0].Label != "Trip to Paris" || rows[0].Link == nil || rows[0].Link.Kind != "assignment" {
		t.Fatalf("the student got the rows %+v", rows)
	}
	if h.countRows(t, "SELECT count(id) FROM notifications WHERE idempotency_key = 'submission.checked:' || replace($1, '-', '') || ':0'", parties.submission) != 1 {
		t.Fatal("the checked row is not keyed by revision 0")
	}
	if len(eventsNamed(h.notificationsOf(t, parties.tutorToken), string(domain.NotificationSubmitted))) != 0 || len(h.mailTo(t, tutorEmail)) != 0 {
		t.Fatal("the tutor was told about work that needs no checking")
	}
	if mail := h.mailTo(t, studentEmail); len(eventsWithSubject(mail, "Assignment checked")) != 1 {
		t.Fatalf("the student got the emails %v", subjects(mail))
	}
	sent := frames.settle(t, h)
	if !hasFrame(sent, parties.student, domain.EntitySubmission, parties.submission) || !hasFrame(sent, parties.tutor, domain.EntitySubmission, parties.submission) {
		t.Fatalf("the submission sent the frames %v", sent)
	}
}

func TestLateSubmission(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	deadline := h.now(t).Add(10 * time.Minute)
	_, early := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Early", string(domain.AnswerPolicyNever), &deadline, workBlocks()...)
	_, late := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Late", string(domain.AnswerPolicyNever), &deadline, workBlocks()...)

	onTime := h.submitted(t, parties.studentToken, early[0].id, 1)
	h.advance(t, 20*time.Minute)
	overdue := h.submitted(t, accessToken(t, h.login(t, studentEmail, accountPassword)), late[0].id, 1)

	if onTime.Late || !overdue.Late {
		t.Fatalf("the work before and after the deadline is late %v and %v", onTime.Late, overdue.Late)
	}
	if h.countLogs(`"msg":"homework.submission.submitted"`, `"late":true`) != 1 || h.countLogs(`"msg":"homework.submission.submitted"`, `"late":false`) != 1 {
		t.Fatal("the submissions were not recorded with their lateness")
	}
}

func TestRemovedMemberKeepsWork(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	_, submissions := h.issueWork(t, parties.tutorToken, toGroup(parties.group.ID), "Group work", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "",
		`{"remove":`+idList([]string{parties.card.ID})+`}`), http.StatusOK)
	if h.membershipsOf(t, parties.card.ID) != 0 {
		t.Fatal("the member was not removed")
	}

	draft := h.saved(t, parties.studentToken, submissions[0].id, answersJSON(t, 1, saveID(1), typedAnswer(workQuestion(5), "Still mine")))
	submitted := h.submitted(t, parties.studentToken, submissions[0].id, draft.Version)

	if submitted.Status != string(domain.SubmissionSubmitted) {
		t.Fatalf("the removed member's work answered %+v", submitted)
	}
}

func TestAssignmentsListsAndCounts(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	other := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	deadline := h.now(t).Add(48 * time.Hour)
	_, dated := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Dated", string(domain.AnswerPolicyNever), &deadline, workBlocks()...)
	_, undated := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Undated", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	_, done := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Done", string(domain.AnswerPolicyNever), nil, automaticBlocks()...)
	h.issueWork(t, parties.tutorToken, toCard(other), "Someone else's", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	h.submitted(t, parties.studentToken, done[0].id, 1)

	firstPage := h.assignments(t, parties.studentToken, url.Values{"limit": {"1"}})
	secondPage := h.assignments(t, parties.studentToken, url.Values{"limit": {"1"}, "cursor": {*firstPage.NextCursor}})
	checked := h.assignments(t, parties.studentToken, url.Values{"status": {"checked"}})
	foreignTutor := h.assignmentsAnswer(t, parties.studentToken, url.Values{"tutor": {other}})
	badCursor := h.assignmentsAnswer(t, parties.studentToken, url.Values{"cursor": {"!"}})

	if len(firstPage.Items) != 1 || firstPage.Items[0].SubmissionID != dated[0].id || len(secondPage.Items) != 1 ||
		secondPage.Items[0].SubmissionID != undated[0].id || secondPage.NextCursor != nil {
		t.Fatalf("the to do pages are %+v and %+v", firstPage, secondPage)
	}
	if firstPage.Counts.Todo != 2 || firstPage.Counts.Closed != 0 || len(checked.Items) != 1 || checked.Items[0].SubmissionID != done[0].id {
		t.Fatalf("the counts are %+v and the checked list %+v", firstPage.Counts, checked.Items)
	}
	assertRefused(t, "another student's tutor filter", foreignTutor, http.StatusNotFound, "not_found")
	if badCursor.status != http.StatusUnprocessableEntity || fieldList(t, badCursor) != "query.cursor:invalid" {
		t.Fatalf("a bad cursor answered %s", describe(badCursor))
	}
}

func eventsWithSubject(messages []mail.SinkMessage, prefix string) []mail.SinkMessage {
	var matching []mail.SinkMessage
	for _, message := range messages {
		if strings.HasPrefix(message.Subject, prefix) {
			matching = append(matching, message)
		}
	}
	return matching
}
