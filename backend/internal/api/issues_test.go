package api_test

import (
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func optionIDsOf(blocks []templateBlockBody) []string {
	var ids []string
	for _, block := range blocks {
		if block.Options == nil {
			continue
		}
		for _, option := range *block.Options {
			ids = append(ids, option.ID)
		}
	}
	return ids
}

func blockIDsOf(blocks []templateBlockBody) []string {
	ids := make([]string, 0, len(blocks))
	for _, block := range blocks {
		ids = append(ids, block.ID)
	}
	return ids
}

func TestIssueSnapshotFreshOptionIdentifiers(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)

	first := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card)))
	second := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card)))

	firstDetail, secondDetail := h.issueDetail(t, parties.tutorToken, first.ID), h.issueDetail(t, parties.tutorToken, second.ID)
	templateOptions := optionIDsOf(parties.template.Blocks)
	firstOptions, secondOptions := optionIDsOf(firstDetail.Blocks), optionIDsOf(secondDetail.Blocks)
	if len(firstOptions) != len(templateOptions) || len(secondOptions) != len(templateOptions) {
		t.Fatalf("the snapshots hold %d and %d options, the template %d", len(firstOptions), len(secondOptions), len(templateOptions))
	}
	for _, option := range firstOptions {
		if slices.Contains(templateOptions, option) || slices.Contains(secondOptions, option) {
			t.Fatalf("option identifier %s is shared", option)
		}
	}
	if !slices.Equal(blockIDsOf(firstDetail.Blocks), blockIDsOf(parties.template.Blocks)) {
		t.Fatalf("the snapshot's question identifiers %v differ from the template's", blockIDsOf(firstDetail.Blocks))
	}
	if firstDetail.Blocks[1].Options == nil || (*firstDetail.Blocks[1].Options)[0].Correct == nil || !*(*firstDetail.Blocks[1].Options)[0].Correct {
		t.Fatal("the tutor's issue lost its answer key")
	}
	if firstDetail.MaxPoints != 7 || !firstDetail.HasManualQuestions || len(firstDetail.Submissions) != 1 {
		t.Fatalf("the issue answered %+v", firstDetail.issueItemBody)
	}
}

func TestIssueGroupMembersAtIssueTime(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	members := h.activeCards(t, parties.tutor, 10, 2)
	ended := h.linkedCard(t, parties.tutor, 12, domain.RelationshipActive)
	group := h.newGroup(t, parties.tutorToken, "Evening club", append(slices.Clone(members), ended))
	h.ended(t, parties.tutorToken, ended)

	issue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toGroup(group.ID)))
	late := h.linkedCard(t, parties.tutor, 13, domain.RelationshipActive)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, group.ID, "", `{"add":`+idList([]string{late})+`}`), http.StatusOK)

	stored := h.submissionsOf(t, issue.ID)
	if len(stored) != 2 || issue.Progress.Students != 2 || issue.Audience.GroupID == nil || *issue.Audience.GroupID != group.ID {
		t.Fatalf("the group issue holds %d submissions: %+v", len(stored), issue)
	}
	ended12, late13 := domain.UserID{12}.String(), domain.UserID{13}.String()
	for _, row := range stored {
		if row.student == ended12 || row.student == late13 || row.status != string(domain.SubmissionNotStarted) {
			t.Fatalf("unexpected submission %+v", row)
		}
	}
}

func TestIssueIdempotency(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	other := h.linkedCard(t, parties.tutor, 11, domain.RelationshipActive)

	first := h.issueAnswer(t, parties.tutorToken, issueKey(1), issueJSON(parties.template.ID, toCard(card)))
	repeated := h.issueAnswer(t, parties.tutorToken, issueKey(1), issueJSON(parties.template.ID, toCard(other)))
	h.markIssueForErasure(t, decodeLibrary[issueItemBody](t, first).ID)
	afterErasure := h.issueAnswer(t, parties.tutorToken, issueKey(1), issueJSON(parties.template.ID, toCard(card)))

	h.mustAnswer(t, first, http.StatusCreated)
	h.mustAnswer(t, repeated, http.StatusCreated)
	if decodeLibrary[issueItemBody](t, first).ID != decodeLibrary[issueItemBody](t, repeated).ID {
		t.Fatalf("the repeated key created another issue: %s", repeated.body)
	}
	if count := h.countRows(t, "SELECT count(id) FROM issues WHERE tutor_id = $1::uuid", parties.tutor.String()); count != 1 {
		t.Fatalf("%d issues were created", count)
	}
	if slots := h.countRows(t, "SELECT coalesce(sum(count), 0) FROM rate_limit_counters WHERE purpose = $1", string(domain.PurposeIssuesHour)); slots != 1 {
		t.Fatalf("the replayed key used a rate slot: %d slots", slots)
	}
	if afterErasure.status != http.StatusNotFound || code(t, afterErasure) != "not_found" {
		t.Fatalf("a replay of an issue marked for erasure answered %s", describe(afterErasure))
	}
}

