package api_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

var (
	tutorLinkedIssueKeys   = []string{"deadline_at", "issue_id", "students", "submitted", "title"}
	studentLinkedIssueKeys = []string{"deadline_at", "late", "status", "submission_id", "title"}
)

type linkedIssueItem struct {
	IssueID      string `json:"issue_id"`
	SubmissionID string `json:"submission_id"`
	Title        string `json:"title"`
	Students     int64  `json:"students"`
	Submitted    int64  `json:"submitted"`
	Status       string `json:"status"`
	Late         bool   `json:"late"`
}

type linkedWork struct {
	status int
	keys   [][]string
	items  []linkedIssueItem
}

func linkedWorkOf(t *testing.T, result response) linkedWork {
	t.Helper()
	work := linkedWork{status: result.status}
	if result.status != http.StatusOK {
		return work
	}
	var page struct {
		Issues *[]json.RawMessage `json:"issues"`
	}
	if err := json.Unmarshal([]byte(result.body), &page); err != nil || page.Issues == nil {
		t.Fatalf("the lesson detail carries no issues list: %s", result.body)
	}
	for _, raw := range *page.Issues {
		var fields map[string]json.RawMessage
		var item linkedIssueItem
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatal(err)
		}
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		work.keys = append(work.keys, keys)
		work.items = append(work.items, item)
	}
	return work
}

func (h *authHarness) linkedWorkAs(t *testing.T, token string, lesson string) linkedWork {
	t.Helper()
	return linkedWorkOf(t, h.lessonPage(t, token, lesson))
}

func (work linkedWork) issueIDs() []string {
	ids := make([]string, 0, len(work.items))
	for _, item := range work.items {
		ids = append(ids, item.IssueID)
	}
	return ids
}

func (work linkedWork) submissionIDs() []string {
	ids := make([]string, 0, len(work.items))
	for _, item := range work.items {
		ids = append(ids, item.SubmissionID)
	}
	return ids
}

func (work linkedWork) keyedAs(allowed []string) bool {
	for _, keys := range work.keys {
		if !slices.Equal(keys, allowed) {
			return false
		}
	}
	return true
}

func (h *authHarness) issuedFor(t *testing.T, token string, template string, audience string, lesson *string) string {
	t.Helper()
	issue := h.issued(t, token, issueJSONWith(template, audience, lesson, nil, string(domain.AnswerPolicyAfterCheck)))
	h.exec2(t, "UPDATE issues SET issued_at = $2::timestamptz - make_interval(mins => (SELECT count(id) FROM issues)::int) WHERE id = $1::uuid",
		issue.ID, rfc3339(authStart))
	return issue.ID
}

func submissionOf(t *testing.T, h *authHarness, issue string, student domain.UserID) string {
	t.Helper()
	for _, row := range h.submissionsOf(t, issue) {
		if row.student == student.String() {
			return row.id
		}
	}
	t.Fatalf("the issue %s holds no submission of %s", issue, student)
	return ""
}

type linkedIssueFixture struct {
	parties        visibilityParties
	groupLesson    string
	groupRoster    string
	individual     string
	individualPast string
	provisional    string
	groupFirst     string
	groupSecond    string
	groupPast      string
	individualWork string
	pastWork       string
	provisionalWrk string
	squatterToken  string
	joinerToken    string
}

