package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"ovc/backend/internal/domain"
)

type openAssignmentsRow struct {
	ID              string `json:"id"`
	OpenAssignments *int64 `json:"open_assignments"`
}

type openAssignmentsPage struct {
	Items      []openAssignmentsRow `json:"items"`
	NextCursor *string              `json:"next_cursor"`
}

type openAssignmentsSeed struct {
	ownerToken    string
	otherToken    string
	sharedCard    string
	emptyCard     string
	reclaimedCard string
	otherCard     string
	ownerGroup    string
}

func (h *authHarness) openAssignmentsPage(t *testing.T, token string, query url.Values) openAssignmentsPage {
	t.Helper()
	result := h.send(t, http.MethodGet, studentsPath+"?"+query.Encode(), "", jsonCall(token))
	if result.status != http.StatusOK {
		t.Fatalf("the student list answered %s", describe(result))
	}
	var page openAssignmentsPage
	if err := json.Unmarshal([]byte(result.body), &page); err != nil {
		t.Fatalf("not a student list: %s", result.body)
	}
	return page
}

func (h *authHarness) openAssignmentsByCard(t *testing.T, token string, query url.Values) map[string]int64 {
	t.Helper()
	counts := map[string]int64{}
	for {
		page := h.openAssignmentsPage(t, token, query)
		for _, row := range page.Items {
			if row.OpenAssignments == nil {
				t.Fatalf("the row of %s has no open_assignments", row.ID)
			}
			counts[row.ID] = *row.OpenAssignments
		}
		if page.NextCursor == nil {
			return counts
		}
		query.Set("cursor", *page.NextCursor)
	}
}

func (h *authHarness) issuedWithStatus(t *testing.T, token string, template string, audience string, status domain.SubmissionStatus) issueItemBody {
	t.Helper()
	issue := h.issued(t, token, issueJSON(template, audience))
	if status != domain.SubmissionNotStarted {
		for _, submission := range h.submissionsOf(t, issue.ID) {
			h.setSubmissionStatus(t, submission.id, status)
		}
	}
	return issue
}

func (h *authHarness) seedOpenAssignments(t *testing.T) openAssignmentsSeed {
	t.Helper()
	owner, ownerToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	other, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	ownerTemplate := h.readyTemplate(t, ownerToken, "Trip to Paris", 40).ID
	otherTemplate := h.readyTemplate(t, otherToken, "Trip to Rome", 41).ID
	student := h.seedStudentAccount(t, 10, domain.AccountActive)
	seed := openAssignmentsSeed{
		ownerToken: ownerToken, otherToken: otherToken,
		sharedCard:    h.seedCard(t, owner, 10, domain.RelationshipActive, &student),
		emptyCard:     h.linkedCard(t, owner, 11, domain.RelationshipActive),
		reclaimedCard: h.linkedCard(t, owner, 12, domain.RelationshipActive),
		otherCard:     h.seedCard(t, other, 20, domain.RelationshipActive, &student),
	}
	seed.ownerGroup = h.newGroup(t, ownerToken, "Evening group", []string{seed.sharedCard}).ID
	otherGroup := h.newGroup(t, otherToken, "Morning group", []string{seed.otherCard}).ID
	for _, status := range domain.SubmissionStatuses {
		h.issuedWithStatus(t, ownerToken, ownerTemplate, toCard(seed.sharedCard), status)
	}
	h.issuedWithStatus(t, ownerToken, ownerTemplate, toGroup(seed.ownerGroup), domain.SubmissionNotStarted)
	withdrawn := h.issuedWithStatus(t, ownerToken, ownerTemplate, toCard(seed.sharedCard), domain.SubmissionNotStarted)
	h.mustAnswer(t, h.withdrawIssue(t, ownerToken, withdrawn.ID), http.StatusOK)
	erased := h.issuedWithStatus(t, ownerToken, ownerTemplate, toCard(seed.sharedCard), domain.SubmissionNotStarted)
	h.markIssueForErasure(t, erased.ID)
	h.issuedWithStatus(t, ownerToken, ownerTemplate, toCard(seed.reclaimedCard), domain.SubmissionNotStarted)
	h.setRelationshipState(t, seed.reclaimedCard, domain.RelationshipReclaimPending)
	h.issuedWithStatus(t, otherToken, otherTemplate, toCard(seed.otherCard), domain.SubmissionNotStarted)
	h.issuedWithStatus(t, otherToken, otherTemplate, toGroup(otherGroup), domain.SubmissionNotStarted)
	return seed
}

func TestStudentsOpenAssignmentsCountsTheCallersOpenWork(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	seed := h.seedOpenAssignments(t)

	counts := h.openAssignmentsByCard(t, seed.ownerToken, url.Values{})

	if counts[seed.sharedCard] != 5 || counts[seed.emptyCard] != 0 || counts[seed.reclaimedCard] != 1 || len(counts) != 3 {
		t.Fatalf("the owner's open assignments are %v", counts)
	}
}

func TestStudentsOpenAssignmentsIgnoresAnotherTutorsIssues(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	seed := h.seedOpenAssignments(t)

	counts := h.openAssignmentsByCard(t, seed.otherToken, url.Values{})

	if counts[seed.otherCard] != 2 || len(counts) != 1 {
		t.Fatalf("the other tutor's open assignments are %v", counts)
	}
}

func TestStudentsOpenAssignmentsHoldAcrossFiltersAndPages(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	seed := h.seedOpenAssignments(t)
	queries := map[string]url.Values{
		"the active state":   {"state": {"active"}},
		"the group":          {"group": {seed.ownerGroup}},
		"a search":           {"q": {"seeded"}},
		"pages of one card":  {"limit": {"1"}},
		"no filter":          {},
		"pages of two cards": {"limit": {"2"}},
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			counts := h.openAssignmentsByCard(t, seed.ownerToken, query)

			if counts[seed.sharedCard] != 5 {
				t.Fatalf("the shared card's open assignments are %v", counts)
			}
		})
	}
}
