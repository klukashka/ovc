package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	saveA = "0d8f2a1c-5b4e-4c3a-9e7f-1a2b3c4d5e6f"
	saveB = "1e9a3b2d-6c5f-4d4b-8f8a-2b3c4d5e6f7a"
)

func problemCodes(problems []templateProblemBody) []string {
	codes := make([]string, 0, len(problems))
	for _, problem := range problems {
		codes = append(codes, problem.Code)
	}
	slices.Sort(codes)
	return codes
}

func TestTemplateAutosaveReplay(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	template := h.createTemplate(t, token, "Quiz", nil)
	saved := h.saveTemplate(t, token, template.ID, saveJSON(t, "Quiz", 1, saveA, instructionBlock(blockID(1, 1), "Read")))

	replayed := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Ignored", 1, saveA))
	stale := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Stale", 1, saveB))

	h.mustAnswer(t, replayed, http.StatusOK)
	current := decodeLibrary[templateBody](t, replayed)
	if saved.Version != 2 || current.Version != 2 || current.Title != "Quiz" || len(current.Blocks) != 1 {
		t.Fatalf("the replay answered %+v after %+v", current, saved)
	}
	if stale.status != http.StatusConflict || code(t, stale) != "conflict" {
		t.Fatalf("a stale version answered %s", describe(stale))
	}
	if h.countLogs(`"msg":"library.save_conflict"`, template.ID) != 1 {
		t.Fatal("the conflict was not recorded")
	}
}

func TestTemplateProblems(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	empty := h.createTemplate(t, token, "Quiz", nil)
	emptyOptions := []templateOptionJSON{{ID: optionID(2, 1), Text: " ", Correct: flag(false)}}
	noCorrect := []templateOptionJSON{{ID: optionID(2, 2), Text: "A", Correct: flag(false)}, {ID: optionID(2, 3), Text: "B", Correct: flag(false)}}
	incomplete := []templateBlockJSON{
		instructionBlock(blockID(2, 1), "  "),
		{ID: blockID(2, 2), Type: "single_choice", Prompt: text(" "), Options: &emptyOptions, Points: points(1)},
		{ID: blockID(2, 3), Type: "multiple_choice", Prompt: text("Pick"), Options: &noCorrect, Scoring: text("partial"), Points: points(1)},
		{ID: blockID(2, 4), Type: "short_text", Prompt: text("Say"), Points: points(1)},
		{ID: blockID(2, 5), Type: "short_text", Prompt: text("Say"), AcceptedAnswers: textList("ok", "   "), Points: points(1)},
		{ID: blockID(2, 6), Type: "short_number", Prompt: text("Count"), Value: text("1e3"), Tolerance: text("-1"), Points: points(1)},
	}
	complete := []templateBlockJSON{
		instructionBlock(blockID(3, 1), "Read"),
		singleChoiceBlock(blockID(3, 2), "Capital?", optionID(3, 1), optionID(3, 2)),
		{ID: blockID(3, 3), Type: "short_number", Prompt: text("Count"), Value: text("-1.5"), Tolerance: text("0.000001"), Points: points(5)},
		{ID: blockID(3, 4), Type: "file", Prompt: text("Upload"), AllowedKinds: textList("pdf", "images"), MaxFiles: points(2), Points: points(0)},
	}

	partial := h.saveTemplate(t, token, empty.ID, saveJSON(t, "Quiz", 1, saveA, incomplete...))
	ready := h.saveTemplate(t, token, empty.ID, saveJSON(t, "Quiz", 2, saveB, complete...))

	if empty.Ready || len(empty.Problems) != 1 || empty.Problems[0].Code != "no_blocks" || empty.Problems[0].BlockID != nil {
		t.Fatalf("an empty template answered %+v", empty.Problems)
	}
	want := []string{
		"accepted_answer_empty", "correct_count", "no_accepted_answer", "no_correct_option", "option_empty", "prompt_missing", "text_missing",
		"tolerance_invalid", "too_few_options", "value_invalid",
	}
	if partial.Ready || !slices.Equal(problemCodes(partial.Problems), want) {
		t.Fatalf("the incomplete template answered %v", problemCodes(partial.Problems))
	}
	if partial.Problems[0].BlockID == nil || *partial.Problems[0].BlockID != blockID(2, 1) {
		t.Fatalf("the problems are not in block order: %+v", partial.Problems)
	}
	if !ready.Ready || len(ready.Problems) != 0 || ready.QuestionCount != 3 || ready.MaxPoints != 7 {
		t.Fatalf("the complete template answered %+v", ready)
	}
}

