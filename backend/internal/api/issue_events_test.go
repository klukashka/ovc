package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const issueTitle = "Trip to Paris"

func TestIssueFramesRecipients(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	cards := h.activeCards(t, parties.tutor, 10, 3)
	outsider := h.linkedCard(t, parties.tutor, 13, domain.RelationshipActive)
	group := h.newGroup(t, parties.tutorToken, "Evening club", cards)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, group.ID, "", `{"remove":`+idList([]string{cards[2]})+`}`), http.StatusOK)
	frames := h.recordFrames(t)
	first, second, removed, other := domain.UserID{10}, domain.UserID{11}, domain.UserID{12}, domain.UserID{13}

	issue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toGroup(group.ID)))

	sent := frames.settle(t, h)
	checkStudentHomeworkFrames(t, sent, first)
	checkStudentHomeworkFrames(t, sent, second)
	stored := h.submissionsOf(t, issue.ID)
	if len(stored) != 2 || outsider == "" {
		t.Fatalf("the issue holds %d submissions", len(stored))
	}
	if !hasFrame(sent, parties.tutor, domain.EntityIssue, issue.ID) {
		t.Fatal("the tutor got no issue frame")
	}
	for _, row := range stored {
		student := domain.UserID{10}
		if row.student == second.String() {
			student = second
		}
		if !hasFrame(sent, parties.tutor, domain.EntitySubmission, row.id) || !hasFrame(sent, student, domain.EntitySubmission, row.id) {
			t.Fatalf("the submission %s was not signalled to the tutor and its student", row.id)
		}
	}
	if got := entityKinds(sent, first); len(got) != 1 {
		t.Fatalf("the first student got frames %v", got)
	}
	for name, user := range map[string]domain.UserID{"a removed member": removed, "another student of the tutor": other} {
		if got := framesTo(sent, user); len(got) != 0 {
			t.Fatalf("%s got frames %v", name, got)
		}
	}
	created := eventsNamed(h.notificationsOf(t, h.studentToken(t, 10)), string(domain.NotificationIssueCreated))
	if len(created) != 1 || created[0].Label == nil || *created[0].Label != issueTitle || created[0].Link == nil ||
		created[0].Link.Kind != "assignment" || created[0].Colour == nil || notificationFrames(sent, first) != 1 {
		t.Fatalf("the student's issue.created rows are %+v", created)
	}
	if rows := h.notificationsOf(t, h.studentToken(t, 12)); len(rows) != 0 {
		t.Fatalf("the removed member got rows %+v", rows)
	}
}

func TestWithdrawNotifiesReadableStudents(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	cards := h.activeCards(t, parties.tutor, 10, 3)
	group := h.newGroup(t, parties.tutorToken, "Evening club", cards)
	lesson := h.scheduled(t, parties.tutorToken, groupLessonFor(group.ID, "2030-03-12", "10:00")).Lessons[0].ID
	issue := h.issued(t, parties.tutorToken, issueJSONWith(parties.template.ID, toGroup(group.ID), &lesson, nil, "after_check"))
	h.setRelationshipState(t, cards[2], domain.RelationshipReclaimPending)
	frames := h.recordFrames(t)
	readable, hidden := domain.UserID{10}, domain.UserID{12}

	withdrawn := h.withdrawIssue(t, parties.tutorToken, issue.ID)
	repeated := h.withdrawIssue(t, parties.tutorToken, issue.ID)

	h.mustAnswer(t, withdrawn, http.StatusOK)
	h.mustAnswer(t, repeated, http.StatusOK)
	if decodeLibrary[issueItemBody](t, withdrawn).Status != "withdrawn" || decodeLibrary[issueItemBody](t, repeated).Version != decodeLibrary[issueItemBody](t, withdrawn).Version {
		t.Fatalf("the withdrawal answered %s then %s", withdrawn.body, repeated.body)
	}
	sent := frames.settle(t, h)
	checkStudentHomeworkFrames(t, sent, readable, lesson)
	if issueFrame := onlyFrame(t, framesOfKind(homeworkFramesTo(sent, parties.tutor), domain.EntityIssue), "the tutor's issue frame"); issueFrame.lessons != lesson {
		t.Fatalf("the tutor's withdrawal frame is %+v", issueFrame)
	}
	if studentFrame := onlyFrame(t, homeworkFramesTo(sent, readable), "the readable student's frame"); studentFrame.lessons != lesson {
		t.Fatalf("the readable student's withdrawal frame is %+v", studentFrame)
	}
	submissions := h.submissionsOf(t, issue.ID)
	if !hasFrame(sent, readable, domain.EntitySubmission, submissions[0].id) || notificationFrames(sent, readable) != 1 {
		t.Fatalf("the readable student got frames %v", framesTo(sent, readable))
	}
	if got := framesTo(sent, hidden); len(got) != 0 {
		t.Fatalf("the reclaim-pending student got frames %v", got)
	}
	rows := h.notificationsOf(t, h.studentToken(t, 10))
	withdrawnRows, createdRows := eventsNamed(rows, string(domain.NotificationIssueWithdrawn)), eventsNamed(rows, string(domain.NotificationIssueCreated))
	if len(withdrawnRows) != 1 || withdrawnRows[0].Label != nil || withdrawnRows[0].Link != nil || len(createdRows) != 1 || createdRows[0].Label != nil {
		t.Fatalf("the readable student's rows after the withdrawal are %+v", rows)
	}
	if got := eventsNamed(h.notificationsOf(t, h.studentToken(t, 12)), string(domain.NotificationIssueWithdrawn)); len(got) != 0 {
		t.Fatalf("the reclaim-pending student got %+v", got)
	}
	listed := h.issuesPage(t, parties.tutorToken, map[string][]string{"status": {"withdrawn"}})
	if len(listed.Items) != 1 || h.issueDetailAnswer(t, parties.tutorToken, issue.ID).status != http.StatusOK {
		t.Fatal("the tutor no longer reads the withdrawn issue")
	}
}

