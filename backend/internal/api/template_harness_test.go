package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const libraryTemplatesRoute = libraryRoot + "/templates"

type templateOptionJSON struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Correct *bool  `json:"correct,omitempty"`
}

type templateBlockJSON struct {
	ID              string                `json:"id"`
	Type            string                `json:"type"`
	Text            *string               `json:"text,omitempty"`
	MaterialID      *string               `json:"material_id,omitempty"`
	Prompt          *string               `json:"prompt,omitempty"`
	ImageIDs        *[]string             `json:"image_ids,omitempty"`
	Options         *[]templateOptionJSON `json:"options,omitempty"`
	Scoring         *string               `json:"scoring,omitempty"`
	AcceptedAnswers *[]string             `json:"accepted_answers,omitempty"`
	Value           *string               `json:"value,omitempty"`
	Tolerance       *string               `json:"tolerance,omitempty"`
	MaxLength       *int32                `json:"max_length,omitempty"`
	AllowedKinds    *[]string             `json:"allowed_kinds,omitempty"`
	MaxFiles        *int32                `json:"max_files,omitempty"`
	Points          *int32                `json:"points,omitempty"`
	Explanation     *string               `json:"explanation,omitempty"`
}

type templateFileBody struct {
	AttachmentID string  `json:"attachment_id"`
	Width        *int32  `json:"width"`
	URL          string  `json:"url"`
	Title        *string `json:"title"`
	Deleted      *bool   `json:"deleted"`
}

type templateBlockBody struct {
	templateBlockJSON
	Material *templateFileBody  `json:"material"`
	Images   []templateFileBody `json:"images"`
}

type templateProblemBody struct {
	BlockID *string `json:"block_id"`
	Code    string  `json:"code"`
}

type templateBody struct {
	ID            string                `json:"id"`
	Title         string                `json:"title"`
	FolderID      *string               `json:"folder_id"`
	Blocks        []templateBlockBody   `json:"blocks"`
	Version       int64                 `json:"version"`
	Ready         bool                  `json:"ready"`
	Problems      []templateProblemBody `json:"problems"`
	QuestionCount int32                 `json:"question_count"`
	MaxPoints     int32                 `json:"max_points"`
}

func blockID(group byte, n byte) string {
	return fmt.Sprintf("%08x-0000-4000-8000-%012x", uint32(group)<<8|uint32(n), uint64(group)<<8|uint64(n))
}

func optionID(group byte, n byte) string {
	return fmt.Sprintf("%08x-1111-4111-9111-%012x", uint32(group)<<8|uint32(n), uint64(group)<<8|uint64(n))
}

func text(value string) *string {
	return &value
}

func points(value int32) *int32 {
	return &value
}

func flag(value bool) *bool {
	return &value
}

func textList(values ...string) *[]string {
	return &values
}

func instructionBlock(id string, body string) templateBlockJSON {
	return templateBlockJSON{ID: id, Type: string(domain.BlockInstruction), Text: text(body)}
}

func singleChoiceBlock(id string, prompt string, first string, second string) templateBlockJSON {
	options := []templateOptionJSON{{ID: first, Text: "Paris", Correct: flag(true)}, {ID: second, Text: "Rome", Correct: flag(false)}}
	return templateBlockJSON{ID: id, Type: string(domain.BlockSingleChoice), Prompt: text(prompt), Options: &options, Points: points(2)}
}

func materialBlock(id string, material string) templateBlockJSON {
	return templateBlockJSON{ID: id, Type: string(domain.BlockMaterial), MaterialID: text(material)}
}