func TestTemplateStructuralRefusals(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	template := h.createTemplate(t, token, "Quiz", nil)
	question := func(change func(*templateBlockJSON)) templateBlockJSON {
		block := singleChoiceBlock(blockID(4, 1), "Capital?", optionID(4, 1), optionID(4, 2))
		change(&block)
		return block
	}
	many := make([]templateBlockJSON, 0, domain.TemplateMaxBlocks+1)
	for index := range domain.TemplateMaxBlocks + 1 {
		many = append(many, instructionBlock(blockID(5, byte(index)), "Read"))
	}
	elevenOptions := make([]templateOptionJSON, 0, 11)
	for index := range 11 {
		elevenOptions = append(elevenOptions, templateOptionJSON{ID: optionID(6, byte(index)), Text: "A", Correct: flag(false)})
	}
	answers := make([]string, 21)
	for index := range answers {
		answers[index] = "a"
	}
	cases := []struct {
		name   string
		title  string
		blocks []templateBlockJSON
		path   string
		code   string
	}{
		{"too many blocks", "Quiz", many, "body.blocks", "too_many"},
		{"an identifier that is not version 4", "Quiz", []templateBlockJSON{instructionBlock("00000000-0000-1000-8000-000000000001", "Read")}, "body.blocks[0].id", "invalid"},
		{"a repeated block", "Quiz", []templateBlockJSON{instructionBlock(blockID(4, 2), "A"), instructionBlock(blockID(4, 2), "B")}, "body.blocks[1].id", "invalid"},
		{"a field of another type", "Quiz", []templateBlockJSON{{ID: blockID(4, 3), Type: "instruction", Prompt: text("x")}}, "body.blocks[0].prompt", "not_allowed"},
		{"a missing maximum length", "Quiz", []templateBlockJSON{{ID: blockID(4, 4), Type: "long_text", Points: points(1)}}, "body.blocks[0].max_length", "required"},
		{"a missing material", "Quiz", []templateBlockJSON{{ID: blockID(4, 5), Type: "material"}}, "body.blocks[0].material_id", "required"},
		{"a long prompt", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { b.Prompt = text(strings.Repeat("я", 10_001)) })}, "body.blocks[0].prompt", "too_long"},
		{"a null character", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { b.Prompt = text("a\u0000b") })}, "body.blocks[0].prompt", "invalid"},
		{"too many options", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { b.Options = &elevenOptions })}, "body.blocks[0].options", "too_many"},
		{"a long option", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { (*b.Options)[1].Text = strings.Repeat("o", 501) })}, "body.blocks[0].options[1].text", "too_long"},
		{"an option without correctness", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { (*b.Options)[0].Correct = nil })}, "body.blocks[0].options[0].correct", "required"},
		{"a repeated option", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { (*b.Options)[1].ID = optionID(4, 1) })}, "body.blocks[0].options[1].id", "invalid"},
		{"points above the bound", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { b.Points = points(101) })}, "body.blocks[0].points", "invalid"},
		{"missing points", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { b.Points = nil })}, "body.blocks[0].points", "required"},
		{"too many images", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) {
			b.ImageIDs = textList(fixtureID(7, 1), fixtureID(7, 2), fixtureID(7, 3), fixtureID(7, 4))
		})}, "body.blocks[0].image_ids", "too_many"},
		{"a repeated image", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { b.ImageIDs = textList(fixtureID(7, 1), fixtureID(7, 1)) })}, "body.blocks[0].image_ids[1]", "invalid"},
		{"too many accepted answers", "Quiz", []templateBlockJSON{{ID: blockID(4, 6), Type: "short_text", AcceptedAnswers: &answers, Points: points(1)}}, "body.blocks[0].accepted_answers", "too_many"},
		{"a long accepted answer", "Quiz", []templateBlockJSON{{ID: blockID(4, 6), Type: "short_text", AcceptedAnswers: textList(strings.Repeat("a", 501)), Points: points(1)}}, "body.blocks[0].accepted_answers[0]", "too_long"},
		{"a long value", "Quiz", []templateBlockJSON{{ID: blockID(4, 7), Type: "short_number", Value: text(strings.Repeat("1", 65)), Points: points(1)}}, "body.blocks[0].value", "too_long"},
		{"a zero maximum length", "Quiz", []templateBlockJSON{{ID: blockID(4, 8), Type: "long_text", MaxLength: points(0), Points: points(1)}}, "body.blocks[0].max_length", "invalid"},
		{"too many files", "Quiz", []templateBlockJSON{{ID: blockID(4, 9), Type: "file", AllowedKinds: textList("pdf"), MaxFiles: points(6), Points: points(1)}}, "body.blocks[0].max_files", "invalid"},
		{"no allowed kinds", "Quiz", []templateBlockJSON{{ID: blockID(4, 9), Type: "file", AllowedKinds: &[]string{}, MaxFiles: points(1), Points: points(1)}}, "body.blocks[0].allowed_kinds", "invalid"},
		{"a repeated allowed kind", "Quiz", []templateBlockJSON{{ID: blockID(4, 9), Type: "file", AllowedKinds: textList("pdf", "pdf"), MaxFiles: points(1), Points: points(1)}}, "body.blocks[0].allowed_kinds", "invalid"},
		{"an empty title", " ", nil, "body.title", "required"},
		{"a long title", strings.Repeat("t", 201), nil, "body.title", "too_long"},
		{"a control character in the title", "Quiz\u0007", nil, "body.title", "invalid"},
		{"a long instruction", "Quiz", []templateBlockJSON{instructionBlock(blockID(4, 10), strings.Repeat("я", 10_001))}, "body.blocks[0].text", "too_long"},
		{"a long explanation", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { b.Explanation = text(strings.Repeat("e", 10_001)) })}, "body.blocks[0].explanation", "too_long"},
		{"a long tolerance", "Quiz", []templateBlockJSON{{ID: blockID(4, 7), Type: "short_number", Tolerance: text(strings.Repeat("1", 65)), Points: points(1)}}, "body.blocks[0].tolerance", "too_long"},
		{"a maximum length above the bound", "Quiz", []templateBlockJSON{{ID: blockID(4, 8), Type: "long_text", MaxLength: points(20_001), Points: points(1)}}, "body.blocks[0].max_length", "invalid"},
		{"negative points", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { b.Points = points(-1) })}, "body.blocks[0].points", "invalid"},
		{"no files", "Quiz", []templateBlockJSON{{ID: blockID(4, 9), Type: "file", AllowedKinds: textList("pdf"), MaxFiles: points(0), Points: points(1)}}, "body.blocks[0].max_files", "invalid"},
		{"a malformed image", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { b.ImageIDs = textList("nonsense") })}, "body.blocks[0].image_ids[0]", "invalid"},
		{"a malformed material", "Quiz", []templateBlockJSON{{ID: blockID(4, 5), Type: "material", MaterialID: text("nonsense")}}, "body.blocks[0].material_id", "invalid"},
		{"a null character in an option", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { (*b.Options)[0].Text = "a\u0000" })}, "body.blocks[0].options[0].text", "invalid"},
		{"a null character in an accepted answer", "Quiz", []templateBlockJSON{{ID: blockID(4, 6), Type: "short_text", AcceptedAnswers: textList("a\u0000"), Points: points(1)}}, "body.blocks[0].accepted_answers[0]", "invalid"},
		{"points on an instruction", "Quiz", []templateBlockJSON{{ID: blockID(4, 11), Type: "instruction", Points: points(1)}}, "body.blocks[0].points", "not_allowed"},
		{"options on a short text", "Quiz", []templateBlockJSON{{ID: blockID(4, 12), Type: "short_text", Options: &[]templateOptionJSON{}, Points: points(1)}}, "body.blocks[0].options", "not_allowed"},
		{"a material on a question", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { b.MaterialID = text(randomCardID) })}, "body.blocks[0].material_id", "not_allowed"},
		{"scoring on a single choice", "Quiz", []templateBlockJSON{question(func(b *templateBlockJSON) { b.Scoring = text("partial") })}, "body.blocks[0].scoring", "not_allowed"},
		{"a value on a long text", "Quiz", []templateBlockJSON{{ID: blockID(4, 8), Type: "long_text", MaxLength: points(5), Value: text("1"), Points: points(1)}}, "body.blocks[0].value", "not_allowed"},
		{"files on a material", "Quiz", []templateBlockJSON{{ID: blockID(4, 5), Type: "material", MaterialID: text(randomCardID), MaxFiles: points(1)}}, "body.blocks[0].max_files", "not_allowed"},
	}

	for _, refused := range cases {
		h.nextSecond(t)
		answer := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, refused.title, 1, saveA, refused.blocks...))

		if path, field := fieldOf(t, answer); path != refused.path || field != refused.code {
			t.Errorf("%s answered %s at %s", refused.name, field, path)
		}
	}
	if h.countRows(t, "SELECT version FROM templates WHERE id = $1::uuid", template.ID) != 1 {
		t.Fatal("a refused save changed the template")
	}
}

