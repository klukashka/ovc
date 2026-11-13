package api_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

var pointsOnlySave = map[int32]byte{2: 12, 3: 13, 4: 14}

type contractPublication struct {
	returnedVersion int64
	revision        int32
	items           []reviewItemJSON
	corrected       string
}

func (h *authHarness) contractReview(
	t *testing.T, check contractCheck, parties endingParties, work contractWork, label string, afterSubmission bool,
) contractPublication {
	t.Helper()
	corrected := h.correctedFile(t, parties.tutorToken, work.full, contractQuestion(7), "corrected.pdf", 120)
	items := []reviewItemJSON{
		overridden(contractQuestion(2), 97, "Generous"), commented(scored(contractQuestion(6), 73), "Well argued"),
		commented(scored(contractQuestion(7), 71), "See the marks", corrected),
	}
	h.reviewSaved(t, parties.tutorToken, work.full, reviewJSON(t, 1, 0, saveID(1), "Overall good", items...))
	if drafted := h.submissionAnswer(t, parties.studentToken, work.full).body; !strings.Contains(drafted, `"review":null`) {
		t.Errorf("%sa draft review reached the student: %s", label, drafted)
	}
	publication := contractPublication{revision: 1, items: items, corrected: corrected}
	if afterSubmission {
		refused := h.publishReview(t, parties.tutorToken, work.full, domain.ReviewReturn, 1)
		assertRefused(t, label+"a return after revealing", refused, http.StatusConflict, "return_not_allowed")
		h.published(t, parties.tutorToken, work.full, domain.ReviewCheck, 1)
		h.contractChecked(t, check, parties, work, label, true, publication)
		return publication
	}
	h.published(t, parties.tutorToken, work.full, domain.ReviewReturn, 1)
	returned := h.submissionAnswer(t, parties.studentToken, work.full)
	check.submission(label+"returned", []byte(returned.body), false)
	if !strings.Contains(returned.body, "Well argued") || !strings.Contains(returned.body, "Overall good") {
		t.Errorf("%sthe returned work hides the published review: %s", label, returned.body)
	}
	publication.returnedVersion = decodeLibrary[studentSubmissionBody](t, returned).Version
	return publication
}

func (h *authHarness) contractRecheck(
	t *testing.T, check contractCheck, parties endingParties, work contractWork, label string, revealed bool, publication contractPublication,
) {
	t.Helper()
	resubmitted := h.submitWork(t, parties.studentToken, work.full, publication.returnedVersion)
	h.mustAnswer(t, resubmitted, http.StatusOK)
	check.submission(label+"resubmitted", []byte(resubmitted.body), false)
	h.reviewSaved(t, parties.tutorToken, work.full, reviewJSON(t, 2, 0, saveID(6), "Overall good", publication.items...))
	h.published(t, parties.tutorToken, work.full, domain.ReviewCheck, 2)
	publication.revision = 2
	h.contractChecked(t, check, parties, work, label, revealed, publication)
}

func (h *authHarness) contractChecked(
	t *testing.T, check contractCheck, parties endingParties, work contractWork, label string, revealed bool, publication contractPublication,
) {
	t.Helper()
	checked := h.submissionAnswer(t, parties.studentToken, work.full)
	check.submission(label+"reviewed", []byte(checked.body), revealed)
	minted, _ := h.addressOf(t, parties.studentToken, publication.corrected, "?ref=review_item:"+h.revisionID(t, work.full, publication.revision))
	h.mustAnswer(t, minted, http.StatusOK)
	check.object(label+"corrected mint", json.RawMessage(minted.body), []string{"url", "expires_at"})
	changed := slices.Clone(publication.items)
	changed[1] = commented(scored(contractQuestion(6), 1), "Well argued")
	next := publication.revision + 1
	h.reviewSaved(t, parties.tutorToken, work.full, reviewJSON(t, next, 0, saveID(pointsOnlySave[next]), "Overall good", changed...))
	h.published(t, parties.tutorToken, work.full, domain.ReviewCheck, next)
	after := h.submissionAnswer(t, parties.studentToken, work.full)
	check.submission(label+"points-only", []byte(after.body), revealed)
	if addresslessBody(t, checked.body) != addresslessBody(t, after.body) {
		t.Errorf("%sa points-only revision changed the student's view", label)
	}
	h.reviewSaved(t, parties.tutorToken, work.full, reviewJSON(t, next+1, 0, saveID(pointsOnlySave[next+1]), "Overall better", changed...))
	h.published(t, parties.tutorToken, work.full, domain.ReviewCheck, next+1)
	check.submission(label+"updated", []byte(h.submissionAnswer(t, parties.studentToken, work.full).body), revealed)
}

func (h *authHarness) contractOverridesOnly(t *testing.T, check contractCheck, parties endingParties, work contractWork, label string, revealed bool) {
	t.Helper()
	before := h.submissionAnswer(t, parties.studentToken, work.automatic)
	lessonBefore := h.lessonPage(t, parties.studentToken, work.lesson)
	h.reviewSaved(t, parties.tutorToken, work.automatic, reviewJSON(t, 1, 0, saveID(30), "", overridden(contractQuestion(2), 0, "Copied")))
	h.published(t, parties.tutorToken, work.automatic, domain.ReviewCheck, 1)
	after := h.submissionAnswer(t, parties.studentToken, work.automatic)
	lessonAfter := h.lessonPage(t, parties.studentToken, work.lesson)
	check.submission(label+"overrides only", []byte(after.body), revealed)
	if addresslessBody(t, before.body) != addresslessBody(t, after.body) || !strings.Contains(after.body, `"review":null`) {
		t.Errorf("%sa revision of overrides only changed the student's view: %s", label, after.body)
	}
	if lessonBefore.status != http.StatusOK || lessonBefore.body != lessonAfter.body {
		t.Errorf("%sa revision of overrides only changed the student's lesson detail: %s then %s", label, lessonBefore.body, lessonAfter.body)
	}
}

func (h *authHarness) contractLimitRefusals(t *testing.T, check contractCheck, parties endingParties, closing string, full string) {
	t.Helper()
	var throttled response
	for index := range 3 {
		answer := h.saveAnswersNow(t, parties.studentToken, closing, answersJSON(t, 99, saveID(byte(40+index))))
		if answer.status == http.StatusTooManyRequests {
			throttled = answer
		}
	}
	assertRefused(t, "a burst of saves", throttled, http.StatusTooManyRequests, "rate_limited")
	check.refusal("rate_limited", []byte(throttled.body))
	_, spare := h.issueTemplate(t, parties.tutorToken, full, toCard(parties.card.ID), string(domain.AnswerPolicyNever), nil)
	if _, err := h.database.Pool.Exec(t.Context(), "UPDATE tutor_profiles SET file_quota_bytes = file_usage_bytes WHERE user_id = $1::uuid",
		parties.tutor.String()); err != nil {
		t.Fatal(err)
	}
	over := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, spare[0].id, contractQuestion(7)), "over.pdf", pdfFile(300))
	assertRefused(t, "an answer upload over the quota", over, http.StatusInsufficientStorage, "quota_exceeded")
	check.refusal("quota_exceeded", []byte(over.body))
}