func TestIssueRefusals(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	_, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	_, unconfirmedToken := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 3, "unconfirmed@example.org")
	active := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	invited := h.seedCard(t, parties.tutor, 11, domain.RelationshipInvited, nil)
	ended := h.linkedCard(t, parties.tutor, 12, domain.RelationshipEnded)
	reclaiming := h.linkedCard(t, parties.tutor, 13, domain.RelationshipReclaimPending)
	discarded := h.linkedCard(t, parties.tutor, 14, domain.RelationshipDiscarded)
	draft := h.createTemplate(t, parties.tutorToken, "Unfinished", nil, instructionBlock(blockID(41, 1), ""))
	foreignTemplate := h.readyTemplate(t, otherToken, "Foreign", 42)
	foreignGroup := h.newGroup(t, otherToken, "Foreign club", nil)
	archived := h.newGroup(t, parties.tutorToken, "Old club", []string{h.linkedCard(t, parties.tutor, 15, domain.RelationshipActive)})
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPost, archived.ID, "/archive", ""), http.StatusOK)
	empty := h.newGroup(t, parties.tutorToken, "Empty club", nil)
	foreignLesson := h.scheduledLessonOf(t, otherToken)
	past := h.now(t).Add(-time.Minute)

	cases := map[string]struct {
		token  string
		body   string
		status int
		code   string
	}{
		"a template that is not ready": {parties.tutorToken, issueJSON(draft.ID, toCard(active)), http.StatusConflict, "not_editable"},
		"an invited card":              {parties.tutorToken, issueJSON(parties.template.ID, toCard(invited)), http.StatusConflict, "relationship_not_active"},
		"an ended card":                {parties.tutorToken, issueJSON(parties.template.ID, toCard(ended)), http.StatusConflict, "relationship_not_active"},
		"a reclaim-pending card":       {parties.tutorToken, issueJSON(parties.template.ID, toCard(reclaiming)), http.StatusConflict, "relationship_not_active"},
		"a discarded card":             {parties.tutorToken, issueJSON(parties.template.ID, toCard(discarded)), http.StatusNotFound, "not_found"},
		"an archived group":            {parties.tutorToken, issueJSON(parties.template.ID, toGroup(archived.ID)), http.StatusConflict, "audience_not_active"},
		"an empty group":               {parties.tutorToken, issueJSON(parties.template.ID, toGroup(empty.ID)), http.StatusConflict, "audience_not_active"},
		"another tutor's template":     {parties.tutorToken, issueJSON(foreignTemplate.ID, toCard(active)), http.StatusNotFound, "not_found"},
		"another tutor's group":        {parties.tutorToken, issueJSON(parties.template.ID, toGroup(foreignGroup.ID)), http.StatusNotFound, "not_found"},
		"another tutor's lesson":       {parties.tutorToken, issueJSONWith(parties.template.ID, toCard(active), &foreignLesson, nil, "never"), http.StatusNotFound, "not_found"},
		"an unconfirmed tutor":         {unconfirmedToken, issueJSON(parties.template.ID, toCard(active)), http.StatusForbidden, "email_not_verified"},
		"a deadline in the past":       {parties.tutorToken, issueJSONWith(parties.template.ID, toCard(active), nil, &past, "never"), http.StatusUnprocessableEntity, "validation_failed"},
		"both audiences":               {parties.tutorToken, issueJSON(parties.template.ID, `{"relationship_id":"`+active+`","group_id":"`+empty.ID+`"}`), http.StatusUnprocessableEntity, "validation_failed"},
		"an unknown policy":            {parties.tutorToken, issueJSONWith(parties.template.ID, toCard(active), nil, nil, "sometimes"), http.StatusUnprocessableEntity, "validation_failed"},
		"a random template identifier": {parties.tutorToken, issueJSON("0b3c7a52-3f7e-4e21-9a3d-5c1f0e2d4b6a", toCard(active)), http.StatusNotFound, "not_found"},
		"a random audience identifier": {parties.tutorToken, issueJSON(parties.template.ID, toCard("0b3c7a52-3f7e-4e21-9a3d-5c1f0e2d4b6a")), http.StatusNotFound, "not_found"},
	}
	for name, testCase := range cases {
		answer := h.issueAnswer(t, testCase.token, "", testCase.body)

		if answer.status != testCase.status || code(t, answer) != testCase.code {
			t.Errorf("%s answered %s", name, describe(answer))
		}
	}
	if count := h.countRows(t, "SELECT count(id) FROM issues WHERE tutor_id = $1::uuid", parties.tutor.String()); count != 0 {
		t.Fatalf("a refused request created %d issues", count)
	}
}