func TestTemplateForeignAttachments(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	owner, ownerToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	intruder, intruderToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, newcomerEmail)
	foreignImage := h.pendingPromptImage(t, owner, 70, 1)
	foreignMaterial := h.createMaterial(t, ownerToken, h.pendingLibraryFile(t, owner, 70, 2, 10), "Worksheet", nil)
	ownDeleted := h.createMaterial(t, intruderToken, h.pendingLibraryFile(t, intruder, 70, 3, 10), "Old", nil)
	h.mustAnswer(t, h.deleteMaterial(t, intruderToken, ownDeleted.ID), http.StatusNoContent)
	libraryFile := h.pendingLibraryFile(t, intruder, 70, 4, 10)
	usedImage := h.pendingPromptImage(t, intruder, 70, 5)
	h.createTemplate(t, intruderToken, "Other", nil, imageQuestion(blockID(8, 9), usedImage))
	template := h.createTemplate(t, intruderToken, "Quiz", nil)
	random := h.saveTemplateAnswer(t, intruderToken, template.ID, saveJSON(t, "Quiz", 1, saveA, imageQuestion(blockID(8, 1), fixtureID(70, 9))))

	refusals := map[string]templateBlockJSON{
		"another tutor's prompt image":                 imageQuestion(blockID(8, 1), foreignImage),
		"another tutor's material":                     materialBlock(blockID(8, 2), foreignMaterial.ID),
		"a deleted material the template never showed": materialBlock(blockID(8, 3), ownDeleted.ID),
		"a library upload as a prompt image":           imageQuestion(blockID(8, 4), libraryFile),
		"another template's prompt image":              imageQuestion(blockID(8, 5), usedImage),
		"a random material":                            materialBlock(blockID(8, 6), randomCardID),
	}

	for name, block := range refusals {
		h.nextSecond(t)
		refused := h.saveTemplateAnswer(t, intruderToken, template.ID, saveJSON(t, "Quiz", 1, saveA, block))

		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, random) {
			t.Errorf("%s answered %s", name, describe(refused))
		}
	}
	if h.uploadState(t, foreignImage) != string(domain.AttachmentPending) || h.templateReferences(t, template.ID) != 0 {
		t.Fatal("a refused save bound or referenced a file")
	}
	if created := h.createTemplateAnswer(t, intruderToken, "", newTemplateJSON(t, "New", nil, imageQuestion(blockID(8, 7), foreignImage))); created.status != http.StatusNotFound {
		t.Fatalf("creating with another tutor's image answered %s", describe(created))
	}
}

func imageQuestion(id string, image string) templateBlockJSON {
	block := singleChoiceBlock(id, "Look", optionID(9, id[7]), optionID(10, id[7]))
	block.ImageIDs = textList(image)
	return block
}

func TestTemplateDuplicateSharesFiles(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	folder := h.createFolder(t, token, "Unit", nil)
	image := h.pendingPromptImage(t, tutor, 71, 1)
	material := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 71, 2, 30), "Worksheet", nil)
	source := h.createTemplate(t, token, "Quiz", nil, imageQuestion(blockID(11, 1), image), materialBlock(blockID(11, 2), material.ID))
	usage, attachments := h.fileUsage(t, tutor), h.countRows(t, "SELECT count(id) FROM attachments WHERE uploader_id = $1::uuid", tutor.String())

	answer := h.duplicateTemplate(t, token, source.ID, "Quiz (copy)", &folder.ID)

	h.mustAnswer(t, answer, http.StatusCreated)
	duplicate := decodeLibrary[templateBody](t, answer)
	if duplicate.ID == source.ID || duplicate.Title != "Quiz (copy)" || duplicate.FolderID == nil || *duplicate.FolderID != folder.ID {
		t.Fatalf("the duplicate is %+v", duplicate)
	}
	if len(duplicate.Blocks) != 2 || duplicate.Blocks[0].ID != blockID(11, 1) || duplicate.Blocks[1].Material == nil {
		t.Fatalf("the duplicate's blocks are %+v", duplicate.Blocks)
	}
	if h.templateReferences(t, duplicate.ID) != 2 || h.fileUsage(t, tutor) != usage || usage != 40 {
		t.Fatalf("the duplicate holds %d references and the usage moved from %d to %d", h.templateReferences(t, duplicate.ID), usage, h.fileUsage(t, tutor))
	}
	if h.countRows(t, "SELECT count(id) FROM attachments WHERE uploader_id = $1::uuid", tutor.String()) != attachments {
		t.Fatal("duplicating copied a file")
	}
	if !strings.Contains(duplicate.Blocks[0].Images[0].URL, "/api/v1/files/"+image+"?t=") {
		t.Fatalf("the duplicate's image address is %q", duplicate.Blocks[0].Images[0].URL)
	}
}

