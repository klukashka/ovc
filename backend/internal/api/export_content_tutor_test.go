package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/schemas"
)

const (
	thirdExportEmail      = "third-export@example.org"
	outsiderTutorName     = "Outsider tutor name"
	outsiderCardName      = "Outsider card name"
	outsiderNotes         = "Outsider private notes"
	outsiderLessonTitle   = "Outsider lesson"
	outsiderMessage       = "Outsider message"
	outsiderReply         = "Reply to the outsider"
	outsiderWorkTitle     = "Outsider work"
	outsiderMaterialTitle = "Outsider material"
	outsiderFilename      = "outsider.pdf"
	secondAccountName     = "Second account name"
	tutorStudentsSheet    = "Students"
	tutorLessonsSheet     = "Lessons"
	tutorFileWorkGroup    = 83
	tutorOutsiderGroup    = 82
	tutorPurgedGroup      = 84
)

var tutorDataKeys = []string{
	"account", "exported_at", "files", "format_version", "groups", "issues", "lessons", "library_folders", "library_items", "messages", "role",
	"series", "students", "submissions", "templates",
}

type tutorDataCopy struct {
	FormatVersion  int                            `json:"format_version"`
	Role           string                         `json:"role"`
	Account        json.RawMessage                `json:"account"`
	Students       []json.RawMessage              `json:"students"`
	Groups         []json.RawMessage              `json:"groups"`
	Lessons        []json.RawMessage              `json:"lessons"`
	Series         []json.RawMessage              `json:"series"`
	Issues         []json.RawMessage              `json:"issues"`
	Submissions    []json.RawMessage              `json:"submissions"`
	LibraryFolders []json.RawMessage              `json:"library_folders"`
	LibraryItems   []json.RawMessage              `json:"library_items"`
	Templates      []json.RawMessage              `json:"templates"`
	Messages       []exportHistory                `json:"messages"`
	Files          []schemas.TutorExportFileEntry `json:"files"`
}

type tutorArchive struct {
	studentArchive
	tutor tutorDataCopy
	keys  []string
}

type tutorMessage struct {
	raw     json.RawMessage
	message schemas.TutorExportMessage
}

func openTutorArchive(t *testing.T, downloaded downloadedArchive) tutorArchive {
	t.Helper()
	archive := tutorArchive{studentArchive: openStudentArchive(t, downloaded)}
	if err := json.Unmarshal(downloaded.entries["data.json"], &archive.tutor); err != nil {
		t.Fatalf("the tutor's data copy does not decode: %v", err)
	}
	for key := range decodedAs[map[string]json.RawMessage](t, downloaded.entries["data.json"]) {
		archive.keys = append(archive.keys, key)
	}
	slices.Sort(archive.keys)
	return archive
}

func (h *authHarness) tutorArchiveOf(t *testing.T, owner exportAccount) (repositories.Export, tutorArchive) {
	t.Helper()
	row := h.readyExportOf(t, owner)
	minted, address := h.mintExport(t, owner.token)
	h.mustAnswer(t, minted, http.StatusOK)
	return row, openTutorArchive(t, downloadArchive(t, address.URL))
}

func decodedItems[Item any](t *testing.T, raws []json.RawMessage) []Item {
	t.Helper()
	items := make([]Item, 0, len(raws))
	for _, raw := range raws {
		items = append(items, decodedAs[Item](t, raw))
	}
	return items
}

func identifierOf(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	return decodedAs[struct {
		ID string `json:"id"`
	}](t, raw).ID
}

func (a tutorArchive) holdsAnywhere(text string) bool {
	if a.holdsText(text) {
		return true
	}
	for name, content := range a.entries {
		if strings.Contains(name, text) || bytes.Contains(content, []byte(text)) {
			return true
		}
	}
	for _, content := range a.parts {
		if bytes.Contains(content, []byte(text)) {
			return true
		}
	}
	return false
}

func (a tutorArchive) heldTexts(texts ...string) []string {
	var held []string
	for _, text := range texts {
		if a.holdsAnywhere(text) {
			held = append(held, text)
		}
	}
	return held
}

func formulaSheets(archive studentArchive) []string {
	var sheets []string
	for name, rows := range archive.sheets {
		for _, row := range rows {
			if slices.ContainsFunc(row, func(cell archiveCell) bool { return cell.Formula != nil }) && !slices.Contains(sheets, name) {
				sheets = append(sheets, name)
			}
		}
	}
	return sheets
}

func linkParts(archive studentArchive) []string {
	var parts []string
	for part := range archive.parts {
		if strings.Contains(part, "hyperlink") || strings.Contains(part, "externalLink") {
			parts = append(parts, part)
		}
	}
	return parts
}

func attachLines(calendar string) []string {
	var lines []string
	for _, line := range strings.Split(calendar, "\r\n") {
		if strings.HasPrefix(line, "ATTACH") {
			lines = append(lines, line)
		}
	}
	return lines
}

func exportRecordsHolding(logs string, texts ...string) []string {
	var lines []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, `"msg":"export.`) && slices.ContainsFunc(texts, func(text string) bool { return strings.Contains(line, text) }) {
			lines = append(lines, line)
		}
	}
	return lines
}

func (a tutorArchive) cards(t *testing.T) map[string]schemas.TutorExportCard {
	t.Helper()
	cards := map[string]schemas.TutorExportCard{}
	for _, card := range decodedItems[schemas.TutorExportCard](t, a.tutor.Students) {
		cards[card.ID] = card
	}
	return cards
}

func (a tutorArchive) lessons(t *testing.T) map[string]schemas.TutorExportLesson {
	t.Helper()
	lessons := map[string]schemas.TutorExportLesson{}
	for _, lesson := range decodedItems[schemas.TutorExportLesson](t, a.tutor.Lessons) {
		lessons[lesson.ID] = lesson
	}
	return lessons
}