func TestIssueBounds(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	h.setSnapshotTotal(t, parties.tutor, domain.IssueSnapshotBudgetBytes)

	overBudget := h.issueAnswer(t, parties.tutorToken, "", issueJSON(parties.template.ID, toCard(card)))
	h.setSnapshotTotal(t, parties.tutor, 0)
	h.fillHomeworkSlots(t, parties.tutor, domain.PurposeIssuesHour, domain.IssuesPerHour-1)
	hundredth := h.issueAnswer(t, parties.tutorToken, "", issueJSON(parties.template.ID, toCard(card)))
	hundredFirst := h.issueAnswer(t, parties.tutorToken, "", issueJSON(parties.template.ID, toCard(card)))

	if path, field := fieldOf(t, overBudget); overBudget.status != http.StatusUnprocessableEntity || path != "body" || field != "too_many" {
		t.Fatalf("an issue past the snapshot budget answered %s", describe(overBudget))
	}
	h.mustAnswer(t, hundredth, http.StatusCreated)
	if hundredFirst.status != http.StatusTooManyRequests || hundredFirst.headers.Get("Retry-After") == "" {
		t.Fatalf("the 101st issue in an hour answered %s", describe(hundredFirst))
	}
	if total := h.countRows(t, "SELECT snapshot_bytes_total FROM tutor_profiles WHERE user_id = $1::uuid", parties.tutor.String()); total <= 0 {
		t.Fatalf("the snapshot total is %d after issuing", total)
	}
}

func TestIssueLessonBound(t *testing.T) {
	t.Parallel()
	h := newHomeworkHarnessWithCaps(t, domain.HomeworkCaps{IssuesPerLesson: 2})
	parties := h.issueParties(t)
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	lesson := h.lessonWith(t, parties.tutorToken, card)
	withdrawn := h.issued(t, parties.tutorToken, issueJSONWith(parties.template.ID, toCard(card), &lesson, nil, "never"))
	h.mustAnswer(t, h.withdrawIssue(t, parties.tutorToken, withdrawn.ID), http.StatusOK)
	h.issued(t, parties.tutorToken, issueJSONWith(parties.template.ID, toCard(card), &lesson, nil, "never"))
	h.issued(t, parties.tutorToken, issueJSONWith(parties.template.ID, toCard(card), &lesson, nil, "never"))

	third := h.issueAnswer(t, parties.tutorToken, "", issueJSONWith(parties.template.ID, toCard(card), &lesson, nil, "never"))
	unlinked := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card)))
	relinked := h.patchIssue(t, parties.tutorToken, unlinked.ID, `{"lesson_id":"`+lesson+`","version":1}`)

	for name, refused := range map[string]response{"issuing": third, "linking by a change": relinked} {
		if path, field := fieldOf(t, refused); refused.status != http.StatusUnprocessableEntity || path != "body.lesson_id" || field != "too_many" {
			t.Errorf("%s past the lesson's bound answered %s", name, describe(refused))
		}
	}
}