func blocksJSON(t *testing.T, blocks ...templateBlockJSON) string {
	t.Helper()
	if blocks == nil {
		blocks = []templateBlockJSON{}
	}
	encoded, err := json.Marshal(blocks)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func newTemplateJSON(t *testing.T, title string, folder *string, blocks ...templateBlockJSON) string {
	t.Helper()
	return `{"title":` + jsonText(title) + `,"folder_id":` + optionalJSON(folder) + `,"blocks":` + blocksJSON(t, blocks...) + `}`
}

func saveJSON(t *testing.T, title string, version int64, save string, blocks ...templateBlockJSON) string {
	t.Helper()
	return `{"title":` + jsonText(title) + `,"version":` + fmt.Sprint(version) + `,"save_id":` + jsonText(save) + `,"blocks":` +
		blocksJSON(t, blocks...) + `}`
}

func (h *authHarness) createTemplateAnswer(t *testing.T, token string, key string, body string) response {
	t.Helper()
	headers := jsonCall(token)
	if key != "" {
		headers["Idempotency-Key"] = key
	}
	return h.send(t, http.MethodPost, libraryTemplatesRoute, body, headers)
}

func (h *authHarness) createTemplate(t *testing.T, token string, title string, folder *string, blocks ...templateBlockJSON) templateBody {
	t.Helper()
	answer := h.createTemplateAnswer(t, token, "", newTemplateJSON(t, title, folder, blocks...))
	h.mustAnswer(t, answer, http.StatusCreated)
	return decodeLibrary[templateBody](t, answer)
}

func (h *authHarness) templateAnswer(t *testing.T, token string, id string, suffix string) response {
	t.Helper()
	return h.libraryCall(t, token, http.MethodGet, libraryTemplatesRoute+"/"+id+suffix, "")
}

func (h *authHarness) saveTemplateAnswer(t *testing.T, token string, id string, body string) response {
	t.Helper()
	return h.libraryCall(t, token, http.MethodPut, libraryTemplatesRoute+"/"+id, body)
}

func (h *authHarness) saveTemplate(t *testing.T, token string, id string, body string) templateBody {
	t.Helper()
	answer := h.saveTemplateAnswer(t, token, id, body)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[templateBody](t, answer)
}

func (h *authHarness) moveTemplate(t *testing.T, token string, id string, folder *string, version int64) response {
	t.Helper()
	return h.libraryCall(t, token, http.MethodPatch, libraryTemplatesRoute+"/"+id, `{"folder_id":`+optionalJSON(folder)+`,"version":`+fmt.Sprint(version)+`}`)
}

func (h *authHarness) duplicateTemplate(t *testing.T, token string, id string, title string, folder *string) response {
	t.Helper()
	return h.libraryCall(t, token, http.MethodPost, libraryTemplatesRoute+"/"+id+"/duplicate", `{"title":`+jsonText(title)+`,"folder_id":`+optionalJSON(folder)+`}`)
}

func (h *authHarness) deleteTemplate(t *testing.T, token string, id string) response {
	t.Helper()
	return h.libraryCall(t, token, http.MethodDelete, libraryTemplatesRoute+"/"+id, "")
}

func (h *authHarness) pendingPromptImage(t *testing.T, tutor domain.UserID, group byte, n byte) string {
	t.Helper()
	id := fixtureID(group, n)
	h.insertPendingLibrary(t, id, tutor, domain.ContextPromptImage, 10, h.now(t))
	if _, err := h.database.Pool.Exec(t.Context(), "UPDATE attachments SET media_type = 'image/png', filename = 'prompt.png', width = $2, height = $2 WHERE id = $1::uuid",
		id, libraryPictureDimension); err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *authHarness) templateReferences(t *testing.T, template string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(attachment_id) FROM attachment_references WHERE ref_kind = 'template' AND ref_id = $1::uuid", template)
}

func (h *authHarness) fillTemplateSaves(t *testing.T, tutor domain.UserID, count int) {
	t.Helper()
	window, _ := domain.FixedWindow(h.now(t), domain.TemplateSaveHourWindow)
	subject := h.keys.ThrottleSubject(string(domain.PurposeTemplateSavesHour), tutor.String())
	if _, err := h.database.Pool.Exec(t.Context(), "INSERT INTO rate_limit_counters (purpose, subject, window_start, count) VALUES ($1, $2, $3, $4)",
		string(domain.PurposeTemplateSavesHour), subject, window, count); err != nil {
		t.Fatal(err)
	}
}

func anotherTutorsTemplate(method string, suffix string, body string) denialCase {
	return func(t *testing.T, h *authHarness) (response, response) {
		t.Helper()
		owner, ownerToken, intruder := libraryOwners(t, h)
		template := h.createTemplate(t, ownerToken, "Quiz", nil)
		deleted := h.createTemplate(t, ownerToken, "Deleted", nil)
		h.mustAnswer(t, h.deleteTemplate(t, ownerToken, deleted.ID), http.StatusNoContent)
		outOfScope := h.libraryCall(t, intruder, method, libraryTemplatesRoute+"/"+template.ID+suffix, body)
		random := h.libraryCall(t, intruder, method, libraryTemplatesRoute+"/"+randomCardID+suffix, body)
		if method != http.MethodDelete {
			deletedAnswer := h.libraryCall(t, ownerToken, method, libraryTemplatesRoute+"/"+deleted.ID+suffix, body)
			ownRandom := h.libraryCall(t, ownerToken, method, libraryTemplatesRoute+"/"+randomCardID+suffix, body)
			if deletedAnswer.status != http.StatusNotFound || refusal(t, deletedAnswer) != refusal(t, ownRandom) {
				t.Fatalf("a deleted template answered %s", describe(deletedAnswer))
			}
		}
		if h.countRows(t, "SELECT count(id) FROM templates WHERE tutor_id = $1::uuid AND deleted_at IS NULL AND version = 1", owner.String()) != 1 {
			t.Fatal("another tutor changed a template")
		}
		return outOfScope, random
	}
}

func anotherTutorsTemplateFolder(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	_, ownerToken, intruder := libraryOwners(t, h)
	folder := h.createFolder(t, ownerToken, "Unit 1", nil)
	randomID := randomCardID
	outOfScope := h.createTemplateAnswer(t, intruder, "", newTemplateJSON(t, "Quiz", &folder.ID))
	random := h.createTemplateAnswer(t, intruder, "", newTemplateJSON(t, "Quiz", &randomID))
	return outOfScope, random
}

func (h *authHarness) libraryWriteCount(t *testing.T, tutor domain.UserID) int64 {
	t.Helper()
	var count int64
	subject := h.keys.ThrottleSubject(string(domain.PurposeLibraryWrites), tutor.String())
	if err := h.database.Pool.QueryRow(t.Context(), "SELECT COALESCE(sum(count), 0)::bigint FROM rate_limit_counters WHERE purpose = $1 AND subject = $2",
		string(domain.PurposeLibraryWrites), subject).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (h *authHarness) clearLibraryWrites(t *testing.T, tutor domain.UserID) {
	t.Helper()
	subject := h.keys.ThrottleSubject(string(domain.PurposeLibraryWrites), tutor.String())
	if _, err := h.database.Pool.Exec(t.Context(), "DELETE FROM rate_limit_counters WHERE purpose = $1 AND subject = $2",
		string(domain.PurposeLibraryWrites), subject); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) setLibraryWrites(t *testing.T, tutor domain.UserID, count int) {
	t.Helper()
	window, _ := domain.FixedWindow(h.now(t), domain.LibraryWriteWindow)
	subject := h.keys.ThrottleSubject(string(domain.PurposeLibraryWrites), tutor.String())
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO rate_limit_counters (purpose, subject, window_start, count) VALUES ($1, $2, $3, $4)
ON CONFLICT (purpose, subject, window_start) DO UPDATE SET count = excluded.count`, string(domain.PurposeLibraryWrites), subject, window, count); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) nextSecond(t *testing.T) {
	t.Helper()
	h.at(t, h.now(t).Add(time.Second))
}
