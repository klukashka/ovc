package api_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"testing"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/library"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

const (
	libraryRoot             = "/api/v1/library"
	libraryFoldersRoute     = libraryRoot + "/folders"
	libraryMaterialsRoute   = libraryRoot + "/materials"
	libraryPictureDimension = 4
)

type libraryFolderBody struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id"`
}

type libraryPathBody struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type libraryItemBody struct {
	Kind         string            `json:"kind"`
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	FolderID     *string           `json:"folder_id"`
	Path         []libraryPathBody `json:"path"`
	AttachmentID string            `json:"attachment_id"`
	Filename     string            `json:"filename"`
	MediaType    string            `json:"media_type"`
	SizeBytes    int64             `json:"size_bytes"`
	Width        *int32            `json:"width"`
	URL          *string           `json:"url"`
}

type libraryBody struct {
	Folder        *libraryFolderBody  `json:"folder"`
	Path          []libraryPathBody   `json:"path"`
	Folders       []libraryFolderBody `json:"folders"`
	Items         []libraryItemBody   `json:"items"`
	NextCursor    *string             `json:"next_cursor"`
	TemplateCount int64               `json:"template_count"`
}

type fieldRefusal struct {
	Error struct {
		Code   string `json:"code"`
		Fields []struct {
			Path string `json:"path"`
			Code string `json:"code"`
		} `json:"fields"`
	} `json:"error"`
}

func libraryCapsHook(t *testing.T, caps domain.LibraryCaps) optionsHook {
	t.Helper()
	return func(options *api.Options, shared *clock.Shared, logger *slog.Logger, database *testdb.Database) {
		keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
		if err != nil {
			t.Fatal(err)
		}
		options.Library = library.New(library.Dependencies{
			Store: repositories.NewStore(database.Pool), Clock: shared, Random: randomPerTest(t), Keys: keys, Addresses: options.Files,
			Logger: logger, Caps: caps,
		})
	}
}

func newLibraryHarnessWithCaps(t *testing.T, caps domain.LibraryCaps) *authHarness {
	t.Helper()
	return newAuthHarnessStoring(t, flowProxies(), api.NewHandler, func(local *teststorage.Storage) storage.Store { return local.Store },
		libraryCapsHook(t, caps))
}

func optionalJSON(value *string) string {
	if value == nil {
		return "null"
	}
	return jsonText(*value)
}

func (h *authHarness) libraryCall(t *testing.T, token string, method string, path string, body string) response {
	t.Helper()
	return h.send(t, method, path, body, jsonCall(token))
}

func (h *authHarness) createFolderAnswer(t *testing.T, token string, name string, parent *string) response {
	t.Helper()
	return h.libraryCall(t, token, http.MethodPost, libraryFoldersRoute, `{"name":`+jsonText(name)+`,"parent_id":`+optionalJSON(parent)+`}`)
}

func (h *authHarness) createFolder(t *testing.T, token string, name string, parent *string) libraryFolderBody {
	t.Helper()
	answer := h.createFolderAnswer(t, token, name, parent)
	h.mustAnswer(t, answer, http.StatusCreated)
	return decodeLibrary[libraryFolderBody](t, answer)
}

func (h *authHarness) patchFolder(t *testing.T, token string, id string, body string) response {
	t.Helper()
	return h.libraryCall(t, token, http.MethodPatch, libraryFoldersRoute+"/"+id, body)
}

func (h *authHarness) deleteFolder(t *testing.T, token string, id string) response {
	t.Helper()
	return h.libraryCall(t, token, http.MethodDelete, libraryFoldersRoute+"/"+id, "")
}

func (h *authHarness) pendingLibraryFile(t *testing.T, tutor domain.UserID, group byte, n byte, size int64) string {
	t.Helper()
	id := fixtureID(group, n)
	h.insertPendingLibrary(t, id, tutor, domain.ContextLibrary, size, h.now(t))
	return id
}

func (h *authHarness) pendingLibraryPicture(t *testing.T, tutor domain.UserID, group byte, n byte) string {
	t.Helper()
	id := h.pendingLibraryFile(t, tutor, group, n, 10)
	if _, err := h.database.Pool.Exec(t.Context(), "UPDATE attachments SET media_type = 'image/png', filename = 'diagram.png', width = $2, height = $2 WHERE id = $1::uuid",
		id, libraryPictureDimension); err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *authHarness) createMaterialAnswer(t *testing.T, token string, attachment string, title string, folder *string) response {
	t.Helper()
	return h.libraryCall(t, token, http.MethodPost, libraryMaterialsRoute,
		`{"attachment_id":`+jsonText(attachment)+`,"title":`+jsonText(title)+`,"folder_id":`+optionalJSON(folder)+`}`)
}

func (h *authHarness) createMaterial(t *testing.T, token string, attachment string, title string, folder *string) libraryItemBody {
	t.Helper()
	answer := h.createMaterialAnswer(t, token, attachment, title, folder)
	h.mustAnswer(t, answer, http.StatusCreated)
	return decodeLibrary[libraryItemBody](t, answer)
}

func (h *authHarness) patchMaterial(t *testing.T, token string, id string, body string) response {
	t.Helper()
	return h.libraryCall(t, token, http.MethodPatch, libraryMaterialsRoute+"/"+id, body)
}