func TestTemplateDeleteReleasesFiles(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	kept, dropped := h.pendingPromptImage(t, tutor, 72, 1), h.pendingPromptImage(t, tutor, 72, 2)
	material := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 72, 3, 30), "Worksheet", nil)
	template := h.createTemplate(t, token, "Quiz", nil, imageQuestion(blockID(12, 1), kept), imageQuestion(blockID(12, 2), dropped),
		materialBlock(blockID(12, 3), material.ID))

	h.saveTemplate(t, token, template.ID, saveJSON(t, "Quiz", 1, saveA, imageQuestion(blockID(12, 1), kept), materialBlock(blockID(12, 3), material.ID)))
	afterSave := h.fileUsage(t, tutor)
	deleted := h.deleteTemplate(t, token, template.ID)
	repeated := h.deleteTemplate(t, token, template.ID)
	read := h.templateAnswer(t, token, template.ID, "")

	if h.uploadState(t, dropped) != string(domain.AttachmentDeleted) || afterSave != 40 {
		t.Fatalf("the removed image is %s and the usage %d", h.uploadState(t, dropped), afterSave)
	}
	h.mustAnswer(t, deleted, http.StatusNoContent)
	h.mustAnswer(t, repeated, http.StatusNoContent)
	if read.status != http.StatusNotFound {
		t.Fatalf("a deleted template answered %s", describe(read))
	}
	if h.uploadState(t, kept) != string(domain.AttachmentDeleted) || h.uploadState(t, material.AttachmentID) != string(domain.AttachmentBound) {
		t.Fatalf("the image is %s and the material file %s", h.uploadState(t, kept), h.uploadState(t, material.AttachmentID))
	}
	if h.fileUsage(t, tutor) != 30 || h.templateReferences(t, template.ID) != 0 {
		t.Fatal("the deleted template kept a charge or a reference")
	}
	queued := "SELECT count(id) FROM outbox WHERE kind = 'storage_delete' AND idempotency_key = 'storage_delete:' || $1"
	if h.countRows(t, queued, kept) != 1 || h.countRows(t, queued, dropped) != 1 || h.countRows(t, queued, material.AttachmentID) != 0 {
		t.Fatal("the released images were not queued for deletion")
	}
	if h.countRows(t, "SELECT count(id) FROM templates WHERE id = $1::uuid AND blocks = '[]'::jsonb AND blocks_bytes = 0 AND folder_id IS NULL", template.ID) != 1 {
		t.Fatal("the deleted template kept its blocks")
	}
}

func TestTemplateBodyLimit(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	template := h.createTemplate(t, token, "Quiz", nil)
	blocks := make([]templateBlockJSON, 0, domain.TemplateMaxBlocks)
	for index := range domain.TemplateMaxBlocks {
		blocks = append(blocks, instructionBlock(blockID(13, byte(index)), strings.Repeat("я", domain.TemplateTextMaxRunes)))
	}

	answer := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Quiz", 1, saveA, blocks...))

	if answer.status != http.StatusRequestEntityTooLarge || code(t, answer) != "payload_too_large" {
		t.Fatalf("a body over 1 MiB answered %s", describe(answer))
	}
}

func TestTemplateStudentViewAllowlist(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	image := h.pendingPromptImage(t, tutor, 73, 1)
	material := h.createMaterial(t, token, h.pendingLibraryPicture(t, tutor, 73, 2), "Map", nil)
	sentinels := []string{"acceptedsentinel", "4242.125", "0.000777", "explanationsentinel", "all_or_nothing", "partial"}
	choices := []templateOptionJSON{{ID: optionID(14, 3), Text: "Left", Correct: flag(true)}, {ID: optionID(14, 4), Text: "Right", Correct: flag(true)}}
	blocks := []templateBlockJSON{
		instructionBlock(blockID(14, 1), "Read the text"),
		materialBlock(blockID(14, 2), material.ID),
		imageQuestion(blockID(14, 3), image),
		{ID: blockID(14, 4), Type: "multiple_choice", Prompt: text("Pick"), Options: &choices, Scoring: text("partial"), Points: points(3), Explanation: text(sentinels[3])},
		{ID: blockID(14, 5), Type: "short_text", Prompt: text("Say"), AcceptedAnswers: textList(sentinels[0]), Points: points(1), Explanation: text(sentinels[3])},
		{ID: blockID(14, 6), Type: "short_number", Prompt: text("Count"), Value: text(sentinels[1]), Tolerance: text(sentinels[2]), Points: points(1)},
		{ID: blockID(14, 7), Type: "long_text", Prompt: text("Write"), MaxLength: points(500), Points: points(4)},
		{ID: blockID(14, 8), Type: "file", Prompt: text("Upload"), AllowedKinds: textList("pdf"), MaxFiles: points(2), Points: points(4)},
	}
	template := h.createTemplate(t, token, "Quiz", nil, blocks...)

	answer := h.templateAnswer(t, token, template.ID, "/student-view")

	h.mustAnswer(t, answer, http.StatusOK)
	if answer.headers.Get("Cache-Control") != "no-store" {
		t.Fatal("the student view is cacheable")
	}
	for _, sentinel := range sentinels {
		if strings.Contains(answer.body, sentinel) {
			t.Fatalf("the student view carries %q", sentinel)
		}
	}
	top := objectKeys(t, json.RawMessage(answer.body), []string{"title", "blocks"})
	var studentBlocks []json.RawMessage
	if err := json.Unmarshal(top["blocks"], &studentBlocks); err != nil || len(studentBlocks) != len(blocks) {
		t.Fatalf("the student view holds %d blocks: %v", len(studentBlocks), err)
	}
	question := []string{"id", "type", "prompt", "images"}
	allowed := map[string][]string{
		"instruction":     {"id", "type", "text"},
		"material":        {"id", "type", "material"},
		"single_choice":   append(slices.Clone(question), "options"),
		"multiple_choice": append(slices.Clone(question), "options"),
		"short_text":      question,
		"short_number":    question,
		"long_text":       append(slices.Clone(question), "max_length"),
		"file":            append(slices.Clone(question), "allowed_kinds", "max_files"),
	}
	fileKeys := []string{"filename", "media_type", "size_bytes", "width", "height", "url", "expires_at"}
	for index, raw := range studentBlocks {
		blockType := blocks[index].Type
		fields := objectKeys(t, raw, allowed[blockType])
		if materialEntry, found := fields["material"]; found {
			objectKeys(t, materialEntry, append(slices.Clone(fileKeys), "title"))
		}
		for _, entry := range arrayItems(t, fields["images"]) {
			objectKeys(t, entry, fileKeys)
		}
		for _, entry := range arrayItems(t, fields["options"]) {
			objectKeys(t, entry, []string{"id", "text"})
		}
	}
	if len(arrayItems(t, objectKeys(t, studentBlocks[2], allowed["single_choice"])["images"])) != 1 {
		t.Fatal("the prompt image is missing from the student view")
	}
}

