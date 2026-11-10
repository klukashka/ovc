package api_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/library"
)

func TestLibraryWriteSessionRecheck(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, unconfirmed := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 2, newcomerEmail)
	actor := h.authenticatedTutor(t, token)
	if _, err := h.operators().Block(context.Background(), tutorEmail); err != nil {
		t.Fatal(err)
	}

	_, blockedErr := h.options.Library.CreateFolder(context.Background(), actor, library.NewFolder{Name: "Unit 1"})
	accepted := h.createFolderAnswer(t, unconfirmed, "Unit 1", nil)

	if !errors.Is(blockedErr, library.ErrUnauthenticated) {
		t.Fatalf("a blocked tutor's write answered %v", blockedErr)
	}
	if h.countRows(t, "SELECT count(id) FROM folders WHERE tutor_id = $1::uuid", tutor.String()) != 0 {
		t.Fatal("a blocked tutor's write created a folder")
	}
	if accepted.status != http.StatusCreated || accepted.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("an unconfirmed tutor's folder answered %s", describe(accepted))
	}
}

func (h *authHarness) fillLibraryWrites(t *testing.T, tutor domain.UserID, count int) {
	t.Helper()
	window, _ := domain.FixedWindow(h.now(t), domain.LibraryWriteWindow)
	subject := h.keys.ThrottleSubject(string(domain.PurposeLibraryWrites), tutor.String())
	if _, err := h.database.Pool.Exec(t.Context(), "INSERT INTO rate_limit_counters (purpose, subject, window_start, count) VALUES ($1, $2, $3, $4)",
		string(domain.PurposeLibraryWrites), subject, window, count); err != nil {
		t.Fatal(err)
	}
}

func TestLibraryWriteLimit(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.fillLibraryWrites(t, tutor, domain.LibraryWritesPerHour-1)

	last := h.createFolderAnswer(t, token, "Unit 1", nil)
	refused := h.createFolderAnswer(t, token, "Unit 2", nil)

	h.mustAnswer(t, last, http.StatusCreated)
	if refused.status != http.StatusTooManyRequests || code(t, refused) != "rate_limited" || refused.headers.Get("Retry-After") == "" {
		t.Fatalf("the 601st library write answered %s", describe(refused))
	}
	if refused.headers.Get("Cache-Control") != "no-store" || h.countLogs(`"msg":"library.rate_limited"`, `"limit":"library_writes"`) != 1 {
		t.Fatal("the refusal is not uncached and recorded once")
	}
	if h.countRows(t, "SELECT count(id) FROM folders WHERE tutor_id = $1::uuid", tutor.String()) != 1 {
		t.Fatal("the refused write created a folder")
	}
}

func TestFolderDeleteIgnoresDeletedItems(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	folder := h.createFolder(t, token, "Unit 1", nil)
	material := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 40, 1, 64), "Worksheet", &folder.ID)
	h.mustAnswer(t, h.deleteMaterial(t, token, material.ID), http.StatusNoContent)

	deleted := h.deleteFolder(t, token, folder.ID)
	repeated := h.deleteFolder(t, token, folder.ID)

	if deleted.status != http.StatusNoContent || deleted.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("a folder holding only a deleted material answered %s", describe(deleted))
	}
	if repeated.status != http.StatusNotFound || code(t, repeated) != "not_found" {
		t.Fatalf("a repeated folder delete answered %s", describe(repeated))
	}
}

func TestFolderDeleteRefusesNonEmpty(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	parent := h.createFolder(t, token, "Unit 1", nil)
	h.createFolder(t, token, "Lesson 1", &parent.ID)
	holding := h.createFolder(t, token, "Unit 2", nil)
	h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 41, 1, 64), "Worksheet", &holding.ID)

	withChild := h.deleteFolder(t, token, parent.ID)
	withMaterial := h.deleteFolder(t, token, holding.ID)

	for name, refused := range map[string]response{"a folder with a child folder": withChild, "a folder with a material": withMaterial} {
		if refused.status != http.StatusConflict || code(t, refused) != "not_editable" {
			t.Fatalf("deleting %s answered %s", name, describe(refused))
		}
	}
	if h.countRows(t, "SELECT count(id) FROM folders WHERE tutor_id = $1::uuid", tutor.String()) != 3 {
		t.Fatal("a refused delete removed a folder")
	}
}