func TestIssueListFilters(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	cards := h.activeCards(t, parties.tutor, 10, 2)
	group := h.newGroup(t, parties.tutorToken, "Evening club", cards)
	soon, later := h.now(t).Add(2*time.Hour), h.now(t).Add(48*time.Hour)
	toFirst := h.issued(t, parties.tutorToken, issueJSONWith(parties.template.ID, toCard(cards[0]), nil, &soon, "never"))
	h.advance(t, time.Second)
	toGroup := h.issued(t, parties.tutorToken, issueJSONWith(parties.template.ID, toGroup(group.ID), nil, &later, "never"))
	h.advance(t, time.Second)
	toSecond := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(cards[1])))
	submitted := h.submissionsOf(t, toSecond.ID)[0].id
	h.submitStored(t, submitted, h.now(t))

	all := h.issuesPage(t, parties.tutorToken, url.Values{})
	byDeadline := h.issuesPage(t, parties.tutorToken, url.Values{"deadline_before": {h.now(t).Add(24 * time.Hour).Format(time.RFC3339)}})
	byStudent := h.issuesPage(t, parties.tutorToken, url.Values{"student": {cards[0]}})
	byGroup := h.issuesPage(t, parties.tutorToken, url.Values{"group": {group.ID}})
	needsChecking := h.issuesPage(t, parties.tutorToken, url.Values{"needs_checking": {"true"}})
	paged := h.issuesPage(t, parties.tutorToken, url.Values{"limit": {"2"}})
	rest := h.issuesPage(t, parties.tutorToken, url.Values{"limit": {"2"}, "cursor": {*paged.NextCursor}})
	_, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	foreignStudent := h.issuesAnswer(t, otherToken, url.Values{"student": {cards[0]}})
	foreignGroup := h.issuesAnswer(t, otherToken, url.Values{"group": {group.ID}})

	if got := issueIDs(all.Items); !slices.Equal(got, []string{toSecond.ID, toGroup.ID, toFirst.ID}) {
		t.Fatalf("the list answered %v", got)
	}
	if got := issueIDs(byDeadline.Items); !slices.Equal(got, []string{toFirst.ID}) {
		t.Fatalf("the deadline filter answered %v", got)
	}
	if got := issueIDs(byStudent.Items); !slices.Equal(got, []string{toGroup.ID, toFirst.ID}) || byStudent.Items[0].Submission == nil {
		t.Fatalf("the student filter answered %v", got)
	}
	if got := issueIDs(byGroup.Items); !slices.Equal(got, []string{toGroup.ID}) || byGroup.Items[0].Progress.Students != 2 {
		t.Fatalf("the group filter answered %+v", byGroup.Items)
	}
	if got := issueIDs(needsChecking.Items); !slices.Equal(got, []string{toSecond.ID}) || needsChecking.Items[0].Progress.NeedsChecking != 1 {
		t.Fatalf("the needs-checking filter answered %+v", needsChecking.Items)
	}
	if got := append(issueIDs(paged.Items), issueIDs(rest.Items)...); !slices.Equal(got, issueIDs(all.Items)) || rest.NextCursor != nil {
		t.Fatalf("the pages answered %v", got)
	}
	for name, refused := range map[string]response{"another tutor's student": foreignStudent, "another tutor's group": foreignGroup} {
		if refused.status != http.StatusNotFound {
			t.Errorf("%s answered %s", name, describe(refused))
		}
	}
}