func objectKeys(t *testing.T, raw json.RawMessage, allowed []string) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("not an object: %s", raw)
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	want := slices.Clone(allowed)
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		t.Fatalf("the object holds %v, want exactly %v", keys, want)
	}
	return object
}

func arrayItems(t *testing.T, raw json.RawMessage) []json.RawMessage {
	t.Helper()
	if raw == nil {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("not an array: %s", raw)
	}
	return items
}

func TestTemplateSavesPerSecond(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	template := h.createTemplate(t, token, "Quiz", nil)

	first := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Quiz", 1, saveA))
	second := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Quiz", 2, saveB))
	third := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Quiz", 3, blockID(15, 1)))

	h.mustAnswer(t, first, http.StatusOK)
	h.mustAnswer(t, second, http.StatusOK)
	if third.status != http.StatusTooManyRequests || code(t, third) != "rate_limited" || third.headers.Get("Retry-After") != "1" {
		t.Fatalf("the third save within a second answered %s", describe(third))
	}
	if h.countLogs(`"msg":"library.rate_limited"`, `"limit":"template_saves"`) != 1 {
		t.Fatal("the refused save was not recorded")
	}
}

func TestTemplateAutosaveBeyondHourlyWriteLimit(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	template := h.createTemplate(t, token, "Quiz", nil)
	start := h.now(t)
	const saves = 700

	for index := range saves {
		h.at(t, start.Add(time.Duration(index)*500*time.Millisecond))
		answer := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Quiz", int64(index+1), fmt.Sprintf("%08x-2222-4222-a222-222222222222", index)))
		if answer.status != http.StatusOK {
			t.Fatalf("save %d answered %s", index+1, describe(answer))
		}
	}
	libraryWrites := h.libraryWriteCount(t, tutor)
	h.setLibraryWrites(t, tutor, domain.LibraryWritesPerHour-1)
	last := h.createFolderAnswer(t, token, "Unit 1", nil)
	refused := h.createFolderAnswer(t, token, "Unit 2", nil)

	if libraryWrites != 1 {
		t.Fatalf("the autosaves counted %d library writes", libraryWrites-1)
	}
	h.mustAnswer(t, last, http.StatusCreated)
	if refused.status != http.StatusTooManyRequests {
		t.Fatalf("the 601st library write answered %s", describe(refused))
	}
}

func TestTemplateSaveHourlyLimit(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	template := h.createTemplate(t, token, "Quiz", nil)
	h.fillTemplateSaves(t, tutor, domain.TemplateSavesPerHour)

	refused := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Quiz", 1, saveA))

	if refused.status != http.StatusTooManyRequests || refused.headers.Get("Retry-After") == "" {
		t.Fatalf("the 3,601st save answered %s", describe(refused))
	}
}

func TestTemplateCreateIdempotency(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	first := h.createTemplateAnswer(t, token, "key-1", newTemplateJSON(t, "Quiz", nil))
	h.mustAnswer(t, first, http.StatusCreated)
	created := decodeLibrary[templateBody](t, first)
	h.setLibraryWrites(t, tutor, domain.LibraryWritesPerHour)

	replayed := h.createTemplateAnswer(t, token, "key-1", newTemplateJSON(t, "Other", nil, instructionBlock(blockID(16, 1), "x")))
	malformed := h.createTemplateAnswer(t, token, "key 1", newTemplateJSON(t, "Quiz", nil))
	h.clearLibraryWrites(t, tutor)
	h.mustAnswer(t, h.deleteTemplate(t, token, created.ID), http.StatusNoContent)
	afterDelete := h.createTemplateAnswer(t, token, "key-1", newTemplateJSON(t, "Quiz", nil))

	h.mustAnswer(t, replayed, http.StatusCreated)
	if again := decodeLibrary[templateBody](t, replayed); again.ID != created.ID || again.Title != "Quiz" {
		t.Fatalf("the replay answered %+v", again)
	}
	if malformed.status != http.StatusUnprocessableEntity {
		t.Fatalf("a malformed key answered %s", describe(malformed))
	}
	if afterDelete.status != http.StatusNotFound {
		t.Fatalf("a key of a deleted template answered %s", describe(afterDelete))
	}
	if h.countRows(t, "SELECT count(id) FROM templates WHERE tutor_id = $1::uuid", tutor.String()) != 1 {
		t.Fatal("a replay created a template")
	}
}