func (a tutorArchive) submissions(t *testing.T) map[string]schemas.TutorExportSubmission {
	t.Helper()
	submissions := map[string]schemas.TutorExportSubmission{}
	for _, submission := range decodedItems[schemas.TutorExportSubmission](t, a.tutor.Submissions) {
		submissions[submission.ID] = submission
	}
	return submissions
}

func (a tutorArchive) tutorMessages(t *testing.T) []tutorMessage {
	t.Helper()
	var messages []tutorMessage
	for _, history := range a.tutor.Messages {
		for _, raw := range history.Messages {
			messages = append(messages, tutorMessage{raw: raw, message: decodedAs[schemas.TutorExportMessage](t, raw)})
		}
	}
	return messages
}

func (a tutorArchive) messageWithBody(t *testing.T, body string) (tutorMessage, bool) {
	t.Helper()
	for _, found := range a.tutorMessages(t) {
		if found.message.Body != nil && *found.message.Body == body {
			return found, true
		}
	}
	return tutorMessage{}, false
}

func (a tutorArchive) rowStartingWith(sheet string, first string) []string {
	for _, row := range a.rows(sheet) {
		if len(row) > 0 && row[0] == first {
			return row
		}
	}
	return nil
}

func cellAt(row []string, column int) string {
	if column < len(row) {
		return row[column]
	}
	return ""
}

func (h *authHarness) answeredFileWork(t *testing.T, tutor string, student exportAccount, card string, title string, name string) (string, string) {
	t.Helper()
	question := blockID(tutorFileWorkGroup, 1)
	template := h.workTemplate(t, tutor, title, fileBlock(question, string(domain.AnswerFilePDF)))
	_, submissions := h.issueTemplate(t, tutor, template, toCard(card), studentExportPolicy, nil)
	file := h.answerFile(t, student.token, submissions[0].id, question, name, 80)
	draft := h.saved(t, student.token, submissions[0].id, answersJSON(t, 1, saveID(1), attached(question, file)))
	h.mustAnswer(t, h.submitWork(t, student.token, submissions[0].id, draft.Version), http.StatusOK)
	return submissions[0].id, file
}

func (h *authHarness) sharedWithOutsider(t *testing.T, world exportWorld, student exportAccount) studentCard {
	t.Helper()
	h.mustAnswer(t, h.patchMe(t, world.outsider.token, fmt.Sprintf(`{"name":%q}`, outsiderTutorName)), http.StatusOK)
	card := h.linkedSignedIn(t, world.outsider.token, student.token, outsiderCardName, student.email)
	h.withNotes(t, world.outsider.token, card.ID, outsiderNotes)
	lesson := oneOffFor(card.ID, "2030-03-14", "15:00")
	lesson.Title = outsiderLessonTitle
	h.scheduled(t, world.outsider.token, lesson)
	upload := h.libraryUpload(t, world.outsider.token, domain.ContextLibrary, outsiderFilename, pdfFile(90))
	h.mustAnswer(t, upload, http.StatusCreated)
	material := h.createMaterial(t, world.outsider.token, uploadedID(t, upload), outsiderMaterialTitle, nil).ID
	template := h.workTemplate(t, world.outsider.token, outsiderWorkTitle, materialBlock(blockID(tutorOutsiderGroup, 1), material))
	h.issueTemplate(t, world.outsider.token, template, toCard(card.ID), studentExportPolicy, nil)
	channel := h.directChannel(t, card.ID).ID
	h.sent(t, world.outsider.token, channel, outsiderMessage)
	h.sent(t, student.token, channel, outsiderReply)
	return card
}

func TestTutorExportScope(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.richStudentWorld(t)
	tutor := world.tutor.token
	h.withNotes(t, tutor, world.firstCard.ID, sentinelNotes)
	outsiderCard := h.sharedWithOutsider(t, world.exportWorld, world.first)
	erased := h.workTemplate(t, tutor, "Erased work", instructionBlock(blockID(89, 1), "Read"))
	erasedIssue, _ := h.issueTemplate(t, tutor, erased, toCard(world.firstCard.ID), studentExportPolicy, nil)
	h.markIssueForErasure(t, erasedIssue)
	h.mustAnswer(t, h.deleteTemplate(t, tutor, erased), http.StatusNoContent)
	deletedTemplate := h.workTemplate(t, tutor, "Deleted template", instructionBlock(blockID(89, 2), "Gone"))
	h.mustAnswer(t, h.deleteTemplate(t, tutor, deletedTemplate), http.StatusNoContent)
	deletedUpload := h.libraryUpload(t, tutor, domain.ContextLibrary, "deleted-sheet.pdf", pdfFile(70))
	h.mustAnswer(t, deletedUpload, http.StatusCreated)
	deletedMaterial := h.createMaterial(t, tutor, uploadedID(t, deletedUpload), "Deleted material", nil).ID
	h.mustAnswer(t, h.deleteMaterial(t, tutor, deletedMaterial), http.StatusNoContent)
	h.reviewSaved(t, tutor, world.submission, reviewJSON(t, 2, 0, saveID(3), "Later feedback",
		commented(scored(blockID(exportWorkGroup, exportFileQuestion), 3), "Second look", world.corrected)))
	h.published(t, tutor, world.submission, domain.ReviewCheck, 2)
	second := h.signedInAgain(t, world.second)
	h.mustAnswer(t, h.patchMe(t, second.token, fmt.Sprintf(`{"name":%q}`, secondAccountName)), http.StatusOK)
	_, reclaimAnswer := h.answeredFileWork(t, tutor, second, world.secondCard.ID, "Reclaimed work", "reclaimed-answer.pdf")
	h.setRelationshipState(t, world.secondCard.ID, domain.RelationshipReclaimPending)
	discarded, _ := h.provisionalStudent(t, world.tutor)
	h.mustAnswer(t, h.removeStudent(t, tutor, discarded.ID), http.StatusOK)

	_, archive := h.tutorArchiveOf(t, h.signedInAgain(t, world.tutor))

	leaked := archive.heldTexts(
		outsiderTutorName, outsiderCardName, outsiderNotes, outsiderLessonTitle, outsiderMessage, outsiderReply, outsiderWorkTitle,
		outsiderMaterialTitle, outsiderFilename, outsiderTutorEmail, "Erased work", "Deleted template", "Deleted material", "deleted-sheet.pdf",
		"Overall good", secondAccountName,
	)
	if len(leaked) != 0 {
		t.Errorf("the tutor's export holds %q", leaked)
	}
	cards := archive.cards(t)
	if _, found := cards[outsiderCard.ID]; found {
		t.Error("the tutor's export holds the other tutor's card of the shared student")
	}
	if _, found := cards[discarded.ID]; found {
		t.Error("the tutor's export holds a discarded card")
	}
	reclaiming, found := cards[world.secondCard.ID]
	if !found || reclaiming.State != domain.RelationshipReclaimPending || reclaiming.Account != nil {
		t.Errorf("the reclaim-pending card is exported as %+v", reclaiming)
	}
	if row := archive.rowStartingWith(tutorStudentsSheet, "Second student"); row == nil || cellAt(row, 4) != "" || cellAt(row, 5) != "" {
		t.Errorf("the reclaim-pending card's row is %q", row)
	}
	if slices.Contains(archive.archivedAttachments(), reclaimAnswer) {
		t.Error("an answer file of a reclaim-pending link is archived")
	}
	if first, found := cards[world.firstCard.ID]; !found || first.Notes != sentinelNotes {
		t.Error("the tutor's export lacks the tutor's own card with its notes")
	}
	work, found := archive.submissions(t)[world.submission]
	if !found || len(work.Revisions) != 1 || work.Revisions[0].Revision != 2 || !archive.holdsText("Later feedback") {
		t.Errorf("the reviewed submission is exported with the revisions %+v", work.Revisions)
	}
}