func TestEveryFolderIsListed(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)
	first := h.createFolder(t, token, "b unit", nil)
	second := h.createFolder(t, token, "A unit", nil)
	child := h.createFolder(t, token, "Lesson", &first.ID)
	grandchild := h.createFolder(t, token, "Homework", &child.ID)

	answer := h.libraryCall(t, token, http.MethodGet, libraryFoldersRoute, "")

	h.mustAnswer(t, answer, http.StatusOK)
	listed := decodeLibrary[struct {
		Folders []libraryFolderBody `json:"folders"`
	}](t, answer).Folders
	want := []libraryFolderBody{second, first, grandchild, child}
	if !slices.EqualFunc(listed, want, func(a, b libraryFolderBody) bool {
		return a.ID == b.ID && a.Name == b.Name && optionalJSON(a.ParentID) == optionalJSON(b.ParentID)
	}) {
		t.Fatalf("listed %+v, want %+v", listed, want)
	}
	if answer.headers.Get("Cache-Control") != "no-store" {
		t.Fatal("the folder list is cacheable")
	}
}

func TestFolderDepthAndCycles(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	top := h.createFolder(t, token, "Unit 1", nil)
	middle := h.createFolder(t, token, "Lesson 1", &top.ID)
	bottom := h.createFolder(t, token, "Homework", &middle.ID)
	other := h.createFolder(t, token, "Unit 2", nil)
	h.createFolder(t, token, "Lesson 2", &other.ID)

	fourthLevel := h.createFolderAnswer(t, token, "Too deep", &bottom.ID)
	intoItself := h.patchFolder(t, token, top.ID, `{"parent_id":`+jsonText(top.ID)+`}`)
	intoDescendant := h.patchFolder(t, token, top.ID, `{"parent_id":`+jsonText(bottom.ID)+`}`)
	subtreeTooDeep := h.patchFolder(t, token, other.ID, `{"parent_id":`+jsonText(middle.ID)+`}`)
	toRoot := h.patchFolder(t, token, middle.ID, `{"parent_id":null}`)

	for name, refused := range map[string]response{
		"a fourth level": fourthLevel, "a move into itself": intoItself, "a move into a descendant": intoDescendant,
		"a move leaving a subtree too deep": subtreeTooDeep,
	} {
		if path, field := fieldOf(t, refused); path != "body.parent_id" || field != "invalid" {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
	h.mustAnswer(t, toRoot, http.StatusOK)
	if moved := decodeLibrary[libraryFolderBody](t, toRoot); moved.ParentID != nil || moved.Name != "Lesson 1" {
		t.Fatalf("the moved folder is %+v", moved)
	}
}

func TestFolderNamesAreUniqueAmongSiblingsAndValidated(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	unit := h.createFolder(t, token, "Ёлка", nil)
	other := h.createFolder(t, token, "Other", nil)

	cases := map[string]struct {
		answer response
		path   string
		code   string
	}{
		"a sibling with the same normalised name": {h.createFolderAnswer(t, token, "  ЕЛКА ", nil), "body.name", "taken"},
		"a rename onto a sibling's name":          {h.patchFolder(t, token, other.ID, `{"name":"елка"}`), "body.name", "taken"},
		"a control character":                     {h.createFolderAnswer(t, token, "Unit\u0007", nil), "body.name", "invalid"},
		"a name of spaces":                        {h.createFolderAnswer(t, token, "   ", nil), "body.name", "required"},
		"a name too long":                         {h.createFolderAnswer(t, token, strings.Repeat("я", 101), nil), "body.name", "too_long"},
		"a name normalising past its bound":       {h.createFolderAnswer(t, token, strings.Repeat("\ufdfa", 50), nil), "body.name", "too_long"},
		"a patch without fields":                  {h.patchFolder(t, token, other.ID, `{}`), "body", "required"},
	}
	sameNameElsewhere := h.createFolderAnswer(t, token, "Ёлка", &other.ID)
	recased := h.patchFolder(t, token, unit.ID, `{"name":"ёлка"}`)

	for name, check := range cases {
		if path, field := fieldOf(t, check.answer); path != check.path || field != check.code {
			t.Errorf("%s answered %s", name, describe(check.answer))
		}
	}
	h.mustAnswer(t, sameNameElsewhere, http.StatusCreated)
	h.mustAnswer(t, recased, http.StatusOK)
}

func TestLibraryCaps(t *testing.T) {
	t.Parallel()
	h := newLibraryHarnessWithCaps(t, domain.LibraryCaps{Folders: 2, Materials: 2, Templates: 2, Rows: 3})
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.createFolder(t, token, "Unit 1", nil)
	h.createFolder(t, token, "Unit 2", nil)
	first := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 42, 1, 10), "First", nil)
	h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 42, 2, 10), "Second", nil)

	thirdFolder := h.createFolderAnswer(t, token, "Unit 3", nil)
	thirdLive := h.createMaterialAnswer(t, token, h.pendingLibraryFile(t, tutor, 42, 3, 10), "Third", nil)
	h.mustAnswer(t, h.deleteMaterial(t, token, first.ID), http.StatusNoContent)
	replacement := h.createMaterialAnswer(t, token, fixtureID(42, 3), "Third", nil)
	beyondRows := h.createMaterialAnswer(t, token, h.pendingLibraryFile(t, tutor, 42, 4, 10), "Fourth", nil)

	for name, refused := range map[string]response{"a third folder": thirdFolder, "a third live material": thirdLive, "a fourth material row": beyondRows} {
		if path, field := fieldOf(t, refused); path != "body" || field != "too_many" {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
	h.mustAnswer(t, replacement, http.StatusCreated)
}

func TestLibraryQuotaCharges(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.setQuota(t, tutor, 1_000, 100)
	fits, tooLarge := h.pendingLibraryFile(t, tutor, 43, 1, 300), h.pendingLibraryFile(t, tutor, 43, 2, 700)

	material := h.createMaterial(t, token, fits, "Worksheet", nil)
	afterCreate := h.fileUsage(t, tutor)
	refused := h.createMaterialAnswer(t, token, tooLarge, "Too large", nil)
	afterRefusal := h.fileUsage(t, tutor)
	h.mustAnswer(t, h.deleteMaterial(t, token, material.ID), http.StatusNoContent)
	afterDelete := h.fileUsage(t, tutor)

	if afterCreate != 400 || afterRefusal != 400 || afterDelete != 100 {
		t.Fatalf("usage moved %d → %d → %d", afterCreate, afterRefusal, afterDelete)
	}
	if refused.status != http.StatusInsufficientStorage || code(t, refused) != "quota_exceeded" || refused.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("a material over quota answered %s", describe(refused))
	}
	if h.uploadState(t, tooLarge) != string(domain.AttachmentPending) || h.countRows(t, "SELECT count(id) FROM materials WHERE tutor_id = $1::uuid", tutor.String()) != 1 {
		t.Fatal("a refused material bound its upload or kept its row")
	}
	if h.countLogs(`"msg":"quota.exceeded"`, `"context":"library"`) != 1 {
		t.Fatal("the refused binding was not recorded")
	}
}

