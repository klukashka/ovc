package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	assignmentsRoute = "/api/v1/assignments"
	submissionsRoute = "/api/v1/submissions/"
	workGroup        = 70
)

type answerValueJSON struct {
	OptionIDs     *[]string `json:"option_ids,omitempty"`
	Text          *string   `json:"text,omitempty"`
	AttachmentIDs *[]string `json:"attachment_ids,omitempty"`
}

type answerJSON struct {
	QuestionID string          `json:"question_id"`
	Value      answerValueJSON `json:"value"`
}

type answersSaveJSON struct {
	Version int64        `json:"version"`
	SaveID  string       `json:"save_id"`
	Answers []answerJSON `json:"answers"`
}

type submissionFileBody struct {
	AttachmentID string `json:"attachment_id"`
	Filename     string `json:"filename"`
	URL          string `json:"url"`
}

type studentAnswerBody struct {
	QuestionID string               `json:"question_id"`
	Value      answerValueJSON      `json:"value"`
	Files      []submissionFileBody `json:"files"`
}

type revealedQuestionBody struct {
	QuestionID       string    `json:"question_id"`
	CorrectOptionIDs *[]string `json:"correct_option_ids"`
	AcceptedAnswers  *[]string `json:"accepted_answers"`
}

type studentSubmissionBody struct {
	ID                  string              `json:"id"`
	Title               string              `json:"title"`
	Status              string              `json:"status"`
	Late                bool                `json:"late"`
	Version             int64               `json:"version"`
	Answers             []studentAnswerBody `json:"answers"`
	EditableQuestionIDs []string            `json:"editable_question_ids"`
	Revealed            *struct {
		Questions []revealedQuestionBody `json:"questions"`
	} `json:"revealed"`
}

type tutorAnswerBody struct {
	QuestionID        string               `json:"question_id"`
	Normalised        *string              `json:"normalised"`
	NormaliserVersion *int32               `json:"normaliser_version"`
	AutoPoints        *int32               `json:"auto_points"`
	AutoCorrect       *bool                `json:"auto_correct"`
	FrozenAt          *time.Time           `json:"frozen_at"`
	Files             []submissionFileBody `json:"files"`
}

type tutorSubmissionBody struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	AutoPoints  *int32            `json:"auto_points"`
	FinalPoints *int32            `json:"final_points"`
	MaxPoints   int32             `json:"max_points"`
	Answers     []tutorAnswerBody `json:"answers"`
	Revisions   []json.RawMessage `json:"revisions"`
}

type assignmentItemBody struct {
	SubmissionID string `json:"submission_id"`
	Status       string `json:"status"`
}

type assignmentPageBody struct {
	Items      []assignmentItemBody `json:"items"`
	NextCursor *string              `json:"next_cursor"`
	Counts     struct {
		Todo   int64 `json:"todo"`
		Closed int64 `json:"closed"`
	} `json:"counts"`
}

type workParties struct {
	endingParties
	issue      string
	submission string
}

func saveID(n byte) string {
	return fmt.Sprintf("%08x-2222-4222-a222-%012x", n, n)
}

func chosen(question string, options ...string) answerJSON {
	return answerJSON{QuestionID: question, Value: answerValueJSON{OptionIDs: &options}}
}

func typedAnswer(question string, value string) answerJSON {
	return answerJSON{QuestionID: question, Value: answerValueJSON{Text: &value}}
}

func attached(question string, files ...string) answerJSON {
	return answerJSON{QuestionID: question, Value: answerValueJSON{AttachmentIDs: &files}}
}