func TestTutorExportLabels(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	third := h.exportAccountAs(t, domain.RoleStudent, 4, thirdExportEmail)
	thirdCard := h.linkedSignedIn(t, world.tutor.token, third.token, "Third student", thirdExportEmail)
	purgedLesson := oneOffFor(thirdCard.ID, "2030-03-12", "10:00")
	purgedLesson.Title = "Lesson of the purged student"
	former := h.scheduled(t, world.tutor.token, purgedLesson).Lessons[0].ID
	provisional, _ := h.provisionalStudent(t, world.tutor)
	removedLesson := oneOffFor(provisional.ID, "2030-03-12", "12:00")
	removedLesson.Title = "Lesson of the removed link"
	removed := h.scheduled(t, world.tutor.token, removedLesson).Lessons[0].ID
	h.mustAnswer(t, h.removeStudent(t, world.tutor.token, provisional.ID), http.StatusOK)
	h.mustAnswer(t, h.deleteMe(t, third.token, third.password), http.StatusNoContent)
	h.tickAt(t, h.deletedAtOf(t, third.id).Add(domain.DeletedAccountRetention))
	tutor, first, second := h.signedInAgain(t, world.tutor), h.signedInAgain(t, world.first), h.signedInAgain(t, world.second)
	rosterLesson := groupLessonFor(world.group.ID, "2030-05-20", "11:00")
	rosterLesson.Title = "Group lesson with a deleted member"
	roster := h.scheduled(t, tutor.token, rosterLesson).Lessons[0].ID
	h.cancelWithRoster(t, roster, world.first.id, world.second.id)
	group := h.groupChannel(t, world.group.ID).ID
	own := h.sent(t, tutor.token, group, "The tutor in the group")
	h.sent(t, first.token, group, "First in the group")
	h.sent(t, second.token, group, "Sent before deletion")
	h.mustAnswer(t, h.react(t, first.token, own.ID, string(domain.ReactionHeart)), http.StatusNoContent)
	h.mustAnswer(t, h.react(t, second.token, own.ID, string(domain.ReactionThumbsUp)), http.StatusNoContent)
	h.mustAnswer(t, h.deleteMe(t, second.token, second.password), http.StatusNoContent)

	_, archive := h.tutorArchiveOf(t, tutor)

	if row := archive.messageRow("First in the group"); row == nil || row[2] != "First student" {
		t.Errorf("a student's message row is %q, want the card's display name", row)
	}
	if row := archive.messageRow("The tutor in the group"); row == nil || row[2] != "You" {
		t.Errorf("the tutor's own message row is %q", row)
	}
	if row := archive.messageRow("Sent before deletion"); row == nil || row[2] != "Deleted user" {
		t.Errorf("a deleted sender's message row is %q", row)
	}
	deleted, found := archive.messageWithBody(t, "Sent before deletion")
	if !found || deleted.message.SenderLabelKind == nil || *deleted.message.SenderLabelKind != domain.LabelDeletedUser {
		t.Error("the data copy does not label the deleted sender as deleted_user")
	}
	reacted, found := archive.messageWithBody(t, "The tutor in the group")
	var members []schemas.ReactionMember
	if found {
		for _, reaction := range reacted.message.Reactions {
			if reaction.Members != nil {
				members = append(members, *reaction.Members...)
			}
		}
	}
	labelled := slices.ContainsFunc(members, func(member schemas.ReactionMember) bool {
		return member.Label != nil && *member.Label == "First student"
	})
	if !found || !labelled {
		t.Errorf("the reactions on the tutor's message carry the members %+v", members)
	}
	identified := slices.ContainsFunc([]domain.UserID{world.first.id, world.second.id, third.id}, func(account domain.UserID) bool {
		return bytes.Contains(reacted.raw, []byte(account.String()))
	})
	if identified {
		t.Errorf("a reacted message in the tutor's export carries an account identifier: %s", reacted.raw)
	}
	lessons := archive.lessons(t)
	if kind := lessons[removed].Audience.LabelKind; kind == nil || *kind != domain.LabelRemovedStudent {
		t.Errorf("the removed link's lesson audience is %+v", lessons[removed].Audience)
	}
	if kind := lessons[former].Audience.LabelKind; kind == nil || *kind != domain.LabelFormerStudent {
		t.Errorf("the purged student's lesson audience is %+v", lessons[former].Audience)
	}
	if row := archive.rowStartingWith(tutorLessonsSheet, "Lesson of the removed link"); cellAt(row, 6) != "Removed student" {
		t.Errorf("the removed link's lesson row is %q", row)
	}
	if row := archive.rowStartingWith(tutorLessonsSheet, "Lesson of the purged student"); cellAt(row, 6) != "Former student" {
		t.Errorf("the purged student's lesson row is %q", row)
	}
	deletedMember := slices.ContainsFunc(lessons[roster].Audience.Members, func(member schemas.LessonMember) bool {
		return member.Label != nil && *member.Label == "Second student" && member.LabelKind != nil && *member.LabelKind == domain.LabelDeletedAccount
	})
	if !deletedMember {
		t.Errorf("the deleted member of the group lesson is exported as %+v", lessons[roster].Audience.Members)
	}
}

