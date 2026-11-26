package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/schemas"
)

const (
	studentExportPolicy = string(domain.AnswerPolicyAfterCheck)
	exportFileQuestion  = 2
	exportWorkGroup     = 88
)

type archiveCell struct {
	Reference string  `xml:"r,attr"`
	Type      string  `xml:"t,attr"`
	Formula   *string `xml:"f"`
	Value     string  `xml:"v"`
	Inline    string  `xml:"is>t"`
}

type archiveRow struct {
	Cells []archiveCell `xml:"c"`
}

type archiveWorksheet struct {
	Rows []archiveRow `xml:"sheetData>row"`
}

type archiveSheetNames struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
	} `xml:"sheets>sheet"`
}

type exportHistory struct {
	Channel  json.RawMessage   `json:"channel"`
	Messages []json.RawMessage `json:"messages"`
}

type studentDataCopy struct {
	FormatVersion int                              `json:"format_version"`
	Role          string                           `json:"role"`
	Tutors        []json.RawMessage                `json:"tutors"`
	Lessons       []json.RawMessage                `json:"lessons"`
	Homework      []json.RawMessage                `json:"homework"`
	Submissions   []json.RawMessage                `json:"submissions"`
	Messages      []exportHistory                  `json:"messages"`
	Files         []schemas.StudentExportFileEntry `json:"files"`
}

type studentArchive struct {
	entries  map[string][]byte
	parts    map[string][]byte
	sheets   map[string][][]archiveCell
	calendar string
	data     studentDataCopy
}

func unzipped(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string][]byte{}
	for _, file := range reader.File {
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(opened)
		if err := errors.Join(readErr, opened.Close()); err != nil {
			t.Fatal(err)
		}
		parts[file.Name] = content
	}
	return parts
}

func openStudentArchive(t *testing.T, downloaded downloadedArchive) studentArchive {
	t.Helper()
	if downloaded.status != http.StatusOK {
		t.Fatalf("the archive download answered %d", downloaded.status)
	}
	archive := studentArchive{entries: downloaded.entries, sheets: map[string][][]archiveCell{}}
	archive.parts = unzipped(t, downloaded.entries["tables.xlsx"])
	var names archiveSheetNames
	if err := xml.Unmarshal(archive.parts["xl/workbook.xml"], &names); err != nil {
		t.Fatal(err)
	}
	for index, sheet := range names.Sheets {
		var parsed archiveWorksheet
		if err := xml.Unmarshal(archive.parts[fmt.Sprintf("xl/worksheets/sheet%d.xml", index+1)], &parsed); err != nil {
			t.Fatal(err)
		}
		rows := make([][]archiveCell, 0, len(parsed.Rows))
		for _, row := range parsed.Rows {
			rows = append(rows, row.Cells)
		}
		archive.sheets[sheet.Name] = rows
	}
	archive.calendar = strings.ReplaceAll(string(downloaded.entries["calendar.ics"]), "\r\n ", "")
	if err := json.Unmarshal(downloaded.entries["data.json"], &archive.data); err != nil {
		t.Fatalf("the data copy does not decode: %v", err)
	}
	return archive
}

func cellText(cell archiveCell) string {
	if cell.Type == "inlineStr" {
		return cell.Inline
	}
	return cell.Value
}

func columnOf(reference string) int {
	column := 0
	for _, letter := range reference {
		if letter < 'A' || letter > 'Z' {
			break
		}
		column = column*26 + int(letter-'A') + 1
	}
	return column - 1
}

func (a studentArchive) rows(name string) [][]string {
	var rows [][]string
	for index, row := range a.sheets[name] {
		if index == 0 {
			continue
		}
		var texts []string
		for _, cell := range row {
			column := columnOf(cell.Reference)
			for len(texts) <= column {
				texts = append(texts, "")
			}
			texts[column] = cellText(cell)
		}
		rows = append(rows, texts)
	}
	return rows
}

func (a studentArchive) cellTexts() []string {
	var texts []string
	for _, rows := range a.sheets {
		for _, row := range rows {
			for _, cell := range row {
				texts = append(texts, cellText(cell))
			}
		}
	}
	return texts
}

func (a studentArchive) holdsText(text string) bool {
	if bytes.Contains(a.entries["data.json"], []byte(text)) || strings.Contains(a.calendar, text) {
		return true
	}
	return slices.ContainsFunc(a.cellTexts(), func(cell string) bool { return strings.Contains(cell, text) })
}

func (a studentArchive) messages(t *testing.T) []schemas.StudentExportMessage {
	t.Helper()
	var messages []schemas.StudentExportMessage
	for _, history := range a.data.Messages {
		for _, raw := range history.Messages {
			messages = append(messages, decodedAs[schemas.StudentExportMessage](t, raw))
		}
	}
	return messages
}