func issueIDs(items []issueItemBody) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func TestDeadlineChangeRecomputesLate(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	cards := h.activeCards(t, parties.tutor, 10, 2)
	group := h.newGroup(t, parties.tutorToken, "Evening club", cards)
	deadline := h.now(t).Add(time.Hour)
	issue := h.issued(t, parties.tutorToken, issueJSONWith(parties.template.ID, toGroup(group.ID), nil, &deadline, "never"))
	stored := h.submissionsOf(t, issue.ID)
	h.submitStored(t, stored[0].id, h.now(t).Add(30*time.Minute))
	earlier := h.now(t).Add(10 * time.Minute)

	moved := h.patchIssue(t, parties.tutorToken, issue.ID, `{"deadline_at":"`+earlier.Format(time.RFC3339Nano)+`","version":1}`)
	stale := h.patchIssue(t, parties.tutorToken, issue.ID, `{"deadline_at":null,"version":1}`)
	afterMove := h.submissionsOf(t, issue.ID)
	removed := h.patchIssue(t, parties.tutorToken, issue.ID, `{"deadline_at":null,"version":2}`)
	afterRemoval := h.submissionsOf(t, issue.ID)
	unchanged := h.patchIssue(t, parties.tutorToken, issue.ID, `{"deadline_at":null,"version":3}`)

	h.mustAnswer(t, moved, http.StatusOK)
	if !afterMove[0].late || afterMove[1].late || decodeLibrary[issueItemBody](t, moved).Progress.Late != 1 {
		t.Fatalf("the earlier deadline left %+v", afterMove)
	}
	if stale.status != http.StatusConflict || code(t, stale) != "conflict" {
		t.Fatalf("a stale version answered %s", describe(stale))
	}
	h.mustAnswer(t, removed, http.StatusOK)
	if afterRemoval[0].late || decodeLibrary[issueItemBody](t, removed).DeadlineAt != nil {
		t.Fatalf("removing the deadline left %+v", afterRemoval)
	}
	h.mustAnswer(t, unchanged, http.StatusOK)
	if decodeLibrary[issueItemBody](t, unchanged).Version != 3 {
		t.Fatalf("a change that changes nothing answered %s", unchanged.body)
	}
}

func TestIssueChangeRefusesALessonOutsideTheAudience(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	_, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	otherCard := h.linkedCard(t, parties.tutor, 11, domain.RelationshipActive)
	issue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card)))
	otherAudience := h.lessonWith(t, parties.tutorToken, otherCard)
	foreign := h.scheduledLessonOf(t, otherToken)

	otherAudienceAnswer := h.patchIssue(t, parties.tutorToken, issue.ID, `{"lesson_id":"`+otherAudience+`","version":1}`)
	foreignAnswer := h.patchIssue(t, parties.tutorToken, issue.ID, `{"lesson_id":"`+foreign+`","version":1}`)

	for name, refused := range map[string]response{"a lesson of another audience": otherAudienceAnswer, "another tutor's lesson": foreignAnswer} {
		if refused.status != http.StatusNotFound || code(t, refused) != "not_found" {
			t.Errorf("linking %s answered %s", name, describe(refused))
		}
	}
	if detail := h.issueDetail(t, parties.tutorToken, issue.ID); detail.LessonID != nil || detail.Version != 1 {
		t.Fatalf("a refused change linked the issue: %+v", detail.issueItemBody)
	}
}

func TestTheIssueListOfASharedStudentHoldsOnlyTheCallersWork(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	student, _ := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	firstCard := h.seedCard(t, parties.tutor, 1, domain.RelationshipActive, &student)
	secondCard := h.seedCard(t, second, sharedStudentCard, domain.RelationshipActive, &student)
	secondTemplate := h.readyTemplate(t, secondToken, "Exercises", 41)
	firstIssue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(firstCard)))
	secondIssue := h.issued(t, secondToken, issueJSON(secondTemplate.ID, toCard(secondCard)))

	firstList := h.issuesPage(t, parties.tutorToken, url.Values{"student": {firstCard}})
	secondList := h.issuesPage(t, secondToken, url.Values{"student": {secondCard}})
	secondUnfiltered := h.issuesPage(t, secondToken, url.Values{})

	if got := issueIDs(secondUnfiltered.Items); !slices.Equal(got, []string{secondIssue.ID}) {
		t.Fatalf("the second tutor's unfiltered list answered %v", got)
	}
	if got := issueIDs(firstList.Items); !slices.Equal(got, []string{firstIssue.ID}) || firstList.Items[0].Submission == nil {
		t.Fatalf("the first tutor's list of the shared student answered %+v", firstList.Items)
	}
	if got := issueIDs(secondList.Items); !slices.Equal(got, []string{secondIssue.ID}) || secondList.Items[0].Submission == nil {
		t.Fatalf("the second tutor's list of the shared student answered %+v", secondList.Items)
	}
}