func (h *authHarness) linkedIssueFixture(t *testing.T) linkedIssueFixture {
	t.Helper()
	parties := h.visibilityParties(t)
	template := h.readyTemplate(t, parties.tutorToken, "Trip to Paris", 40).ID
	squatter := h.linkedCard(t, parties.tutor, 30, domain.RelationshipProvisional)
	joiner := h.linkedCard(t, parties.tutor, 12, domain.RelationshipActive)
	fixture := linkedIssueFixture{
		parties:        parties,
		groupLesson:    h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-12", "18:00")).Lessons[0].ID,
		groupRoster:    h.scheduled(t, parties.tutorToken, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0].ID,
		individual:     h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-14", "10:00")).Lessons[0].ID,
		individualPast: h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-15", "10:00")).Lessons[0].ID,
		provisional:    h.scheduled(t, parties.tutorToken, oneOffFor(squatter, "2030-03-16", "10:00")).Lessons[0].ID,
		squatterToken:  h.studentToken(t, 30),
		joinerToken:    h.studentToken(t, 12),
	}
	group, card := toGroup(parties.group), toCard(parties.card)
	fixture.groupFirst = h.issuedFor(t, parties.tutorToken, template, group, &fixture.groupLesson)
	withdrawn := h.issuedFor(t, parties.tutorToken, template, group, &fixture.groupLesson)
	marked := h.issuedFor(t, parties.tutorToken, template, group, &fixture.groupLesson)
	h.issuedFor(t, parties.tutorToken, template, group, nil)
	fixture.groupSecond = h.issuedFor(t, parties.tutorToken, template, group, &fixture.groupLesson)
	fixture.groupPast = h.issuedFor(t, parties.tutorToken, template, group, &fixture.groupRoster)
	fixture.individualWork = h.issuedFor(t, parties.tutorToken, template, card, &fixture.individual)
	fixture.pastWork = h.issuedFor(t, parties.tutorToken, template, card, &fixture.individualPast)
	fixture.provisionalWrk = h.issuedFor(t, parties.tutorToken, template, toCard(squatter), &fixture.provisional)
	h.mustAnswer(t, h.withdrawIssue(t, parties.tutorToken, withdrawn), http.StatusOK)
	h.markIssueForErasure(t, marked)
	h.submitStored(t, submissionOf(t, h, fixture.groupFirst, parties.student), h.now(t))
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group, "", `{"add":["`+joiner+`"]}`), http.StatusOK)
	return fixture
}

func TestLessonLinkedIssuesVisibility(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	fixture := h.linkedIssueFixture(t)
	parties := fixture.parties
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.student)

	tutorView := h.linkedWorkAs(t, parties.tutorToken, fixture.groupLesson)
	member := h.linkedWorkAs(t, parties.studentToken, fixture.groupLesson)
	otherMember := h.linkedWorkAs(t, h.studentToken(t, 11), fixture.groupLesson)
	joiner := h.linkedWorkAs(t, fixture.joinerToken, fixture.groupLesson)
	provisional := h.linkedWorkAs(t, fixture.squatterToken, fixture.provisional)
	secondTutor := h.lessonPage(t, secondToken, fixture.groupLesson)
	random := h.lessonPage(t, secondToken, randomCardID)

	if !slices.Equal(tutorView.issueIDs(), []string{fixture.groupSecond, fixture.groupFirst}) || !tutorView.keyedAs(tutorLinkedIssueKeys) {
		t.Fatalf("the tutor's linked issues are %v with keys %v, want the oldest issued first", tutorView.issueIDs(), tutorView.keys)
	}
	if later := tutorView.items[0]; later.Students != 2 || later.Submitted != 0 || later.Title != "Trip to Paris" {
		t.Fatalf("the tutor's oldest linked issue is %+v", later)
	}
	if submitted := tutorView.items[1]; submitted.Students != 2 || submitted.Submitted != 1 {
		t.Fatalf("the tutor's newer linked issue is %+v", submitted)
	}
	wantMember := []string{submissionOf(t, h, fixture.groupSecond, parties.student), submissionOf(t, h, fixture.groupFirst, parties.student)}
	if !slices.Equal(member.submissionIDs(), wantMember) || !member.keyedAs(studentLinkedIssueKeys) || member.items[1].Status != "submitted" {
		t.Fatalf("the member's linked work is %+v with keys %v", member.items, member.keys)
	}
	wantOther := []string{submissionOf(t, h, fixture.groupSecond, parties.otherStudent), submissionOf(t, h, fixture.groupFirst, parties.otherStudent)}
	if !slices.Equal(otherMember.submissionIDs(), wantOther) || otherMember.items[1].Status != "not_started" {
		t.Fatalf("the second member's linked work is %+v", otherMember.items)
	}
	if joiner.status != http.StatusOK || len(joiner.items) != 0 {
		t.Fatalf("a member who joined after the issues sees %d %+v", joiner.status, joiner.items)
	}
	wantProvisional := []string{submissionOf(t, h, fixture.provisionalWrk, domain.UserID{30})}
	if !slices.Equal(provisional.submissionIDs(), wantProvisional) || !provisional.keyedAs(studentLinkedIssueKeys) {
		t.Fatalf("the provisional student's linked work is %+v", provisional.items)
	}
	if secondTutor.status != http.StatusNotFound || refusal(t, secondTutor) != refusal(t, random) {
		t.Fatalf("the second tutor's lesson page answered %d %s", secondTutor.status, secondTutor.body)
	}
}

