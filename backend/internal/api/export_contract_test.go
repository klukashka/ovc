package api_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

var (
	exportStudentFileKeys = []string{"filename", "media_type", "size_bytes", "width", "height", "path"}
	exportAnswerFileKeys  = []string{"attachment_id", "filename", "media_type", "size_bytes", "width", "height", "path"}
	exportFileEntryKeys   = []string{"path", "filename", "media_type", "size_bytes", "sha256", "references"}
)

func (c contractCheck) exportSubmission(label string, raw json.RawMessage, revealed bool) {
	c.t.Helper()
	allowed := slices.Clone(submissionKeys)
	if revealed {
		allowed = append(allowed, "revealed")
	}
	decoded := c.object(label, raw, allowed)
	c.object(label+".tutor", decoded["tutor"], tutorRefKeys)
	for _, block := range c.array(label+".blocks", decoded["blocks"]) {
		typed := decodedAs[struct {
			Type string `json:"type"`
		}](c.t, block)
		fields := c.object(label+".blocks."+typed.Type, block, blockKeysByType[typed.Type])
		if images, found := fields["images"]; found {
			for _, image := range c.array(label+".images", images) {
				c.object(label+".image", image, exportStudentFileKeys)
			}
		}
		if material, found := fields["material"]; found {
			c.object(label+".material", material, append([]string{"title"}, exportStudentFileKeys...))
		}
	}
	for _, answer := range c.array(label+".answers", decoded["answers"]) {
		fields := c.object(label+".answer", answer, []string{"question_id", "value", "files"})
		for _, file := range c.array(label+".files", fields["files"]) {
			c.object(label+".file", file, exportAnswerFileKeys)
		}
	}
	if string(decoded["review"]) != "null" {
		review := c.object(label+".review", decoded["review"], []string{"feedback", "items"})
		for _, item := range c.array(label+".review.items", review["items"]) {
			fields := c.object(label+".review.item", item, []string{"question_id", "comment", "corrected_files"})
			for _, file := range c.array(label+".review.files", fields["corrected_files"]) {
				c.object(label+".review.file", file, exportAnswerFileKeys)
			}
		}
	}
	if revealed {
		c.noSentinelNumber(label+".revealed", decoded["revealed"])
		delete(decoded, "revealed")
	}
	stripped, err := json.Marshal(decoded)
	if err != nil {
		c.t.Fatal(err)
	}
	c.noSentinel(label, stripped)
}

type revealedArchive struct {
	archive  studentArchive
	revealed map[string]bool
}

func (h *authHarness) withRouteReveals(t *testing.T, token string, archive studentArchive) revealedArchive {
	t.Helper()
	revealed := map[string]bool{}
	for _, raw := range archive.data.Submissions {
		identified := decodedAs[struct {
			ID string `json:"id"`
		}](t, raw)
		routed := h.submissionAnswer(t, token, identified.ID)
		revealed[identified.ID] = routed.status == http.StatusOK && strings.Contains(routed.body, `"revealed":`)
	}
	return revealedArchive{archive: archive, revealed: revealed}
}

func (c contractCheck) exportArchive(label string, checked revealedArchive) ([]string, int) {
	c.t.Helper()
	archive := checked.archive
	revealedByTitle := map[string]bool{}
	for _, raw := range archive.data.Submissions {
		identified := decodedAs[struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}](c.t, raw)
		revealed := checked.revealed[identified.ID]
		revealedByTitle[identified.Title] = revealed
		c.exportSubmission(label+" submission", raw, revealed)
	}
	statuses := make([]string, 0, len(archive.data.Homework))
	for _, raw := range archive.data.Homework {
		fields := c.object(label+" homework", raw, assignmentItemKeys)
		statuses = append(statuses, decodedAs[string](c.t, fields["status"]))
		c.noSentinel(label+" homework", raw)
	}
	for _, entry := range archive.data.Files {
		encoded, err := json.Marshal(entry)
		if err != nil {
			c.t.Fatal(err)
		}
		c.object(label+" file entry", encoded, exportFileEntryKeys)
	}
	for _, raw := range archive.data.Tutors {
		c.noSentinel(label+" tutor", raw)
	}
	for name, rows := range archive.sheets {
		for index, row := range rows {
			for _, cell := range row {
				text := cellText(cell)
				for _, found := range pointSentinelsIn(text) {
					c.t.Errorf("%s sheet %s carries the point sentinel %s", label, name, found)
				}
				if name != "Answers" || index == 0 || columnOf(cell.Reference) < 5 {
					c.noSentinelInText(label+" sheet "+name, text)
				}
			}
		}
	}
	keyRows := 0
	for _, row := range archive.rows("Answers") {
		revealed, known := revealedByTitle[row[0]]
		keyed := len(row) > 5 && strings.Contains(strings.Join(row[5:], "\n"), "sentinel-")
		if !known || keyed != revealed {
			c.t.Errorf("%s: the answers row of %q holds keys %v while its submission is revealed %v", label, row[0], keyed, revealed)
		}
		if keyed {
			keyRows++
		}
	}

	c.noSentinelInText(label+" calendar", archive.calendar)
	c.noSentinelInText(label+" entry names", strings.Join(archive.entryNames(), "\n"))
	return statuses, keyRows
}

