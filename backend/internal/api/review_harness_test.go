package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"ovc/backend/internal/domain"
)

const reviewQueueRoute = "/api/v1/review-queue"

type reviewItemJSON struct {
	QuestionID             string   `json:"question_id"`
	Points                 *int32   `json:"points,omitempty"`
	OverridePoints         *int32   `json:"override_points,omitempty"`
	OverrideReason         *string  `json:"override_reason,omitempty"`
	Comment                string   `json:"comment,omitempty"`
	CorrectedAttachmentIDs []string `json:"corrected_attachment_ids,omitempty"`
}

type reviewSaveJSON struct {
	Revision int32            `json:"revision"`
	Version  int64            `json:"version"`
	SaveID   string           `json:"save_id"`
	Feedback string           `json:"feedback"`
	Items    []reviewItemJSON `json:"items"`
}

type tutorReviewItemBody struct {
	QuestionID     string               `json:"question_id"`
	Points         *int32               `json:"points"`
	OverridePoints *int32               `json:"override_points"`
	OverrideReason *string              `json:"override_reason"`
	Comment        string               `json:"comment"`
	CorrectedFiles []submissionFileBody `json:"corrected_files"`
}

type tutorRevisionBody struct {
	Revision int32                 `json:"revision"`
	State    string                `json:"state"`
	Action   *string               `json:"action"`
	Feedback string                `json:"feedback"`
	Items    []tutorReviewItemBody `json:"items"`
	Version  int64                 `json:"version"`
}

type reviewedWorkBody struct {
	ID          string              `json:"id"`
	Status      string              `json:"status"`
	Version     int64               `json:"version"`
	FinalPoints *int32              `json:"final_points"`
	Revisions   []tutorRevisionBody `json:"revisions"`
}

type studentReviewItemBody struct {
	QuestionID     string               `json:"question_id"`
	Comment        string               `json:"comment"`
	CorrectedFiles []submissionFileBody `json:"corrected_files"`
}

type studentReviewBody struct {
	Feedback string                  `json:"feedback"`
	Items    []studentReviewItemBody `json:"items"`
}

type studentReviewedBody struct {
	Status              string             `json:"status"`
	Version             int64              `json:"version"`
	EditableQuestionIDs []string           `json:"editable_question_ids"`
	Review              *studentReviewBody `json:"review"`
}

type reviewQueueItemBody struct {
	SubmissionID string `json:"submission_id"`
	HasDraft     bool   `json:"has_draft"`
}

type reviewQueueBody struct {
	Items      []reviewQueueItemBody `json:"items"`
	NextCursor *string               `json:"next_cursor"`
	Count      int64                 `json:"count"`
}

func scored(question string, value int32) reviewItemJSON {
	return reviewItemJSON{QuestionID: question, Points: &value}
}

func commented(item reviewItemJSON, comment string, files ...string) reviewItemJSON {
	item.Comment, item.CorrectedAttachmentIDs = comment, files
	return item
}

func overridden(question string, value int32, reason string) reviewItemJSON {
	return reviewItemJSON{QuestionID: question, OverridePoints: &value, OverrideReason: &reason}
}