func (h *authHarness) deleteMaterial(t *testing.T, token string, id string) response {
	t.Helper()
	return h.libraryCall(t, token, http.MethodDelete, libraryMaterialsRoute+"/"+id, "")
}

func (h *authHarness) libraryAnswer(t *testing.T, token string, query url.Values) response {
	t.Helper()
	path := libraryRoot
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return h.libraryCall(t, token, http.MethodGet, path, "")
}

func (h *authHarness) libraryPage(t *testing.T, token string, query url.Values) libraryBody {
	t.Helper()
	answer := h.libraryAnswer(t, token, query)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[libraryBody](t, answer)
}

func (h *authHarness) fileUsage(t *testing.T, tutor domain.UserID) int64 {
	t.Helper()
	return h.countRows(t, "SELECT file_usage_bytes FROM tutor_profiles WHERE user_id = $1::uuid", tutor.String())
}

func decodeLibrary[Body any](t *testing.T, answer response) Body {
	t.Helper()
	var decoded Body
	if err := json.Unmarshal([]byte(answer.body), &decoded); err != nil {
		t.Fatalf("%s: %v", describe(answer), err)
	}
	return decoded
}

func fieldOf(t *testing.T, answer response) (string, string) {
	t.Helper()
	refusal := decodeLibrary[fieldRefusal](t, answer)
	if answer.status != http.StatusUnprocessableEntity || refusal.Error.Code != "validation_failed" || len(refusal.Error.Fields) != 1 {
		t.Fatalf("not a single field refusal: %s", describe(answer))
	}
	return refusal.Error.Fields[0].Path, refusal.Error.Fields[0].Code
}

func itemIDs(items []libraryItemBody) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func libraryOwners(t *testing.T, h *authHarness) (domain.UserID, string, string) {
	t.Helper()
	owner, ownerToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, intruderToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	return owner, ownerToken, intruderToken
}

func anotherTutorsFolder(method string, body string) denialCase {
	return func(t *testing.T, h *authHarness) (response, response) {
		t.Helper()
		owner, ownerToken, intruder := libraryOwners(t, h)
		folder := h.createFolder(t, ownerToken, "Unit 1", nil)
		outOfScope := h.libraryCall(t, intruder, method, libraryFoldersRoute+"/"+folder.ID, body)
		random := h.libraryCall(t, intruder, method, libraryFoldersRoute+"/"+randomCardID, body)
		if h.countRows(t, "SELECT count(id) FROM folders WHERE tutor_id = $1::uuid AND name = 'Unit 1'", owner.String()) != 1 {
			t.Fatal("another tutor changed a folder")
		}
		return outOfScope, random
	}
}

func anotherTutorsMaterial(method string, body string) denialCase {
	return func(t *testing.T, h *authHarness) (response, response) {
		t.Helper()
		owner, ownerToken, intruder := libraryOwners(t, h)
		material := h.createMaterial(t, ownerToken, h.pendingLibraryFile(t, owner, 60, 1, 10), "Worksheet", nil)
		deleted := h.createMaterial(t, ownerToken, h.pendingLibraryFile(t, owner, 60, 2, 10), "Deleted", nil)
		h.mustAnswer(t, h.deleteMaterial(t, ownerToken, deleted.ID), http.StatusNoContent)
		outOfScope := h.libraryCall(t, intruder, method, libraryMaterialsRoute+"/"+material.ID, body)
		random := h.libraryCall(t, intruder, method, libraryMaterialsRoute+"/"+randomCardID, body)
		if method == http.MethodPatch {
			deletedAnswer := h.patchMaterial(t, ownerToken, deleted.ID, body)
			ownRandom := h.patchMaterial(t, ownerToken, randomCardID, body)
			if deletedAnswer.status != http.StatusNotFound || refusal(t, deletedAnswer) != refusal(t, ownRandom) {
				t.Fatalf("editing a deleted material answered %s", describe(deletedAnswer))
			}
		}
		if h.countRows(t, "SELECT count(id) FROM materials WHERE tutor_id = $1::uuid AND title = 'Worksheet' AND deleted_at IS NULL", owner.String()) != 1 {
			t.Fatal("another tutor changed a material")
		}
		return outOfScope, random
	}
}

func anotherTutorsParentFolder(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	_, ownerToken, intruder := libraryOwners(t, h)
	folder := h.createFolder(t, ownerToken, "Unit 1", nil)
	outOfScope := h.createFolderAnswer(t, intruder, "Inside", &folder.ID)
	randomID := randomCardID
	random := h.createFolderAnswer(t, intruder, "Inside", &randomID)
	return outOfScope, random
}

func anotherTutorsLibraryUpload(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	owner, _, intruder := libraryOwners(t, h)
	upload := h.pendingLibraryFile(t, owner, 61, 1, 10)
	outOfScope := h.createMaterialAnswer(t, intruder, upload, "Stolen", nil)
	random := h.createMaterialAnswer(t, intruder, randomCardID, "Stolen", nil)
	if h.uploadState(t, upload) != string(domain.AttachmentPending) {
		t.Fatal("another tutor bound the upload")
	}
	return outOfScope, random
}

func anotherTutorsListedFolder(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	_, ownerToken, intruder := libraryOwners(t, h)
	folder := h.createFolder(t, ownerToken, "Unit 1", nil)
	return h.libraryAnswer(t, intruder, url.Values{"folder": {folder.ID}}), h.libraryAnswer(t, intruder, url.Values{"folder": {randomCardID}})
}