type pathRule struct {
	routeOnly  []string
	fileKey    string
	invitation bool
}

func jsonPaths(t *testing.T, raw json.RawMessage, prefix string, into map[string]bool) {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return
	}
	switch trimmed[0] {
	case '{':
		for key, value := range decodedAs[map[string]json.RawMessage](t, trimmed) {
			path := prefix + "." + key
			into[path] = true
			jsonPaths(t, value, path, into)
		}
	case '[':
		for _, item := range decodedAs[[]json.RawMessage](t, trimmed) {
			jsonPaths(t, item, prefix+"[]", into)
		}
	}
}

func (rule pathRule) routeOnlyPath(path string) bool {
	return slices.ContainsFunc(rule.routeOnly, func(key string) bool {
		return path == "."+key || strings.HasPrefix(path, "."+key+".") || strings.HasPrefix(path, "."+key+"[]")
	})
}

func (rule pathRule) routeShape(t *testing.T, raws []json.RawMessage) []string {
	t.Helper()
	paths := map[string]bool{}
	for _, raw := range raws {
		jsonPaths(t, raw, "", paths)
	}
	shaped := map[string]bool{}
	for path := range paths {
		last := path[strings.LastIndex(path, ".")+1:]
		switch {
		case rule.routeOnlyPath(path), rule.invitation && path == ".invitation.url":
		case rule.invitation && path == ".invitation.expires_at":
			shaped[path] = true
		case last == "expires_at":
		case last == "url":
			shaped[strings.TrimSuffix(path, "url")+rule.fileKey] = true
		default:
			shaped[path] = true
		}
	}
	return sortedKeys(shaped)
}

func exportShape(t *testing.T, raws []json.RawMessage) []string {
	t.Helper()
	paths := map[string]bool{}
	for _, raw := range raws {
		jsonPaths(t, raw, "", paths)
	}
	return sortedKeys(paths)
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func (h *authHarness) routedObject(t *testing.T, token string, path string) json.RawMessage {
	t.Helper()
	result := h.send(t, http.MethodGet, path, "", jsonCall(token))
	if result.status != http.StatusOK {
		t.Fatalf("%s answered %d %s", path, result.status, result.body)
	}
	return json.RawMessage(result.body)
}

func (h *authHarness) routedByID(t *testing.T, token string, route string, items []json.RawMessage) []json.RawMessage {
	t.Helper()
	routed := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		routed = append(routed, h.routedObject(t, token, route+"/"+identifierOf(t, item)))
	}
	return routed
}

func (h *authHarness) pagedItems(t *testing.T, token string, path string) []json.RawMessage {
	t.Helper()
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	var items []json.RawMessage
	cursor := ""
	for {
		query := separator + "limit=100"
		if cursor != "" {
			query += "&cursor=" + url.QueryEscape(cursor)
		}
		body := h.routeBody(t, token, path+query)
		items = append(items, decodedAs[[]json.RawMessage](t, body["items"])...)
		next := decodedAs[*string](t, body["next_cursor"])
		if next == nil {
			return items
		}
		cursor = *next
	}
}

func (h *authHarness) routedLibraryItems(t *testing.T, token string) []json.RawMessage {
	t.Helper()
	items := h.pagedItems(t, token, libraryRoot)
	for _, folder := range h.routeArray(t, token, libraryFoldersRoute, "folders") {
		items = append(items, h.pagedItems(t, token, libraryRoot+"?folder="+identifierOf(t, folder))...)
	}
	return items
}

func templatesOf(t *testing.T, items []json.RawMessage) []json.RawMessage {
	t.Helper()
	var templates []json.RawMessage
	for _, item := range items {
		if decodedAs[struct {
			Kind string `json:"kind"`
		}](t, item).Kind == string(domain.LibraryItemTemplate) {
			templates = append(templates, item)
		}
	}
	return templates
}

func historyMessages(histories []exportHistory) []json.RawMessage {
	var messages []json.RawMessage
	for _, history := range histories {
		messages = append(messages, history.Messages...)
	}
	return messages
}