func reviewJSON(t *testing.T, revision int32, version int64, save string, feedback string, items ...reviewItemJSON) string {
	t.Helper()
	if items == nil {
		items = []reviewItemJSON{}
	}
	encoded, err := json.Marshal(reviewSaveJSON{Revision: revision, Version: version, SaveID: save, Feedback: feedback, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func (h *authHarness) saveReview(t *testing.T, token string, submission string, body string) response {
	t.Helper()
	h.nextSecond(t)
	return h.send(t, http.MethodPut, submissionsRoute+submission+"/review", body, jsonCall(token))
}

func (h *authHarness) reviewSaved(t *testing.T, token string, submission string, body string) reviewedWorkBody {
	t.Helper()
	answer := h.saveReview(t, token, submission, body)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[reviewedWorkBody](t, answer)
}

func (h *authHarness) publishReview(t *testing.T, token string, submission string, action domain.ReviewAction, revision int32) response {
	t.Helper()
	return h.send(t, http.MethodPost, submissionsRoute+submission+"/"+string(action), fmt.Sprintf(`{"revision":%d}`, revision), jsonCall(token))
}

func (h *authHarness) published(t *testing.T, token string, submission string, action domain.ReviewAction, revision int32) reviewedWorkBody {
	t.Helper()
	answer := h.publishReview(t, token, submission, action, revision)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[reviewedWorkBody](t, answer)
}

func (h *authHarness) reviewedWork(t *testing.T, token string, submission string) reviewedWorkBody {
	t.Helper()
	answer := h.submissionAnswer(t, token, submission)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[reviewedWorkBody](t, answer)
}

func (h *authHarness) studentReviewed(t *testing.T, token string, submission string) studentReviewedBody {
	t.Helper()
	answer := h.submissionAnswer(t, token, submission)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[studentReviewedBody](t, answer)
}

func (h *authHarness) reviewQueueAnswer(t *testing.T, token string, query url.Values) response {
	t.Helper()
	path := reviewQueueRoute
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return h.send(t, http.MethodGet, path, "", jsonCall(token))
}

func (h *authHarness) reviewQueue(t *testing.T, token string, query url.Values) reviewQueueBody {
	t.Helper()
	answer := h.reviewQueueAnswer(t, token, query)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[reviewQueueBody](t, answer)
}

func (h *authHarness) correctedFile(t *testing.T, token string, submission string, question string, name string, size int) string {
	t.Helper()
	answer := h.homeworkUpload(t, token, homeworkRoute(domain.ContextCorrected, submission, question), name, pdfFile(size))
	h.mustAnswer(t, answer, http.StatusCreated)
	return uploadedID(t, answer)
}

func (h *authHarness) revisionID(t *testing.T, submission string, revision int32) string {
	t.Helper()
	var id string
	if err := h.database.Pool.QueryRow(t.Context(), "SELECT id::text FROM review_revisions WHERE submission_id = $1::uuid AND revision = $2",
		submission, revision).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *authHarness) reviewItemReferences(t *testing.T, attachment string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(ref_id) FROM attachment_references WHERE ref_kind = 'review_item' AND attachment_id = $1::uuid", attachment)
}

type reviewParties struct {
	endingParties
	issue      string
	submission string
}

func (h *authHarness) submittedForReview(t *testing.T, policy domain.AnswerPolicy) reviewParties {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	issue, submissions := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Trip to Paris", string(policy), nil, workBlocks()...)
	submission := submissions[0].id
	options := h.workOptions(t, parties.studentToken, submission)
	draft := h.saved(t, parties.studentToken, submission, answersJSON(t, 1, saveID(1), chosen(workQuestion(1), options[workQuestion(1)][0]),
		typedAnswer(workQuestion(5), "We saw the tower")))
	h.submitted(t, parties.studentToken, submission, draft.Version)
	return reviewParties{endingParties: parties, issue: issue, submission: submission}
}

func (h *authHarness) submittedByCard(t *testing.T, tutorToken string, card string, studentToken string, policy domain.AnswerPolicy) string {
	t.Helper()
	_, submissions := h.issueWork(t, tutorToken, toCard(card), "Trip to Paris", string(policy), nil, workBlocks()...)
	submission := submissions[0].id
	draft := h.saved(t, studentToken, submission, answersJSON(t, 1, saveID(1), typedAnswer(workQuestion(5), "We saw the tower")))
	h.submitted(t, studentToken, submission, draft.Version)
	return submission
}

func fullyScored(comment string, files ...string) []reviewItemJSON {
	return []reviewItemJSON{commented(scored(workQuestion(5), 4), comment), commented(scored(workQuestion(6), 3), "", files...)}
}

func anotherTutorsReviewWrite(suffix string, body string) denialCase {
	return func(t *testing.T, h *authHarness) (response, response) {
		t.Helper()
		owners := h.issueOwners(t)
		h.shareStudentWith(t, owners.intruder, domain.UserID{10})
		submission := h.submittedByCard(t, owners.ownerToken, owners.card, h.studentToken(t, 10), domain.AnswerPolicyNever)
		outOfScope := h.send(t, http.MethodPost, submissionsRoute+submission+suffix, body, jsonCall(owners.intruderToken))
		random := h.send(t, http.MethodPost, submissionsRoute+randomSubmissionID+suffix, body, jsonCall(owners.intruderToken))
		if after := h.reviewedWork(t, owners.ownerToken, submission); after.Status != string(domain.SubmissionSubmitted) || len(after.Revisions) != 0 {
			t.Fatalf("another tutor reviewed the work: %+v", after)
		}
		return outOfScope, random
	}
}

func anotherTutorsReviewSave(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	owners := h.issueOwners(t)
	h.shareStudentWith(t, owners.intruder, domain.UserID{10})
	submission := h.submittedByCard(t, owners.ownerToken, owners.card, h.studentToken(t, 10), domain.AnswerPolicyNever)
	body := reviewJSON(t, 1, 0, saveID(9), "Taken over")
	outOfScope := h.send(t, http.MethodPut, submissionsRoute+submission+"/review", body, jsonCall(owners.intruderToken))
	random := h.send(t, http.MethodPut, submissionsRoute+randomSubmissionID+"/review", body, jsonCall(owners.intruderToken))
	if after := h.reviewedWork(t, owners.ownerToken, submission); len(after.Revisions) != 0 {
		t.Fatalf("another tutor saved a review: %+v", after)
	}
	return outOfScope, random
}

func anotherTutorsQueueFilter(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	owners := h.issueOwners(t)
	issue := h.issued(t, owners.ownerToken, issueJSON(owners.template.ID, toCard(owners.card)))
	group := h.newGroup(t, owners.ownerToken, "Evening club", []string{owners.card})
	outOfScope := h.reviewQueueAnswer(t, owners.intruderToken, url.Values{"issue": {issue.ID}})
	for name, query := range map[string]url.Values{"group": {"group": {group.ID}}, "student": {"student": {owners.card}}} {
		foreign := h.reviewQueueAnswer(t, owners.intruderToken, query)
		if foreign.status != http.StatusNotFound {
			t.Fatalf("filtering by another tutor's %s answered %s", name, describe(foreign))
		}
	}
	random := h.reviewQueueAnswer(t, owners.intruderToken, url.Values{"issue": {randomIssueID}})
	return outOfScope, random
}