func (a studentArchive) entryNames() []string {
	names := make([]string, 0, len(a.entries))
	for name := range a.entries {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (h *authHarness) studentArchiveOf(t *testing.T, owner exportAccount) (repositories.Export, studentArchive) {
	t.Helper()
	row := h.readyExportOf(t, owner)
	minted, address := h.mintExport(t, owner.token)
	h.mustAnswer(t, minted, http.StatusOK)
	return row, openStudentArchive(t, downloadArchive(t, address.URL))
}

func canonicalJSON(t *testing.T, raw json.RawMessage, dropped ...string) string {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	switch trimmed[0] {
	case '{':
		var decoded map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &decoded); err != nil {
			t.Fatal(err)
		}
		canonical := map[string]json.RawMessage{}
		for key, value := range decoded {
			if slices.Contains(dropped, key) {
				continue
			}
			canonical[key] = json.RawMessage(canonicalJSON(t, value, dropped...))
		}
		encoded, err := json.Marshal(canonical)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	case '[':
		var decoded []json.RawMessage
		if err := json.Unmarshal(trimmed, &decoded); err != nil {
			t.Fatal(err)
		}
		parts := make([]string, 0, len(decoded))
		for _, value := range decoded {
			parts = append(parts, canonicalJSON(t, value, dropped...))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		t.Fatal(err)
	}
	return compact.String()
}

type studentExportWorld struct {
	exportWorld
	tutorFile   string
	material    string
	answerFile  string
	corrected   string
	chatFile    string
	issue       string
	submission  string
	lesson      string
	groupLesson string
	groupChat   string
	directChat  string
	mateMessage chatMessage
}

func exportWorkBlocks(picture string, material string) []templateBlockJSON {
	return []templateBlockJSON{
		imageQuestion(blockID(exportWorkGroup, 1), picture),
		fileBlock(blockID(exportWorkGroup, exportFileQuestion), string(domain.AnswerFilePDF)),
		materialBlock(blockID(exportWorkGroup, 3), material),
	}
}

func (h *authHarness) richStudentWorld(t *testing.T) studentExportWorld {
	t.Helper()
	world := studentExportWorld{exportWorld: h.exportWorld(t)}
	picture := h.libraryUpload(t, world.tutor.token, domain.ContextPromptImage, "prompt.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, picture, http.StatusCreated)
	worksheet := h.libraryUpload(t, world.tutor.token, domain.ContextLibrary, "worksheet.pdf", pdfFile(200))
	h.mustAnswer(t, worksheet, http.StatusCreated)
	world.tutorFile = uploadedID(t, worksheet)
	world.material = h.createMaterial(t, world.tutor.token, world.tutorFile, "Worksheet", nil).ID
	template := h.workTemplate(t, world.tutor.token, "Export work", exportWorkBlocks(uploadedID(t, picture), world.material)...)
	world.lesson = h.scheduled(t, world.tutor.token, oneOffFor(world.firstCard.ID, "2030-03-12", "10:00")).Lessons[0].ID
	world.groupLesson = h.scheduled(t, world.tutor.token, groupLessonFor(world.group.ID, "2030-03-13", "11:00")).Lessons[0].ID
	issue, submissions := h.issueTemplate(t, world.tutor.token, template, toCard(world.firstCard.ID), studentExportPolicy, nil)
	world.issue, world.submission = issue, submissions[0].id
	world.answerFile = h.answerFile(t, world.first.token, world.submission, blockID(exportWorkGroup, exportFileQuestion), "answer.pdf", 300)
	draft := h.saved(t, world.first.token, world.submission,
		answersJSON(t, 1, saveID(1), attached(blockID(exportWorkGroup, exportFileQuestion), world.answerFile)))
	h.mustAnswer(t, h.submitWork(t, world.first.token, world.submission, draft.Version), http.StatusOK)
	world.corrected = h.correctedFile(t, world.tutor.token, world.submission, blockID(exportWorkGroup, exportFileQuestion), "corrected.pdf", 120)
	h.reviewSaved(t, world.tutor.token, world.submission, reviewJSON(t, 1, 0, saveID(2), "Overall good",
		commented(scored(blockID(exportWorkGroup, exportFileQuestion), 4), "See the marks", world.corrected)))
	h.published(t, world.tutor.token, world.submission, domain.ReviewCheck, 1)
	world.groupChat = h.groupChannel(t, world.group.ID).ID
	world.directChat = h.directChannel(t, world.firstCard.ID).ID
	shared := h.chatUpload(t, world.second.token, world.groupChat, "notes.txt", plainText(128))
	h.mustAnswer(t, shared, http.StatusCreated)
	world.chatFile = uploadedID(t, shared)
	world.mateMessage = h.sent(t, world.second.token, world.groupChat, "A mate's message", world.chatFile)
	h.sent(t, world.first.token, world.groupChat, "My own message")
	h.sent(t, world.tutor.token, world.groupChat, "The tutor's message")
	h.sent(t, world.tutor.token, world.directChat, "A direct message")
	h.mustAnswer(t, h.react(t, world.first.token, world.mateMessage.ID, string(domain.ReactionHeart)), http.StatusNoContent)
	world.first = h.signedInAgain(t, world.first)
	return world
}