func TestTutorExportFollowsTheTutorSchemas(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.richStudentWorld(t)
	tutor := h.signedInAgain(t, world.tutor)

	_, archive := h.tutorArchiveOf(t, tutor)

	token := tutor.token
	if !slices.Equal(archive.keys, tutorDataKeys) || archive.tutor.Role != string(domain.RoleTutor) {
		t.Fatalf("the tutor's data copy has the keys %v and the role %q", archive.keys, archive.tutor.Role)
	}
	work, found := archive.submissions(t)[world.submission]
	if !found || work.FinalPoints == nil || work.MaxPoints == 0 || len(work.Revisions) != 1 || len(work.Revisions[0].Items) == 0 ||
		work.Revisions[0].Items[0].Points == nil {
		t.Errorf("the checked submission lacks its scores and points: %+v", work)
	}
	var keyed bool
	for _, template := range decodedItems[schemas.TutorExportTemplate](t, archive.tutor.Templates) {
		for _, block := range template.Blocks {
			if block.Options != nil && block.Points != nil && slices.ContainsFunc(*block.Options, func(option schemas.TemplateOption) bool {
				return option.Correct != nil && *option.Correct
			}) {
				keyed = true
			}
		}
	}
	if !keyed {
		t.Error("no exported template block carries its correct option and points")
	}
	routedItems := h.routedLibraryItems(t, token)
	shapes := []struct {
		label    string
		exported []json.RawMessage
		routed   []json.RawMessage
		rule     pathRule
	}{
		{"students", archive.tutor.Students, h.routedByID(t, token, studentsRoute, archive.tutor.Students),
			pathRule{routeOnly: []string{"next_lessons"}, fileKey: "path", invitation: true}},
		{"groups", archive.tutor.Groups, h.routedByID(t, token, groupsRoute, archive.tutor.Groups),
			pathRule{routeOnly: []string{"next_lessons", "future_lesson_count", "has_open_series"}, fileKey: "path"}},
		{"lessons", archive.tutor.Lessons, h.routedByID(t, token, lessonsRoute, archive.tutor.Lessons), pathRule{routeOnly: []string{"issues"}, fileKey: "path"}},
		{"issues", archive.tutor.Issues, h.routedByID(t, token, issuesRoute, archive.tutor.Issues), pathRule{fileKey: "path"}},
		{"submissions", archive.tutor.Submissions, h.routedByID(t, token, strings.TrimSuffix(submissionsRoute, "/"), archive.tutor.Submissions),
			pathRule{fileKey: "path"}},
		{"templates", archive.tutor.Templates, h.routedByID(t, token, libraryTemplatesRoute, templatesOf(t, routedItems)), pathRule{fileKey: "path"}},
		{"library items", archive.tutor.LibraryItems, routedItems, pathRule{fileKey: "file_path"}},
		{"messages", historyMessages(archive.tutor.Messages), h.routedHistories(t, token, archive.tutor.Messages), pathRule{fileKey: "path"}},
	}
	var differing []string
	for _, shape := range shapes {
		exported, routed := exportShape(t, shape.exported), shape.rule.routeShape(t, shape.routed)
		if !slices.Equal(exported, routed) {
			differing = append(differing, fmt.Sprintf("%s:\nexport %v\nroute  %v", shape.label, exported, routed))
		}
	}
	if len(differing) != 0 {
		t.Errorf("the export's keys differ from the routes':\n%s", strings.Join(differing, "\n"))
	}
}

func (h *authHarness) routedHistories(t *testing.T, token string, histories []exportHistory) []json.RawMessage {
	t.Helper()
	var messages []json.RawMessage
	for _, history := range histories {
		messages = append(messages, h.routedHistory(t, token, identifierOf(t, history.Channel))...)
	}
	return messages
}

func latestPublishedOnly(t *testing.T, raw json.RawMessage) json.RawMessage {
	t.Helper()
	fields := decodedAs[map[string]json.RawMessage](t, raw)
	var latest json.RawMessage
	latestRevision := int32(-1)
	for _, revision := range decodedAs[[]json.RawMessage](t, fields["revisions"]) {
		header := decodedAs[struct {
			Revision int32  `json:"revision"`
			State    string `json:"state"`
		}](t, revision)
		if header.State == string(domain.ReviewPublished) && header.Revision > latestRevision {
			latest, latestRevision = revision, header.Revision
		}
	}
	kept := []json.RawMessage{}
	if latest != nil {
		kept = append(kept, latest)
	}
	encodedKept, err := json.Marshal(kept)
	if err != nil {
		t.Fatal(err)
	}
	fields["revisions"] = encodedKept
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func unavailableAttachments(t *testing.T, raw json.RawMessage, fileKey string) []string {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	var found []string
	switch trimmed[0] {
	case '{':
		fields := decodedAs[map[string]json.RawMessage](t, trimmed)
		identifier, identified := fields["attachment_id"]
		if !identified && fields["filename"] != nil {
			identifier, identified = fields["id"]
		}
		if value, has := fields[fileKey]; identified && has && string(bytes.TrimSpace(value)) == "null" {
			found = append(found, decodedAs[string](t, identifier))
		}
		for _, value := range fields {
			found = append(found, unavailableAttachments(t, value, fileKey)...)
		}
	case '[':
		for _, item := range decodedAs[[]json.RawMessage](t, trimmed) {
			found = append(found, unavailableAttachments(t, item, fileKey)...)
		}
	}
	return found
}

type unavailableFile struct {
	attachment string
	query      string
}

func referencedUnavailable(t *testing.T, raw json.RawMessage, fileKey string, kind domain.ReferenceKind, item string) []unavailableFile {
	t.Helper()
	query := "?ref=" + string(kind) + ":" + item
	if kind == domain.ReferenceMessage {
		query = ""
	}
	var files []unavailableFile
	for _, attachment := range unavailableAttachments(t, raw, fileKey) {
		files = append(files, unavailableFile{attachment: attachment, query: query})
	}
	return files
}

func (h *authHarness) unavailableFilesOf(t *testing.T, data tutorDataCopy) []unavailableFile {
	t.Helper()
	var files []unavailableFile
	for _, raw := range data.Lessons {
		files = append(files, referencedUnavailable(t, raw, "path", domain.ReferenceLesson, identifierOf(t, raw))...)
	}
	for _, raw := range data.Issues {
		files = append(files, referencedUnavailable(t, raw, "path", domain.ReferenceIssueSnapshot, identifierOf(t, raw))...)
	}
	for _, raw := range data.Templates {
		files = append(files, referencedUnavailable(t, raw, "path", domain.ReferenceTemplate, identifierOf(t, raw))...)
	}
	for _, raw := range data.LibraryItems {
		files = append(files, referencedUnavailable(t, raw, "file_path", domain.ReferenceMaterial, identifierOf(t, raw))...)
	}
	for _, raw := range historyMessages(data.Messages) {
		files = append(files, referencedUnavailable(t, raw, "path", domain.ReferenceMessage, identifierOf(t, raw))...)
	}
	for _, raw := range data.Submissions {
		fields := decodedAs[map[string]json.RawMessage](t, raw)
		submission := decodedAs[string](t, fields["id"])
		files = append(files, referencedUnavailable(t, fields["blocks"], "path", domain.ReferenceIssueSnapshot, decodedAs[string](t, fields["issue_id"]))...)
		files = append(files, referencedUnavailable(t, fields["answers"], "path", domain.ReferenceAnswer, submission)...)
		for _, revision := range decodedAs[[]schemas.TutorExportRevision](t, fields["revisions"]) {
			items, err := json.Marshal(revision.Items)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, referencedUnavailable(t, items, "path", domain.ReferenceReviewItem, h.revisionID(t, submission, revision.Revision))...)
		}
	}
	return files
}