func answersJSON(t *testing.T, version int64, save string, answers ...answerJSON) string {
	t.Helper()
	if answers == nil {
		answers = []answerJSON{}
	}
	encoded, err := json.Marshal(answersSaveJSON{Version: version, SaveID: save, Answers: answers})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func workQuestion(n byte) string {
	return blockID(workGroup, n)
}

func workOption(n byte) string {
	return optionID(workGroup, n)
}

func workBlocks() []templateBlockJSON {
	multiple := []templateOptionJSON{
		{ID: workOption(3), Text: "Red", Correct: flag(true)}, {ID: workOption(4), Text: "Blue", Correct: flag(true)},
		{ID: workOption(5), Text: "Green", Correct: flag(false)},
	}
	return []templateBlockJSON{
		singleChoiceBlock(workQuestion(1), "The capital of France", workOption(1), workOption(2)),
		{ID: workQuestion(2), Type: string(domain.BlockMultipleChoice), Prompt: text("Pick the flag colours"), Options: &multiple,
			Scoring: text(string(domain.ScoringPartial)), Points: points(4)},
		{ID: workQuestion(3), Type: string(domain.BlockShortText), Prompt: text("Name the tree"), AcceptedAnswers: textList("Ёлка"), Points: points(3)},
		{ID: workQuestion(4), Type: string(domain.BlockShortNumber), Prompt: text("Half of five"), Value: text("2.5"), Tolerance: text("0.1"),
			Points: points(2)},
		{ID: workQuestion(5), Type: string(domain.BlockLongText), Prompt: text("Describe the trip"), MaxLength: points(50), Points: points(5)},
		{ID: workQuestion(6), Type: string(domain.BlockFile), Prompt: text("Photograph your work"),
			AllowedKinds: textList(string(domain.AnswerFileImages), string(domain.AnswerFilePDF)), MaxFiles: points(2), Points: points(5)},
	}
}

func automaticBlocks() []templateBlockJSON {
	return []templateBlockJSON{
		singleChoiceBlock(workQuestion(1), "The capital of France", workOption(1), workOption(2)),
		{ID: workQuestion(3), Type: string(domain.BlockShortText), Prompt: text("Name the tree"), AcceptedAnswers: textList("Ёлка"), Points: points(3)},
	}
}

func (h *authHarness) workTemplate(t *testing.T, token string, title string, blocks ...templateBlockJSON) string {
	t.Helper()
	template := h.createTemplate(t, token, title, nil, blocks...)
	if !template.Ready {
		t.Fatalf("the work template is not ready: %+v", template.Problems)
	}
	return template.ID
}

func (h *authHarness) issueTemplate(t *testing.T, token string, template string, audience string, policy string, deadline *time.Time,
) (string, []issueSubmissionRow) {
	t.Helper()
	issue := h.issued(t, token, issueJSONWith(template, audience, nil, deadline, policy))
	return issue.ID, h.submissionsOf(t, issue.ID)
}

func (h *authHarness) issueWork(t *testing.T, token string, audience string, title string, policy string, deadline *time.Time,
	blocks ...templateBlockJSON,
) (string, []issueSubmissionRow) {
	t.Helper()
	return h.issueTemplate(t, token, h.workTemplate(t, token, title, blocks...), audience, policy, deadline)
}

func (h *authHarness) workParties(t *testing.T, blocks ...templateBlockJSON) workParties {
	t.Helper()
	parties := h.activeStudentInAGroup(t)
	issue, submissions := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Trip to Paris", string(domain.AnswerPolicyNever), nil, blocks...)
	return workParties{endingParties: parties, issue: issue, submission: submissions[0].id}
}

func (h *authHarness) saveAnswers(t *testing.T, token string, submission string, body string) response {
	t.Helper()
	h.nextSecond(t)
	return h.saveAnswersNow(t, token, submission, body)
}

func (h *authHarness) saveAnswersNow(t *testing.T, token string, submission string, body string) response {
	t.Helper()
	return h.send(t, http.MethodPut, submissionsRoute+submission+"/answers", body, jsonCall(token))
}

func (h *authHarness) saved(t *testing.T, token string, submission string, body string) studentSubmissionBody {
	t.Helper()
	answer := h.saveAnswers(t, token, submission, body)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[studentSubmissionBody](t, answer)
}

func (h *authHarness) submitWork(t *testing.T, token string, submission string, version int64) response {
	t.Helper()
	h.nextSecond(t)
	return h.send(t, http.MethodPost, submissionsRoute+submission+"/submit", fmt.Sprintf(`{"version":%d}`, version), jsonCall(token))
}

func (h *authHarness) submitted(t *testing.T, token string, submission string, version int64) studentSubmissionBody {
	t.Helper()
	answer := h.submitWork(t, token, submission, version)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[studentSubmissionBody](t, answer)
}

func (h *authHarness) submissionAnswer(t *testing.T, token string, submission string) response {
	t.Helper()
	return h.send(t, http.MethodGet, submissionsRoute+submission, "", jsonCall(token))
}

func (h *authHarness) studentWork(t *testing.T, token string, submission string) studentSubmissionBody {
	t.Helper()
	answer := h.submissionAnswer(t, token, submission)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[studentSubmissionBody](t, answer)
}

func (h *authHarness) tutorWork(t *testing.T, token string, submission string) tutorSubmissionBody {
	t.Helper()
	answer := h.submissionAnswer(t, token, submission)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[tutorSubmissionBody](t, answer)
}

func (h *authHarness) assignmentsAnswer(t *testing.T, token string, query url.Values) response {
	t.Helper()
	path := assignmentsRoute
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return h.send(t, http.MethodGet, path, "", jsonCall(token))
}

func (h *authHarness) assignments(t *testing.T, token string, query url.Values) assignmentPageBody {
	t.Helper()
	answer := h.assignmentsAnswer(t, token, query)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[assignmentPageBody](t, answer)
}

func (h *authHarness) answerFile(t *testing.T, token string, submission string, question string, name string, size int) string {
	t.Helper()
	answer := h.homeworkUpload(t, token, homeworkRoute(domain.ContextAnswer, submission, question), name, pdfFile(size))
	h.mustAnswer(t, answer, http.StatusCreated)
	return uploadedID(t, answer)
}

func (h *authHarness) setSubmissionStatus(t *testing.T, submission string, status domain.SubmissionStatus) {
	t.Helper()
	if _, err := h.database.Pool.Exec(t.Context(), "UPDATE submissions SET status = $2, version = version + 1 WHERE id = $1::uuid", submission,
		string(status)); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) answerReferences(t *testing.T, submission string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(attachment_id) FROM attachment_references WHERE ref_kind = 'answer' AND ref_id = $1::uuid", submission)
}

func (h *authHarness) storageDeleteQueued(t *testing.T, attachment string) bool {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = 'storage_delete' AND idempotency_key = 'storage_delete:' || $1", attachment) == 1
}

func (h *authHarness) submissionField(t *testing.T, submission string, column string) string {
	t.Helper()
	var value *string
	if err := h.database.Pool.QueryRow(t.Context(), "SELECT "+column+"::text FROM submissions WHERE id = $1::uuid", submission).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value == nil {
		return ""
	}
	return *value
}

func fileIDs(files []submissionFileBody) string {
	ids := make([]string, 0, len(files))
	for _, file := range files {
		ids = append(ids, file.AttachmentID)
	}
	return strings.Join(ids, ",")
}

func answerTo(answers []studentAnswerBody, question string) (studentAnswerBody, bool) {
	for _, answer := range answers {
		if answer.QuestionID == question {
			return answer, true
		}
	}
	return studentAnswerBody{}, false
}

const randomSubmissionID = "5d2e8a41-7c3b-4f9e-8a6d-2b1c0e9f7a35"

func anotherStudentsSubmission(method string, suffix string, body string) denialCase {
	return func(t *testing.T, h *authHarness) (response, response) {
		t.Helper()
		parties := h.activeStudentInAGroup(t)
		h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
		_, submissions := h.issueWork(t, parties.tutorToken, toCard(parties.card.ID), "Mine", string(domain.AnswerPolicyNever), nil, workBlocks()...)
		classmateToken := h.studentToken(t, 10)
		outOfScope := h.send(t, method, submissionsRoute+submissions[0].id+suffix, body, jsonCall(classmateToken))
		random := h.send(t, method, submissionsRoute+randomSubmissionID+suffix, body, jsonCall(classmateToken))
		if after := h.studentWork(t, parties.studentToken, submissions[0].id); after.Version != 1 || after.Status != string(domain.SubmissionNotStarted) {
			t.Fatalf("another student changed the work: %+v", after)
		}
		return outOfScope, random
	}
}

func anotherTutorsSubmission(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	owners := h.issueOwners(t)
	h.shareStudentWith(t, owners.intruder, domain.UserID{10})
	issue := h.issued(t, owners.ownerToken, issueJSON(owners.template.ID, toCard(owners.card)))
	submission := h.submissionsOf(t, issue.ID)[0].id
	outOfScope := h.submissionAnswer(t, owners.intruderToken, submission)
	random := h.submissionAnswer(t, owners.intruderToken, randomSubmissionID)
	h.linkedCard(t, owners.owner, 11, domain.RelationshipActive)
	byClassmate := h.submissionAnswer(t, h.studentToken(t, 11), submission)
	if byClassmate.status != http.StatusNotFound || refusal(t, byClassmate) != refusal(t, random) {
		t.Fatalf("another student's read answered %s", describe(byClassmate))
	}
	return outOfScope, random
}

func anotherStudentsTutorFilter(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	owners := h.issueOwners(t)
	h.linkedCard(t, owners.owner, 11, domain.RelationshipActive)
	token := h.studentToken(t, 11)
	outOfScope := h.assignmentsAnswer(t, token, url.Values{"tutor": {owners.card}})
	random := h.assignmentsAnswer(t, token, url.Values{"tutor": {randomSubmissionID}})
	return outOfScope, random
}

func (h *authHarness) workOptions(t *testing.T, token string, submission string) map[string][]string {
	t.Helper()
	var view struct {
		Blocks []struct {
			ID      string `json:"id"`
			Options []struct {
				ID string `json:"id"`
			} `json:"options"`
		} `json:"blocks"`
	}
	answer := h.submissionAnswer(t, token, submission)
	h.mustAnswer(t, answer, http.StatusOK)
	if err := json.Unmarshal([]byte(answer.body), &view); err != nil {
		t.Fatal(err)
	}
	options := map[string][]string{}
	for _, block := range view.Blocks {
		for _, option := range block.Options {
			options[block.ID] = append(options[block.ID], option.ID)
		}
	}
	return options
}