type contractExportWork struct {
	full      string
	automatic string
	closing   string
}

func (h *authHarness) answeredContractWork(t *testing.T, parties endingParties, material string, policy string) contractExportWork {
	t.Helper()
	picture := h.libraryUpload(t, parties.tutorToken, domain.ContextPromptImage, "prompt.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, picture, http.StatusCreated)
	full := h.workTemplate(t, parties.tutorToken, "Contract "+policy, contractBlocks(uploadedID(t, picture), material)...)
	automatic := h.workTemplate(t, parties.tutorToken, "Automatic "+policy, contractAutomaticBlocks()...)
	closing := h.workTemplate(t, parties.tutorToken, "Closing "+policy, contractAutomaticBlocks()...)
	_, fullSubmissions := h.issueTemplate(t, parties.tutorToken, full, toCard(parties.card.ID), policy, nil)
	_, automaticSubmissions := h.issueTemplate(t, parties.tutorToken, automatic, toCard(parties.card.ID), policy, nil)
	_, closingSubmissions := h.issueTemplate(t, parties.tutorToken, closing, toCard(parties.card.ID), policy, nil)
	work := contractExportWork{full: fullSubmissions[0].id, automatic: automaticSubmissions[0].id, closing: closingSubmissions[0].id}
	upload := h.answerFile(t, parties.studentToken, work.full, contractQuestion(7), "answer.pdf", 300)
	options := h.workOptions(t, parties.studentToken, work.full)
	draft := h.saved(t, parties.studentToken, work.full, answersJSON(t, 1, saveID(1),
		chosen(contractQuestion(2), options[contractQuestion(2)][1]), typedAnswer(contractQuestion(4), "a guess"), typedAnswer(contractQuestion(5), "12"),
		typedAnswer(contractQuestion(6), "An essay"), attached(contractQuestion(7), upload)))
	h.mustAnswer(t, h.submitWork(t, parties.studentToken, work.full, draft.Version), http.StatusOK)
	automaticOptions := h.workOptions(t, parties.studentToken, work.automatic)
	answered := h.saved(t, parties.studentToken, work.automatic, answersJSON(t, 1, saveID(2),
		chosen(contractQuestion(2), automaticOptions[contractQuestion(2)][0]), typedAnswer(contractQuestion(4), "a guess")))
	h.mustAnswer(t, h.submitWork(t, parties.studentToken, work.automatic, answered.Version), http.StatusOK)
	return work
}

func TestStudentExportAnswerKeyContract(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	check := contractCheck{t: t}
	parties := h.activeStudentInAGroup(t)
	worksheet := h.libraryUpload(t, parties.tutorToken, domain.ContextLibrary, "worksheet.pdf", pdfFile(100))
	h.mustAnswer(t, worksheet, http.StatusCreated)
	material := h.createMaterial(t, parties.tutorToken, uploadedID(t, worksheet), "Worksheet", nil).ID
	works := map[string]contractExportWork{}
	for _, policy := range []string{string(domain.AnswerPolicyNever), string(domain.AnswerPolicyAfterSubmission), string(domain.AnswerPolicyAfterCheck)} {
		works[policy] = h.answeredContractWork(t, parties, material, policy)
	}
	never := works[string(domain.AnswerPolicyNever)].full
	h.reviewSaved(t, parties.tutorToken, never, reviewJSON(t, 1, 0, saveID(3), "Overall good", commented(scored(contractQuestion(6), 73), "Well argued")))
	h.published(t, parties.tutorToken, never, domain.ReviewReturn, 1)
	h.saved(t, parties.studentToken, works[string(domain.AnswerPolicyAfterCheck)].closing, answersJSON(t, 1, saveID(4), typedAnswer(contractQuestion(4), "a draft")))
	student := exportAccount{id: parties.student, token: parties.studentToken, email: studentEmail, password: accountPassword}

	_, beforeArchive := h.studentArchiveOf(t, student)
	before := h.withRouteReveals(t, student.token, beforeArchive)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	h.tick(t)
	withdrawn, _ := h.exportOf(t, parties.student)
	student = h.signedInAgain(t, student)
	_, afterArchive := h.studentArchiveOf(t, student)
	after := h.withRouteReveals(t, student.token, afterArchive)

	if withdrawn.State != domain.ExportFailed || withdrawn.Failure == nil || *withdrawn.Failure != domain.ExportFailureWithdrawn {
		t.Fatalf("the ending left the first export %s", withdrawn.State)
	}
	seen := map[string]bool{}
	keyRows := 0
	for label, checked := range map[string]revealedArchive{"before the ending": before, "after the ending": after} {
		statuses, keyed := check.exportArchive(label, checked)
		keyRows += keyed
		for _, status := range statuses {
			seen[status] = true
		}
	}
	if keyRows == 0 {
		t.Error("no answers row of either export shows a revealed key")
	}
	for _, status := range []string{"not_started", "draft", "submitted", "returned", "checked", "closed"} {
		if !seen[status] {
			t.Errorf("the exports never held work in the status %s: %v", status, seen)
		}
	}
	for _, message := range h.mailTo(t, studentEmail) {
		check.noSentinelInText("email "+message.Subject, message.Subject+"\n"+message.Text+"\n"+message.HTML)
	}
}