func TestMaterialCreationRefusesUploadsItMayNotBind(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	other, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, newcomerEmail)
	foreign := h.pendingLibraryFile(t, other, 44, 1, 10)
	foreignFolder := h.createFolder(t, otherToken, "Unit 1", nil).ID
	prompt := fixtureID(44, 2)
	h.insertPendingLibrary(t, prompt, tutor, domain.ContextPromptImage, 10, h.now(t))
	expired := fixtureID(44, 3)
	h.insertPendingLibrary(t, expired, tutor, domain.ContextLibrary, 10, h.now(t).Add(-25*time.Hour))
	bound := h.pendingLibraryFile(t, tutor, 44, 4, 10)
	h.createMaterial(t, token, bound, "Worksheet", nil)
	random := fixtureID(44, 5)

	answers := map[string]response{
		"another tutor's upload": h.createMaterialAnswer(t, token, foreign, "Stolen", nil),
		"a prompt image":         h.createMaterialAnswer(t, token, prompt, "Prompt", nil),
		"an expired upload":      h.createMaterialAnswer(t, token, expired, "Expired", nil),
		"an already bound file":  h.createMaterialAnswer(t, token, bound, "Again", nil),
		"another tutor's folder": h.createMaterialAnswer(t, token, h.pendingLibraryFile(t, tutor, 44, 6, 10), "Moved", &foreignFolder),
	}
	randomAnswer := h.createMaterialAnswer(t, token, random, "Random", nil)

	for name, refused := range answers {
		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, randomAnswer) {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
	if h.uploadState(t, foreign) != string(domain.AttachmentPending) {
		t.Fatal("another tutor's upload was bound")
	}
}

func TestMaterialEditsRenameAndMove(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	folder := h.createFolder(t, token, "Unit 1", nil)
	material := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 45, 1, 10), "Worksheet", nil)

	moved := h.patchMaterial(t, token, material.ID, `{"title":"Worksheet 2","folder_id":`+jsonText(folder.ID)+`}`)
	unchanged := h.patchMaterial(t, token, material.ID, `{"title":"Worksheet 2"}`)
	toRoot := h.patchMaterial(t, token, material.ID, `{"folder_id":null}`)
	badTitle := h.patchMaterial(t, token, material.ID, `{"title":"Bad\u202etitle"}`)

	h.mustAnswer(t, moved, http.StatusOK)
	if moved.headers.Get("Cache-Control") != "no-store" {
		t.Fatal("a material edit is cacheable")
	}
	if item := decodeLibrary[libraryItemBody](t, moved); item.Title != "Worksheet 2" || item.FolderID == nil || *item.FolderID != folder.ID {
		t.Fatalf("the edited material is %+v", item)
	}
	h.mustAnswer(t, unchanged, http.StatusOK)
	if h.countLogs(`"msg":"library.material.updated"`, material.ID) != 3 {
		t.Fatal("an edit that changed nothing was recorded, or a change was not")
	}
	if item := decodeLibrary[libraryItemBody](t, toRoot); toRoot.status != http.StatusOK || item.FolderID != nil {
		t.Fatalf("moving to the root answered %s", describe(toRoot))
	}
	if path, field := fieldOf(t, badTitle); path != "body.title" || field != "invalid" {
		t.Fatalf("a title with a bidirectional override answered %s", describe(badTitle))
	}
}

