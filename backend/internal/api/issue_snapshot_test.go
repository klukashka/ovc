package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
)

func stableBlocks(t *testing.T, blocks []templateBlockBody) string {
	t.Helper()
	stripped := make([]templateBlockBody, 0, len(blocks))
	for _, block := range blocks {
		copied := block
		if block.Material != nil {
			material := *block.Material
			material.URL = ""
			copied.Material = &material
		}
		copied.Images = make([]templateFileBody, 0, len(block.Images))
		for _, image := range block.Images {
			image.URL = ""
			copied.Images = append(copied.Images, image)
		}
		stripped = append(stripped, copied)
	}
	encoded, err := json.Marshal(stripped)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestTemplateEditsNeverChangeIssues(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.issueParties(t)
	_, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	picture := h.libraryUpload(t, parties.tutorToken, domain.ContextPromptImage, "prompt.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, picture, http.StatusCreated)
	worksheet := h.libraryUpload(t, parties.tutorToken, domain.ContextLibrary, "worksheet.pdf", pdfFile(64))
	h.mustAnswer(t, worksheet, http.StatusCreated)
	material := h.createMaterial(t, parties.tutorToken, uploadedID(t, worksheet), "Worksheet", nil)
	blocks := []templateBlockJSON{materialBlock(blockID(44, 1), material.ID), imageQuestion(blockID(44, 2), uploadedID(t, picture))}
	template := h.createTemplate(t, parties.tutorToken, "Pictured", nil, blocks...)
	issue := h.issued(t, parties.tutorToken, issueJSON(template.ID, toCard(card)))
	before := h.issueDetail(t, parties.tutorToken, issue.ID)

	edited := slices.Clone(blocks)
	edited[1].Prompt = text("A different prompt")
	h.saveTemplate(t, parties.tutorToken, template.ID, saveJSON(t, "Renamed", template.Version, "0d8f2a1c-5b4e-4c3a-9e7f-1a2b3c4d5e6f", edited...))
	h.mustAnswer(t, h.patchMaterial(t, parties.tutorToken, material.ID, `{"title":"Renamed worksheet"}`), http.StatusOK)
	h.mustAnswer(t, h.deleteTemplate(t, parties.tutorToken, template.ID), http.StatusNoContent)
	h.mustAnswer(t, h.deleteMaterial(t, parties.tutorToken, material.ID), http.StatusNoContent)

	after := h.issueDetail(t, parties.tutorToken, issue.ID)
	servedPicture := h.fetch(t, after.Blocks[1].Images[0].URL, nil)
	servedMaterial := h.fetch(t, after.Blocks[0].Material.URL, nil)
	minted, _ := h.addressOf(t, parties.tutorToken, uploadedID(t, picture), "?ref=issue_snapshot:"+issue.ID)
	foreign, _ := h.addressOf(t, otherToken, uploadedID(t, picture), "?ref=issue_snapshot:"+issue.ID)
	random, _ := h.addressOf(t, otherToken, fixtureID(77, 1), "?ref=issue_snapshot:"+issue.ID)

	if stableBlocks(t, before.Blocks) != stableBlocks(t, after.Blocks) || after.Title != "Pictured" {
		t.Fatalf("the snapshot changed:\n%s\n%s", stableBlocks(t, before.Blocks), stableBlocks(t, after.Blocks))
	}
	if after.Blocks[0].Material == nil || after.Blocks[0].Material.Title == nil || *after.Blocks[0].Material.Title != "Worksheet" {
		t.Fatalf("the material block followed the rename: %+v", after.Blocks[0].Material)
	}
	if servedPicture.status != http.StatusOK || servedMaterial.status != http.StatusOK || servedMaterial.body != string(pdfFile(64)) {
		t.Fatalf("the snapshot's files answered %s and %s", describe(servedPicture), describe(servedMaterial))
	}
	h.mustAnswer(t, minted, http.StatusOK)
	if foreign.status != http.StatusNotFound || refusal(t, foreign) != refusal(t, random) {
		t.Fatalf("another tutor's mint answered %s", describe(foreign))
	}
	if h.referencesOfKind(t, uploadedID(t, picture), domain.ReferenceIssueSnapshot) != 1 || h.attachmentStateOf(t, uploadedID(t, worksheet)) != "bound" {
		t.Fatal("the snapshot lost its references")
	}
}

func TestIssueSnapshotDownloadsFollowTheIssue(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.issueParties(t)
	card := h.linkedCard(t, parties.tutor, 10, domain.RelationshipActive)
	picture := h.libraryUpload(t, parties.tutorToken, domain.ContextPromptImage, "prompt.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, picture, http.StatusCreated)
	image := uploadedID(t, picture)
	template := h.createTemplate(t, parties.tutorToken, "Pictured", nil, imageQuestion(blockID(47, 1), image))
	issue := h.issued(t, parties.tutorToken, issueJSON(template.ID, toCard(card)))
	reference := "?ref=issue_snapshot:" + issue.ID

	byStudent, studentAddress := h.addressOf(t, h.studentToken(t, 10), image, reference)
	random, _ := h.addressOf(t, h.studentToken(t, 10), fixtureID(78, 1), reference)
	servedStudent := h.fetch(t, studentAddress.URL, nil)
	h.mustAnswer(t, h.withdrawIssue(t, parties.tutorToken, issue.ID), http.StatusOK)
	studentWithdrawn, _ := h.addressOf(t, h.studentToken(t, 10), image, reference)
	afterWithdrawal := h.issueDetail(t, parties.tutorToken, issue.ID)
	servedWithdrawn := h.fetch(t, afterWithdrawal.Blocks[0].Images[0].URL, nil)
	mintedWithdrawn, _ := h.addressOf(t, parties.tutorToken, image, reference)
	h.markIssueForErasure(t, issue.ID)
	servedMarked := h.fetch(t, afterWithdrawal.Blocks[0].Images[0].URL, nil)
	mintedMarked, _ := h.addressOf(t, parties.tutorToken, image, reference)

	if byStudent.status != http.StatusOK || servedStudent.status != http.StatusOK {
		t.Fatalf("the student's mint answered %s and its address %s", describe(byStudent), describe(servedStudent))
	}
	if studentWithdrawn.status != http.StatusNotFound || refusal(t, studentWithdrawn) != refusal(t, random) {
		t.Fatalf("the student's mint on a withdrawn issue answered %s", describe(studentWithdrawn))
	}
	if servedWithdrawn.status != http.StatusOK || mintedWithdrawn.status != http.StatusOK {
		t.Fatalf("the tutor's address on a withdrawn issue answered %s and %s", describe(servedWithdrawn), describe(mintedWithdrawn))
	}
	if servedMarked.status != http.StatusNotFound || mintedMarked.status != http.StatusNotFound {
		t.Fatalf("an issue marked for erasure answered %s and %s", describe(servedMarked), describe(mintedMarked))
	}
}

type readabilityCase struct {
	student      byte
	tutor        byte
	relationship domain.RelationshipState
	issue        domain.IssueStatus
	marked       bool
}

func (c readabilityCase) tutorID() domain.UserID {
	if c.tutor == 0 {
		return domain.UserID{1}
	}
	return domain.UserID{c.tutor}
}

func readabilityCases() []readabilityCase {
	states := []domain.RelationshipState{
		domain.RelationshipActive, domain.RelationshipProvisional, domain.RelationshipEnded, domain.RelationshipReclaimPending, domain.RelationshipDiscarded,
	}
	var cases []readabilityCase
	next := byte(10)
	for _, state := range states {
		for _, variant := range []readabilityCase{{issue: domain.IssueActive}, {issue: domain.IssueWithdrawn}, {issue: domain.IssueActive, marked: true}} {
			variant.student, variant.relationship = next, state
			cases = append(cases, variant)
			next++
		}
	}
	return cases
}

type readableWork struct {
	image      string
	issue      string
	submission string
	lesson     string
	answer     string
	corrected  string
	revision   string
}

type readableTemplate struct {
	image    string
	template string
}

func (h *authHarness) readableTemplate(t *testing.T, tutorToken string, block byte) readableTemplate {
	t.Helper()
	picture := h.libraryUpload(t, tutorToken, domain.ContextPromptImage, "prompt.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, picture, http.StatusCreated)
	image := uploadedID(t, picture)
	template := h.workTemplate(t, tutorToken, "Readable", append(workBlocks(), imageQuestion(blockID(block, 1), image))...)
	return readableTemplate{image: image, template: template}
}

func (h *authHarness) readableWork(t *testing.T, tutorToken string, template readableTemplate, testCase readabilityCase, hour int) readableWork {
	t.Helper()
	student := h.seedStudentAccount(t, testCase.student, domain.AccountActive)
	card := h.seedCard(t, testCase.tutorID(), testCase.student, domain.RelationshipActive, &student)
	token := h.studentToken(t, testCase.student)
	work := readableWork{image: template.image}
	work.lesson = h.scheduled(t, tutorToken, oneOffFor(card, "2030-03-12", fmt.Sprintf("%02d:00", hour))).Lessons[0].ID
	work.issue = h.issued(t, tutorToken, issueJSONWith(template.template, toCard(card), &work.lesson, nil, string(domain.AnswerPolicyAfterCheck))).ID
	work.submission = h.submissionsOf(t, work.issue)[0].id
	work.answer = h.answerFile(t, token, work.submission, workQuestion(6), "answer.pdf", 48)
	draft := h.saved(t, token, work.submission, answersJSON(t, 1, saveID(1), attached(workQuestion(6), work.answer), typedAnswer(workQuestion(5), "We saw the tower")))
	if testCase.issue != domain.IssueWithdrawn {
		h.submitted(t, token, work.submission, draft.Version)
		work.corrected = h.correctedFile(t, tutorToken, work.submission, workQuestion(6), "corrected.pdf", 40)
		h.reviewSaved(t, tutorToken, work.submission, reviewJSON(t, 1, 0, saveID(1), "Good", fullyScored("Nice", work.corrected)...))
		h.published(t, tutorToken, work.submission, domain.ReviewReturn, 1)
		work.revision = h.revisionID(t, work.submission, 1)
	}
	h.cancelWithRoster(t, work.lesson, student)
	if testCase.issue == domain.IssueWithdrawn {
		h.mustAnswer(t, h.withdrawIssue(t, tutorToken, work.issue), http.StatusOK)
	}
	if testCase.marked {
		h.markIssueForErasure(t, work.issue)
	}
	h.setRelationshipState(t, card, testCase.relationship)
	return work
}

func (h *authHarness) mintsAs(t *testing.T, token string, work readableWork) (bool, bool, bool) {
	t.Helper()
	snapshot, _ := h.addressOf(t, token, work.image, "?ref=issue_snapshot:"+work.issue)
	answer, _ := h.addressOf(t, token, work.answer, "?ref=answer:"+work.submission)
	corrected := false
	if work.revision != "" {
		minted, _ := h.addressOf(t, token, work.corrected, "?ref=review_item:"+work.revision)
		corrected = minted.status == http.StatusOK
	}
	return snapshot.status == http.StatusOK, answer.status == http.StatusOK, corrected
}

func issueCreatedEmails(messages []mail.SinkMessage) int {
	count := 0
	for _, message := range messages {
		if !strings.HasPrefix(message.Subject, "Assignment returned") {
			count++
		}
	}
	return count
}

type homeTodo struct {
	Count int64                `json:"count"`
	Items []assignmentItemBody `json:"items"`
}

func (h *authHarness) homeTodoHolds(t *testing.T, token string, submission string) bool {
	t.Helper()
	todo := decodedAs[homeTodo](t, h.home(t, token, "")["todo"])
	return todo.Count == 1 && len(todo.Items) == 1 && todo.Items[0].SubmissionID == submission
}

type readabilityAgreement struct {
	readable int
	wrong    []string
}

func (h *authHarness) readabilityAgreement(t *testing.T, cases []readabilityCase, works []readableWork, now time.Time) readabilityAgreement {
	t.Helper()
	var agreement readabilityAgreement
	for index, testCase := range cases {
		standing := domain.TutorStanding{State: domain.AccountActive}
		if testCase.tutorID() == closingTutor {
			deletedAt := h.deletedAtOf(t, testCase.tutorID())
			standing = domain.TutorStanding{State: domain.AccountDeleted, DeletedAt: &deletedAt}
		}
		want := domain.ReadableStudentSubmissionAt(testCase.issue, testCase.marked, testCase.relationship, standing, now)
		emailWant := domain.ReadableStudentSubmission(testCase.issue, testCase.marked, testCase.relationship)
		work := works[index]
		token := h.studentToken(t, testCase.student)
		created := eventsNamed(h.notificationsOf(t, token), string(domain.NotificationIssueCreated))
		labelled := len(created) == 1 && created[0].Label != nil && created[0].Link != nil
		emailed := issueCreatedEmails(h.mailTo(t, memberEmail(testCase.student))) == 1
		read := h.submissionAnswer(t, token, work.submission).status == http.StatusOK
		listed := h.assignments(t, token, nil).Counts.Todo == 1
		homed := h.homeTodoHolds(t, token, work.submission)
		linked := slices.Contains(h.linkedWorkAs(t, token, work.lesson).submissionIDs(), work.submission)
		snapshot, answer, corrected := h.mintsAs(t, token, work)
		marked := h.markFeedbackSeen(t, token, work.submission, h.storedVersionOr(t, work.submission, 1)).status == http.StatusNoContent
		correctedWant := want && work.revision != ""
		if labelled != want || emailed != emailWant || read != want || listed != want || homed != want || linked != want || snapshot != want ||
			answer != want || corrected != correctedWant || marked != want {
			agreement.wrong = append(agreement.wrong, fmt.Sprintf("%+v: the predicate says %v, the row label %v, the email %v, the read %v, the list %v, "+
				"Home's to-do %v, the lesson's linked work %v, the snapshot mint %v, the answer mint %v, the corrected file mint %v, "+
				"the feedback-seen mark %v", testCase, want, labelled, emailed, read, listed, homed, linked, snapshot, answer, corrected, marked))
		}
		if (work.revision == "") != (testCase.issue == domain.IssueWithdrawn) {
			agreement.wrong = append(agreement.wrong, fmt.Sprintf("%+v: a reviewed revision exists %v", testCase, work.revision != ""))
		}
		if want {
			agreement.readable++
		}
	}
	return agreement
}

const closingTutorEmail = "closing-tutor@example.org"

var closingTutor = domain.UserID{2}

func TestTheStudentSubmissionReadabilityCopiesAgree(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, closingToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, closingTutor[0], closingTutorEmail)
	template := h.readableTemplate(t, tutorToken, 76)
	closingTemplate := h.readableTemplate(t, closingToken, 77)
	cases := readabilityCases()
	works := make([]readableWork, 0, len(cases)+1)
	for index, testCase := range cases {
		works = append(works, h.readableWork(t, tutorToken, template, testCase, 5+index))
	}
	closing := readabilityCase{student: 40, tutor: closingTutor[0], relationship: domain.RelationshipEnded, issue: domain.IssueActive}
	works = append(works, h.readableWork(t, closingToken, closingTemplate, closing, 5+len(cases)))
	cases = append(cases, closing)
	h.tick(t)
	h.mustAnswer(t, h.deleteMe(t, closingToken, accountPassword), http.StatusNoContent)
	closedUntil := h.deletedAtOf(t, closingTutor).Add(domain.DeletedAccountRetention)
	h.tick(t)
	h.at(t, closedUntil.Add(-time.Minute))

	before := h.readabilityAgreement(t, cases, works, h.now(t))
	h.at(t, closedUntil)
	after := h.readabilityAgreement(t, cases, works, h.now(t))

	if len(before.wrong) != 0 || len(after.wrong) != 0 {
		t.Fatalf("the copies disagree before account_closed_until: %v; and from it: %v", before.wrong, after.wrong)
	}
	if before.readable != 4 || after.readable != 3 {
		t.Fatalf("%d and %d cases are readable, want the active, provisional and ended links of an active issue and, before "+
			"account_closed_until, the deleted tutor's ended link", before.readable, after.readable)
	}
}