func TestTutorExportMatchesTheRoutes(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.richStudentWorld(t)
	tutor := world.tutor.token
	folder := h.createFolder(t, tutor, "Export folder", nil)
	folderUpload := h.libraryUpload(t, tutor, domain.ContextLibrary, "folder-sheet.pdf", pdfFile(60))
	h.mustAnswer(t, folderUpload, http.StatusCreated)
	h.createMaterial(t, tutor, uploadedID(t, folderUpload), "Folder sheet", &folder.ID)
	h.attachToLesson(t, tutor, world.lesson, world.material)
	h.scheduled(t, tutor, oneOffFor(world.firstCard.ID, mondaySeriesStart, "16:00").weekly([]int{monday}, 3))
	withdrawn := h.workTemplate(t, tutor, "Withdrawn work", fileBlock(blockID(90, 1), string(domain.AnswerFilePDF)))
	withdrawnIssue, _ := h.issueTemplate(t, tutor, withdrawn, toCard(world.firstCard.ID), studentExportPolicy, nil)
	h.mustAnswer(t, h.withdrawIssue(t, tutor, withdrawnIssue), http.StatusOK)
	_, reclaimAnswer := h.answeredFileWork(t, tutor, h.signedInAgain(t, world.second), world.secondCard.ID, "Reclaimed work", "reclaimed-answer.pdf")
	h.setRelationshipState(t, world.secondCard.ID, domain.RelationshipReclaimPending)
	owner := h.signedInAgain(t, world.tutor)

	_, archive := h.tutorArchiveOf(t, owner)

	token := owner.token
	data := archive.tutor
	cards := h.pagedItems(t, token, studentsRoute)
	assertSameItems(t, "students", data.Students, h.routedByID(t, token, studentsRoute, cards), "next_lessons")
	groups := h.routedByID(t, token, groupsRoute, h.routeArray(t, token, groupsRoute, "groups"))
	assertSameItems(t, "groups", data.Groups, groups, "next_lessons", "future_lesson_count", "has_open_series")
	var lessons, series []json.RawMessage
	seenSeries := map[string]bool{}
	for _, id := range h.calendarIDs(t, token, authStart.Add(-24*time.Hour), authStart.Add(60*24*time.Hour)) {
		lesson := h.routedObject(t, token, lessonsRoute+"/"+id)
		lessons = append(lessons, lesson)
		if raw := decodedAs[map[string]json.RawMessage](t, lesson)["series"]; string(raw) != "null" && !seenSeries[identifierOf(t, raw)] {
			seenSeries[identifierOf(t, raw)] = true
			series = append(series, raw)
		}
	}
	assertSameItems(t, "lessons", data.Lessons, lessons, "issues")
	assertSameItems(t, "series", data.Series, series)
	listed := append(h.pagedItems(t, token, issuesRoute+"?status=active"), h.pagedItems(t, token, issuesRoute+"?status=withdrawn")...)
	issues := h.routedByID(t, token, issuesRoute, listed)
	assertSameItems(t, "issues", data.Issues, issues)
	var submissions []json.RawMessage
	for _, issue := range issues {
		for _, submission := range decodedAs[[]json.RawMessage](t, decodedAs[map[string]json.RawMessage](t, issue)["submissions"]) {
			submissions = append(submissions, latestPublishedOnly(t, h.routedObject(t, token, submissionsRoute+identifierOf(t, submission))))
		}
	}
	assertSameItems(t, "submissions", data.Submissions, submissions)
	assertSameItems(t, "library folders", data.LibraryFolders, h.routeArray(t, token, libraryFoldersRoute, "folders"))
	items := h.routedLibraryItems(t, token)
	assertSameItems(t, "library items", data.LibraryItems, items, "file_path", "url", "expires_at")
	assertSameItems(t, "templates", data.Templates, h.routedByID(t, token, libraryTemplatesRoute, templatesOf(t, items)))
	channels := h.pagedItems(t, token, channelsRoute)
	exportedChannels := make([]json.RawMessage, 0, len(data.Messages))
	for _, history := range data.Messages {
		assertExportedPeerReadIsNull(t, history.Channel)
		exportedChannels = append(exportedChannels, history.Channel)
	}
	assertSameItems(t, "channels", exportedChannels, channels, "peer_read")
	var exportedMessages, routedMessages []json.RawMessage
	for _, channel := range channels {
		history, _ := archive.historyOf(identifierOf(t, channel))
		exportedMessages = append(exportedMessages, history.Messages...)
		routedMessages = append(routedMessages, h.routedHistory(t, token, identifierOf(t, channel))...)
	}
	assertSameItems(t, "messages", exportedMessages, routedMessages)
	unavailable := h.unavailableFilesOf(t, data)
	var served []string
	for _, file := range unavailable {
		if answer, _ := h.addressOf(t, token, file.attachment, file.query); answer.status != http.StatusNotFound {
			served = append(served, fmt.Sprintf("%s%s answered %d", file.attachment, file.query, answer.status))
		}
	}
	t.Logf("the tutor's data copy lists %d files as not available", len(unavailable))
	if len(served) != 0 {
		t.Errorf("the routes mint files the export lists as not available: %v", served)
	}
	if slices.Contains(archive.archivedAttachments(), reclaimAnswer) {
		t.Error("the answer file of a reclaim-pending link is archived")
	}
}