func TestMaterialDeleteKeepsOtherReferences(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	h.setQuota(t, tutor, 1_000_000, 0)
	shared := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 46, 1, 300), "Shared", nil)
	alone := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 46, 2, 200), "Alone", nil)
	h.exec(t, "INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, created_at) VALUES ($1::uuid, 'template', gen_random_uuid(), now())",
		shared.AttachmentID)

	deletedShared := h.deleteMaterial(t, token, shared.ID)
	deletedAlone := h.deleteMaterial(t, token, alone.ID)
	repeated := h.deleteMaterial(t, token, alone.ID)
	edited := h.patchMaterial(t, token, alone.ID, `{"title":"Back"}`)

	for _, answer := range []response{deletedShared, deletedAlone, repeated} {
		h.mustAnswer(t, answer, http.StatusNoContent)
	}
	if edited.status != http.StatusNotFound {
		t.Fatalf("editing a deleted material answered %s", describe(edited))
	}
	if h.uploadState(t, shared.AttachmentID) != string(domain.AttachmentBound) || h.uploadState(t, alone.AttachmentID) != string(domain.AttachmentDeleted) {
		t.Fatal("the files did not follow their last references")
	}
	if h.fileUsage(t, tutor) != 300 {
		t.Fatalf("usage is %d instead of the shared file's 300 bytes", h.fileUsage(t, tutor))
	}
	if h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = 'storage_delete' AND idempotency_key = 'storage_delete:' || $1", alone.AttachmentID) != 1 ||
		h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = 'storage_delete' AND idempotency_key = 'storage_delete:' || $1", shared.AttachmentID) != 0 {
		t.Fatal("the storage deletions do not follow the last references")
	}
	if listed := h.libraryPage(t, token, url.Values{}); len(listed.Items) != 0 {
		t.Fatalf("deleted materials are listed: %+v", listed.Items)
	}
}

func TestLibraryListingShowsOneFolder(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	unit := h.createFolder(t, token, "Unit", nil)
	lesson := h.createFolder(t, token, "Lesson", &unit.ID)
	homework := h.createFolder(t, token, "Homework", &lesson.ID)
	h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 47, 1, 10), "At the root", nil)
	picture := h.createMaterial(t, token, h.pendingLibraryPicture(t, tutor, 47, 2), "Diagram", &lesson.ID)
	document := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 47, 3, 10), "Notes", &lesson.ID)

	root := h.libraryAnswer(t, token, url.Values{})
	inLesson := h.libraryPage(t, token, url.Values{"folder": {lesson.ID}})
	unknown := h.libraryAnswer(t, token, url.Values{"folder": {fixtureID(47, 9)}})

	h.mustAnswer(t, root, http.StatusOK)
	rootPage := decodeLibrary[libraryBody](t, root)
	if rootPage.Folder != nil || len(rootPage.Path) != 0 || len(rootPage.Folders) != 1 || rootPage.Folders[0].ID != unit.ID || len(rootPage.Items) != 1 {
		t.Fatalf("the root is %+v", rootPage)
	}
	if root.headers.Get("Cache-Control") != "no-store" || rootPage.TemplateCount != 0 || rootPage.Items[0].Path != nil {
		t.Fatalf("the root answer is cacheable or carries a path: %s", describe(root))
	}
	if inLesson.Folder == nil || inLesson.Folder.ID != lesson.ID || len(inLesson.Path) != 2 || inLesson.Path[0].ID != unit.ID || inLesson.Path[1].ID != lesson.ID {
		t.Fatalf("the lesson folder is %+v", inLesson)
	}
	if len(inLesson.Folders) != 1 || inLesson.Folders[0].ID != homework.ID || !slices.Equal(itemIDs(inLesson.Items), []string{picture.ID, document.ID}) {
		t.Fatalf("the lesson folder holds %+v and %+v", inLesson.Folders, inLesson.Items)
	}
	if inLesson.Items[0].URL == nil || !strings.HasPrefix(*inLesson.Items[0].URL, "/api/v1/files/"+picture.AttachmentID+"?t=") || inLesson.Items[1].URL != nil {
		t.Fatalf("the addresses are %v and %v", inLesson.Items[0].URL, inLesson.Items[1].URL)
	}
	if unknown.status != http.StatusNotFound || code(t, unknown) != "not_found" {
		t.Fatalf("another folder answered %s", describe(unknown))
	}
}