func TestTemplateMove(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	folder := h.createFolder(t, token, "Unit", nil)
	template := h.createTemplate(t, token, "Quiz", nil)

	moved := h.moveTemplate(t, token, template.ID, &folder.ID, 1)
	stale := h.moveTemplate(t, token, template.ID, nil, 1)
	same := h.moveTemplate(t, token, template.ID, &folder.ID, 2)
	randomID := randomCardID
	unknownFolder := h.moveTemplate(t, token, template.ID, &randomID, 2)

	h.mustAnswer(t, moved, http.StatusOK)
	if body := decodeLibrary[templateBody](t, moved); body.Version != 2 || body.FolderID == nil || *body.FolderID != folder.ID {
		t.Fatalf("the move answered %+v", body)
	}
	if stale.status != http.StatusConflict || code(t, stale) != "conflict" {
		t.Fatalf("a stale move answered %s", describe(stale))
	}
	h.mustAnswer(t, same, http.StatusOK)
	if body := decodeLibrary[templateBody](t, same); body.Version != 2 {
		t.Fatalf("a move to the current folder answered version %d", body.Version)
	}
	if unknownFolder.status != http.StatusNotFound {
		t.Fatalf("a move to an unknown folder answered %s", describe(unknownFolder))
	}
	if listed := h.libraryPage(t, token, url.Values{"folder": {folder.ID}}); len(listed.Items) != 1 || listed.Items[0].Kind != "template" {
		t.Fatalf("the folder lists %+v", listed.Items)
	}
	if conflict := h.deleteFolder(t, token, folder.ID); conflict.status != http.StatusConflict {
		t.Fatalf("a folder holding a template answered %s", describe(conflict))
	}
}

func TestTemplateJSONBudget(t *testing.T) {
	t.Parallel()
	h := newLibraryHarnessWithCaps(t, domain.LibraryCaps{TemplateBlockBytes: 400})
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	body := strings.Repeat("a", 250)
	first := h.createTemplate(t, token, "First", nil, instructionBlock(blockID(17, 1), body))

	refused := h.createTemplateAnswer(t, token, "", newTemplateJSON(t, "Second", nil, instructionBlock(blockID(17, 2), body)))
	refusedSave := h.saveTemplateAnswer(t, token, first.ID, saveJSON(t, "First", 1, saveA, instructionBlock(blockID(17, 1), body+body)))
	h.mustAnswer(t, h.deleteTemplate(t, token, first.ID), http.StatusNoContent)
	accepted := h.createTemplateAnswer(t, token, "", newTemplateJSON(t, "Second", nil, instructionBlock(blockID(17, 2), body)))

	for name, answer := range map[string]response{"a create": refused, "a save": refusedSave} {
		if path, field := fieldOf(t, answer); path != "body" || field != "too_many" {
			t.Errorf("%s past the budget answered %s at %s", name, field, path)
		}
	}
	h.mustAnswer(t, accepted, http.StatusCreated)
	if h.countRows(t, "SELECT count(id) FROM templates WHERE id = $1::uuid AND blocks = '[]'::jsonb AND blocks_bytes = 0", first.ID) != 1 {
		t.Fatal("the deleted template kept its blocks")
	}
}

func TestTemplateCaps(t *testing.T) {
	t.Parallel()
	h := newLibraryHarnessWithCaps(t, domain.LibraryCaps{Templates: 1, Rows: 2})
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	first := h.createTemplate(t, token, "First", nil)

	second := h.createTemplateAnswer(t, token, "", newTemplateJSON(t, "Second", nil))
	duplicate := h.duplicateTemplate(t, token, first.ID, "Copy", nil)
	h.mustAnswer(t, h.deleteTemplate(t, token, first.ID), http.StatusNoContent)
	afterDelete := h.createTemplate(t, token, "Third", nil)
	rowsFull := h.createMaterialAnswer(t, token, h.pendingLibraryFile(t, tutor, 74, 1, 10), "Worksheet", nil)

	for name, answer := range map[string]response{"a create": second, "a duplicate": duplicate, "a material past the rows": rowsFull} {
		if path, field := fieldOf(t, answer); path != "body" || field != "too_many" {
			t.Errorf("%s answered %s at %s", name, field, path)
		}
	}
	if afterDelete.ID == "" {
		t.Fatal("a deleted template still counted toward the cap")
	}
}

func TestLibrarySearchFindsTemplates(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	folder := h.createFolder(t, token, "Unit", nil)
	found := h.createTemplate(t, token, "Ёлка quiz", &folder.ID, singleChoiceBlock(blockID(18, 1), "Q", optionID(18, 1), optionID(18, 2)))
	material := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 75, 1, 10), "Ёлки worksheet", nil)
	gone := h.createTemplate(t, token, "Ёлка old", nil)
	h.createTemplate(t, token, "Other", nil)
	h.mustAnswer(t, h.deleteTemplate(t, token, gone.ID), http.StatusNoContent)

	search := h.libraryPage(t, token, url.Values{"q": {"елк"}})
	root := h.libraryPage(t, token, url.Values{})

	if !slices.Equal(itemIDs(search.Items), []string{found.ID, material.ID}) {
		t.Fatalf("the search found %v", itemIDs(search.Items))
	}
	if search.Items[0].Kind != "template" || len(search.Items[0].Path) != 1 || search.Items[0].Path[0].ID != folder.ID {
		t.Fatalf("the template's search item is %+v", search.Items[0])
	}
	if search.TemplateCount != 2 || root.TemplateCount != 2 {
		t.Fatalf("the template count is %d", search.TemplateCount)
	}
	if len(root.Items) != 2 || root.Items[0].Kind != "template" || root.Items[0].Title != "Other" {
		t.Fatalf("the root lists %+v", root.Items)
	}
}