func TestTutorExportFormats(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.exportWorld(t)
	direct := h.directChannel(t, world.firstCard.ID).ID
	h.sent(t, world.first.token, direct, "=1+2")
	h.sent(t, world.first.token, direct, "<b>&amp;</t>")
	h.withNotes(t, world.tutor.token, world.firstCard.ID, `=HYPERLINK("http://example.org")`)
	description := "Line one\nATTACH:http://example.org/x"
	lesson := oneOffFor(world.firstCard.ID, "2030-03-12", "10:00")
	lesson.Title, lesson.Description = "Plan; ATTACH:x", &description
	h.scheduled(t, world.tutor.token, lesson)
	question := blockID(87, 1)
	template := h.workTemplate(t, world.tutor.token, "Files", fileBlock(question, string(domain.AnswerFilePDF)))
	_, submissions := h.issueTemplate(t, world.tutor.token, template, toCard(world.firstCard.ID), studentExportPolicy, nil)
	first := h.answerFile(t, world.first.token, submissions[0].id, question, "../x.pdf", 64)
	second := h.answerFile(t, world.first.token, submissions[0].id, question, "../x.pdf", 65)
	draft := h.saved(t, world.first.token, submissions[0].id, answersJSON(t, 1, saveID(1), attached(question, first, second)))
	h.mustAnswer(t, h.submitWork(t, world.first.token, submissions[0].id, draft.Version), http.StatusOK)

	_, archive := h.tutorArchiveOf(t, h.signedInAgain(t, world.tutor))

	formula, markup := archive.messageRow("=1+2"), archive.messageRow("<b>&amp;</t>")
	if formula == nil || markup == nil {
		t.Fatalf("the messages sheet lost formula-like or markup text: %q %q", formula, markup)
	}
	if row := archive.rowStartingWith(tutorStudentsSheet, "First student"); cellAt(row, 7) != `=HYPERLINK("http://example.org")` {
		t.Errorf("the formula-like notes are stored as %q", row)
	}
	if sheets := formulaSheets(archive.studentArchive); len(sheets) != 0 {
		t.Errorf("the sheets %v hold a formula", sheets)
	}
	if parts := linkParts(archive.studentArchive); len(parts) != 0 {
		t.Errorf("the workbook holds the parts %v", parts)
	}
	if lines := attachLines(archive.calendar); len(lines) != 0 {
		t.Errorf("the calendar holds the property lines %q", lines)
	}
	if !strings.Contains(archive.calendar, "SUMMARY:Plan\\; ATTACH:x") {
		t.Errorf("the lesson title did not stay inside SUMMARY:\n%s", archive.calendar)
	}
	if archive.entries["files/answers/"+first+"-x.pdf"] == nil || archive.entries["files/answers/"+second+"-x.pdf"] == nil {
		t.Errorf("the answer files %s and %s are not stored under their safe names: %v", first, second, archive.entryNames())
	}
}

func forbiddenKeys(t *testing.T, raw json.RawMessage, parent string) []string {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	var found []string
	switch trimmed[0] {
	case '{':
		for key, value := range decodedAs[map[string]json.RawMessage](t, trimmed) {
			if key == "url" || (key == "expires_at" && parent != "invitation") {
				found = append(found, parent+"."+key)
			}
			found = append(found, forbiddenKeys(t, value, key)...)
		}
	case '[':
		for _, item := range decodedAs[[]json.RawMessage](t, trimmed) {
			found = append(found, forbiddenKeys(t, item, parent)...)
		}
	}
	return found
}

func TestTutorExportHoldsNoSignedAddress(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.richStudentWorld(t)
	invited := h.newCard(t, world.tutor.token, "Invited student", "invited-export@example.org")
	invitationToken := linkToken(t, invited.Invitation)

	_, archive := h.tutorArchiveOf(t, h.signedInAgain(t, world.tutor))

	if held := archive.heldTexts(invitationToken, "/invite/", "X-Amz-Signature", "X-Amz-Credential", "/api/v1/files/", "?t="); len(held) != 0 {
		t.Errorf("the tutor's export holds %q", held)
	}
	if records := exportRecordsHolding(h.logs.String(), invitationToken, "/invite/"); len(records) != 0 {
		t.Errorf("export records carry the invitation link: %v", records)
	}
	if keys := forbiddenKeys(t, archive.entries["data.json"], ""); len(keys) != 0 {
		t.Errorf("the tutor's data copy holds address keys %v", keys)
	}
	card, found := archive.cards(t)[invited.ID]
	if !found || card.Invitation == nil || card.Invitation.Status != domain.InvitationOpen {
		t.Fatalf("the invited card is exported as %+v", card)
	}
	var invitationKeys []string
	for _, raw := range archive.tutor.Students {
		if identifierOf(t, raw) == invited.ID {
			invitationKeys = sortedKeys(keySetOf(t, decodedAs[map[string]json.RawMessage](t, raw)["invitation"]))
		}
	}
	if !slices.Equal(invitationKeys, []string{"expires_at", "status"}) {
		t.Errorf("the invited card's invitation holds the keys %v", invitationKeys)
	}
	if len(archive.tutor.Files) == 0 {
		t.Fatal("the rich world exported no files to the tutor")
	}
}