func TestLibrarySearchNormalised(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	unit := h.createFolder(t, token, "Unit", nil)
	lesson := h.createFolder(t, token, "Lesson", &unit.ID)
	tree := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 48, 1, 10), "«Ёлка» — ЁЖ", &lesson.ID)
	percent := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 48, 2, 10), "100% done", nil)
	h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 48, 3, 10), "100 done", nil)
	underscore := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 48, 4, 10), "snake_case", nil)
	h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 48, 5, 10), "snakeXcase", nil)
	backslash := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 48, 6, 10), `back\slash`, nil)

	byStem := h.libraryPage(t, token, url.Values{"q": {"елк"}})
	byPercent := h.libraryPage(t, token, url.Values{"q": {"%"}})
	byUnderscore := h.libraryPage(t, token, url.Values{"q": {"_"}})
	byBackslash := h.libraryPage(t, token, url.Values{"q": {`\`}})
	blank := h.libraryAnswer(t, token, url.Values{"q": {"   "}})
	withFolder := h.libraryAnswer(t, token, url.Values{"q": {"елк"}, "folder": {unit.ID}})

	if len(byStem.Items) != 1 || byStem.Items[0].ID != tree.ID || len(byStem.Items[0].Path) != 2 || byStem.Items[0].Path[1].ID != lesson.ID {
		t.Fatalf("searching елк found %+v", byStem.Items)
	}
	if byStem.Folder != nil || len(byStem.Path) != 0 || len(byStem.Folders) != 0 {
		t.Fatalf("a search answered a folder view %+v", byStem)
	}
	for name, found := range map[string]struct {
		page libraryBody
		want string
	}{"%": {byPercent, percent.ID}, "_": {byUnderscore, underscore.ID}, `\`: {byBackslash, backslash.ID}} {
		if !slices.Equal(itemIDs(found.page.Items), []string{found.want}) || len(found.page.Items[0].Path) != 0 {
			t.Fatalf("searching %s found %+v", name, found.page.Items)
		}
	}
	if path, field := fieldOf(t, blank); path != "query.q" || field != "required" {
		t.Fatalf("a blank search answered %s", describe(blank))
	}
	if path, field := fieldOf(t, withFolder); path != "query.folder" || field != "not_allowed" {
		t.Fatalf("a search within a folder answered %s", describe(withFolder))
	}
}

func TestLibraryKeysetPagination(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	titles := []string{"b", "A", "a", "c", "B", "d", "a"}
	created := make([]string, 0, len(titles))
	for index, title := range titles {
		created = append(created, h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 49, byte(index+1), 10), title, nil).ID)
	}

	var seen []string
	query := url.Values{"limit": {"3"}}
	pages := 0
	for {
		page := h.libraryPage(t, token, query)
		pages++
		seen = append(seen, itemIDs(page.Items)...)
		if page.NextCursor == nil {
			break
		}
		query.Set("cursor", *page.NextCursor)
	}
	searched := h.libraryPage(t, token, url.Values{"q": {"a"}, "limit": {"1"}})
	malformed := h.libraryAnswer(t, token, url.Values{"cursor": {"not a cursor"}})
	invalidText := h.libraryAnswer(t, token, url.Values{"cursor": {base64.RawURLEncoding.EncodeToString([]byte("material\n" + created[0] + "\n\xff\x00"))}})

	if pages != 3 || len(seen) != len(created) {
		t.Fatalf("%d pages listed %d of %d materials", pages, len(seen), len(created))
	}
	sorted, want := slices.Clone(seen), slices.Clone(created)
	slices.Sort(sorted)
	slices.Sort(want)
	if !slices.Equal(sorted, want) || len(slices.Compact(sorted)) != len(created) {
		t.Fatalf("listed %v instead of each of %v once", seen, created)
	}
	if searched.NextCursor == nil || len(searched.Items) != 1 {
		t.Fatalf("a search page of one answered %+v", searched)
	}
	for name, refused := range map[string]response{"a malformed cursor": malformed, "a cursor whose title is not text": invalidText} {
		if path, field := fieldOf(t, refused); path != "query.cursor" || field != "invalid" {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
}

func TestPictureMaterialsAreDownloadedThroughTheirMaterial(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	upload := h.libraryUpload(t, token, domain.ContextLibrary, "worksheet.pdf", pdfFile(64))
	h.mustAnswer(t, upload, http.StatusCreated)
	material := h.createMaterial(t, token, uploadedID(t, upload), "Worksheet", nil)

	inline, _ := h.addressOf(t, token, material.AttachmentID, "?ref=material:"+material.ID+"&disposition=inline")
	minted, address := h.addressOf(t, token, material.AttachmentID, "?ref=material:"+material.ID)
	served := h.fetch(t, address.URL, nil)
	h.mustAnswer(t, h.deleteMaterial(t, token, material.ID), http.StatusNoContent)
	afterDelete := h.fetch(t, address.URL, nil)

	if path, field := fieldOf(t, inline); path != "query.disposition" || field != "invalid" {
		t.Fatalf("an inline PDF answered %s", describe(inline))
	}
	if minted.status != http.StatusOK || minted.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("minting answered %s", describe(minted))
	}
	if served.status != http.StatusOK || served.body != string(pdfFile(64)) || !strings.HasPrefix(served.headers.Get("Content-Disposition"), "attachment") {
		t.Fatalf("serving answered %d with %q", served.status, served.headers.Get("Content-Disposition"))
	}
	if afterDelete.status != http.StatusNotFound {
		t.Fatalf("a deleted material's address answered %s", describe(afterDelete))
	}
	if h.fileUsage(t, tutor) != 0 {
		t.Fatal("the deleted material kept its charge")
	}
}

func TestAttachmentMintRequiresReference(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	other, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, newcomerEmail)
	_, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	first := h.createMaterial(t, token, h.pendingLibraryPicture(t, tutor, 50, 1), "First", nil)
	second := h.createMaterial(t, token, h.pendingLibraryPicture(t, tutor, 50, 2), "Second", nil)
	detached := h.createMaterial(t, token, h.pendingLibraryPicture(t, tutor, 50, 3), "Detached", nil)
	h.exec(t, "INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, created_at) VALUES ($1::uuid, 'template', gen_random_uuid(), now())",
		detached.AttachmentID)
	h.mustAnswer(t, h.deleteMaterial(t, token, detached.ID), http.StatusNoContent)
	foreign := h.createMaterial(t, otherToken, h.pendingLibraryPicture(t, other, 50, 4), "Foreign", nil)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	withFirst := h.scheduled(t, token, oneOffFor(card, "2030-03-12", "18:00")).Lessons[0]
	withSecond := h.scheduled(t, token, oneOffFor(card, "2030-03-13", "18:00")).Lessons[0]
	h.attached(t, token, withFirst.ID, 1, first.ID)
	h.attached(t, token, withSecond.ID, 1, second.ID)

	refusals := map[string]string{
		"another tutor's attachment with the tutor's material": foreign.AttachmentID + "?ref=material:" + first.ID,
		"the tutor's attachment with another of its materials": first.AttachmentID + "?ref=material:" + second.ID,
		"a detached material":                               detached.AttachmentID + "?ref=material:" + detached.ID,
		"a missing reference":                               first.AttachmentID,
		"another kind naming the material":                  first.AttachmentID + "?ref=template:" + first.ID,
		"a visible lesson with another lesson's attachment": second.AttachmentID + "?ref=lesson:" + withFirst.ID,
		"a lesson reference naming the material":            first.AttachmentID + "?ref=lesson:" + first.ID,
	}
	random, _ := h.addressOf(t, token, fixtureID(50, 9), "?ref=material:"+first.ID)
	malformed, _ := h.addressOf(t, token, first.AttachmentID, "?ref=material:nonsense")
	repeated, _ := h.addressOf(t, token, first.AttachmentID, "?ref=material:"+first.ID+"&ref=material:"+first.ID)
	student, _ := h.addressOf(t, studentToken, first.AttachmentID, "?ref=material:"+first.ID)
	inline, address := h.addressOf(t, token, first.AttachmentID, "?ref=material:"+first.ID+"&disposition=inline")
	swapped := h.fetch(t, strings.Replace(address.URL, first.AttachmentID, second.AttachmentID, 1), nil)

	for name, query := range refusals {
		refused, _ := h.addressOf(t, token, query, "")
		if refused.status != http.StatusNotFound || refusal(t, refused) != refusal(t, random) {
			t.Errorf("%s answered %s", name, describe(refused))
		}
	}
	for name, invalid := range map[string]response{"a malformed reference": malformed, "a repeated reference": repeated} {
		if path, field := fieldOf(t, invalid); path != "query.ref" || field != "invalid" {
			t.Errorf("%s answered %s", name, describe(invalid))
		}
	}
	if student.status != http.StatusNotFound || swapped.status != http.StatusNotFound {
		t.Fatalf("a student answered %s and a swapped address %s", describe(student), describe(swapped))
	}
	if inline.status != http.StatusOK || !strings.Contains(address.URL, "/api/v1/files/"+first.AttachmentID+"?t=") {
		t.Fatalf("an inline picture answered %s", describe(inline))
	}
}

func TestLibraryRecordsCarryNoContent(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	const folderSentinel, renamedSentinel, titleSentinel, retitledSentinel, searchSentinel = "Folder sentinel", "Renamed sentinel",
		"Title sentinel", "Retitled sentinel", "sentinelsearch"
	folder := h.createFolder(t, token, folderSentinel, nil)
	other := h.createFolder(t, token, "Other", nil)
	picture := h.pendingLibraryPicture(t, tutor, 51, 1)

	h.mustAnswer(t, h.patchFolder(t, token, folder.ID, `{"name":`+jsonText(renamedSentinel)+`,"parent_id":`+jsonText(other.ID)+`}`), http.StatusOK)
	material := h.createMaterial(t, token, picture, titleSentinel, &folder.ID)
	h.mustAnswer(t, h.patchMaterial(t, token, material.ID, `{"title":`+jsonText(retitledSentinel)+`,"folder_id":null}`), http.StatusOK)
	listed := h.libraryPage(t, token, url.Values{})
	h.libraryPage(t, token, url.Values{"q": {searchSentinel}})
	h.mustAnswer(t, h.deleteMaterial(t, token, material.ID), http.StatusNoContent)
	h.mustAnswer(t, h.patchFolder(t, token, folder.ID, `{"parent_id":null}`), http.StatusOK)
	h.mustAnswer(t, h.deleteFolder(t, token, folder.ID), http.StatusNoContent)
	records := h.logs.String()

	if len(listed.Items) != 1 || listed.Items[0].URL == nil {
		t.Fatalf("the listing is %+v", listed.Items)
	}
	forbidden := []string{folderSentinel, renamedSentinel, titleSentinel, retitledSentinel, searchSentinel, "diagram.png", "/api/v1/files/", "?t=",
		strings.SplitN(*listed.Items[0].URL, "?t=", 2)[1]}
	for _, value := range forbidden {
		if strings.Contains(records, value) {
			t.Fatalf("a record carries %q", value)
		}
	}
	for event, fragments := range map[string][]string{
		"library.folder.created":   {folder.ID, tutor.String()},
		"library.folder.updated":   {folder.ID, `"change":"rename"`},
		"library.folder.deleted":   {folder.ID},
		"library.material.created": {material.ID, picture},
		"library.material.updated": {material.ID, `"change":"move"`},
		"library.material.deleted": {material.ID},
		"file.reference.added":     {picture, `"ref_kind":"material"`},
		"file.reference.removed":   {picture, `"ref_kind":"material"`},
	} {
		if h.countLogs(append([]string{`"msg":"` + event + `"`}, fragments...)...) != 1 {
			t.Errorf("no single %s record with %v", event, fragments)
		}
	}
	if h.countLogs(`"msg":"library.folder.updated"`, folder.ID, `"change":"move"`) != 2 {
		t.Error("the folder's two moves were not recorded")
	}
}

func TestTutorPurgeLibrary(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	top := h.createFolder(t, token, "Unit", nil)
	middle := h.createFolder(t, token, "Lesson", &top.ID)
	bottom := h.createFolder(t, token, "Homework", &middle.ID)
	kept := h.libraryUpload(t, token, domain.ContextLibrary, "worksheet.pdf", pdfFile(64))
	removed := h.libraryUpload(t, token, domain.ContextLibrary, "notes.pdf", pdfFile(32))
	h.mustAnswer(t, h.libraryUpload(t, token, domain.ContextLibrary, "pending.pdf", pdfFile(16)), http.StatusCreated)
	live := h.createMaterial(t, token, uploadedID(t, kept), "Worksheet", &bottom.ID)
	gone := h.createMaterial(t, token, uploadedID(t, removed), "Notes", nil)
	h.mustAnswer(t, h.deleteMaterial(t, token, gone.ID), http.StatusNoContent)
	uploads := h.uploadsIn(t, tutor, libraryContexts...)
	h.mustAnswer(t, h.deleteMe(t, token, accountPassword), http.StatusNoContent)

	h.at(t, authStart.Add(domain.DeletedAccountRetention+time.Hour))
	h.tick(t)
	h.tick(t)

	if h.countRows(t, "SELECT count(id) FROM users WHERE id = $1::uuid", tutor.String()) != 0 {
		t.Fatal("the tutor was not purged")
	}
	for table, query := range map[string]string{
		"folders":    "SELECT count(id) FROM folders WHERE tutor_id = $1::uuid",
		"materials":  "SELECT count(id) FROM materials WHERE tutor_id = $1::uuid",
		"charges":    "SELECT count(attachment_id) FROM quota_charges WHERE tutor_id = $1::uuid",
		"references": "SELECT count(attachment_id) FROM attachment_references WHERE ref_kind = 'material' AND ref_id = $1::uuid",
	} {
		argument := tutor.String()
		if table == "references" {
			argument = live.ID
		}
		if count := h.countRows(t, query, argument); count != 0 {
			t.Errorf("%d %s survived the purge", count, table)
		}
	}
	if len(uploads) != 3 {
		t.Fatalf("the tutor held %d library uploads", len(uploads))
	}
	for _, upload := range uploads {
		if h.uploadState(t, upload.ID) != "gone" {
			t.Errorf("the purged tutor's library file %s survived", upload.ID)
		}
		if _, stored := h.storedObject(t, upload.Key); stored {
			t.Errorf("the purged tutor's library file %s kept its object", upload.ID)
		}
	}
}

func TestUnconfirmedTutorCleanupRemovesTheLibrary(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)
	top := h.createFolder(t, token, "Unit", nil)
	h.createFolder(t, token, "Lesson", &top.ID)

	h.at(t, authStart.Add(8*24*time.Hour))
	h.tick(t)

	if h.countRows(t, "SELECT count(id) FROM users WHERE id = $1::uuid", tutor.String()) != 0 {
		t.Fatal("the unconfirmed tutor was not cleaned up")
	}
	if h.countRows(t, "SELECT count(id) FROM folders WHERE tutor_id = $1::uuid", tutor.String()) != 0 {
		t.Fatal("the unconfirmed tutor's folders survived")
	}
}

func TestLibraryPurgeBatchesStopAtTheirSize(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	for index := range 3 {
		h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 52, byte(index+1), 10), "Material", nil)
	}
	steps := library.Purge{Random: h.random, Logger: h.logger}.TutorSteps()
	var batches []int64

	err := h.store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		for _, step := range steps {
			for {
				deleted, err := step(context.Background(), repos, tutor, 2, h.now(t))
				if err != nil {
					return err
				}
				if deleted == 0 {
					break
				}
				batches = append(batches, deleted)
			}
		}
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(batches, []int64{2, 1, 2, 1}) {
		t.Fatalf("the batches removed %v rows", batches)
	}
	if h.fileUsage(t, tutor) != 0 || h.countRows(t, "SELECT count(id) FROM materials WHERE tutor_id = $1::uuid", tutor.String()) != 0 {
		t.Fatal("the batches left charges or materials")
	}
}

func TestLibraryEditsRefuseAnotherTutorsFolder(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, newcomerEmail)
	foreign := h.createFolder(t, otherToken, "Unit 1", nil)
	own := h.createFolder(t, token, "Unit 2", nil)
	material := h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 53, 1, 10), "Worksheet", nil)

	folderMove := h.patchFolder(t, token, own.ID, `{"parent_id":`+jsonText(foreign.ID)+`}`)
	randomFolderMove := h.patchFolder(t, token, own.ID, `{"parent_id":`+jsonText(randomCardID)+`}`)
	materialMove := h.patchMaterial(t, token, material.ID, `{"folder_id":`+jsonText(foreign.ID)+`}`)
	randomMaterialMove := h.patchMaterial(t, token, material.ID, `{"folder_id":`+jsonText(randomCardID)+`}`)

	for name, pair := range map[string][2]response{"a folder move": {folderMove, randomFolderMove}, "a material move": {materialMove, randomMaterialMove}} {
		if pair[0].status != http.StatusNotFound || refusal(t, pair[0]) != refusal(t, pair[1]) {
			t.Fatalf("%s into another tutor's folder answered %s", name, describe(pair[0]))
		}
	}
	if listed := h.libraryPage(t, token, url.Values{}); len(listed.Folders) != 1 || len(listed.Items) != 1 {
		t.Fatalf("a refused move changed the root: %+v", listed)
	}
}

func TestLibrarySearchRefusesTextThatIsNotUTF8(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)

	refused := h.libraryAnswer(t, token, url.Values{"q": {"a\xffb"}})

	if path, field := fieldOf(t, refused); path != "query.q" || field != "invalid" {
		t.Fatalf("a search that is not UTF-8 answered %s", describe(refused))
	}
}

func TestLibraryWritesWaitForAReclaimOnTheUserRow(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountUnverified, 1, tutorEmail)
	reclaim := h.holdRowThen(t, "SELECT "+lockedRowPosition+" FROM users WHERE id = $1::uuid FOR UPDATE", tutor.String(),
		"UPDATE sessions SET revoked_at = now() WHERE user_id = $1::uuid AND revoked_at IS NULL")
	pending := h.sendInBackground(http.MethodPost, libraryFoldersRoute, `{"name":"Unit 1","parent_id":null}`, jsonCall(token))
	h.waitForLockWaiters(t, 1)

	reclaim()
	answer := <-pending

	if answer.err != nil {
		t.Fatal(answer.err)
	}
	if answer.result.status != http.StatusUnauthorized || code(t, answer.result) != "unauthenticated" {
		t.Fatalf("a write that waited on a reclaim answered %s", describe(answer.result))
	}
	if h.countRows(t, "SELECT count(id) FROM folders WHERE tutor_id = $1::uuid", tutor.String()) != 0 {
		t.Fatal("the squatter's write landed after the reclaim")
	}
}