func TestLibraryKeysetPaginationAcrossKinds(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	var want []string
	for index, title := range []string{"a", "b", "c", "d", "e"} {
		want = append(want, h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 76, byte(index), 10), title, nil).ID)
		want = append(want, h.createTemplate(t, token, title, nil).ID)
	}

	var got []string
	query := url.Values{"limit": {"3"}}
	for range 10 {
		page := h.libraryPage(t, token, query)
		got = append(got, itemIDs(page.Items)...)
		if page.NextCursor == nil {
			break
		}
		query.Set("cursor", *page.NextCursor)
	}

	if !slices.Equal(got, want) {
		t.Fatalf("the pages listed %v, want %v", got, want)
	}
}

func TestTemplateDownloadsThroughTheirTemplate(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, newcomerEmail)
	upload := h.libraryUpload(t, token, domain.ContextPromptImage, "prompt.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, upload, http.StatusCreated)
	image := uploadedID(t, upload)
	template := h.createTemplate(t, token, "Quiz", nil, imageQuestion(blockID(19, 1), image))
	foreign := h.createTemplate(t, otherToken, "Foreign", nil)

	served := h.fetch(t, template.Blocks[0].Images[0].URL, nil)
	minted, _ := h.addressOf(t, token, image, "?ref=template:"+template.ID+"&disposition=inline")
	byOther, _ := h.addressOf(t, otherToken, image, "?ref=template:"+template.ID)
	withForeign, _ := h.addressOf(t, token, image, "?ref=template:"+foreign.ID)
	random, _ := h.addressOf(t, token, fixtureID(77, 1), "?ref=template:"+template.ID)
	h.mustAnswer(t, h.deleteTemplate(t, token, template.ID), http.StatusNoContent)
	afterDelete := h.fetch(t, template.Blocks[0].Images[0].URL, nil)

	if served.status != http.StatusOK || !strings.HasPrefix(served.headers.Get("Content-Disposition"), "inline") {
		t.Fatalf("the prompt image answered %s", describe(served))
	}
	h.mustAnswer(t, minted, http.StatusOK)
	for name, refused := range map[string]response{"another tutor": byOther, "another tutor's template": withForeign} {
		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, random) {
			t.Errorf("%s answered %s", name, describe(refused))
		}
	}
	if afterDelete.status != http.StatusNotFound {
		t.Fatalf("a deleted template's address answered %s", describe(afterDelete))
	}
}

func TestTemplateRecordsCarryNoContent(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	const titleSentinel, promptSentinel, answerSentinel, explanationSentinel, keySentinel, copySentinel = "Title sentinel",
		"Prompt sentinel", "answersentinel", "Explanation sentinel", "key-sentinel", "Copy sentinel"
	folder := h.createFolder(t, token, "Unit", nil)
	image := h.pendingPromptImage(t, tutor, 78, 1)
	question := templateBlockJSON{
		ID: blockID(20, 1), Type: "short_text", Prompt: text(promptSentinel), ImageIDs: textList(image), AcceptedAnswers: textList(answerSentinel),
		Points: points(1), Explanation: text(explanationSentinel),
	}
	created := h.createTemplateAnswer(t, token, keySentinel, newTemplateJSON(t, titleSentinel, nil, question))
	h.mustAnswer(t, created, http.StatusCreated)
	template := decodeLibrary[templateBody](t, created)
	h.saveTemplate(t, token, template.ID, saveJSON(t, titleSentinel, 1, saveA, question))
	h.mustAnswer(t, h.moveTemplate(t, token, template.ID, &folder.ID, 2), http.StatusOK)
	duplicate := h.duplicateTemplate(t, token, template.ID, copySentinel, nil)
	h.mustAnswer(t, duplicate, http.StatusCreated)
	h.mustAnswer(t, h.templateAnswer(t, token, template.ID, "/student-view"), http.StatusOK)
	h.mustAnswer(t, h.deleteTemplate(t, token, template.ID), http.StatusNoContent)
	records := h.logs.String()

	for _, value := range []string{titleSentinel, promptSentinel, answerSentinel, explanationSentinel, keySentinel, copySentinel, "prompt.png", "/api/v1/files/", "?t="} {
		if strings.Contains(records, value) {
			t.Fatalf("a record carries %q", value)
		}
	}
	copied := decodeLibrary[templateBody](t, duplicate)
	for event, fragments := range map[string][]string{
		"library.template.created":    {template.ID, tutor.String()},
		"library.template.saved":      {template.ID, `"block_count":1`, `"ready":true`, `"references_added":0`},
		"library.template.moved":      {template.ID},
		"library.template.duplicated": {copied.ID, template.ID},
		"library.template.deleted":    {template.ID},
	} {
		if h.countLogs(append([]string{`"msg":"` + event + `"`}, fragments...)...) != 1 {
			t.Errorf("no single %s record with %v", event, fragments)
		}
	}
	if h.countLogs(`"msg":"file.reference.added"`, image, `"ref_kind":"template"`) != 2 {
		t.Error("the template references were not recorded")
	}
}