func TestWithdrawRefusesSubmittedWork(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	issue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card)))
	h.submitStored(t, h.submissionsOf(t, issue.ID)[0].id, h.now(t))

	refused := h.withdrawIssue(t, parties.tutorToken, issue.ID)
	changed := h.patchIssue(t, parties.tutorToken, issue.ID, `{"deadline_at":null,"version":1}`)
	h.markIssueForErasure(t, issue.ID)
	marked := h.withdrawIssue(t, parties.tutorToken, issue.ID)

	if refused.status != http.StatusConflict || code(t, refused) != "issue_has_submissions" {
		t.Fatalf("withdrawing submitted work answered %s", describe(refused))
	}
	h.mustAnswer(t, changed, http.StatusOK)
	if marked.status != http.StatusNotFound || h.issueDetailAnswer(t, parties.tutorToken, issue.ID).status != http.StatusNotFound {
		t.Fatalf("an issue marked for erasure answered %s", describe(marked))
	}
	if listed := h.issuesPage(t, parties.tutorToken, nil); len(listed.Items) != 0 {
		t.Fatalf("an issue marked for erasure is listed: %+v", listed.Items)
	}
}

func TestIssueCreatedEmails(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	confirmed := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	unconfirmed := h.linkedCard(t, parties.tutor, 11, domain.RelationshipProvisional)
	withdrawnTo := h.linkedCard(t, parties.tutor, 12, domain.RelationshipActive)
	capped := h.linkedCard(t, parties.tutor, 13, domain.RelationshipActive)
	deadline := time.Date(2030, time.March, 20, 15, 0, 0, 0, time.UTC)
	sent := h.issued(t, parties.tutorToken, issueJSONWith(parties.template.ID, toCard(confirmed), nil, &deadline, "never"))
	h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(unconfirmed)))
	withdrawn := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(withdrawnTo)))
	h.mustAnswer(t, h.withdrawIssue(t, parties.tutorToken, withdrawn.ID), http.StatusOK)
	h.fillDailyEmails(t, domain.UserID{13}, domain.HomeworkEmailsPerDay)
	h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(capped)))

	h.tick(t)

	messages := h.mailTo(t, memberEmail(10))
	submission := h.submissionsOf(t, sent.ID)[0].id
	if len(messages) != 1 || !strings.Contains(messages[0].Subject, issueTitle) || !strings.Contains(messages[0].Text, publicOrigin+"/homework/"+submission) {
		t.Fatalf("the confirmed student got %+v", messages)
	}
	if !strings.Contains(messages[0].Text, "March 2030") || containsAny(messages[0].Text+messages[0].HTML, "The capital of France", "Rome", "Describe the trip", "points") {
		t.Fatalf("the email body is %q", messages[0].Text)
	}
	for name, email := range map[string]string{"the unconfirmed student": memberEmail(11), "a withdrawn issue's student": memberEmail(12), "a capped student": memberEmail(13)} {
		if got := h.mailTo(t, email); len(got) != 0 {
			t.Errorf("%s got %d emails", name, len(got))
		}
	}
	if !strings.Contains(h.logs.String(), `"reason":"daily_cap"`) {
		t.Fatal("the capped email was not dropped as daily_cap")
	}
	if rows := eventsNamed(h.notificationsOf(t, h.studentToken(t, 13)), string(domain.NotificationIssueCreated)); len(rows) != 1 {
		t.Fatalf("the capped student's row is missing: %+v", rows)
	}
}

func (h *authHarness) fillDailyEmails(t *testing.T, student domain.UserID, count int) {
	t.Helper()
	window, _ := domain.FixedWindow(h.now(t), domain.LessonEmailWindow)
	subject := h.keys.ThrottleSubject(string(domain.PurposeHomeworkEmails), student.String())
	if _, err := h.database.Pool.Exec(t.Context(), "INSERT INTO rate_limit_counters (purpose, subject, window_start, count) VALUES ($1, $2, $3, $4)",
		string(domain.PurposeHomeworkEmails), subject, window, count); err != nil {
		t.Fatal(err)
	}
}

func TestIssueRecordsCarryNoContent(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)

	issue := h.issueAnswer(t, parties.tutorToken, issueKey(7), issueJSON(parties.template.ID, toCard(card)))
	id := decodeLibrary[issueItemBody](t, issue).ID
	h.mustAnswer(t, h.patchIssue(t, parties.tutorToken, id, `{"deadline_at":"2030-04-01T10:00:00Z","version":1}`), http.StatusOK)
	h.mustAnswer(t, h.withdrawIssue(t, parties.tutorToken, id), http.StatusOK)
	h.tick(t)

	logs := h.logs.String()
	for _, event := range []string{"homework.issue.created", "homework.issue.updated", "homework.issue.withdrawn"} {
		if !strings.Contains(logs, `"msg":"`+event+`"`) {
			t.Errorf("no %s record", event)
		}
	}
	if containsAny(logs, issueTitle, "The capital of France", "Paris", "Describe the trip", issueKey(7), "A seeded student", "/api/v1/files/") {
		t.Fatal("a record carries a title, a prompt, an option, a display name, a key or an address")
	}
}
