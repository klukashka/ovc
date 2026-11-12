package api_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

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
	relationship domain.RelationshipState
	issue        domain.IssueStatus
	marked       bool
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

func TestTheStudentSubmissionReadabilityCopiesAgree(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.issueParties(t)
	cases := readabilityCases()
	submissions := make([]string, 0, len(cases))
	for _, testCase := range cases {
		student := h.seedStudentAccount(t, testCase.student, domain.AccountActive)
		card := h.seedCard(t, parties.tutor, testCase.student, domain.RelationshipActive, &student)
		issue := h.issued(t, parties.tutorToken, issueJSON(parties.template.ID, toCard(card)))
		submissions = append(submissions, h.submissionsOf(t, issue.ID)[0].id)
		if testCase.issue == domain.IssueWithdrawn {
			h.mustAnswer(t, h.withdrawIssue(t, parties.tutorToken, issue.ID), http.StatusOK)
		}
		if testCase.marked {
			h.markIssueForErasure(t, issue.ID)
		}
		h.setRelationshipState(t, card, testCase.relationship)
	}

	h.tick(t)

	readableCount := 0
	for index, testCase := range cases {
		want := domain.ReadableStudentSubmission(testCase.issue, testCase.marked, testCase.relationship)
		token := h.studentToken(t, testCase.student)
		created := eventsNamed(h.notificationsOf(t, token), string(domain.NotificationIssueCreated))
		labelled := len(created) == 1 && created[0].Label != nil && created[0].Link != nil
		emailed := len(h.mailTo(t, memberEmail(testCase.student))) == 1
		read := h.submissionAnswer(t, token, submissions[index]).status == http.StatusOK
		listed := h.assignments(t, token, nil).Counts.Todo == 1
		if labelled != want || emailed != want || read != want || listed != want {
			t.Errorf("%+v: the predicate says %v, the row label %v, the email %v, the read %v, the list %v", testCase, want, labelled, emailed, read, listed)
		}
		if want {
			readableCount++
		}
	}
	if readableCount != 3 {
		t.Fatalf("%d cases are readable, want the active, provisional and ended links of an active issue", readableCount)
	}
}