func TestTutorPurgeTemplates(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	upload := h.libraryUpload(t, token, domain.ContextPromptImage, "prompt.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, upload, http.StatusCreated)
	file := h.libraryUpload(t, token, domain.ContextLibrary, "worksheet.pdf", pdfFile(64))
	material := h.createMaterial(t, token, uploadedID(t, file), "Worksheet", nil)
	h.mustAnswer(t, h.deleteMaterial(t, token, material.ID), http.StatusNoContent)
	folder := h.createFolder(t, token, "Unit", nil)
	template := h.createTemplate(t, token, "Quiz", &folder.ID, imageQuestion(blockID(21, 1), uploadedID(t, upload)))
	h.saveTemplate(t, token, template.ID, saveJSON(t, "Quiz", 1, saveA, imageQuestion(blockID(21, 1), uploadedID(t, upload))))
	gone := h.createTemplate(t, token, "Gone", nil)
	h.mustAnswer(t, h.deleteTemplate(t, token, gone.ID), http.StatusNoContent)
	uploads := h.uploadsIn(t, tutor, libraryContexts...)
	h.mustAnswer(t, h.deleteMe(t, token, accountPassword), http.StatusNoContent)

	h.at(t, authStart.Add(domain.DeletedAccountRetention+time.Hour))
	h.tick(t)
	h.tick(t)

	for table, query := range map[string]string{
		"templates":  "SELECT count(id) FROM templates WHERE tutor_id = $1::uuid",
		"charges":    "SELECT count(attachment_id) FROM quota_charges WHERE tutor_id = $1::uuid",
		"references": "SELECT count(attachment_id) FROM attachment_references WHERE ref_kind = 'template' AND ref_id = $1::uuid",
	} {
		argument := tutor.String()
		if table == "references" {
			argument = template.ID
		}
		if count := h.countRows(t, query, argument); count != 0 {
			t.Errorf("%d %s survived the purge", count, table)
		}
	}
	for _, stored := range uploads {
		if _, kept := h.storedObject(t, stored.Key); kept || h.uploadState(t, stored.ID) != "gone" {
			t.Errorf("the purged tutor's file %s survived", stored.ID)
		}
	}
}

func TestTemplateSaveIdentifiersInOtherSpellings(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	template := h.createTemplate(t, token, "Quiz", nil)

	braced := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Quiz", 1, "{"+saveA+"}"))
	compact := h.moveTemplate(t, token, template.ID, text(strings.ReplaceAll(randomCardID, "-", "")), 1)

	for name, answer := range map[string]response{"a braced save identifier": braced, "a compact folder": compact} {
		if answer.status != http.StatusUnprocessableEntity || code(t, answer) != "validation_failed" {
			t.Errorf("%s answered %s", name, describe(answer))
		}
	}
}

func TestTemplatePromptImagesAndQuota(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	first, second, expired := h.pendingPromptImage(t, tutor, 79, 1), h.pendingPromptImage(t, tutor, 79, 2), h.pendingPromptImage(t, tutor, 79, 3)
	h.exec(t, "UPDATE attachments SET expires_at = created_at WHERE id = $1::uuid", expired)
	h.exec(t, "UPDATE tutor_profiles SET file_quota_bytes = 10 WHERE user_id = $1::uuid", tutor.String())
	template := h.createTemplate(t, token, "Quiz", nil, imageQuestion(blockID(22, 1), first))

	both := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Quiz", 1, saveA, imageQuestion(blockID(22, 1), first), imageQuestion(blockID(22, 2), second)))
	swapped := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Quiz", 1, saveB, imageQuestion(blockID(22, 2), second)))
	h.nextSecond(t)
	stale := h.saveTemplateAnswer(t, token, template.ID, saveJSON(t, "Quiz", 2, blockID(22, 9), imageQuestion(blockID(22, 3), expired)))

	if both.status != http.StatusInsufficientStorage || code(t, both) != "quota_exceeded" {
		t.Fatalf("a save past the quota answered %s", describe(both))
	}
	h.mustAnswer(t, swapped, http.StatusOK)
	if h.fileUsage(t, tutor) != 10 || h.uploadState(t, first) != string(domain.AttachmentDeleted) {
		t.Fatalf("the swap left the usage at %d", h.fileUsage(t, tutor))
	}
	if stale.status != http.StatusNotFound {
		t.Fatalf("an expired prompt image answered %s", describe(stale))
	}
}

func TestTemplateKeepsADeletedMaterialItAlreadyShows(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	material := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 80, 1, 10), "Worksheet", nil)
	template := h.createTemplate(t, token, "Quiz", nil, materialBlock(blockID(23, 1), material.ID))
	h.mustAnswer(t, h.deleteMaterial(t, token, material.ID), http.StatusNoContent)

	saved := h.saveTemplate(t, token, template.ID, saveJSON(t, "Quiz", 1, saveA, materialBlock(blockID(23, 1), material.ID), instructionBlock(blockID(23, 2), "Read")))

	if len(saved.Blocks) != 2 || saved.Blocks[0].Material == nil || saved.Blocks[0].Material.Deleted == nil || !*saved.Blocks[0].Material.Deleted {
		t.Fatalf("the kept material block is %+v", saved.Blocks[0])
	}
	if h.uploadState(t, material.AttachmentID) != string(domain.AttachmentBound) || h.fileUsage(t, tutor) != 10 {
		t.Fatal("the template no longer holds the deleted material's file")
	}
}

func TestDeletedTemplateAddressesStopWhileItsDuplicateServes(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	upload := h.libraryUpload(t, token, domain.ContextPromptImage, "prompt.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, upload, http.StatusCreated)
	image := uploadedID(t, upload)
	original := h.createTemplate(t, token, "Quiz", nil, imageQuestion(blockID(24, 1), image))
	copied := h.duplicateTemplate(t, token, original.ID, "Copy", nil)
	h.mustAnswer(t, copied, http.StatusCreated)
	duplicate := decodeLibrary[templateBody](t, copied)
	h.mustAnswer(t, h.deleteTemplate(t, token, original.ID), http.StatusNoContent)

	stopped := h.fetch(t, original.Blocks[0].Images[0].URL, nil)
	minted, _ := h.addressOf(t, token, image, "?ref=template:"+original.ID)
	served := h.fetch(t, duplicate.Blocks[0].Images[0].URL, nil)

	if stopped.status != http.StatusNotFound || minted.status != http.StatusNotFound {
		t.Fatalf("the deleted template's address answered %s and minting %s", describe(stopped), describe(minted))
	}
	if served.status != http.StatusOK || h.uploadState(t, image) != string(domain.AttachmentBound) {
		t.Fatalf("the duplicate's address answered %s", describe(served))
	}
}