func TestLessonLinkedIssuesFollowFormerAudiences(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	fixture := h.linkedIssueFixture(t)
	parties := fixture.parties
	h.cancelWithRoster(t, fixture.groupRoster, parties.student, parties.otherStudent)
	h.cancelWithRoster(t, fixture.individualPast, parties.student)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group, "", `{"remove":["`+parties.other+`"]}`), http.StatusOK)
	h.endRelationship(t, parties.card)
	removedToken := h.studentToken(t, 11)

	removedScheduled := h.lessonPage(t, removedToken, fixture.groupLesson)
	removedRoster := h.linkedWorkAs(t, removedToken, fixture.groupRoster)
	endedScheduled := h.lessonPage(t, parties.studentToken, fixture.individual)
	endedRoster := h.linkedWorkAs(t, parties.studentToken, fixture.individualPast)
	random := h.lessonPage(t, parties.studentToken, randomCardID)

	for name, result := range map[string]response{"a removed member's scheduled group lesson": removedScheduled, "an ended relationship's scheduled lesson": endedScheduled} {
		if result.status != http.StatusNotFound || refusal(t, result) != refusal(t, random) {
			t.Errorf("%s answered %d %s", name, result.status, result.body)
		}
	}
	if want := []string{submissionOf(t, h, fixture.groupPast, parties.otherStudent)}; !slices.Equal(removedRoster.submissionIDs(), want) {
		t.Fatalf("the removed member's roster lesson lists %v, want only their own %v", removedRoster.submissionIDs(), want)
	}
	if want := []string{submissionOf(t, h, fixture.pastWork, parties.student)}; !slices.Equal(endedRoster.submissionIDs(), want) {
		t.Fatalf("the ended relationship's roster lesson lists %v, want %v", endedRoster.submissionIDs(), want)
	}
}

func TestOnlyTheLessonReadCarriesLinkedIssues(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	fixture := h.linkedIssueFixture(t)
	token := fixture.parties.tutorToken
	h.exec2(t, "UPDATE lessons SET status = 'ended', ended_at = $2::timestamptz WHERE id = $1::uuid", fixture.individual, rfc3339(authStart))

	created := h.createLessons(t, token, groupLessonFor(fixture.parties.group, "2030-03-20", "18:00"), "")
	edited := h.editLesson(t, token, fixture.groupLesson, lessonPatch{Scope: "this", Version: 1, Title: pointer("Renamed")})
	materials := h.attachMaterials(t, token, fixture.groupLesson, 2)
	cancelled := h.cancelCall(t, token, fixture.groupLesson, "this", 2)
	ended := h.endCall(t, token, fixture.individual)
	read := h.linkedWorkAs(t, token, fixture.groupLesson)

	answers := map[string]response{"creation": created, "an edit": edited, "materials": materials, "a cancellation": cancelled, "an end": ended}
	for name, result := range answers {
		if result.status != http.StatusOK && result.status != http.StatusCreated {
			t.Fatalf("%s answered %d %s", name, result.status, result.body)
		}
		if strings.Contains(result.body, `"issues"`) {
			t.Errorf("%s carries the linked issues: %s", name, result.body)
		}
	}
	if len(read.items) != 2 {
		t.Fatalf("the lesson read after the writes lists %+v", read.items)
	}
}