func keySetOf(t *testing.T, raw json.RawMessage) map[string]bool {
	t.Helper()
	keys := map[string]bool{}
	for key := range decodedAs[map[string]json.RawMessage](t, raw) {
		keys[key] = true
	}
	return keys
}

func TestATutorsExportBuiltAfterAStudentsDeletionExpiresAtItsPurgeDueTime(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	direct := h.directChannel(t, world.secondCard.ID).ID
	h.sent(t, world.second.token, direct, "Before my deletion")
	h.mustAnswer(t, h.deleteMe(t, world.second.token, world.second.password), http.StatusNoContent)
	due := h.deletedAtOf(t, world.second.id).Add(domain.DeletedAccountRetention)
	h.at(t, due.Add(-24*time.Hour))

	row, archive := h.tutorArchiveOf(t, h.signedInAgain(t, world.tutor))
	h.at(t, due.Add(-time.Minute))
	early, _ := h.mintExport(t, h.signedInAgain(t, world.tutor).token)
	h.at(t, due)
	late, _ := h.mintExport(t, h.signedInAgain(t, world.tutor).token)
	h.tick(t)
	h.tick(t)

	if row.ExpiresAt == nil || !row.ExpiresAt.Equal(due) {
		t.Fatalf("the tutor's export expires at %v, want the student's purge due time %v", row.ExpiresAt, due)
	}
	card, found := archive.cards(t)[world.secondCard.ID]
	if !found || card.DisplayName != "Second student" || card.EndedBy == nil || *card.EndedBy != domain.EndedByDeletion {
		t.Errorf("the deleted student's card is exported as %+v", card)
	}
	if message := archive.messageRow("Before my deletion"); message == nil || message[2] != "Deleted user" {
		t.Errorf("the deleted student's message row is %q", message)
	}
	if early.status != http.StatusOK || late.status != http.StatusNotFound {
		t.Fatalf("minting answered %d a minute before the due time and %d at it", early.status, late.status)
	}
	expired, _ := h.exportOf(t, world.tutor.id)
	if expired.State != domain.ExportExpired || h.objectExists(t, *row.StorageKey) {
		t.Fatalf("at the due time the tutor's export is %s with its object kept %v", expired.State, h.objectExists(t, *row.StorageKey))
	}
}

func TestATutorsExportAfterAStudentsPurgeHoldsOnlyTheRelabelledHistory(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.exportWorld(t)
	direct := h.directChannel(t, world.secondCard.ID).ID
	upload := h.chatUpload(t, world.second.token, direct, "purged.txt", plainText(64))
	h.mustAnswer(t, upload, http.StatusCreated)
	chatFile := uploadedID(t, upload)
	h.sent(t, world.second.token, direct, "My last words", chatFile)
	h.sent(t, world.tutor.token, direct, "The tutor stays")
	lesson := oneOffFor(world.secondCard.ID, "2030-03-12", "10:00")
	lesson.Title = "Lesson of the purged student"
	purgedLesson := h.scheduled(t, world.tutor.token, lesson).Lessons[0].ID
	question := blockID(tutorPurgedGroup, 1)
	template := h.workTemplate(t, world.tutor.token, "Purged work", fileBlock(question, string(domain.AnswerFilePDF)))
	_, submissions := h.issueTemplate(t, world.tutor.token, template, toCard(world.secondCard.ID), studentExportPolicy, nil)
	answer := h.answerFile(t, world.second.token, submissions[0].id, question, "purged-answer.pdf", 64)
	draft := h.saved(t, world.second.token, submissions[0].id, answersJSON(t, 1, saveID(1), attached(question, answer)))
	h.mustAnswer(t, h.submitWork(t, world.second.token, submissions[0].id, draft.Version), http.StatusOK)
	h.mustAnswer(t, h.deleteMe(t, world.second.token, world.second.password), http.StatusNoContent)
	h.tickAt(t, h.deletedAtOf(t, world.second.id).Add(domain.DeletedAccountRetention))

	row, archive := h.tutorArchiveOf(t, h.signedInAgain(t, world.tutor))

	if h.userExists(t, secondExportEmail) {
		t.Fatal("the student was not purged")
	}
	if _, found := archive.cards(t)[world.secondCard.ID]; found {
		t.Error("the tutor's export holds the purged student's discarded card")
	}
	if kind := archive.lessons(t)[purgedLesson].Audience.LabelKind; kind == nil || *kind != domain.LabelFormerStudent {
		t.Errorf("the purged student's lesson audience is %+v", archive.lessons(t)[purgedLesson].Audience)
	}
	if lessonRow := archive.rowStartingWith(tutorLessonsSheet, "Lesson of the purged student"); cellAt(lessonRow, 6) != "Former student" {
		t.Errorf("the purged student's lesson row is %q", lessonRow)
	}
	if archive.messageRow(removedMessageText) == nil || archive.messageRow("The tutor stays") == nil {
		t.Error("the tutor's export lacks the relabelled history of the direct chat")
	}
	if held := archive.heldTexts("My last words", "purged.txt", "purged-answer.pdf", "Second student", secondExportEmail); len(held) != 0 {
		t.Errorf("the tutor's export after the purge holds %q", held)
	}
	archived := archive.archivedAttachments()
	if slices.Contains(archived, chatFile) || slices.Contains(archived, answer) {
		t.Error("the tutor's export holds a file of the purged student")
	}
	if row.ExpiresAt == nil || row.BuiltAt == nil || !row.ExpiresAt.Equal(row.BuiltAt.Add(domain.ExportRetention)) {
		t.Errorf("the export after the purge expires at %v, want three days after %v", row.ExpiresAt, row.BuiltAt)
	}
}
